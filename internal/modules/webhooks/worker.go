package webhooks

import (
	"context"
	"time"
)

type Worker struct {
	service      *Service
	pollInterval time.Duration
}

func NewWorker(service *Service, pollInterval time.Duration) *Worker {
	if pollInterval <= 0 {
		pollInterval = 2 * time.Second
	}

	return &Worker{
		service:      service,
		pollInterval: pollInterval,
	}
}

func (w *Worker) Run(ctx context.Context) error {
	if err := w.service.RunCycle(ctx); err != nil {
		return err
	}

	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := w.service.RunCycle(ctx); err != nil {
				return err
			}
		}
	}
}
