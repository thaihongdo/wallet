package usecase

import (
	"encoding/json"

	"github.com/google/uuid"

	"github.com/thaihongdo/go-wallet/internal/domain"
)

// transferEventPayload builds the JSON payload for a transfer.completed
// outbox event.
func transferEventPayload(op domain.Operation) ([]byte, error) {
	return json.Marshal(struct {
		OperationID  uuid.UUID    `json:"operation_id"`
		FromWalletID uuid.UUID    `json:"from_wallet_id"`
		ToWalletID   uuid.UUID    `json:"to_wallet_id"`
		Amount       domain.Money `json:"amount"`
	}{
		OperationID:  op.ID,
		FromWalletID: *op.FromWalletID,
		ToWalletID:   op.ToWalletID,
		Amount:       op.Amount,
	})
}

// topUpEventPayload builds the JSON payload for a wallet.topped_up
// outbox event.
func topUpEventPayload(op domain.Operation) ([]byte, error) {
	return json.Marshal(struct {
		OperationID uuid.UUID    `json:"operation_id"`
		WalletID    uuid.UUID    `json:"wallet_id"`
		Amount      domain.Money `json:"amount"`
	}{
		OperationID: op.ID,
		WalletID:    op.ToWalletID,
		Amount:      op.Amount,
	})
}
