package directoryhttp

import (
	"net/netip"
	"sync"
	"time"
)

// Limits are per API process. Forwarding headers are deliberately ignored:
// deployments behind a proxy must enforce source-IP policy at the ingress.
type Limits struct {
	RequestTimeout time.Duration
	MaxConcurrent  int
	RatePerMinute  int
	Burst          int
	MaxClients     int
	ClientTTL      time.Duration
}

func DefaultLimits() Limits {
	return Limits{RequestTimeout: 2 * time.Second, MaxConcurrent: 8,
		RatePerMinute: 60, Burst: 10, MaxClients: 10000, ClientTTL: 10 * time.Minute}
}

type bucket struct {
	credits       int64
	updated, seen time.Time
}
type limiter struct {
	mu        sync.Mutex
	clients   map[netip.Addr]bucket
	limits    Limits
	now       func() time.Time
	nextSweep time.Time
}

func newLimiter(l Limits) *limiter {
	return &limiter{clients: make(map[netip.Addr]bucket), limits: l, now: time.Now}
}

func clientIP(remote string) (netip.Addr, bool) {
	addr, err := netip.ParseAddrPort(remote)
	if err != nil {
		return netip.Addr{}, false
	}
	return addr.Addr().WithZone("").Unmap(), true
}

// An occupied client table fails closed; active entries are never evicted to
// grant fresh bursts. Sweeping is at most once a minute, bounded by MaxClients.
func (l *limiter) allow(ip netip.Addr) (allowed, full bool, retry int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if !now.Before(l.nextSweep) {
		for key, b := range l.clients {
			if now.Sub(b.seen) >= l.limits.ClientTTL {
				delete(l.clients, key)
			}
		}
		l.nextSweep = now.Add(time.Minute)
	}
	b, ok := l.clients[ip]
	// One token is one minute of nanosecond credits. Integer refill avoids
	// floating-point boundary drift in admission and Retry-After rounding.
	const tokenCredits = int64(time.Minute)
	capacity := int64(l.limits.Burst) * tokenCredits
	if !ok {
		if len(l.clients) >= l.limits.MaxClients {
			return false, true, 1
		}
		b = bucket{credits: capacity, updated: now}
	}
	rate := int64(l.limits.RatePerMinute)
	elapsed := max(int64(0), now.Sub(b.updated).Nanoseconds())
	// Clamp before multiplying: even a very old bucket cannot overflow.
	if elapsed >= (capacity-b.credits+rate-1)/rate {
		b.credits = capacity
	} else {
		b.credits += elapsed * rate
	}
	b.updated, b.seen = now, now
	if b.credits < tokenCredits {
		l.clients[ip] = b
		perSecond := rate * int64(time.Second)
		return false, false, int((tokenCredits - b.credits + perSecond - 1) / perSecond)
	}
	b.credits -= tokenCredits
	l.clients[ip] = b
	return true, false, 0
}
