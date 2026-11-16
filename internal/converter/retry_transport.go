package converter

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"time"
)

const (
	maxHTTPAttempts = 5
	maxRetryAfter   = 30 * time.Second
)

type retryTransport struct {
	base  http.RoundTripper
	delay func(*http.Response, int) time.Duration
}

func (t retryTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	for attempt := 0; ; attempt++ {
		response, err := t.base.RoundTrip(request)
		if !shouldRetry(response, err) || attempt+1 >= maxHTTPAttempts {
			return response, err
		}
		if response != nil {
			_, _ = io.Copy(io.Discard, response.Body)
			_ = response.Body.Close()
		}
		delayForAttempt := t.delay
		if delayForAttempt == nil {
			delayForAttempt = retryDelay
		}
		delay := delayForAttempt(response, attempt)
		select {
		case <-request.Context().Done():
			return nil, request.Context().Err()
		case <-time.After(delay):
		}
	}
}

func shouldRetry(response *http.Response, err error) bool {
	if err != nil {
		return true
	}
	return response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500
}

func retryDelay(response *http.Response, attempt int) time.Duration {
	if response != nil {
		if seconds, err := strconv.Atoi(response.Header.Get("Retry-After")); err == nil && seconds > 0 {
			delay := time.Duration(seconds) * time.Second
			if delay > maxRetryAfter {
				return maxRetryAfter
			}
			return delay
		}
	}
	return time.Second << attempt
}

func withRetryClient(client *http.Client) *http.Client {
	copy := *client
	base := copy.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	copy.Transport = retryTransport{base: base}
	previousRedirectPolicy := copy.CheckRedirect
	copy.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		if len(via) == 0 {
			return nil
		}
		if request.URL.User != nil || (request.URL.Scheme != "http" && request.URL.Scheme != "https") {
			return fmt.Errorf("redirect URL must be HTTP(S) without credentials")
		}
		if ip := net.ParseIP(request.URL.Hostname()); ip != nil && (ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() || ip.IsLinkLocalUnicast()) {
			return fmt.Errorf("redirect IP %q is not allowed", request.URL.Hostname())
		}
		if !sameHost(via[0].URL.Hostname(), request.URL.Hostname()) && !trustedImageHost(request.URL.Hostname()) {
			return fmt.Errorf("redirect host %q is not allowed", request.URL.Hostname())
		}
		if previousRedirectPolicy != nil {
			return previousRedirectPolicy(request, via)
		}
		if len(via) >= 10 {
			return fmt.Errorf("stopped after 10 redirects")
		}
		return nil
	}
	return &copy
}
