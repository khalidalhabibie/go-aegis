package transfers

import (
	"context"
	"errors"

	"github.com/rs/zerolog"
)

type Signer interface {
	SignTransfer(ctx context.Context, transfer Transfer) (SignedTransfer, error)
}

type Broadcaster interface {
	BroadcastTransfer(ctx context.Context, transfer Transfer, signed SignedTransfer) (string, error)
}

type SignedTransfer struct {
	RawTransaction string
}

type Processor struct {
	repository  Repository
	signer      Signer
	broadcaster Broadcaster
	log         zerolog.Logger
}

func NewProcessor(repository Repository, signer Signer, broadcaster Broadcaster, log zerolog.Logger) *Processor {
	return &Processor{
		repository:  repository,
		signer:      signer,
		broadcaster: broadcaster,
		log:         log,
	}
}

func (p *Processor) ProcessTransfer(ctx context.Context, transferID string) (Transfer, error) {
	transfer, err := p.repository.GetByID(ctx, transferID)
	if err != nil {
		return Transfer{}, err
	}

	for {
		switch transfer.Status {
		case StatusCreated:
			transfer, err = p.transition(ctx, transfer, StatusValidated, nil)
		case StatusValidated:
			transfer, err = p.transition(ctx, transfer, StatusQueued, nil)
		case StatusQueued:
			transfer, err = p.transition(ctx, transfer, StatusSigning, nil)
		case StatusSigning:
			signedTransfer, signErr := p.signer.SignTransfer(ctx, transfer)
			if signErr != nil {
				return transfer, signErr
			}

			txHash, broadcastErr := p.broadcaster.BroadcastTransfer(ctx, transfer, signedTransfer)
			if broadcastErr != nil {
				return transfer, broadcastErr
			}

			transfer, err = p.transition(ctx, transfer, StatusSubmitted, &txHash)
		case StatusSubmitted:
			transfer, err = p.transition(ctx, transfer, StatusPendingOnChain, nil)
		case StatusPendingOnChain:
			p.log.Info().
				Str("transfer_id", transfer.ID).
				Str("status", transfer.Status).
				Str("tx_hash", transfer.TxHash).
				Msg("transfer already pending on chain")
			return transfer, nil
		case StatusFailed:
			p.log.Warn().
				Str("transfer_id", transfer.ID).
				Str("status", transfer.Status).
				Msg("transfer processing skipped because transfer already failed")
			return transfer, nil
		default:
			return Transfer{}, InvalidStateError{
				TransferID: transfer.ID,
				Expected:   StatusCreated,
				Actual:     transfer.Status,
			}
		}

		if err != nil {
			return Transfer{}, err
		}

		if transfer.Status == StatusPendingOnChain {
			p.log.Info().
				Str("transfer_id", transfer.ID).
				Str("status", transfer.Status).
				Str("tx_hash", transfer.TxHash).
				Msg("transfer moved to pending on chain")
			return transfer, nil
		}
	}
}

func (p *Processor) FailTransfer(ctx context.Context, transferID string) error {
	transfer, err := p.repository.GetByID(ctx, transferID)
	if err != nil {
		return err
	}

	switch transfer.Status {
	case StatusPendingOnChain, StatusFailed:
		return nil
	}

	_, err = p.transition(ctx, transfer, StatusFailed, nil)
	if err != nil && errors.Is(err, ErrInvalidTransferState) {
		return nil
	}

	return err
}

func (p *Processor) transition(ctx context.Context, transfer Transfer, toStatus string, txHash *string) (Transfer, error) {
	updatedTransfer, err := p.repository.TransitionStatus(ctx, TransitionParams{
		ID:         transfer.ID,
		FromStatus: transfer.Status,
		ToStatus:   toStatus,
		TxHash:     txHash,
	})
	if err != nil {
		return Transfer{}, err
	}

	event := p.log.Info()
	if toStatus == StatusFailed {
		event = p.log.Error()
	}

	event.
		Str("transfer_id", updatedTransfer.ID).
		Str("from_status", transfer.Status).
		Str("to_status", updatedTransfer.Status).
		Str("tx_hash", updatedTransfer.TxHash).
		Msg("transfer status updated")

	return updatedTransfer, nil
}
