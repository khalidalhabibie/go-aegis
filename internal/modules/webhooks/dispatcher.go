package webhooks

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"
)

type dialContextFunc func(ctx context.Context, network, address string) (net.Conn, error)

type Dispatcher interface {
	Dispatch(ctx context.Context, delivery Delivery) (DispatchResult, error)
}

type HTTPDispatcher struct {
	client               *http.Client
	signer               *Signer
	targetPolicy         TargetPolicy
	responseBodyMaxBytes int
	lookupIPAddrs        lookupIPAddrsFunc
	dialContext          dialContextFunc
	now                  func() time.Time
}

func NewHTTPDispatcher(timeout time.Duration, signer *Signer, targetPolicy TargetPolicy, responseBodyMaxBytes int) *HTTPDispatcher {
	return &HTTPDispatcher{
		client:               &http.Client{Timeout: timeout},
		signer:               signer,
		targetPolicy:         targetPolicy,
		responseBodyMaxBytes: responseBodyMaxBytes,
		lookupIPAddrs:        net.DefaultResolver.LookupIPAddr,
		dialContext:          (&net.Dialer{Timeout: timeout}).DialContext,
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
	client.Transport = d.transport()
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

func (d *HTTPDispatcher) transport() http.RoundTripper {
	if d.client != nil && d.client.Transport != nil {
		if _, ok := d.client.Transport.(*http.Transport); !ok {
			return d.client.Transport
		}
	}

	baseTransport := http.DefaultTransport.(*http.Transport).Clone()
	if d.client != nil {
		if existing, ok := d.client.Transport.(*http.Transport); ok && existing != nil {
			baseTransport = existing.Clone()
		}
	}

	baseTransport.Proxy = nil
	baseTransport.DialContext = d.dialValidatedTarget
	return baseTransport
}

func (d *HTTPDispatcher) dialValidatedTarget(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("split webhook dial target: %w", err)
	}

	hostname := normalizeHostname(host)
	if hostname == "" {
		return nil, fmt.Errorf("webhook dial target is missing hostname")
	}

	targetURL := &url.URL{
		Scheme: "https",
		Host:   net.JoinHostPort(hostname, port),
	}

	_, _, ips, err := resolveDispatchTarget(ctx, targetURL, d.targetPolicy, d.lookupIPAddrs)
	if err != nil {
		return nil, err
	}

	var dialErr error
	for _, ip := range ips {
		conn, err := d.dialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if err == nil {
			return conn, nil
		}

		dialErr = errors.Join(dialErr, err)
	}

	return nil, fmt.Errorf("dial webhook target %q: %w", hostname, dialErr)
}
