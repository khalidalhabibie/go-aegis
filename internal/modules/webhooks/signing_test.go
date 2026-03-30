package webhooks

import (
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
