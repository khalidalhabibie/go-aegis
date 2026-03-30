package bootstrap

import (
	"context"
	"errors"
	"time"

	"aegis/internal/config"
	"aegis/internal/platform/blockchain"
	"aegis/internal/platform/logger"
	"aegis/internal/platform/postgres"
	"aegis/internal/platform/rabbitmq"
	redisplatform "aegis/internal/platform/redis"

	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
)

type Container struct {
	Config     *config.Config
	Logger     zerolog.Logger
	Postgres   *pgxpool.Pool
	Redis      *goredis.Client
	RabbitMQ   *rabbitmq.Client
	Blockchain *blockchain.Adapter
	StartedAt  time.Time
}

func NewContainer(ctx context.Context, cfg *config.Config) (*Container, error) {
	log := logger.New(cfg.Log.Level, cfg.App.Name, cfg.App.Env)

	startupCtx, cancel := context.WithTimeout(ctx, cfg.App.StartupTimeout)
	defer cancel()

	pgPool, err := postgres.New(startupCtx, cfg.Database)
	if err != nil {
		return nil, err
	}

	redisClient, err := redisplatform.New(startupCtx, cfg.Redis)
	if err != nil {
		pgPool.Close()
		return nil, err
	}

	rabbitClient, err := rabbitmq.New(cfg.RabbitMQ)
	if err != nil {
		_ = redisClient.Close()
		pgPool.Close()
		return nil, err
	}

	blockchainAdapter, err := blockchain.New(startupCtx, cfg.Blockchain, log)
	if err != nil {
		_ = rabbitClient.Close()
		_ = redisClient.Close()
		pgPool.Close()
		return nil, err
	}

	log.Info().Msg("runtime dependencies initialized")

	return &Container{
		Config:     cfg,
		Logger:     log,
		Postgres:   pgPool,
		Redis:      redisClient,
		RabbitMQ:   rabbitClient,
		Blockchain: blockchainAdapter,
		StartedAt:  time.Now().UTC(),
	}, nil
}

func (c *Container) Close() error {
	var err error

	if c == nil {
		return nil
	}

	if c.Blockchain != nil {
		err = errors.Join(err, c.Blockchain.Close())
	}

	if c.RabbitMQ != nil {
		err = errors.Join(err, c.RabbitMQ.Close())
	}

	if c.Redis != nil {
		err = errors.Join(err, c.Redis.Close())
	}

	if c.Postgres != nil {
		c.Postgres.Close()
	}

	return err
}
