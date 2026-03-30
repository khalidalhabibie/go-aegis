package app

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"aegis/internal/config"

	"github.com/rs/zerolog"
)

func TestSuperviseWorkerSubsystemFailsAfterFailureBudget(t *testing.T) {
	originalInitial := initialSubsystemRestartBackoff
	originalMax := maxSubsystemRestartBackoff
	originalHealthyReset := subsystemHealthyRunResetThreshold
	originalFailureBudget := maxConsecutiveSubsystemFailures
	t.Cleanup(func() {
		initialSubsystemRestartBackoff = originalInitial
		maxSubsystemRestartBackoff = originalMax
		subsystemHealthyRunResetThreshold = originalHealthyReset
		maxConsecutiveSubsystemFailures = originalFailureBudget
	})

	initialSubsystemRestartBackoff = 5 * time.Millisecond
	maxSubsystemRestartBackoff = 5 * time.Millisecond
	subsystemHealthyRunResetThreshold = time.Hour
	maxConsecutiveSubsystemFailures = 3

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var waitGroup sync.WaitGroup
	fatalErrs := make(chan error, 1)
	tracker := newWorkerStatusTracker(&config.Config{
		App: config.AppConfig{
			Name:    "aegis",
			Version: "test",
			Env:     "test",
		},
	})
	tracker.RegisterSubsystem("test_subsystem")

	superviseWorkerSubsystem(ctx, &waitGroup, fatalErrs, tracker, "test_subsystem", zerolog.Nop(), func(context.Context) error {
		return errors.New("boom")
	})

	select {
	case err := <-fatalErrs:
		if err == nil {
			t.Fatal("expected fatal error")
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected subsystem to exceed failure budget")
	}

	cancel()
	waitGroup.Wait()
}
