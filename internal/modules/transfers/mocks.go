package transfers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/rs/zerolog"
)

type MockSigner struct {
	log zerolog.Logger
}

func NewMockSigner(log zerolog.Logger) *MockSigner {
	return &MockSigner{log: log}
}

func (s *MockSigner) SignTransfer(_ context.Context, transfer Transfer) (SignedTransfer, error) {
	s.log.Info().
		Str("transfer_id", transfer.ID).
		Str("status", transfer.Status).
		Msg("mock signer signed transfer")

	return SignedTransfer{
		RawTransaction: fmt.Sprintf("signed:%s:%s:%s", transfer.ID, transfer.SourceWalletID, transfer.Amount),
	}, nil
}

type MockBroadcaster struct {
	log zerolog.Logger
}

func NewMockBroadcaster(log zerolog.Logger) *MockBroadcaster {
	return &MockBroadcaster{log: log}
}

func (b *MockBroadcaster) BroadcastTransfer(_ context.Context, transfer Transfer, signed SignedTransfer) (string, error) {
	sum := sha256.Sum256([]byte(signed.RawTransaction))
	txHash := "0x" + hex.EncodeToString(sum[:])

	b.log.Info().
		Str("transfer_id", transfer.ID).
		Str("tx_hash", txHash).
		Msg("mock broadcaster submitted signed transaction")

	return txHash, nil
}
