package usecase_test

import (
	"context"
	"maps"
	"sort"
	"sync"

	"github.com/google/uuid"

	"github.com/thaihongdo/go-wallet/internal/domain"
	"github.com/thaihongdo/go-wallet/internal/usecase"
)

// snapshotable is implemented by every fake repo used inside fakeTxManager
// so it can be rolled back when the wrapped function returns an error.
type snapshotable interface {
	snapshot() any
	restore(any)
}

// fakeTxManager simulates a database transaction over a fixed set of
// in-memory fakes: it snapshots their state before running fn and
// restores it if fn returns a non-nil error (or panics), exactly like a
// real transaction rollback.
type fakeTxManager struct {
	repos []snapshotable
}

func newFakeTxManager(repos ...snapshotable) *fakeTxManager {
	return &fakeTxManager{repos: repos}
}

func (tm *fakeTxManager) WithinTx(ctx context.Context, fn func(ctx context.Context) error) (err error) {
	snaps := make([]any, len(tm.repos))
	for i, r := range tm.repos {
		snaps[i] = r.snapshot()
	}
	defer func() {
		if p := recover(); p != nil {
			for i, r := range tm.repos {
				r.restore(snaps[i])
			}
			panic(p)
		}
	}()
	if err = fn(ctx); err != nil {
		for i, r := range tm.repos {
			r.restore(snaps[i])
		}
	}
	return err
}

// fakeWalletRepo is an in-memory WalletRepository.
type fakeWalletRepo struct {
	mu      sync.Mutex
	wallets map[uuid.UUID]domain.Wallet

	createErr        error
	lockErr          error
	updateBalanceErr error
}

func newFakeWalletRepo() *fakeWalletRepo {
	return &fakeWalletRepo{wallets: map[uuid.UUID]domain.Wallet{}}
}

func (r *fakeWalletRepo) put(w domain.Wallet) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.wallets[w.ID] = w
}

func (r *fakeWalletRepo) Create(_ context.Context, w *domain.Wallet) error {
	if r.createErr != nil {
		return r.createErr
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.wallets[w.ID] = *w
	return nil
}

func (r *fakeWalletRepo) GetByID(_ context.Context, id uuid.UUID) (*domain.Wallet, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	w, ok := r.wallets[id]
	if !ok {
		return nil, domain.ErrWalletNotFound
	}
	cp := w
	return &cp, nil
}

func (r *fakeWalletRepo) LockByIDs(_ context.Context, ids []uuid.UUID) ([]*domain.Wallet, error) {
	if r.lockErr != nil {
		return nil, r.lockErr
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*domain.Wallet
	sortedIDs := append([]uuid.UUID(nil), ids...)
	sort.Slice(sortedIDs, func(i, j int) bool { return sortedIDs[i].String() < sortedIDs[j].String() })
	for _, id := range sortedIDs {
		if w, ok := r.wallets[id]; ok {
			cp := w
			out = append(out, &cp)
		}
	}
	return out, nil
}

func (r *fakeWalletRepo) UpdateBalance(_ context.Context, id uuid.UUID, balance domain.Money) error {
	if r.updateBalanceErr != nil {
		return r.updateBalanceErr
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	w, ok := r.wallets[id]
	if !ok {
		return domain.ErrWalletNotFound
	}
	w.Balance = balance
	r.wallets[id] = w
	return nil
}

func (r *fakeWalletRepo) snapshot() any {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := make(map[uuid.UUID]domain.Wallet, len(r.wallets))
	maps.Copy(cp, r.wallets)
	return cp
}

func (r *fakeWalletRepo) restore(s any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.wallets = s.(map[uuid.UUID]domain.Wallet)
}

// fakeOperationRepo is an in-memory OperationRepository.
type fakeOperationRepo struct {
	mu          sync.Mutex
	byKey       map[string]domain.Operation
	createCalls int

	createErr error
}

func newFakeOperationRepo() *fakeOperationRepo {
	return &fakeOperationRepo{byKey: map[string]domain.Operation{}}
}

func (r *fakeOperationRepo) Create(_ context.Context, op *domain.Operation) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.createCalls++
	if r.createErr != nil {
		return r.createErr
	}
	if _, exists := r.byKey[op.IdempotencyKey]; exists {
		return domain.ErrDuplicateIdempotencyKey
	}
	r.byKey[op.IdempotencyKey] = *op
	return nil
}

func (r *fakeOperationRepo) GetByIdempotencyKey(_ context.Context, key string) (*domain.Operation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	op, ok := r.byKey[key]
	if !ok {
		return nil, nil
	}
	cp := op
	return &cp, nil
}

// seed inserts an operation directly, bypassing Create, to simulate a
// concurrent request that already won the idempotency-key race.
func (r *fakeOperationRepo) seed(op domain.Operation) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byKey[op.IdempotencyKey] = op
}

func (r *fakeOperationRepo) snapshot() any {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := make(map[string]domain.Operation, len(r.byKey))
	maps.Copy(cp, r.byKey)
	return cp
}

func (r *fakeOperationRepo) restore(s any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byKey = s.(map[string]domain.Operation)
}

// fakeLedgerRepo is an in-memory LedgerRepository.
type fakeLedgerRepo struct {
	mu      sync.Mutex
	entries []domain.LedgerEntry

	insertErr error
}

func newFakeLedgerRepo() *fakeLedgerRepo {
	return &fakeLedgerRepo{}
}

func (r *fakeLedgerRepo) Insert(_ context.Context, entries ...domain.LedgerEntry) error {
	if r.insertErr != nil {
		return r.insertErr
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries = append(r.entries, entries...)
	return nil
}

func (r *fakeLedgerRepo) ListByWallet(_ context.Context, walletID uuid.UUID, cursor *uuid.UUID, limit int) ([]domain.LedgerEntry, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var matched []domain.LedgerEntry
	for _, e := range r.entries {
		if e.WalletID == walletID {
			matched = append(matched, e)
		}
	}
	sort.Slice(matched, func(i, j int) bool {
		return matched[i].ID.String() > matched[j].ID.String()
	})

	if cursor != nil {
		idx := 0
		for idx < len(matched) && matched[idx].ID.String() >= cursor.String() {
			idx++
		}
		matched = matched[idx:]
	}

	if limit > 0 && len(matched) > limit {
		matched = matched[:limit]
	}
	return matched, nil
}

func (r *fakeLedgerRepo) snapshot() any {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]domain.LedgerEntry(nil), r.entries...)
}

func (r *fakeLedgerRepo) restore(s any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries = s.([]domain.LedgerEntry)
}

// fakeOutboxRepo is an in-memory OutboxRepository.
type fakeOutboxRepo struct {
	mu     sync.Mutex
	events []domain.OutboxEvent

	insertErr error
}

func newFakeOutboxRepo() *fakeOutboxRepo {
	return &fakeOutboxRepo{}
}

func (r *fakeOutboxRepo) Insert(_ context.Context, e domain.OutboxEvent) error {
	if r.insertErr != nil {
		return r.insertErr
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, e)
	return nil
}

func (r *fakeOutboxRepo) snapshot() any {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]domain.OutboxEvent(nil), r.events...)
}

func (r *fakeOutboxRepo) restore(s any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = s.([]domain.OutboxEvent)
}

// sequentialIDs returns an IDGenerator that yields deterministic, sorted
// UUIDv7-shaped IDs from a pre-seeded list, falling back to a real
// uuid.NewV7 once exhausted, so tests can both assert on specific IDs and
// run usecases that generate many entries.
func sequentialIDs() usecase.IDGenerator {
	var mu sync.Mutex
	return func() uuid.UUID {
		mu.Lock()
		defer mu.Unlock()
		id, err := uuid.NewV7()
		if err != nil {
			panic(err)
		}
		return id
	}
}
