package config

import (
	"net"
	"strconv"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
)

type Config struct {
	App          AppConfig
	Log          LogConfig
	HTTP         HTTPConfig
	InternalAuth InternalAuthConfig
	CallbackURL  CallbackURLConfig
	Worker       WorkerConfig
	Webhook      WebhookConfig
	Database     DatabaseConfig
	Redis        RedisConfig
	RabbitMQ     RabbitMQConfig
	Blockchain   BlockchainConfig
}

type AppConfig struct {
	Name            string        `env:"APP_NAME" envDefault:"aegis"`
	Env             string        `env:"APP_ENV" envDefault:"development"`
	Version         string        `env:"APP_VERSION" envDefault:"dev"`
	StartupTimeout  time.Duration `env:"APP_STARTUP_TIMEOUT" envDefault:"20s"`
	ShutdownTimeout time.Duration `env:"APP_SHUTDOWN_TIMEOUT" envDefault:"15s"`
}

type LogConfig struct {
	Level string `env:"LOG_LEVEL" envDefault:"info"`
}

type HTTPConfig struct {
	Host              string        `env:"HTTP_HOST" envDefault:"0.0.0.0"`
	Port              int           `env:"HTTP_PORT" envDefault:"8080"`
	ReadTimeout       time.Duration `env:"HTTP_READ_TIMEOUT" envDefault:"10s"`
	ReadHeaderTimeout time.Duration `env:"HTTP_READ_HEADER_TIMEOUT" envDefault:"5s"`
	WriteTimeout      time.Duration `env:"HTTP_WRITE_TIMEOUT" envDefault:"15s"`
	IdleTimeout       time.Duration `env:"HTTP_IDLE_TIMEOUT" envDefault:"60s"`
}

func (c HTTPConfig) Address() string {
	return net.JoinHostPort(c.Host, strconv.Itoa(c.Port))
}

type InternalAuthConfig struct {
	HeaderName string `env:"INTERNAL_AUTH_HEADER" envDefault:"X-Aegis-Internal-Key"`
	APIKey     string `env:"INTERNAL_AUTH_API_KEY"`
}

type CallbackURLConfig struct {
	AllowedHosts        []string `env:"CALLBACK_URL_ALLOWED_HOSTS" envSeparator:","`
	AllowPrivateTargets bool     `env:"CALLBACK_URL_ALLOW_PRIVATE_TARGETS" envDefault:"false"`
}

type WorkerConfig struct {
	ConsumerTag                   string        `env:"WORKER_CONSUMER_TAG" envDefault:"aegis-worker"`
	TransferMaxRetries            int           `env:"WORKER_TRANSFER_MAX_RETRIES" envDefault:"3"`
	TransferRetryDelay            time.Duration `env:"WORKER_TRANSFER_RETRY_DELAY" envDefault:"2s"`
	TransferProcessLockTTL        time.Duration `env:"WORKER_TRANSFER_PROCESS_LOCK_TTL" envDefault:"30s"`
	TransferOutboxPollInterval    time.Duration `env:"WORKER_TRANSFER_OUTBOX_POLL_INTERVAL" envDefault:"2s"`
	TransferOutboxBatchSize       int           `env:"WORKER_TRANSFER_OUTBOX_BATCH_SIZE" envDefault:"25"`
	TransferOutboxRetryDelay      time.Duration `env:"WORKER_TRANSFER_OUTBOX_RETRY_DELAY" envDefault:"2s"`
	TransferOutboxProcessingAfter time.Duration `env:"WORKER_TRANSFER_OUTBOX_PROCESSING_AFTER" envDefault:"30s"`
	WebhookPollInterval           time.Duration `env:"WORKER_WEBHOOK_POLL_INTERVAL" envDefault:"2s"`
}

type WebhookConfig struct {
	Timeout              time.Duration `env:"WEBHOOK_TIMEOUT" envDefault:"10s"`
	MaxAttempts          int           `env:"WEBHOOK_MAX_ATTEMPTS" envDefault:"5"`
	InitialBackoff       time.Duration `env:"WEBHOOK_INITIAL_BACKOFF" envDefault:"2s"`
	BatchSize            int           `env:"WEBHOOK_BATCH_SIZE" envDefault:"25"`
	LeaseDuration        time.Duration `env:"WEBHOOK_LEASE_DURATION" envDefault:"30s"`
	SigningSecret        string        `env:"WEBHOOK_SIGNING_SECRET"`
	ResponseBodyMaxBytes int           `env:"WEBHOOK_RESPONSE_BODY_MAX_BYTES" envDefault:"512"`
}

type DatabaseConfig struct {
	URL             string        `env:"DATABASE_URL,required"`
	MaxConns        int32         `env:"DATABASE_MAX_CONNS" envDefault:"20"`
	MinConns        int32         `env:"DATABASE_MIN_CONNS" envDefault:"2"`
	MaxConnLifetime time.Duration `env:"DATABASE_MAX_CONN_LIFETIME" envDefault:"30m"`
	MaxConnIdleTime time.Duration `env:"DATABASE_MAX_CONN_IDLE_TIME" envDefault:"5m"`
}

type RedisConfig struct {
	Addr     string `env:"REDIS_ADDR" envDefault:"127.0.0.1:6379"`
	Username string `env:"REDIS_USERNAME"`
	Password string `env:"REDIS_PASSWORD"`
	DB       int    `env:"REDIS_DB" envDefault:"0"`
}

type RabbitMQConfig struct {
	URL                string `env:"RABBITMQ_URL,required"`
	PrefetchCount      int    `env:"RABBITMQ_PREFETCH_COUNT" envDefault:"10"`
	Exchange           string `env:"RABBITMQ_EXCHANGE" envDefault:"aegis.events"`
	TransferQueue      string `env:"RABBITMQ_TRANSFER_QUEUE" envDefault:"transfer_requests"`
	TransferRoutingKey string `env:"RABBITMQ_TRANSFER_ROUTING_KEY" envDefault:"transfers.requested"`
}

type BlockchainConfig struct {
	RPCURL         string        `env:"EVM_RPC_URL"`
	ChainID        int64         `env:"EVM_CHAIN_ID" envDefault:"1"`
	RequestTimeout time.Duration `env:"EVM_REQUEST_TIMEOUT" envDefault:"10s"`
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}
