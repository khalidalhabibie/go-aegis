package app

import (
	"fmt"

	"aegis/internal/config"
	"aegis/internal/platform/rabbitmq"
)

func ensureTransferQueue(client *rabbitmq.Client, cfg config.RabbitMQConfig) error {
	if err := client.DeclareExchange(cfg.Exchange); err != nil {
		return fmt.Errorf("declare transfer exchange: %w", err)
	}

	if err := client.DeclareQueue(cfg.TransferQueue); err != nil {
		return fmt.Errorf("declare transfer queue: %w", err)
	}

	if err := client.BindQueue(cfg.TransferQueue, cfg.TransferRoutingKey, cfg.Exchange); err != nil {
		return fmt.Errorf("bind transfer queue: %w", err)
	}

	return nil
}
