package main

import (
	"net"
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// RateLimitConfig bounds requests per client IP to the sensitive VOG
// endpoints (upload, start-disclosure, issue), so one client can't burst
// requests into the (small, fixed-size) PDFium worker pool or hammer the
// validatie.nl / IRMA server calls behind them.
type RateLimitConfig struct {
	// Sustained requests per second per client IP. Defaults to 2.
	RequestsPerSecond float64 `json:"requests_per_second,omitempty"`
	// Burst allowed on top of the sustained rate. Defaults to 10.
	Burst int `json:"burst,omitempty"`
}

const (
	DefaultRateLimitPerSecond = 2
	DefaultRateLimitBurst     = 10

	// visitorExpiry is how long an idle IP's limiter is kept around; without
	// this the visitor map would grow without bound.
	visitorExpiry = 10 * time.Minute
	cleanupPeriod = time.Minute
)

type visitor struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// ipRateLimiter hands out a token bucket per client IP.
type ipRateLimiter struct {
	mutex    sync.Mutex
	visitors map[string]*visitor
	rate     rate.Limit
	burst    int
}

func newIPRateLimiter(config RateLimitConfig) *ipRateLimiter {
	perSecond := config.RequestsPerSecond
	if perSecond <= 0 {
		perSecond = DefaultRateLimitPerSecond
	}
	burst := config.Burst
	if burst <= 0 {
		burst = DefaultRateLimitBurst
	}
	l := &ipRateLimiter{
		visitors: make(map[string]*visitor),
		rate:     rate.Limit(perSecond),
		burst:    burst,
	}
	go l.cleanupLoop()
	return l
}

func (l *ipRateLimiter) allow(ip string) bool {
	l.mutex.Lock()
	v, ok := l.visitors[ip]
	if !ok {
		v = &visitor{limiter: rate.NewLimiter(l.rate, l.burst)}
		l.visitors[ip] = v
	}
	v.lastSeen = time.Now()
	l.mutex.Unlock()
	return v.limiter.Allow()
}

func (l *ipRateLimiter) cleanupLoop() {
	for {
		time.Sleep(cleanupPeriod)
		cutoff := time.Now().Add(-visitorExpiry)
		l.mutex.Lock()
		for ip, v := range l.visitors {
			if v.lastSeen.Before(cutoff) {
				delete(l.visitors, ip)
			}
		}
		l.mutex.Unlock()
	}
}

// rateLimited wraps h so that requests exceeding the per-IP rate get a 429
// instead of reaching the handler.
func rateLimited(limiter *ipRateLimiter, endpoint string, h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r)
		if !limiter.allow(ip) {
			respondWithErr(w, http.StatusTooManyRequests, ErrorRateLimited, "rate limit exceeded", nil, "endpoint", endpoint, "client_ip", ip)
			return
		}
		h(w, r)
	}
}

// clientIP returns the TCP peer address rather than a client-supplied header
// (e.g. X-Forwarded-For), which a client could set to any value and so
// trivially bypass the limiter with.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
