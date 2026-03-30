package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"aegis/internal/config"

	"github.com/rs/zerolog"
)

const (
	workerSubsystemStatusStarting   = "starting"
	workerSubsystemStatusRunning    = "running"
	workerSubsystemStatusRestarting = "restarting"
	workerSubsystemStatusStopped    = "stopped"
	workerSubsystemStatusFatal      = "fatal"
)

type workerHealthReport struct {
	Status      string                           `json:"status"`
	Service     string                           `json:"service"`
	Version     string                           `json:"version"`
	Environment string                           `json:"environment"`
	Timestamp   time.Time                        `json:"timestamp"`
	Uptime      string                           `json:"uptime"`
	Subsystems  map[string]workerSubsystemReport `json:"subsystems"`
}

type workerSubsystemReport struct {
	Status              string `json:"status"`
	RestartCount        int    `json:"restart_count"`
	ConsecutiveFailures int    `json:"consecutive_failures"`
	LastError           string `json:"last_error,omitempty"`
	LastRunDuration     string `json:"last_run_duration,omitempty"`
	LastTransitionAt    string `json:"last_transition_at,omitempty"`
}

type workerStatusTracker struct {
	mu          sync.RWMutex
	service     string
	version     string
	environment string
	startedAt   time.Time
	subsystems  map[string]workerSubsystemState
}

type workerSubsystemState struct {
	Status              string
	RestartCount        int
	ConsecutiveFailures int
	LastError           string
	LastRunDuration     time.Duration
	LastTransitionAt    time.Time
}

func newWorkerStatusTracker(cfg *config.Config) *workerStatusTracker {
	return &workerStatusTracker{
		service:     cfg.App.Name + "-worker",
		version:     cfg.App.Version,
		environment: cfg.App.Env,
		startedAt:   time.Now().UTC(),
		subsystems:  map[string]workerSubsystemState{},
	}
}

func (t *workerStatusTracker) RegisterSubsystem(name string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.subsystems[name] = workerSubsystemState{
		Status:           workerSubsystemStatusStarting,
		LastTransitionAt: time.Now().UTC(),
	}
}

func (t *workerStatusTracker) MarkRunning(name string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	state := t.subsystems[name]
	state.Status = workerSubsystemStatusRunning
	state.LastError = ""
	state.LastTransitionAt = time.Now().UTC()
	t.subsystems[name] = state
}

func (t *workerStatusTracker) MarkRestarting(name string, runtime time.Duration, consecutiveFailures int, err error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	state := t.subsystems[name]
	state.Status = workerSubsystemStatusRestarting
	state.RestartCount++
	state.ConsecutiveFailures = consecutiveFailures
	state.LastRunDuration = runtime
	if err != nil {
		state.LastError = err.Error()
	}
	state.LastTransitionAt = time.Now().UTC()
	t.subsystems[name] = state
}

func (t *workerStatusTracker) MarkStopped(name string, runtime time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()

	state := t.subsystems[name]
	state.Status = workerSubsystemStatusStopped
	state.LastRunDuration = runtime
	state.LastTransitionAt = time.Now().UTC()
	t.subsystems[name] = state
}

func (t *workerStatusTracker) MarkFatal(name string, runtime time.Duration, consecutiveFailures int, err error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	state := t.subsystems[name]
	state.Status = workerSubsystemStatusFatal
	state.RestartCount++
	state.ConsecutiveFailures = consecutiveFailures
	state.LastRunDuration = runtime
	if err != nil {
		state.LastError = err.Error()
	}
	state.LastTransitionAt = time.Now().UTC()
	t.subsystems[name] = state
}

func (t *workerStatusTracker) MarkHealthy(name string, runtime time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()

	state := t.subsystems[name]
	state.Status = workerSubsystemStatusRunning
	state.ConsecutiveFailures = 0
	state.LastError = ""
	state.LastRunDuration = runtime
	state.LastTransitionAt = time.Now().UTC()
	t.subsystems[name] = state
}

func (t *workerStatusTracker) Report() workerHealthReport {
	t.mu.RLock()
	defer t.mu.RUnlock()

	subsystems := make(map[string]workerSubsystemReport, len(t.subsystems))
	overallStatus := "ok"

	for name, state := range t.subsystems {
		if state.Status != workerSubsystemStatusRunning {
			overallStatus = "degraded"
		}

		report := workerSubsystemReport{
			Status:              state.Status,
			RestartCount:        state.RestartCount,
			ConsecutiveFailures: state.ConsecutiveFailures,
		}
		if state.LastError != "" {
			report.LastError = state.LastError
		}
		if state.LastRunDuration > 0 {
			report.LastRunDuration = state.LastRunDuration.Round(time.Millisecond).String()
		}
		if !state.LastTransitionAt.IsZero() {
			report.LastTransitionAt = state.LastTransitionAt.Format(time.RFC3339)
		}

		subsystems[name] = report
	}

	return workerHealthReport{
		Status:      overallStatus,
		Service:     t.service,
		Version:     t.version,
		Environment: t.environment,
		Timestamp:   time.Now().UTC(),
		Uptime:      time.Since(t.startedAt).Round(time.Second).String(),
		Subsystems:  subsystems,
	}
}

func startWorkerHealthServer(
	ctx context.Context,
	cfg config.WorkerConfig,
	tracker *workerStatusTracker,
	log zerolog.Logger,
) (<-chan error, func()) {
	if cfg.HealthPort <= 0 {
		return nil, func() {}
	}
	errs := make(chan error, 1)

	server := &http.Server{
		Addr:              cfg.HealthAddress(),
		Handler:           workerHealthHandler(tracker),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Info().Str("addr", server.Addr).Msg("worker health server started")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errs <- fmt.Errorf("run worker health server: %w", err)
		}
	}()

	stop := func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil && err != context.Canceled {
			log.Error().Err(err).Msg("shutdown worker health server")
		}
	}

	go func() {
		<-ctx.Done()
		stop()
	}()

	return errs, stop
}

func workerHealthHandler(tracker *workerStatusTracker) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			writer.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		report := tracker.Report()
		statusCode := http.StatusOK
		if report.Status != "ok" {
			statusCode = http.StatusServiceUnavailable
		}

		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(statusCode)
		_ = json.NewEncoder(writer).Encode(report)
	})

	return mux
}
