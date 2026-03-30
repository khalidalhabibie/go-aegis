package transfers

import (
	"context"
	"encoding/json"
	"fmt"

	"aegis/internal/config"
	"aegis/internal/platform/rabbitmq"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/rs/zerolog"
)

type JobPublisher interface {
	PublishTransferRequested(ctx context.Context, job TransferJob) error
}

type RabbitMQJobPublisher struct {
	client *rabbitmq.Client
	config config.RabbitMQConfig
	log    zerolog.Logger
}

func NewRabbitMQJobPublisher(client *rabbitmq.Client, cfg config.RabbitMQConfig, log zerolog.Logger) *RabbitMQJobPublisher {
	return &RabbitMQJobPublisher{
		client: client,
		config: cfg,
		log:    log,
	}
}

func (p *RabbitMQJobPublisher) PublishTransferRequested(ctx context.Context, job TransferJob) error {
	body, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("marshal transfer job: %w", err)
	}

	if err := p.client.Publish(ctx, p.config.Exchange, p.config.TransferRoutingKey, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		Body:         body,
		Type:         OutboxEventTypeTransferRequested,
	}); err != nil {
		return fmt.Errorf("publish transfer job: %w", err)
	}

	p.log.Info().
		Str("transfer_id", job.TransferID).
		Int("attempt", job.Attempt).
		Msg("rabbitmq transfer job published")

	return nil
}
