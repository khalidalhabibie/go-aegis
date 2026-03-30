package transfers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"aegis/internal/platform/rabbitmq"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/rs/zerolog"
)

type Consumer struct {
	subscription *rabbitmq.Consumer
	processor    *Processor
	publisher    JobPublisher
	locker       ProcessingLocker
	queueName    string
	log          zerolog.Logger
	maxRetries   int
	retryDelay   time.Duration
	lockTTL      time.Duration
}

func NewConsumer(
	subscription *rabbitmq.Consumer,
	processor *Processor,
	publisher JobPublisher,
	locker ProcessingLocker,
	queueName string,
	maxRetries int,
	retryDelay time.Duration,
	lockTTL time.Duration,
	log zerolog.Logger,
) *Consumer {
	return &Consumer{
		subscription: subscription,
		processor:    processor,
		publisher:    publisher,
		locker:       locker,
		queueName:    queueName,
		maxRetries:   maxRetries,
		retryDelay:   retryDelay,
		lockTTL:      lockTTL,
		log:          log,
	}
}

func (c *Consumer) Run(ctx context.Context) error {
	defer c.subscription.Close()

	for {
		select {
		case <-ctx.Done():
			return nil
		case delivery, ok := <-c.subscription.Deliveries():
			if !ok {
				return errors.New("transfer consumer delivery channel closed")
			}

			if err := c.handleDelivery(ctx, delivery); err != nil {
				return err
			}
		}
	}
}

func (c *Consumer) handleDelivery(ctx context.Context, delivery amqp.Delivery) error {
	var job TransferJob
	if err := json.Unmarshal(delivery.Body, &job); err != nil {
		c.log.Error().Err(err).Str("queue", c.queueName).Msg("rejecting malformed transfer job")
		return delivery.Reject(false)
	}

	c.log.Info().
		Str("transfer_id", job.TransferID).
		Int("attempt", job.Attempt).
		Msg("processing transfer job")

	lock, acquired, err := c.acquireProcessingLock(ctx, job.TransferID)
	if err != nil {
		return err
	}

	if !acquired {
		c.log.Warn().
			Str("transfer_id", job.TransferID).
			Int("attempt", job.Attempt).
			Msg("transfer processing already in progress, requeueing job")
		return delivery.Nack(false, true)
	}

	heartbeatStop := func() {}
	var heartbeatErrs <-chan error
	if lock != nil {
		processCtx, processCancel := context.WithCancel(ctx)
		heartbeatStop, heartbeatErrs = c.startProcessingLockHeartbeat(processCtx, processCancel, lock)
		defer func() {
			if releaseErr := lock.Release(ctx); releaseErr != nil {
				c.log.Error().
					Err(releaseErr).
					Str("transfer_id", job.TransferID).
					Msg("release transfer processing lock")
			}
			processCancel()
		}()
		ctx = processCtx
	}

	transfer, err := c.processor.ProcessTransfer(ctx, job.TransferID)
	heartbeatStop()
	if heartbeatErrs != nil {
		if heartbeatErr := <-heartbeatErrs; heartbeatErr != nil && (err == nil || errors.Is(err, context.Canceled)) {
			err = heartbeatErr
		}
	}

	if err == nil {
		c.log.Info().
			Str("transfer_id", transfer.ID).
			Str("status", transfer.Status).
			Str("tx_hash", transfer.TxHash).
			Int("attempt", job.Attempt).
			Msg("transfer job processed successfully")
		return delivery.Ack(false)
	}

	switch {
	case errors.Is(err, ErrTransient):
		return c.retryJob(ctx, delivery, job, err)
	case errors.Is(err, ErrTransferNotFound), errors.Is(err, ErrInvalidTransferState):
		c.log.Warn().
			Err(err).
			Str("transfer_id", job.TransferID).
			Int("attempt", job.Attempt).
			Msg("acknowledging non-actionable transfer job")
		return delivery.Ack(false)
	default:
		if failErr := c.processor.FailTransfer(ctx, job.TransferID); failErr != nil && !errors.Is(failErr, ErrTransferNotFound) {
			c.log.Error().
				Err(failErr).
				Str("transfer_id", job.TransferID).
				Msg("mark transfer failed")
		}

		c.log.Error().
			Err(err).
			Str("transfer_id", job.TransferID).
			Int("attempt", job.Attempt).
			Msg("transfer job failed permanently")
		return delivery.Ack(false)
	}
}

func (c *Consumer) startProcessingLockHeartbeat(
	ctx context.Context,
	cancelProcessing context.CancelFunc,
	lock ProcessingLock,
) (func(), <-chan error) {
	errs := make(chan error, 1)

	if lock == nil || c.lockTTL <= 0 {
		close(errs)
		return func() {}, errs
	}

	interval := c.lockTTL / 3
	if interval <= 0 {
		interval = time.Second
	}

	if interval > 5*time.Second {
		interval = 5 * time.Second
	}

	heartbeatCtx, stopHeartbeat := context.WithCancel(context.Background())

	go func() {
		defer close(errs)

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-heartbeatCtx.Done():
				return
			case <-ticker.C:
				if err := lock.Refresh(ctx, c.lockTTL); err != nil {
					if cancelProcessing != nil {
						cancelProcessing()
					}

					errs <- TransientError{Operation: "refresh processing lock", Err: err}
					return
				}
			}
		}
	}()

	return stopHeartbeat, errs
}

func (c *Consumer) acquireProcessingLock(ctx context.Context, transferID string) (ProcessingLock, bool, error) {
	if c.locker == nil {
		return nil, true, nil
	}

	lock, acquired, err := c.locker.Acquire(ctx, transferID, c.lockTTL)
	if err != nil {
		return nil, false, fmt.Errorf("acquire transfer processing lock: %w", err)
	}

	return lock, acquired, nil
}

func (c *Consumer) retryJob(ctx context.Context, delivery amqp.Delivery, job TransferJob, err error) error {
	if job.Attempt >= c.maxRetries {
		if failErr := c.processor.FailTransfer(ctx, job.TransferID); failErr != nil && !errors.Is(failErr, ErrTransferNotFound) {
			c.log.Error().
				Err(failErr).
				Str("transfer_id", job.TransferID).
				Int("attempt", job.Attempt).
				Msg("mark transfer failed after retries exhausted")
		}

		c.log.Error().
			Err(err).
			Str("transfer_id", job.TransferID).
			Int("attempt", job.Attempt).
			Msg("transfer job exhausted retries")
		return delivery.Ack(false)
	}

	nextJob := TransferJob{
		TransferID: job.TransferID,
		Attempt:    job.Attempt + 1,
	}

	if c.retryDelay > 0 {
		timer := time.NewTimer(c.retryDelay)
		defer timer.Stop()

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
	}

	if err := c.publisher.PublishTransferRequested(ctx, nextJob); err != nil {
		return fmt.Errorf("republish transfer job: %w", err)
	}

	c.log.Warn().
		Err(err).
		Str("transfer_id", job.TransferID).
		Int("attempt", job.Attempt).
		Int("next_attempt", nextJob.Attempt).
		Msg("transfer job requeued after transient error")

	return delivery.Ack(false)
}
