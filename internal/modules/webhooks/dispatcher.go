package webhooks

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"
)

type Dispatcher interface {
	Dispatch(ctx context.Context, delivery Delivery) (DispatchResult, error)
}

type HTTPDispatcher struct {
	client               *http.Client
	signer               *Signer
	targetPolicy         TargetPolicy
	responseBodyMaxBytes int
	lookupIPAddrs        lookupIPAddrsFunc
	now                  func() time.Time
}

func NewHTTPDispatcher(timeout time.Duration, signer *Signer, targetPolicy TargetPolicy, responseBodyMaxBytes int) *HTTPDispatcher {
	return &HTTPDispatcher{
		client:               &http.Client{Timeout: timeout},
		signer:               signer,
		targetPolicy:         targetPolicy,
		responseBodyMaxBytes: responseBodyMaxBytes,
		lookupIPAddrs:        net.DefaultResolver.LookupIPAddr,
		now:                  time.Now,
	}
}

func (d *HTTPDispatcher) Dispatch(ctx context.Context, delivery Delivery) (DispatchResult, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, delivery.TargetURL, bytes.NewReader(delivery.PayloadJSON))
	if err != nil {
		return DispatchResult{}, fmt.Errorf("build webhook request: %w", err)
	}

	if err := d.validateTarget(ctx, request.URL); err != nil {
		return DispatchResult{}, fmt.Errorf("validate webhook target: %w", err)
	}

	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Aegis-Event", delivery.EventType)
	request.Header.Set("X-Aegis-Transfer-Status", delivery.TransferStatus)
	request.Header.Set("X-Aegis-Delivery-ID", delivery.ID)

	if d.signer != nil {
		now := time.Now
		if d.now != nil {
			now = d.now
		}

		timestamp, signature := d.signer.Sign(delivery.PayloadJSON, now())
		request.Header.Set(TimestampHeaderName, timestamp)
		request.Header.Set(SignatureHeaderName, signature)
	}

	client := *d.client
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return fmt.Errorf("stop after %d redirects", len(via))
		}

		if err := d.validateTarget(req.Context(), req.URL); err != nil {
			return fmt.Errorf("validate redirect target: %w", err)
		}

		return nil
	}

	response, err := client.Do(request)
	if err != nil {
		return DispatchResult{}, fmt.Errorf("send webhook request: %w", err)
	}
	defer response.Body.Close()

	maxBytes := d.responseBodyMaxBytes
	if maxBytes <= 0 {
		maxBytes = 512
	}

	body, err := io.ReadAll(io.LimitReader(response.Body, int64(maxBytes+1)))
	if err != nil {
		return DispatchResult{}, fmt.Errorf("read webhook response body: %w", err)
	}

	return DispatchResult{
		StatusCode: response.StatusCode,
		Body:       sanitizeResponseBody(body, maxBytes),
	}, nil
}

func (d *HTTPDispatcher) validateTarget(ctx context.Context, targetURL *url.URL) error {
	return validateDispatchTarget(ctx, targetURL, d.targetPolicy, d.lookupIPAddrs)
}
