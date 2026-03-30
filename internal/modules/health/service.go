package health

import (
	"context"
	"time"

	"aegis/internal/platform/blockchain"
	"aegis/internal/platform/rabbitmq"

	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"
)

type Dependencies struct {
	Postgres   *pgxpool.Pool
	Redis      *goredis.Client
	RabbitMQ   *rabbitmq.Client
	Blockchain *blockchain.Adapter
}

type Metadata struct {
	Name        string
	Version     string
	Environment string
}

type ComponentStatus struct {
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

type Report struct {
	Status      string                     `json:"status"`
	Service     string                     `json:"service"`
	Version     string                     `json:"version"`
	Environment string                     `json:"environment"`
	Timestamp   time.Time                  `json:"timestamp"`
	Uptime      string                     `json:"uptime"`
	Components  map[string]ComponentStatus `json:"components"`
}

type Service struct {
	deps      Dependencies
	metadata  Metadata
	startedAt time.Time
}

func NewService(deps Dependencies, metadata Metadata, startedAt time.Time) *Service {
	return &Service{
		deps:      deps,
		metadata:  metadata,
		startedAt: startedAt,
	}
}

func (s *Service) Check(ctx context.Context) Report {
	components := map[string]ComponentStatus{
		"postgres": s.checkComponent(ctx, func(checkCtx context.Context) error {
			return s.deps.Postgres.Ping(checkCtx)
		}),
		"redis": s.checkComponent(ctx, func(checkCtx context.Context) error {
			return s.deps.Redis.Ping(checkCtx).Err()
		}),
		"rabbitmq": s.checkComponent(ctx, func(context.Context) error {
			return s.deps.RabbitMQ.Status()
		}),
	}

	if s.deps.Blockchain == nil || !s.deps.Blockchain.Enabled() {
		components["blockchain"] = ComponentStatus{Status: "skipped"}
	} else {
		components["blockchain"] = s.checkComponent(ctx, func(checkCtx context.Context) error {
			return s.deps.Blockchain.Status(checkCtx)
		})
	}

	overallStatus := "ok"
	for _, component := range components {
		if component.Status != "ok" && component.Status != "skipped" {
			overallStatus = "degraded"
			break
		}
	}

	return Report{
		Status:      overallStatus,
		Service:     s.metadata.Name,
		Version:     s.metadata.Version,
		Environment: s.metadata.Environment,
		Timestamp:   time.Now().UTC(),
		Uptime:      time.Since(s.startedAt).Round(time.Second).String(),
		Components:  components,
	}
}

func (s *Service) checkComponent(ctx context.Context, fn func(context.Context) error) ComponentStatus {
	checkCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	if err := fn(checkCtx); err != nil {
		return ComponentStatus{
			Status: "down",
			Error:  err.Error(),
		}
	}

	return ComponentStatus{Status: "ok"}
}
