package usecase

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/thaihongdo/go-wallet/internal/domain"
)

// TopUpInput is the request to credit a single wallet from an external
// source (e.g. a payment gateway).
type TopUpInput struct {
	IdempotencyKey string
	WalletID       uuid.UUID
	Amount         domain.Money
}

// TopUpResult is the outcome of a top-up. Replayed is true when the
// IdempotencyKey had already been used and no new credit occurred; in
// that case Balance reflects the wallet's current balance.
type TopUpResult struct {
	Operation domain.Operation
	Balance   domain.Money
	Replayed  bool
}

// TopUpUsecase credits a single wallet, idempotently.
type TopUpUsecase struct {
	tx      TxManager
	wallets WalletRepository
	ops     OperationRepository
	ledger  LedgerRepository
	outbox  OutboxRepository
	now     Clock
	newID   IDGenerator
}

// NewTopUpUsecase builds a TopUpUsecase from its dependencies.
func NewTopUpUsecase(
	tx TxManager,
	wallets WalletRepository,
	ops OperationRepository,
	ledger LedgerRepository,
	outbox OutboxRepository,
	now Clock,
	newID IDGenerator,
) *TopUpUsecase {
	return &TopUpUsecase{
		tx:      tx,
		wallets: wallets,
		ops:     ops,
		ledger:  ledger,
		outbox:  outbox,
		now:     now,
		newID:   newID,
	}
}

// TopUp validates in.Amount and credits it to in.WalletID. Replaying the
// same IdempotencyKey returns the original operation with Replayed=true
// instead of crediting again, including when a concurrent duplicate
// request wins the race to record the key first.
func (uc *TopUpUsecase) TopUp(ctx context.Context, in TopUpInput) (TopUpResult, error) {
	if in.IdempotencyKey == "" {
		return TopUpResult{}, fmt.Errorf("topup: %w", domain.ErrInvalidInput)
	}
	if !in.Amount.IsPositive() {
		return TopUpResult{}, fmt.Errorf("topup: %w", domain.ErrInvalidAmount)
	}

	var result TopUpResult
	txErr := uc.tx.WithinTx(ctx, func(ctx context.Context) error {
		existing, err := uc.ops.GetByIdempotencyKey(ctx, in.IdempotencyKey)
		if err != nil {
			return fmt.Errorf("lookup idempotency key: %w", err)
		}
		if existing != nil {
			result, err = uc.replayResult(ctx, *existing)
			return err
		}

		wallets, err := uc.wallets.LockByIDs(ctx, []uuid.UUID{in.WalletID})
		if err != nil {
			return fmt.Errorf("lock wallet: %w", err)
		}
		if len(wallets) != 1 {
			return fmt.Errorf("lock wallet: %w", domain.ErrWalletNotFound)
		}
		w := wallets[0]

		if err := w.Credit(in.Amount); err != nil {
			return fmt.Errorf("credit wallet: %w", err)
		}

		now := uc.now()
		op := domain.Operation{
			ID:             uc.newID(),
			Type:           domain.OperationTopUp,
			IdempotencyKey: in.IdempotencyKey,
			FromWalletID:   nil,
			ToWalletID:     w.ID,
			Amount:         in.Amount,
			CreatedAt:      now,
		}
		if err := uc.ops.Create(ctx, &op); err != nil {
			return fmt.Errorf("create operation: %w", err)
		}

		entry := domain.LedgerEntry{
			ID:           uc.newID(),
			OperationID:  op.ID,
			WalletID:     w.ID,
			Direction:    domain.DirectionCredit,
			Amount:       in.Amount,
			BalanceAfter: w.Balance,
			CreatedAt:    now,
		}
		if err := uc.ledger.Insert(ctx, entry); err != nil {
			return fmt.Errorf("insert ledger entry: %w", err)
		}

		if err := uc.wallets.UpdateBalance(ctx, w.ID, w.Balance); err != nil {
			return fmt.Errorf("update balance: %w", err)
		}

		payload, err := topUpEventPayload(op)
		if err != nil {
			return fmt.Errorf("marshal outbox payload: %w", err)
		}
		if err := uc.outbox.Insert(ctx, domain.OutboxEvent{
			ID:            uc.newID(),
			AggregateType: "operation",
			AggregateID:   op.ID,
			EventType:     "wallet.topped_up",
			Payload:       payload,
			CreatedAt:     now,
		}); err != nil {
			return fmt.Errorf("insert outbox event: %w", err)
		}

		result = TopUpResult{Operation: op, Balance: w.Balance, Replayed: false}
		return nil
	})

	if txErr != nil {
		if errors.Is(txErr, domain.ErrDuplicateIdempotencyKey) {
			return uc.replayAfterRace(ctx, in.IdempotencyKey)
		}
		return TopUpResult{}, txErr
	}
	return result, nil
}

func (uc *TopUpUsecase) replayResult(ctx context.Context, op domain.Operation) (TopUpResult, error) {
	w, err := uc.wallets.GetByID(ctx, op.ToWalletID)
	if err != nil {
		return TopUpResult{}, fmt.Errorf("get wallet for replay: %w", err)
	}
	return TopUpResult{Operation: op, Balance: w.Balance, Replayed: true}, nil
}

func (uc *TopUpUsecase) replayAfterRace(ctx context.Context, key string) (TopUpResult, error) {
	op, err := uc.ops.GetByIdempotencyKey(ctx, key)
	if err != nil {
		return TopUpResult{}, fmt.Errorf("lookup idempotency key after race: %w", err)
	}
	if op == nil {
		return TopUpResult{}, fmt.Errorf("lookup idempotency key after race: %w", domain.ErrDuplicateIdempotencyKey)
	}
	return uc.replayResult(ctx, *op)
}
