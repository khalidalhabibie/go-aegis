package webhooks

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestSignerProducesExpectedSignature(t *testing.T) {
	signer := NewSigner("super-secret")
	timestamp, signature := signer.Sign([]byte(`{"ok":true}`), time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC))

	if timestamp != "2026-03-30T12:00:00Z" {
		t.Fatalf("unexpected timestamp %q", timestamp)
	}

	expected := ComputeSignature([]byte("super-secret"), timestamp, []byte(`{"ok":true}`))
	if signature != expected {
		t.Fatalf("expected signature %q, got %q", expected, signature)
	}
}

func TestSanitizeResponseBodyTruncatesAndRemovesNullBytes(t *testing.T) {
	body := []byte("ok\x00-secret-value")
	sanitized := sanitizeResponseBody(body, 4)

	if sanitized != "ok-...[truncated]" {
		t.Fatalf("unexpected sanitized body %q", sanitized)
	}
}

func TestHTTPDispatcherAddsDeterministicSignedHeaders(t *testing.T) {
	fixedTime := time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)
	delivery := Delivery{
		ID:             "delivery-1",
		TargetURL:      "https://example.com/webhooks",
		EventType:      "transfer.status.updated",
		TransferStatus: "SUBMITTED",
		PayloadJSON:    []byte(`{"transfer_id":"t-1","status":"SUBMITTED"}`),
	}

	dispatcher := NewHTTPDispatcher(5*time.Second, NewSigner("super-secret"), 64)
	dispatcher.now = func() time.Time { return fixedTime }
	dispatcher.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if got := r.Header.Get(TimestampHeaderName); got != "2026-03-30T12:00:00Z" {
			t.Fatalf("unexpected timestamp header %q", got)
		}

		expectedSignature := ComputeSignature([]byte("super-secret"), "2026-03-30T12:00:00Z", delivery.PayloadJSON)
		if got := r.Header.Get(SignatureHeaderName); got != expectedSignature {
			t.Fatalf("unexpected signature header %q", got)
		}

		if got := r.Header.Get("X-Aegis-Delivery-ID"); got != delivery.ID {
			t.Fatalf("unexpected delivery id header %q", got)
		}

		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("ok")),
			Header:     make(http.Header),
		}, nil
	})

	result, err := dispatcher.Dispatch(context.Background(), delivery)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if result.StatusCode != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, result.StatusCode)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}
