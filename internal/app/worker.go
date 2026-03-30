package app

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"aegis/internal/bootstrap"
	"aegis/internal/config"
	"aegis/internal/modules/transfers"
	"aegis/internal/modules/webhooks"

	"github.com/rs/zerolog"
)

const (
	initialSubsystemRestartBackoff = 2 * time.Second
	maxSubsystemRestartBackoff     = 30 * time.Second
)

func RunWorker(ctx context.Context) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	container, err := bootstrap.NewContainer(ctx, cfg)
	if err != nil {
		return fmt.Errorf("bootstrap container: %w", err)
	}
	defer func() {
		if closeErr := container.Close(); closeErr != nil {
			container.Logger.Error().Err(closeErr).Msg("close container")
		}
	}()

	if err := ensureTransferQueue(container.RabbitMQ, cfg.RabbitMQ); err != nil {
		return err
	}

	transferRepository := transfers.NewPostgresRepository(container.Postgres)
	transferPublisher := transfers.NewRabbitMQJobPublisher(container.RabbitMQ, cfg.RabbitMQ, container.Logger)
	transferLocker := transfers.NewRedisProcessingLocker(container.Redis, "")
	transferProcessor := transfers.NewProcessor(
		transferRepository,
		transfers.NewMockSigner(container.Logger),
		transfers.NewMockBroadcaster(container.Logger),
		container.Logger,
	)
	outboxDispatcher := transfers.NewOutboxDispatcher(
		transferRepository,
		transferPublisher,
		cfg.Worker.TransferOutboxBatchSize,
		cfg.Worker.TransferOutboxPollInterval,
		cfg.Worker.TransferOutboxRetryDelay,
		cfg.Worker.TransferOutboxProcessingAfter,
		container.Logger,
	)
	webhookRepository := webhooks.NewPostgresRepository(container.Postgres)
	webhookLeaseDuration := cfg.Webhook.LeaseDuration
	minimumWebhookLeaseDuration := cfg.Webhook.Timeout + 5*time.Second
	if webhookLeaseDuration < minimumWebhookLeaseDuration {
		container.Logger.Warn().
			Dur("configured_lease_duration", webhookLeaseDuration).
			Dur("effective_lease_duration", minimumWebhookLeaseDuration).
			Dur("webhook_timeout", cfg.Webhook.Timeout).
			Msg("webhook lease duration shorter than dispatch timeout; using safer effective lease duration")
		webhookLeaseDuration = minimumWebhookLeaseDuration
	}
	webhookDispatcher := webhooks.NewHTTPDispatcher(
		cfg.Webhook.Timeout,
		webhooks.NewSigner(cfg.Webhook.SigningSecret),
		webhooks.TargetPolicy{
			AllowedHosts:        cfg.CallbackURL.AllowedHosts,
			AllowPrivateTargets: cfg.CallbackURL.AllowPrivateTargets,
		},
		cfg.Webhook.ResponseBodyMaxBytes,
	)
	webhookService := webhooks.NewService(
		webhookRepository,
		webhookDispatcher,
		cfg.Webhook.MaxAttempts,
		cfg.Webhook.InitialBackoff,
		cfg.Webhook.BatchSize,
		webhookLeaseDuration,
		container.Logger,
	)

	container.Logger.Info().
		Str("queue", cfg.RabbitMQ.TransferQueue).
		Str("routing_key", cfg.RabbitMQ.TransferRoutingKey).
		Str("consumer_tag", cfg.Worker.ConsumerTag).
		Msg("worker ready")

	var waitGroup sync.WaitGroup

	superviseWorkerSubsystem(ctx, &waitGroup, "transfer_outbox_dispatcher", container.Logger, func(runCtx context.Context) error {
		if err := outboxDispatcher.Run(runCtx); err != nil && !errors.Is(err, context.Canceled) {
			return fmt.Errorf("run transfer outbox dispatcher: %w", err)
		}

		return nil
	})

	superviseWorkerSubsystem(ctx, &waitGroup, "transfer_consumer", container.Logger, func(runCtx context.Context) error {
		subscription, consumeErr := container.RabbitMQ.Consume(cfg.RabbitMQ.TransferQueue, cfg.Worker.ConsumerTag)
		if consumeErr != nil {
			return fmt.Errorf("start transfer consumer: %w", consumeErr)
		}

		transferConsumer := transfers.NewConsumer(
			subscription,
			transferProcessor,
			transferPublisher,
			transferLocker,
			cfg.RabbitMQ.TransferQueue,
			cfg.Worker.TransferMaxRetries,
			cfg.Worker.TransferRetryDelay,
			cfg.Worker.TransferProcessLockTTL,
			container.Logger,
		)

		if err := transferConsumer.Run(runCtx); err != nil && !errors.Is(err, context.Canceled) {
			return fmt.Errorf("run transfer consumer: %w", err)
		}

		return nil
	})

	superviseWorkerSubsystem(ctx, &waitGroup, "webhook_worker", container.Logger, func(runCtx context.Context) error {
		webhookWorker := webhooks.NewWorker(webhookService, cfg.Worker.WebhookPollInterval)
		if err := webhookWorker.Run(runCtx); err != nil && !errors.Is(err, context.Canceled) {
			return fmt.Errorf("run webhook worker: %w", err)
		}

		return nil
	})

	<-ctx.Done()
	container.Logger.Info().Dur("timeout", cfg.App.ShutdownTimeout).Msg("worker shutdown requested")
	waitGroup.Wait()

	return nil
}

func superviseWorkerSubsystem(
	ctx context.Context,
	waitGroup *sync.WaitGroup,
	name string,
	log zerolog.Logger,
	run func(context.Context) error,
) {
	waitGroup.Add(1)

	go func() {
		defer waitGroup.Done()

		backoff := initialSubsystemRestartBackoff

		for {
			if ctx.Err() != nil {
				log.Info().Str("subsystem", name).Msg("worker subsystem stopped")
				return
			}

			log.Info().Str("subsystem", name).Msg("worker subsystem started")

			err := run(ctx)
			switch {
			case err == nil:
				if ctx.Err() != nil {
					log.Info().Str("subsystem", name).Msg("worker subsystem stopped")
					return
				}

				log.Warn().Str("subsystem", name).Msg("worker subsystem exited unexpectedly; restarting")
			case errors.Is(err, context.Canceled):
				log.Info().Str("subsystem", name).Msg("worker subsystem stopped")
				return
			default:
				log.Error().
					Err(err).
					Str("subsystem", name).
					Dur("restart_backoff", backoff).
					Msg("worker subsystem failed; restarting")
			}

			timer := time.NewTimer(backoff)
			select {
			case <-ctx.Done():
				timer.Stop()
				log.Info().Str("subsystem", name).Msg("worker subsystem stopped")
				return
			case <-timer.C:
			}

			backoff *= 2
			if backoff > maxSubsystemRestartBackoff {
				backoff = maxSubsystemRestartBackoff
			}
		}
	}()
}
