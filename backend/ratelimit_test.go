package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/time/rate"
)

func TestIPRateLimiterAllowsBurstThenBlocks(t *testing.T) {
	limiter := newIPRateLimiter(RateLimitConfig{RequestsPerSecond: 1, Burst: 3})

	for i := 0; i < 3; i++ {
		require.True(t, limiter.allow("1.2.3.4"), "burst request %d should be allowed", i)
	}
	require.False(t, limiter.allow("1.2.3.4"), "request beyond the burst should be rate limited")

	// A different client IP gets its own bucket.
	require.True(t, limiter.allow("5.6.7.8"))
}

func TestIPRateLimiterDefaults(t *testing.T) {
	limiter := newIPRateLimiter(RateLimitConfig{})
	require.Equal(t, rate.Limit(DefaultRateLimitPerSecond), limiter.rate)
	require.Equal(t, DefaultRateLimitBurst, limiter.burst)
}

func TestClientIPStripsPort(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	r.RemoteAddr = "203.0.113.5:54321"
	require.Equal(t, "203.0.113.5", clientIP(r))

	// Client-supplied headers must not influence the key, or a client could
	// trivially bypass the limiter by claiming a different IP per request.
	r.Header.Set("X-Forwarded-For", "9.9.9.9")
	require.Equal(t, "203.0.113.5", clientIP(r))
}
