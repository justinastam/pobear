package fetch

import (
	"context"
	"net/url"
	"strings"
	"sync"
	"time"
)

// hostSweepFloor is the map size below which HostThrottle never bothers
// sweeping stale hosts; past it, a sweep runs whenever the map has doubled
// since the last one, so the cost stays amortized O(1) per Wait.
const hostSweepFloor = 1024

// HostThrottle enforces a minimum gap between consecutive requests to the
// same host (limits.rpsPerHost, plus an optional per-host minimum delay such
// as a robots.txt Crawl-delay -- whichever is stricter). Per-host state is a
// single timestamp, created lazily on a host's first request and swept once
// it can no longer delay anything, so a job spread over many hosts stays cheap.
type HostThrottle struct {
	interval time.Duration
	minDelay func(rawURL string) time.Duration

	mu      sync.Mutex
	last    map[string]time.Time
	maxGap  time.Duration
	sweepAt int
}

// NewHostThrottle returns nil when there's nothing to enforce (rpsPerHost <=
// 0 and no minDelay), so callers can skip it entirely. minDelay, if set, is
// consulted on every Wait and must be cheap and safe for concurrent use.
func NewHostThrottle(rpsPerHost int, minDelay func(rawURL string) time.Duration) *HostThrottle {
	if rpsPerHost <= 0 && minDelay == nil {
		return nil
	}
	var interval time.Duration
	if rpsPerHost > 0 {
		interval = time.Second / time.Duration(rpsPerHost)
	}
	return &HostThrottle{
		interval: interval,
		minDelay: minDelay,
		last:     make(map[string]time.Time),
		maxGap:   interval,
		sweepAt:  hostSweepFloor,
	}
}

// Wait reserves rawURL's host's next free slot and sleeps until it. Requests
// to different hosts never wait on each other.
func (h *HostThrottle) Wait(ctx context.Context, rawURL string) error {
	gap := h.interval
	if h.minDelay != nil {
		gap = max(gap, h.minDelay(rawURL))
	}
	key := hostKey(rawURL)

	h.mu.Lock()
	now := time.Now()
	h.maxGap = max(h.maxGap, gap)
	slot := now
	if last, ok := h.last[key]; ok {
		slot = later(now, last.Add(gap))
	}
	h.last[key] = slot
	h.sweep(now)
	h.mu.Unlock()

	wait := slot.Sub(now)
	if wait <= 0 {
		return nil
	}
	timer := time.NewTimer(wait)
	select {
	case <-ctx.Done():
		timer.Stop()
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// sweep drops hosts whose last slot is far enough in the past that no gap
// seen so far could still delay their next request -- forgetting them is
// indistinguishable from keeping them. Callers hold h.mu.
func (h *HostThrottle) sweep(now time.Time) {
	if len(h.last) < h.sweepAt {
		return
	}
	for key, last := range h.last {
		if !last.Add(h.maxGap).After(now) {
			delete(h.last, key)
		}
	}
	h.sweepAt = max(hostSweepFloor, 2*len(h.last))
}

func later(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}

// hostKey is rawURL's host[:port], lowercased; an unparseable URL shares one
// bucket keyed by the raw string, which is conservative rather than unthrottled.
func hostKey(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return rawURL
	}
	return strings.ToLower(u.Host)
}

// Pacer is the Fetcher's Throttler: the per-host throttle first (so a request
// held back for its host doesn't burn a slot of the job-wide budget while it
// waits), then the job-wide Throttle. Either may be nil.
type Pacer struct {
	Global  *Throttle
	PerHost *HostThrottle
}

func (p Pacer) Wait(ctx context.Context, rawURL string) error {
	if p.PerHost != nil {
		if err := p.PerHost.Wait(ctx, rawURL); err != nil {
			return err
		}
	}
	if p.Global != nil {
		return p.Global.Wait(ctx)
	}
	return nil
}
