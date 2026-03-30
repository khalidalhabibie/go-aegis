package app

import (
	"context"
	"errors"
	"fmt"

	"aegis/internal/bootstrap"
	"aegis/internal/config"
	"aegis/internal/modules/transfers"
	"aegis/internal/modules/webhooks"

	"golang.org/x/sync/errgroup"
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

	subscription, err := container.RabbitMQ.Consume(cfg.RabbitMQ.TransferQueue, cfg.Worker.ConsumerTag)
	if err != nil {
		return fmt.Errorf("start transfer consumer: %w", err)
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
	webhookRepository := webhooks.NewPostgresRepository(container.Postgres)
	webhookDispatcher := webhooks.NewHTTPDispatcher(cfg.Webhook.Timeout)
	webhookService := webhooks.NewService(
		webhookRepository,
		webhookDispatcher,
		cfg.Webhook.MaxAttempts,
		cfg.Webhook.InitialBackoff,
		cfg.Webhook.BatchSize,
		container.Logger,
	)
	webhookWorker := webhooks.NewWorker(webhookService, cfg.Worker.WebhookPollInterval)

	container.Logger.Info().
		Str("queue", cfg.RabbitMQ.TransferQueue).
		Str("routing_key", cfg.RabbitMQ.TransferRoutingKey).
		Str("consumer_tag", cfg.Worker.ConsumerTag).
		Msg("worker ready")

	group, groupCtx := errgroup.WithContext(ctx)

	group.Go(func() error {
		if err := outboxDispatcher.Run(groupCtx); err != nil && !errors.Is(err, context.Canceled) {
			return fmt.Errorf("run transfer outbox dispatcher: %w", err)
		}

		return nil
	})

	group.Go(func() error {
		if err := transferConsumer.Run(groupCtx); err != nil && !errors.Is(err, context.Canceled) {
			return fmt.Errorf("run transfer consumer: %w", err)
		}

		return nil
	})

	group.Go(func() error {
		if err := webhookWorker.Run(groupCtx); err != nil && !errors.Is(err, context.Canceled) {
			return fmt.Errorf("run webhook worker: %w", err)
		}

		return nil
	})

	if err := group.Wait(); err != nil {
		return err
	}

	container.Logger.Info().Dur("timeout", cfg.App.ShutdownTimeout).Msg("worker shutdown requested")

	return nil
}
