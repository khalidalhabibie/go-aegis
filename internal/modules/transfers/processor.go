package transfers

import (
	"context"
	"errors"
	"fmt"

	"github.com/rs/zerolog"
)

type Signer interface {
	SignTransfer(ctx context.Context, transfer Transfer) (SignedTransfer, error)
}

type Broadcaster interface {
	BroadcastTransfer(ctx context.Context, transfer Transfer, attempt TransactionAttempt) error
}

type SignedTransfer struct {
	RawTransaction string
	TxHash         string
	Nonce          *int64
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
			transfer, err = p.submitTransfer(ctx, transfer)
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

func (p *Processor) submitTransfer(ctx context.Context, transfer Transfer) (Transfer, error) {
	attempt, err := p.repository.GetLatestAttempt(ctx, transfer.ID)
	switch {
	case err == nil:
		return p.resumeSubmission(ctx, transfer, attempt)
	case errors.Is(err, ErrTransactionAttemptNotFound):
		return p.createAttemptAndSubmit(ctx, transfer)
	default:
		return Transfer{}, err
	}
}

func (p *Processor) createAttemptAndSubmit(ctx context.Context, transfer Transfer) (Transfer, error) {
	signedTransfer, err := p.signer.SignTransfer(ctx, transfer)
	if err != nil {
		return transfer, err
	}

	if signedTransfer.RawTransaction == "" {
		return Transfer{}, fmt.Errorf("signed transfer raw transaction is empty")
	}

	if signedTransfer.TxHash == "" {
		return Transfer{}, fmt.Errorf("signed transfer tx hash is empty")
	}

	rawPayload, err := newTransactionAttemptPayload(signedTransfer.RawTransaction)
	if err != nil {
		return Transfer{}, fmt.Errorf("marshal transaction attempt payload: %w", err)
	}

	attempt, err := p.repository.CreateAttempt(ctx, CreateAttemptParams{
		TransferID: transfer.ID,
		Nonce:      signedTransfer.Nonce,
		RawPayload: rawPayload,
		TxHash:     signedTransfer.TxHash,
		Status:     AttemptStatusSigned,
	})
	if err != nil {
		if errors.Is(err, ErrTransactionAttemptConflict) {
			return p.resumeLatestAttempt(ctx, transfer)
		}

		return Transfer{}, err
	}

	p.log.Info().
		Str("transfer_id", transfer.ID).
		Str("attempt_id", attempt.ID).
		Str("tx_hash", attempt.TxHash).
		Msg("transaction attempt persisted before broadcast")

	return p.resumeSubmission(ctx, transfer, attempt)
}

func (p *Processor) resumeSubmission(ctx context.Context, transfer Transfer, attempt TransactionAttempt) (Transfer, error) {
	p.log.Info().
		Str("transfer_id", transfer.ID).
		Str("attempt_id", attempt.ID).
		Str("attempt_status", attempt.Status).
		Str("tx_hash", attempt.TxHash).
		Msg("resuming transaction submission from durable attempt")

	switch attempt.Status {
	case AttemptStatusSigned:
		updatedAttempt, err := p.repository.UpdateAttempt(ctx, UpdateAttemptParams{
			AttemptID:      attempt.ID,
			ExpectedStatus: AttemptStatusSigned,
			Status:         AttemptStatusBroadcasting,
		})
		if err != nil {
			if errors.Is(err, ErrTransactionAttemptConflict) {
				return p.resumeLatestAttempt(ctx, transfer)
			}

			return Transfer{}, err
		}

		return p.broadcastAttempt(ctx, transfer, updatedAttempt)
	case AttemptStatusBroadcasting:
		// A prior worker may have crashed after the signed transaction was persisted.
		// Reusing the same payload keeps retries idempotent because the tx hash is stable.
		return p.broadcastAttempt(ctx, transfer, attempt)
	case AttemptStatusBroadcasted:
		return p.submitBroadcastedAttempt(ctx, transfer, attempt)
	case AttemptStatusFailed:
		return Transfer{}, fmt.Errorf("latest transaction attempt %s failed: %s", attempt.ID, attempt.ErrorMessage)
	default:
		return Transfer{}, fmt.Errorf("transaction attempt %s has unsupported status %s", attempt.ID, attempt.Status)
	}
}

func (p *Processor) broadcastAttempt(ctx context.Context, transfer Transfer, attempt TransactionAttempt) (Transfer, error) {
	p.log.Info().
		Str("transfer_id", transfer.ID).
		Str("attempt_id", attempt.ID).
		Str("attempt_status", attempt.Status).
		Str("tx_hash", attempt.TxHash).
		Msg("broadcasting durable transaction attempt")

	if err := p.broadcaster.BroadcastTransfer(ctx, transfer, attempt); err != nil {
		nextStatus := AttemptStatusFailed
		if errors.Is(err, ErrTransient) {
			nextStatus = AttemptStatusSigned
		}

		if _, updateErr := p.repository.UpdateAttempt(ctx, UpdateAttemptParams{
			AttemptID:      attempt.ID,
			ExpectedStatus: AttemptStatusBroadcasting,
			Status:         nextStatus,
			ErrorMessage:   err.Error(),
		}); updateErr != nil {
			if errors.Is(updateErr, ErrTransactionAttemptConflict) {
				return p.resumeLatestAttempt(ctx, transfer)
			}

			return Transfer{}, updateErr
		}

		return transfer, err
	}

	broadcastedAttempt, err := p.repository.UpdateAttempt(ctx, UpdateAttemptParams{
		AttemptID:      attempt.ID,
		ExpectedStatus: AttemptStatusBroadcasting,
		Status:         AttemptStatusBroadcasted,
	})
	if err != nil {
		if errors.Is(err, ErrTransactionAttemptConflict) {
			return p.resumeLatestAttempt(ctx, transfer)
		}

		return Transfer{}, err
	}

	return p.submitBroadcastedAttempt(ctx, transfer, broadcastedAttempt)
}

func (p *Processor) submitBroadcastedAttempt(ctx context.Context, transfer Transfer, attempt TransactionAttempt) (Transfer, error) {
	if attempt.TxHash == "" {
		return Transfer{}, fmt.Errorf("transaction attempt %s is missing tx hash", attempt.ID)
	}

	return p.transition(ctx, transfer, StatusSubmitted, &attempt.TxHash)
}

func (p *Processor) resumeLatestAttempt(ctx context.Context, transfer Transfer) (Transfer, error) {
	latestAttempt, err := p.repository.GetLatestAttempt(ctx, transfer.ID)
	if err != nil {
		return Transfer{}, err
	}

	return p.resumeSubmission(ctx, transfer, latestAttempt)
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
