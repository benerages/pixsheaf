package httpapi

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"math"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Client identity and rate limiting (ADR 0022).

type clientKeyType struct{}

// clientKeyFrom returns the client key stored by the rate limiter.
func clientKeyFrom(ctx context.Context) string {
	k, _ := ctx.Value(clientKeyType{}).(string)
	return k
}

// clientIdentifier turns a request into an opaque client key: an HMAC of
// the client address with a key that exists only in memory, so the
// address itself is kept nowhere.
type clientIdentifier struct {
	trusted []netip.Prefix
	secret  []byte
}

func newClientIdentifier(trusted []netip.Prefix) clientIdentifier {
	secret := make([]byte, 32)
	_, _ = rand.Read(secret)
	return clientIdentifier{trusted: trusted, secret: secret}
}

func (c clientIdentifier) isTrusted(a netip.Addr) bool {
	for _, p := range c.trusted {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

// address returns the client address: the connection peer, or, if the
// peer is a trusted proxy, the rightmost X-Forwarded-For entry that is not
// a trusted proxy.
func (c clientIdentifier) address(r *http.Request) netip.Addr {
	ap, err := netip.ParseAddrPort(r.RemoteAddr)
	if err != nil {
		return netip.Addr{}
	}
	addr := ap.Addr().Unmap()
	if !c.isTrusted(addr) {
		return addr
	}
	var hops []string
	for _, v := range r.Header.Values("X-Forwarded-For") {
		hops = append(hops, strings.Split(v, ",")...)
	}
	for i := len(hops) - 1; i >= 0; i-- {
		hop, err := netip.ParseAddr(strings.TrimSpace(hops[i]))
		if err != nil {
			// A malformed entry ends the trusted chain; use what we have.
			return addr
		}
		addr = hop.Unmap()
		if !c.isTrusted(addr) {
			return addr
		}
	}
	return addr
}

// key returns the client key. IPv6 addresses count per /64 prefix.
func (c clientIdentifier) key(r *http.Request) string {
	addr := c.address(r)
	if addr.Is6() {
		if p, err := addr.Prefix(64); err == nil {
			addr = p.Addr()
		}
	}
	mac := hmac.New(sha256.New, c.secret)
	b, _ := addr.MarshalBinary()
	_, _ = mac.Write(b)
	return hex.EncodeToString(mac.Sum(nil)[:16])
}

// rateLimiter is a token bucket per client key: perMinute requests per
// minute with a burst of the same size.
type rateLimiter struct {
	perMinute float64
	now       func() time.Time

	mu      sync.Mutex
	buckets map[string]*bucket
	calls   int
}

type bucket struct {
	tokens float64
	last   time.Time
}

func newRateLimiter(perMinute int) *rateLimiter {
	return &rateLimiter{perMinute: float64(perMinute), now: time.Now, buckets: map[string]*bucket{}}
}

// allow takes a token for key. If none is left, it returns the seconds
// until the next one.
func (l *rateLimiter) allow(key string) (bool, int) {
	if l == nil {
		return true, 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.calls++
	if l.calls%1024 == 0 {
		l.prune(now)
	}
	b, ok := l.buckets[key]
	if !ok {
		b = &bucket{tokens: l.perMinute, last: now}
		l.buckets[key] = b
	}
	rate := l.perMinute / 60
	b.tokens = math.Min(l.perMinute, b.tokens+now.Sub(b.last).Seconds()*rate)
	b.last = now
	if b.tokens >= 1 {
		b.tokens--
		return true, 0
	}
	return false, int(math.Ceil((1 - b.tokens) / rate))
}

// prune drops buckets that have been refilled completely.
func (l *rateLimiter) prune(now time.Time) {
	for k, b := range l.buckets {
		if now.Sub(b.last) > time.Minute {
			delete(l.buckets, k)
		}
	}
}

// limitClients identifies the client of every API request, stores its key
// in the context, and answers 429 when its bucket is empty.
func limitClients(ids clientIdentifier, limiter *rateLimiter, logger *slog.Logger, stats *RequestStats, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		key := ids.key(r)
		r = r.WithContext(context.WithValue(r.Context(), clientKeyType{}, key))
		if ok, wait := limiter.allow(key); limiter != nil && !ok {
			w.Header().Set("Retry-After", strconv.Itoa(max(wait, 1)))
			writeProblem(w, http.StatusTooManyRequests, codeRateLimited, "too many requests; retry later")
			stats.count(http.StatusTooManyRequests, codeRateLimited)
			logRequest(logger, r, http.StatusTooManyRequests, codeRateLimited, 0)
			return
		}
		next.ServeHTTP(w, r)
	})
}
