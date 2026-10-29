package fetch_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Decodo/pobear/services/core/internal/fetch"
)

func TestNewHostThrottleIsNilWhenThereIsNothingToEnforce(t *testing.T) {
	if h := fetch.NewHostThrottle(0, nil); h != nil {
		t.Fatalf("NewHostThrottle(0, nil) = %v, want nil (no per-host limit, no min delay)", h)
	}
}

func TestHostThrottleSpacesRequestsToTheSameHost(t *testing.T) {
	h := fetch.NewHostThrottle(10, nil) // 100ms per host

	start := time.Now()
	for i := 0; i < 4; i++ {
		if err := h.Wait(context.Background(), "https://a.test/page"); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	elapsed := time.Since(start)

	if elapsed < 280*time.Millisecond {
		t.Fatalf("4 calls to one host at rpsPerHost=10 took %v, want >= ~300ms (first is free, then 100ms apart)", elapsed)
	}
	if elapsed > time.Second {
		t.Fatalf("4 calls to one host at rpsPerHost=10 took %v, want roughly 300ms, not a long hang", elapsed)
	}
}

func TestHostThrottleDoesNotSerializeDifferentHosts(t *testing.T) {
	h := fetch.NewHostThrottle(1, nil) // 1s per host

	start := time.Now()
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Go(func() {
			if err := h.Wait(context.Background(), fmt.Sprintf("https://host%d.test/", i)); err != nil {
				t.Errorf("host %d: %v", i, err)
			}
		})
	}
	wg.Wait()

	if elapsed := time.Since(start); elapsed > 200*time.Millisecond {
		t.Fatalf("first requests to 20 distinct hosts took %v, want effectively instant -- hosts must not wait on each other", elapsed)
	}
}

func TestHostThrottleKeysOnHostIgnoringPathAndCase(t *testing.T) {
	h := fetch.NewHostThrottle(5, nil) // 200ms per host

	if err := h.Wait(context.Background(), "https://A.test/one"); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if err := h.Wait(context.Background(), "https://a.test/two?x=1"); err != nil {
		t.Fatal(err)
	}
	if waited := time.Since(start); waited < 150*time.Millisecond {
		t.Fatalf("second request to the same host (different path/case) waited %v, want ~200ms", waited)
	}
}

func TestHostThrottleHonorsStricterMinDelay(t *testing.T) {
	minDelay := func(rawURL string) time.Duration {
		if rawURL == "https://slow.test/" {
			return 250 * time.Millisecond
		}
		return 0
	}
	h := fetch.NewHostThrottle(100, minDelay) // 10ms per host, unless minDelay is stricter

	for _, tc := range []struct {
		url     string
		atLeast time.Duration
		atMost  time.Duration
	}{
		{"https://slow.test/", 200 * time.Millisecond, time.Second},
		{"https://fast.test/", 0, 150 * time.Millisecond},
	} {
		if err := h.Wait(context.Background(), tc.url); err != nil {
			t.Fatal(err)
		}
		start := time.Now()
		if err := h.Wait(context.Background(), tc.url); err != nil {
			t.Fatal(err)
		}
		waited := time.Since(start)
		if waited < tc.atLeast || waited > tc.atMost {
			t.Fatalf("%s: second request waited %v, want between %v and %v", tc.url, waited, tc.atLeast, tc.atMost)
		}
	}
}

func TestHostThrottleMinDelayAloneAppliesWithUnlimitedRPSPerHost(t *testing.T) {
	h := fetch.NewHostThrottle(0, func(string) time.Duration { return 150 * time.Millisecond })

	if err := h.Wait(context.Background(), "https://a.test/"); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if err := h.Wait(context.Background(), "https://a.test/"); err != nil {
		t.Fatal(err)
	}
	if waited := time.Since(start); waited < 100*time.Millisecond {
		t.Fatalf("waited %v, want ~150ms -- a min delay must apply even when rpsPerHost is 0", waited)
	}
}

func TestHostThrottleConcurrentCallersToOneHostAreSpaced(t *testing.T) {
	h := fetch.NewHostThrottle(20, nil) // 50ms per host

	var mu sync.Mutex
	var times []time.Time
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Go(func() {
			if err := h.Wait(context.Background(), "https://a.test/"); err != nil {
				t.Errorf("wait: %v", err)
				return
			}
			mu.Lock()
			times = append(times, time.Now())
			mu.Unlock()
		})
	}
	wg.Wait()

	first, last := times[0], times[0]
	for _, ts := range times {
		if ts.Before(first) {
			first = ts
		}
		if ts.After(last) {
			last = ts
		}
	}
	if span := last.Sub(first); span < 200*time.Millisecond {
		t.Fatalf("6 concurrent requests to one host at rpsPerHost=20 spanned %v, want >= ~250ms", span)
	}
}

func TestHostThrottleRespectsContextCancellation(t *testing.T) {
	h := fetch.NewHostThrottle(1, nil)
	if err := h.Wait(context.Background(), "https://a.test/"); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	err := h.Wait(ctx, "https://a.test/")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context.DeadlineExceeded, got %v", err)
	}
	if waited := time.Since(start); waited > 500*time.Millisecond {
		t.Fatalf("Wait returned %v after its context expired, want promptly", waited)
	}
}

func TestHostThrottleStaysCorrectAcrossManyHosts(t *testing.T) {
	h := fetch.NewHostThrottle(1000, nil) // 1ms per host, so old hosts go stale quickly

	for i := 0; i < 5000; i++ {
		if err := h.Wait(context.Background(), fmt.Sprintf("https://h%d.test/", i)); err != nil {
			t.Fatal(err)
		}
	}

	// A host seen long ago may have been swept; a fresh request to it must
	// still go straight through rather than wait or misbehave.
	start := time.Now()
	if err := h.Wait(context.Background(), "https://h0.test/"); err != nil {
		t.Fatal(err)
	}
	if waited := time.Since(start); waited > 50*time.Millisecond {
		t.Fatalf("request to a long-idle host waited %v, want immediate", waited)
	}
}

func TestPacerAppliesPerHostThenGlobal(t *testing.T) {
	p := fetch.Pacer{Global: fetch.NewThrottle(0), PerHost: fetch.NewHostThrottle(5, nil)}

	if err := p.Wait(context.Background(), "https://a.test/"); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if err := p.Wait(context.Background(), "https://a.test/"); err != nil {
		t.Fatal(err)
	}
	if waited := time.Since(start); waited < 150*time.Millisecond {
		t.Fatalf("Pacer waited %v for a second request to the same host, want ~200ms from the per-host limit", waited)
	}
}

func TestPacerWithNothingConfiguredNeverWaits(t *testing.T) {
	var p fetch.Pacer
	if err := p.Wait(context.Background(), "https://a.test/"); err != nil {
		t.Fatalf("zero Pacer: %v", err)
	}
}
