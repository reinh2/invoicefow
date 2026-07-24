package ratelimit

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestDisabledLimiterAllowsEverything(t *testing.T) {
	for name, limiter := range map[string]*Limiter{
		"zero limit":   New(0, time.Minute),
		"zero period":  New(5, 0),
		"nil receiver": nil,
	} {
		t.Run(name, func(t *testing.T) {
			for attempt := 0; attempt < 50; attempt++ {
				if allowed, _ := limiter.Allow("client"); !allowed {
					t.Fatalf("request %d was refused by a disabled limiter", attempt)
				}
			}
		})
	}
}

func TestLimiterRefusesAfterTheWindowAllowanceAndRecovers(t *testing.T) {
	limiter := New(3, time.Minute)
	current := time.Unix(1_800_000_000, 0).UTC()
	limiter.now = func() time.Time { return current }

	for attempt := 1; attempt <= 3; attempt++ {
		if allowed, _ := limiter.Allow("a"); !allowed {
			t.Fatalf("request %d within the allowance was refused", attempt)
		}
	}
	allowed, retryAfter := limiter.Allow("a")
	if allowed {
		t.Fatal("the fourth request in the window must be refused")
	}
	if retryAfter <= 0 || retryAfter > time.Minute {
		t.Errorf("retryAfter = %v, want a positive duration within the window", retryAfter)
	}

	current = current.Add(time.Minute)
	if allowed, _ := limiter.Allow("a"); !allowed {
		t.Error("a new window must restore the allowance")
	}
}

func TestLimiterKeepsClientsIndependent(t *testing.T) {
	limiter := New(1, time.Minute)

	if allowed, _ := limiter.Allow("first"); !allowed {
		t.Fatal("first client refused")
	}
	if allowed, _ := limiter.Allow("first"); allowed {
		t.Fatal("first client exceeded its allowance")
	}
	// One noisy client must not consume anyone else's allowance.
	if allowed, _ := limiter.Allow("second"); !allowed {
		t.Error("second client was refused because of the first")
	}
}

func TestLimiterBoundsTrackedClients(t *testing.T) {
	limiter := New(1, time.Minute)
	limiter.maxClients = 8
	current := time.Unix(1_800_000_000, 0).UTC()
	limiter.now = func() time.Time { return current }

	// An address-rotating caller must not be able to grow the table without
	// limit; once it is full of live windows, new clients are refused.
	for index := 0; index < 100; index++ {
		limiter.Allow(fmt.Sprintf("client-%d", index))
	}
	if len(limiter.clients) > limiter.maxClients {
		t.Fatalf("tracked clients = %d, want at most %d", len(limiter.clients), limiter.maxClients)
	}

	// Once the windows expire the table drains and new clients are served again.
	current = current.Add(2 * time.Minute)
	if allowed, _ := limiter.Allow("fresh"); !allowed {
		t.Error("a new client was refused after every window had expired")
	}
}

func TestLimiterIsSafeUnderConcurrentUse(t *testing.T) {
	limiter := New(100, time.Minute)
	var group sync.WaitGroup
	for worker := 0; worker < 16; worker++ {
		group.Add(1)
		go func() {
			defer group.Done()
			for attempt := 0; attempt < 50; attempt++ {
				limiter.Allow("shared")
			}
		}()
	}
	group.Wait()

	// 800 attempts against an allowance of 100: the counter must have stopped
	// exactly at the limit rather than racing past it.
	if got := limiter.clients["shared"].count; got != 100 {
		t.Errorf("count = %d, want exactly the limit of 100", got)
	}
}
