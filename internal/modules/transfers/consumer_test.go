package transfers

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

func TestStartProcessingLockHeartbeatCancelsProcessingOnRefreshFailure(t *testing.T) {
	consumer := &Consumer{
		lockTTL: 30 * time.Millisecond,
		log:     zerolog.Nop(),
	}

	var canceled atomic.Bool
	lock := &stubProcessingLock{
		refreshErr: ErrProcessingLockLost,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stopHeartbeat, errs := consumer.startProcessingLockHeartbeat(ctx, func() {
		canceled.Store(true)
		cancel()
	}, lock)
	defer stopHeartbeat()

	select {
	case err := <-errs:
		if !errors.Is(err, ErrTransient) {
			t.Fatalf("expected transient error, got %v", err)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("expected heartbeat error")
	}

	if !canceled.Load() {
		t.Fatal("expected processing context to be canceled after heartbeat failure")
	}
}

func TestStartProcessingLockHeartbeatStopsCleanly(t *testing.T) {
	consumer := &Consumer{
		lockTTL: 30 * time.Millisecond,
		log:     zerolog.Nop(),
	}

	lock := &stubProcessingLock{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stopHeartbeat, errs := consumer.startProcessingLockHeartbeat(ctx, cancel, lock)
	time.Sleep(20 * time.Millisecond)
	stopHeartbeat()

	select {
	case err := <-errs:
		if err != nil {
			t.Fatalf("expected nil heartbeat error, got %v", err)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("expected heartbeat to stop cleanly")
	}
}

type stubProcessingLock struct {
	refreshErr error
}

func (s *stubProcessingLock) Refresh(context.Context, time.Duration) error {
	return s.refreshErr
}

func (s *stubProcessingLock) Release(context.Context) error {
	return nil
}
