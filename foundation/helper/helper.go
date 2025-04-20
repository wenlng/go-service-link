package helper

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
)

// GetHostname .
func GetHostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		return "-"
	}
	return hostname
}

// SplitHostPort .
func SplitHostPort(addr string) (string, int, error) {
	parts := strings.Split(addr, ":")
	if len(parts) != 2 {
		return "", 0, fmt.Errorf("invalid address format: %s", addr)
	}
	port, err := strconv.Atoi(parts[1])
	if err != nil {
		return "", 0, fmt.Errorf("invalid port: %s", parts[1])
	}
	return parts[0], port, nil
}

// IsDurationSet .
func IsDurationSet(d time.Duration) bool {
	return d != 0
}

// WithRetry index backoff and retry
func WithRetry(ctx context.Context, operation func() error) error {
	return WithRetryWithBc(ctx, operation, &backoff.ExponentialBackOff{
		MaxElapsedTime: 30 * time.Second,
	})
}

// WithRetryWithBc index backoff and retry
func WithRetryWithBc(ctx context.Context, operation func() error, bc *backoff.ExponentialBackOff) error {
	b := backoff.NewExponentialBackOff()

	if bc != nil {
		if IsDurationSet(bc.MaxElapsedTime) {
			b.MaxElapsedTime = bc.MaxElapsedTime
		} else {
			b.MaxElapsedTime = 30 * time.Second
		}
		if IsDurationSet(bc.InitialInterval) {
			b.InitialInterval = bc.InitialInterval
		}
		if IsDurationSet(bc.MaxInterval) {
			b.MaxInterval = bc.MaxInterval
		}
		if bc.RandomizationFactor > 0.0 {
			b.RandomizationFactor = bc.RandomizationFactor
		}
		if bc.Multiplier > 0.0 {
			b.Multiplier = bc.Multiplier
		}
	} else {
		b.MaxElapsedTime = 30 * time.Second
	}

	return backoff.Retry(func() error {
		select {
		case <-ctx.Done():
			return backoff.Permanent(ctx.Err())
		default:
			return operation()
		}
	}, backoff.WithContext(b, ctx))
}

// DoRetry perform the operation using the retry logic
func DoRetry(operation func() error, maxRetries int, baseRetryDelay time.Duration) error {
	var err error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		err = operation()
		if err == nil {
			return nil
		}

		// Calculate the delay with random jitter
		b := 1 << uint(attempt-1)
		delay := time.Duration(float64(baseRetryDelay) * float64(b))
		jitter := time.Duration(rand.Intn(100)) * time.Millisecond
		time.Sleep(delay + jitter)

		// If the client fails, try to reconnect
		if attempt < maxRetries {
			err = operation()
			if err == nil {
				return nil
			}
		}
	}
	return nil
}

// EnsureHTTP ...
func EnsureHTTP(url string) string {
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return "http://" + url
	}
	return url
}
