package usecase

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/thaihongdo/go-wallet/internal/domain"
)

// TransferInput is the request to move money between two wallets.
type TransferInput struct {
	IdempotencyKey string
	FromWalletID   uuid.UUID
	ToWalletID     uuid.UUID
	Amount         domain.Money
}

// TransferResult is the outcome of a transfer. Replayed is true when the
// IdempotencyKey had already been used and no new money movement
// occurred; in that case FromBalance/ToBalance reflect the wallets'
// current balances (not the balances at the time of the original
// operation).
type TransferResult struct {
	Operation   domain.Operation
	FromBalance domain.Money
	ToBalance   domain.Money
	Replayed    bool
}

// TransferUsecase moves money from one wallet to another, atomically,
// idempotently, and without deadlocking against concurrent transfers.
type TransferUsecase struct {
	tx      TxManager
	wallets WalletRepository
	ops     OperationRepository
	ledger  LedgerRepository
	outbox  OutboxRepository
	now     Clock
	newID   IDGenerator
}

// NewTransferUsecase builds a TransferUsecase from its dependencies.
func NewTransferUsecase(
	tx TxManager,
	wallets WalletRepository,
	ops OperationRepository,
	ledger LedgerRepository,
	outbox OutboxRepository,
	now Clock,
	newID IDGenerator,
) *TransferUsecase {
	return &TransferUsecase{
		tx:      tx,
		wallets: wallets,
		ops:     ops,
		ledger:  ledger,
		outbox:  outbox,
		now:     now,
		newID:   newID,
	}
}

// Transfer validates in.Amount and moves it from in.FromWalletID to
// in.ToWalletID. Wallet row locks are always acquired in ascending ID
// order (see WalletRepository.LockByIDs), so two transfers between the
// same pair of wallets in opposite directions can never deadlock.
//
// Replaying the same IdempotencyKey returns the original operation with
// Replayed=true instead of moving money again. This holds even under
// concurrent duplicate requests: if two callers race to insert the same
// key, the loser's OperationRepository.Create returns
// ErrDuplicateIdempotencyKey and is resolved into a replay after the
// transaction rolls back.
func (uc *TransferUsecase) Transfer(ctx context.Context, in TransferInput) (TransferResult, error) {
	if in.IdempotencyKey == "" {
		return TransferResult{}, fmt.Errorf("transfer: %w", domain.ErrInvalidInput)
	}
	if !in.Amount.IsPositive() {
		return TransferResult{}, fmt.Errorf("transfer: %w", domain.ErrInvalidAmount)
	}
	if in.FromWalletID == in.ToWalletID {
		return TransferResult{}, fmt.Errorf("transfer: %w", domain.ErrSameWallet)
	}

	var result TransferResult
	txErr := uc.tx.WithinTx(ctx, func(ctx context.Context) error {
		existing, err := uc.ops.GetByIdempotencyKey(ctx, in.IdempotencyKey)
		if err != nil {
			return fmt.Errorf("lookup idempotency key: %w", err)
		}
		if existing != nil {
			result, err = uc.replayResult(ctx, *existing)
			return err
		}

		wallets, err := uc.wallets.LockByIDs(ctx, []uuid.UUID{in.FromWalletID, in.ToWalletID})
		if err != nil {
			return fmt.Errorf("lock wallets: %w", err)
		}
		if len(wallets) != 2 {
			return fmt.Errorf("lock wallets: %w", domain.ErrWalletNotFound)
		}

		var from, to *domain.Wallet
		for _, w := range wallets {
			switch w.ID {
			case in.FromWalletID:
				from = w
			case in.ToWalletID:
				to = w
			}
		}
		if from == nil || to == nil {
			return fmt.Errorf("lock wallets: %w", domain.ErrWalletNotFound)
		}

		if from.Currency != to.Currency {
			return fmt.Errorf("transfer: %w", domain.ErrCurrencyMismatch)
		}

		if err := from.Debit(in.Amount); err != nil {
			return fmt.Errorf("debit source wallet: %w", err)
		}
		if err := to.Credit(in.Amount); err != nil {
			return fmt.Errorf("credit destination wallet: %w", err)
		}

		now := uc.now()
		fromID := in.FromWalletID
		op := domain.Operation{
			ID:             uc.newID(),
			Type:           domain.OperationTransfer,
			IdempotencyKey: in.IdempotencyKey,
			FromWalletID:   &fromID,
			ToWalletID:     in.ToWalletID,
			Amount:         in.Amount,
			CreatedAt:      now,
		}
		if err := uc.ops.Create(ctx, &op); err != nil {
			return fmt.Errorf("create operation: %w", err)
		}

		debitEntry := domain.LedgerEntry{
			ID:           uc.newID(),
			OperationID:  op.ID,
			WalletID:     from.ID,
			Direction:    domain.DirectionDebit,
			Amount:       in.Amount,
			BalanceAfter: from.Balance,
			CreatedAt:    now,
		}
		creditEntry := domain.LedgerEntry{
			ID:           uc.newID(),
			OperationID:  op.ID,
			WalletID:     to.ID,
			Direction:    domain.DirectionCredit,
			Amount:       in.Amount,
			BalanceAfter: to.Balance,
			CreatedAt:    now,
		}
		if err := uc.ledger.Insert(ctx, debitEntry, creditEntry); err != nil {
			return fmt.Errorf("insert ledger entries: %w", err)
		}

		if err := uc.wallets.UpdateBalance(ctx, from.ID, from.Balance); err != nil {
			return fmt.Errorf("update source balance: %w", err)
		}
		if err := uc.wallets.UpdateBalance(ctx, to.ID, to.Balance); err != nil {
			return fmt.Errorf("update destination balance: %w", err)
		}

		payload, err := transferEventPayload(op)
		if err != nil {
			return fmt.Errorf("marshal outbox payload: %w", err)
		}
		if err := uc.outbox.Insert(ctx, domain.OutboxEvent{
			ID:            uc.newID(),
			AggregateType: "operation",
			AggregateID:   op.ID,
			EventType:     "transfer.completed",
			Payload:       payload,
			CreatedAt:     now,
		}); err != nil {
			return fmt.Errorf("insert outbox event: %w", err)
		}

		result = TransferResult{
			Operation:   op,
			FromBalance: from.Balance,
			ToBalance:   to.Balance,
			Replayed:    false,
		}
		return nil
	})

	if txErr != nil {
		if errors.Is(txErr, domain.ErrDuplicateIdempotencyKey) {
			// A concurrent request with the same key committed first.
			// Resolve the replay outside the failed transaction.
			return uc.replayAfterRace(ctx, in.IdempotencyKey)
		}
		return TransferResult{}, txErr
	}
	return result, nil
}

// replayResult builds a TransferResult for an already-recorded
// operation, reading the wallets' current balances (ctx carries the
// in-progress transaction, so this sees a consistent snapshot).
func (uc *TransferUsecase) replayResult(ctx context.Context, op domain.Operation) (TransferResult, error) {
	from, err := uc.wallets.GetByID(ctx, *op.FromWalletID)
	if err != nil {
		return TransferResult{}, fmt.Errorf("get source wallet for replay: %w", err)
	}
	to, err := uc.wallets.GetByID(ctx, op.ToWalletID)
	if err != nil {
		return TransferResult{}, fmt.Errorf("get destination wallet for replay: %w", err)
	}
	return TransferResult{
		Operation:   op,
		FromBalance: from.Balance,
		ToBalance:   to.Balance,
		Replayed:    true,
	}, nil
}

// replayAfterRace handles the case where OperationRepository.Create
// raced against a concurrent request using the same idempotency key and
// lost. It looks the operation up outside any transaction and reports
// current balances.
func (uc *TransferUsecase) replayAfterRace(ctx context.Context, key string) (TransferResult, error) {
	op, err := uc.ops.GetByIdempotencyKey(ctx, key)
	if err != nil {
		return TransferResult{}, fmt.Errorf("lookup idempotency key after race: %w", err)
	}
	if op == nil {
		return TransferResult{}, fmt.Errorf("lookup idempotency key after race: %w", domain.ErrDuplicateIdempotencyKey)
	}
	return uc.replayResult(ctx, *op)
}
