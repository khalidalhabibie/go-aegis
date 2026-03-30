package webhooks

import (
	"bufio"
	"context"
	"io"
	"net"
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

	dispatcher := NewHTTPDispatcher(5*time.Second, NewSigner("super-secret"), TargetPolicy{}, 64)
	dispatcher.now = func() time.Time { return fixedTime }
	dispatcher.lookupIPAddrs = func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
	}
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

func TestHTTPDispatcherRejectsResolvedPrivateTargets(t *testing.T) {
	dispatcher := NewHTTPDispatcher(5*time.Second, nil, TargetPolicy{}, 64)
	dispatcher.lookupIPAddrs = func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("10.0.0.12")}}, nil
	}

	_, err := dispatcher.Dispatch(context.Background(), Delivery{
		ID:          "delivery-private",
		TargetURL:   "https://hooks.example.com/webhooks",
		PayloadJSON: []byte(`{"ok":true}`),
	})
	if err == nil || !strings.Contains(err.Error(), "disallowed IP") {
		t.Fatalf("expected private target rejection, got %v", err)
	}
}

func TestHTTPDispatcherRejectsRedirectToPrivateTarget(t *testing.T) {
	dispatcher := NewHTTPDispatcher(5*time.Second, nil, TargetPolicy{}, 64)
	dispatcher.lookupIPAddrs = func(_ context.Context, host string) ([]net.IPAddr, error) {
		switch host {
		case "hooks.example.com":
			return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
		case "internal.example.local":
			return []net.IPAddr{{IP: net.ParseIP("10.0.0.44")}}, nil
		default:
			return nil, nil
		}
	}
	dispatcher.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "hooks.example.com" {
			t.Fatalf("unexpected initial request host %q", r.URL.Host)
		}

		return &http.Response{
			StatusCode: http.StatusTemporaryRedirect,
			Header: http.Header{
				"Location": []string{"https://internal.example.local/webhooks"},
			},
			Body: io.NopCloser(strings.NewReader("redirect")),
		}, nil
	})

	_, err := dispatcher.Dispatch(context.Background(), Delivery{
		ID:          "delivery-redirect",
		TargetURL:   "https://hooks.example.com/webhooks",
		PayloadJSON: []byte(`{"ok":true}`),
	})
	if err == nil || !strings.Contains(err.Error(), "validate redirect target") {
		t.Fatalf("expected redirect validation failure, got %v", err)
	}
}

func TestHTTPDispatcherPinsDialToValidatedIP(t *testing.T) {
	dispatcher := NewHTTPDispatcher(5*time.Second, nil, TargetPolicy{AllowPrivateTargets: true}, 64)
	dispatcher.lookupIPAddrs = func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
	}

	dialedAddresses := make(chan string, 1)
	requestHosts := make(chan string, 1)
	dispatcher.dialContext = func(_ context.Context, _, address string) (net.Conn, error) {
		clientConn, serverConn := net.Pipe()

		go func() {
			defer serverConn.Close()

			reader := bufio.NewReader(serverConn)
			request, err := http.ReadRequest(reader)
			if err != nil {
				return
			}
			requestHosts <- request.Host

			response := &http.Response{
				StatusCode: http.StatusOK,
				ProtoMajor: 1,
				ProtoMinor: 1,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader("ok")),
			}
			_ = response.Write(serverConn)
		}()

		dialedAddresses <- address
		return clientConn, nil
	}

	result, err := dispatcher.Dispatch(context.Background(), Delivery{
		ID:          "delivery-pinned",
		TargetURL:   "http://hooks.example.com:8080/webhooks",
		PayloadJSON: []byte(`{"ok":true}`),
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if result.StatusCode != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, result.StatusCode)
	}

	select {
	case address := <-dialedAddresses:
		if address != "127.0.0.1:8080" {
			t.Fatalf("expected dial to validated IP, got %q", address)
		}
	case <-time.After(time.Second):
		t.Fatal("expected dial address to be recorded")
	}

	select {
	case host := <-requestHosts:
		if host != "hooks.example.com:8080" {
			t.Fatalf("expected request host to preserve original hostname, got %q", host)
		}
	case <-time.After(time.Second):
		t.Fatal("expected request host to be recorded")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}
