package transfers

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
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
	rawTransaction := fmt.Sprintf("signed:%s:%s:%s", transfer.ID, transfer.SourceWalletID, transfer.Amount)
	sum := sha256.Sum256([]byte(rawTransaction))
	nonce := int64(binary.BigEndian.Uint64(sum[:8]) % 1_000_000)

	s.log.Info().
		Str("transfer_id", transfer.ID).
		Str("status", transfer.Status).
		Msg("mock signer signed transfer")

	return SignedTransfer{
		RawTransaction: rawTransaction,
		TxHash:         "0x" + hex.EncodeToString(sum[:]),
		Nonce:          &nonce,
	}, nil
}

type MockBroadcaster struct {
	log zerolog.Logger
}

func NewMockBroadcaster(log zerolog.Logger) *MockBroadcaster {
	return &MockBroadcaster{log: log}
}

func (b *MockBroadcaster) BroadcastTransfer(_ context.Context, transfer Transfer, attempt TransactionAttempt) error {
	b.log.Info().
		Str("transfer_id", transfer.ID).
		Str("attempt_id", attempt.ID).
		Str("tx_hash", attempt.TxHash).
		Msg("mock broadcaster submitted signed transaction")

	return nil
}
