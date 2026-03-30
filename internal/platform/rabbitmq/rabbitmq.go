package rabbitmq

import (
	"context"
	"errors"
	"fmt"

	"aegis/internal/config"

	amqp "github.com/rabbitmq/amqp091-go"
)

type Client struct {
	Conn          *amqp.Connection
	PrefetchCount int
}

type Consumer struct {
	channel    *amqp.Channel
	deliveries <-chan amqp.Delivery
}

func New(cfg config.RabbitMQConfig) (*Client, error) {
	conn, err := amqp.Dial(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("connect rabbitmq: %w", err)
	}

	channel, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("open rabbitmq channel: %w", err)
	}
	_ = channel.Close()

	return &Client{
		Conn:          conn,
		PrefetchCount: cfg.PrefetchCount,
	}, nil
}

func (c *Client) DeclareExchange(name string) error {
	if name == "" {
		return nil
	}

	if err := c.Status(); err != nil {
		return err
	}

	channel, err := c.openChannel()
	if err != nil {
		return err
	}
	defer channel.Close()

	if err := channel.ExchangeDeclare(name, "topic", true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare exchange %q: %w", name, err)
	}

	return nil
}

func (c *Client) DeclareQueue(name string) error {
	if name == "" {
		return nil
	}

	if err := c.Status(); err != nil {
		return err
	}

	channel, err := c.openChannel()
	if err != nil {
		return err
	}
	defer channel.Close()

	if _, err := channel.QueueDeclare(name, true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare queue %q: %w", name, err)
	}

	return nil
}

func (c *Client) BindQueue(queueName, routingKey, exchange string) error {
	if queueName == "" || exchange == "" {
		return nil
	}

	if err := c.Status(); err != nil {
		return err
	}

	channel, err := c.openChannel()
	if err != nil {
		return err
	}
	defer channel.Close()

	if err := channel.QueueBind(queueName, routingKey, exchange, false, nil); err != nil {
		return fmt.Errorf("bind queue %q: %w", queueName, err)
	}

	return nil
}

func (c *Client) Publish(ctx context.Context, exchange, routingKey string, publishing amqp.Publishing) error {
	if err := c.Status(); err != nil {
		return err
	}

	channel, err := c.openChannel()
	if err != nil {
		return err
	}
	defer channel.Close()

	if err := channel.PublishWithContext(ctx, exchange, routingKey, false, false, publishing); err != nil {
		return fmt.Errorf("publish message: %w", err)
	}

	return nil
}

func (c *Client) Consume(queueName, consumerTag string) (*Consumer, error) {
	if err := c.Status(); err != nil {
		return nil, err
	}

	channel, err := c.openChannel()
	if err != nil {
		return nil, err
	}

	if c.PrefetchCount > 0 {
		if err := channel.Qos(c.PrefetchCount, 0, false); err != nil {
			_ = channel.Close()
			return nil, fmt.Errorf("configure rabbitmq qos: %w", err)
		}
	}

	deliveries, err := channel.Consume(queueName, consumerTag, false, false, false, false, nil)
	if err != nil {
		_ = channel.Close()
		return nil, fmt.Errorf("consume queue %q: %w", queueName, err)
	}

	return &Consumer{
		channel:    channel,
		deliveries: deliveries,
	}, nil
}

func (c *Client) Status() error {
	if c == nil || c.Conn == nil {
		return errors.New("rabbitmq client is not initialized")
	}

	if c.Conn.IsClosed() {
		return errors.New("rabbitmq connection is closed")
	}

	channel, err := c.openChannel()
	if err != nil {
		return err
	}
	_ = channel.Close()

	return nil
}

func (c *Client) Close() error {
	if c == nil {
		return nil
	}

	if c.Conn != nil {
		return ignoreClosed(c.Conn.Close())
	}

	return nil
}

func (c *Client) openChannel() (*amqp.Channel, error) {
	channel, err := c.Conn.Channel()
	if err != nil {
		return nil, fmt.Errorf("open rabbitmq channel: %w", err)
	}

	return channel, nil
}

func (c *Consumer) Deliveries() <-chan amqp.Delivery {
	if c == nil {
		return nil
	}

	return c.deliveries
}

func (c *Consumer) Close() error {
	if c == nil || c.channel == nil {
		return nil
	}

	return ignoreClosed(c.channel.Close())
}

func ignoreClosed(err error) error {
	if err == nil || errors.Is(err, amqp.ErrClosed) {
		return nil
	}

	return err
}
