package converter

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (function roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func TestRetryTransportRetriesTemporaryStatus(t *testing.T) {
	attempts := 0
	transport := retryTransport{
		base: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
			attempts++
			status := http.StatusOK
			if attempts == 1 {
				status = 522
			}
			return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader("response")), Request: request}, nil
		}),
		delay: func(*http.Response, int) time.Duration { return 0 },
	}
	request, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://example.test", nil)
	response, err := transport.RoundTrip(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || attempts != 2 {
		t.Fatalf("status=%d attempts=%d", response.StatusCode, attempts)
	}
}

func TestRetryTransportHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	transport := retryTransport{
		base: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusServiceUnavailable, Body: io.NopCloser(strings.NewReader("retry")), Request: request}, nil
		}),
		delay: func(*http.Response, int) time.Duration { return time.Hour },
	}
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.test", nil)
	_, err := transport.RoundTrip(request)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("RoundTrip() error = %v", err)
	}
}

func TestRetryDelayIsBounded(t *testing.T) {
	response := &http.Response{Header: http.Header{"Retry-After": []string{"86400"}}}
	if delay := retryDelay(response, 0); delay != maxRetryAfter {
		t.Fatalf("retryDelay() = %s", delay)
	}
}
