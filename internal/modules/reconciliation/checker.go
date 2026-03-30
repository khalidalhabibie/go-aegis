package reconciliation

import (
	"context"
	"crypto/sha256"
)

type ReceiptChecker interface {
	CheckReceipt(ctx context.Context, chain, txHash string) (ReceiptStatus, error)
}

type PlaceholderReceiptChecker struct{}

func NewPlaceholderReceiptChecker() *PlaceholderReceiptChecker {
	return &PlaceholderReceiptChecker{}
}

func (c *PlaceholderReceiptChecker) CheckReceipt(_ context.Context, _ string, txHash string) (ReceiptStatus, error) {
	if txHash == "" {
		return ReceiptStatusNotFound, nil
	}

	sum := sha256.Sum256([]byte(txHash))

	switch sum[0] % 3 {
	case 0:
		return ReceiptStatusPending, nil
	case 1:
		return ReceiptStatusConfirmed, nil
	default:
		return ReceiptStatusFailed, nil
	}
}
