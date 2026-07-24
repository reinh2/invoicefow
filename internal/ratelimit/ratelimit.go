// Package ratelimit provides a small in-process request limiter for the one
// endpoint that costs real work: document upload.
//
// It is deliberately not a distributed rate limiter. It protects a single
// public demo process from casual abuse — a script uploading in a loop — and
// makes no claim about coordinated or distributed traffic. A production
// deployment would put a real limiter in front of the process.
package ratelimit

import (
	"sync"
	"time"
)

// Limiter is a fixed-window counter keyed by client. Windows are per-client and
// start at that client's first request, so one caller's burst cannot consume
// another caller's allowance.
type Limiter struct {
	mu      sync.Mutex
	clients map[string]*window
	limit   int
	period  time.Duration
	// maxClients bounds memory: a caller rotating source addresses must not be
	// able to grow this map without limit. When the bound is reached, expired
	// entries are dropped, and a full table of live entries refuses new clients
	// rather than growing.
	maxClients int
	now        func() time.Time
}

type window struct {
	count   int
	resetAt time.Time
}

const defaultMaxClients = 4096

// New returns a limiter allowing limit requests per period for each client.
// A limit of zero or less disables limiting entirely, and New returns nil —
// callers must treat a nil *Limiter as "allow everything", which Allow does.
func New(limit int, period time.Duration) *Limiter {
	if limit <= 0 || period <= 0 {
		return nil
	}
	return &Limiter{
		clients:    make(map[string]*window),
		limit:      limit,
		period:     period,
		maxClients: defaultMaxClients,
		now:        time.Now,
	}
}

// Allow reports whether this client may make one more request now, and how long
// it must wait when it may not.
func (l *Limiter) Allow(client string) (bool, time.Duration) {
	if l == nil {
		return true, 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	existing, found := l.clients[client]
	if found && now.Before(existing.resetAt) {
		if existing.count >= l.limit {
			return false, existing.resetAt.Sub(now)
		}
		existing.count++
		return true, 0
	}

	if !found && len(l.clients) >= l.maxClients {
		l.evictExpired(now)
		if len(l.clients) >= l.maxClients {
			// The table is full of live windows. Refusing is the safe answer: it
			// bounds memory and cannot let an address-rotating caller through.
			return false, l.period
		}
	}
	l.clients[client] = &window{count: 1, resetAt: now.Add(l.period)}
	return true, 0
}

func (l *Limiter) evictExpired(now time.Time) {
	for client, entry := range l.clients {
		if !now.Before(entry.resetAt) {
			delete(l.clients, client)
		}
	}
}
