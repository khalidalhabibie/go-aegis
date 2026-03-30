package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"aegis/internal/config"
)

func TestWorkerStatusTrackerReport(t *testing.T) {
	tracker := newWorkerStatusTracker(&config.Config{
		App: config.AppConfig{
			Name:    "aegis",
			Version: "test",
			Env:     "test",
		},
	})
	tracker.RegisterSubsystem("transfer_consumer")
	tracker.MarkRunning("transfer_consumer")

	report := tracker.Report()
	if report.Status != "ok" {
		t.Fatalf("expected ok status, got %q", report.Status)
	}

	tracker.MarkRestarting("transfer_consumer", 2*time.Second, 1, errTestBoom)
	report = tracker.Report()
	if report.Status != "degraded" {
		t.Fatalf("expected degraded status, got %q", report.Status)
	}

	subsystem := report.Subsystems["transfer_consumer"]
	if subsystem.RestartCount != 1 {
		t.Fatalf("expected restart count 1, got %d", subsystem.RestartCount)
	}
	if subsystem.LastError == "" {
		t.Fatal("expected last error to be set")
	}
}

func TestWorkerHealthHandlerReturnsJSONReport(t *testing.T) {
	tracker := newWorkerStatusTracker(&config.Config{
		App: config.AppConfig{
			Name:    "aegis",
			Version: "test",
			Env:     "test",
		},
	})
	tracker.RegisterSubsystem("webhook_worker")
	tracker.MarkRunning("webhook_worker")

	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	response := httptest.NewRecorder()

	workerHealthHandler(tracker).ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, response.Code)
	}

	var report workerHealthReport
	if err := json.Unmarshal(response.Body.Bytes(), &report); err != nil {
		t.Fatalf("decode health response: %v", err)
	}

	if report.Status != "ok" {
		t.Fatalf("expected report status ok, got %q", report.Status)
	}
	if report.Subsystems["webhook_worker"].Status != workerSubsystemStatusRunning {
		t.Fatalf("expected subsystem running, got %q", report.Subsystems["webhook_worker"].Status)
	}
}

var errTestBoom = testError("boom")

type testError string

func (e testError) Error() string {
	return string(e)
}
