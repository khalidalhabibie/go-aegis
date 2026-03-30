package webhooks

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Dispatcher interface {
	Dispatch(ctx context.Context, delivery Delivery) (DispatchResult, error)
}

type HTTPDispatcher struct {
	client *http.Client
}

func NewHTTPDispatcher(timeout time.Duration) *HTTPDispatcher {
	return &HTTPDispatcher{
		client: &http.Client{Timeout: timeout},
	}
}

func (d *HTTPDispatcher) Dispatch(ctx context.Context, delivery Delivery) (DispatchResult, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, delivery.TargetURL, bytes.NewReader(delivery.PayloadJSON))
	if err != nil {
		return DispatchResult{}, fmt.Errorf("build webhook request: %w", err)
	}

	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Aegis-Event", delivery.EventType)
	request.Header.Set("X-Aegis-Transfer-Status", delivery.TransferStatus)
	request.Header.Set("X-Aegis-Delivery-ID", delivery.ID)

	response, err := d.client.Do(request)
	if err != nil {
		return DispatchResult{}, fmt.Errorf("send webhook request: %w", err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(io.LimitReader(response.Body, 4096))
	if err != nil {
		return DispatchResult{}, fmt.Errorf("read webhook response body: %w", err)
	}

	return DispatchResult{
		StatusCode: response.StatusCode,
		Body:       string(body),
	}, nil
}
