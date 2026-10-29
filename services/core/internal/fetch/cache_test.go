package fetch_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Decodo/pobear/pkg/fault"
	"github.com/Decodo/pobear/pkg/jobspec"
	"github.com/Decodo/pobear/services/core/internal/fetch"
	"github.com/Decodo/pobear/services/core/internal/fetch/tiers"
)

type countingThrottle struct{ waits int }

func (c *countingThrottle) Wait(_ context.Context) error {
	c.waits++
	return nil
}

func okTier() *fakeTier {
	return &fakeTier{results: []fakeTierResult{{result: &tiers.Result{Status: 200, Body: []byte("<h1>hi</h1>"), FinalURL: "https://a.test/final"}}}}
}

func newTestCache(t *testing.T, next fetch.Fetching, ttl time.Duration) (*fetch.Cache, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "cache")
	c, err := fetch.NewCache(next, dir, ttl)
	if err != nil {
		t.Fatalf("NewCache: %v", err)
	}
	return c, dir
}

func TestCacheHitSkipsFetcherThrottleAndRetries(t *testing.T) {
	tier := okTier()
	throttle := &countingThrottle{}
	fetcher := fetch.NewFetcher(map[string]fetch.Tier{"local": tier}, 0, 3, noBackoff, throttle)
	c, _ := newTestCache(t, fetcher, time.Hour)
	cfg := jobspec.Fetch{Use: "local"}

	first, kind, err := c.Fetch(context.Background(), cfg, "https://a.test")
	if err != nil || kind != "" {
		t.Fatalf("first fetch: kind=%q err=%v", kind, err)
	}
	if first.Cached {
		t.Fatal("first fetch reported cached, want a fresh fetch")
	}

	second, kind, err := c.Fetch(context.Background(), cfg, "https://a.test")
	if err != nil || kind != "" {
		t.Fatalf("second fetch: kind=%q err=%v", kind, err)
	}
	if !second.Cached {
		t.Fatal("second fetch not reported cached")
	}
	if string(second.Body) != "<h1>hi</h1>" || second.Status != 200 || second.FinalURL != "https://a.test/final" || second.Tier != "local" {
		t.Fatalf("cached result = %+v, want the stored response", second)
	}
	if tier.calls != 1 {
		t.Errorf("tier calls = %d, want 1", tier.calls)
	}
	if throttle.waits != 1 {
		t.Errorf("throttle waits = %d, want 1 (a hit must not wait on the throttle)", throttle.waits)
	}
}

func TestCacheDoesNotStoreFailedFetches(t *testing.T) {
	tier := &fakeTier{results: []fakeTierResult{{result: &tiers.Result{Status: 403}, fault: fault.Blocked}}}
	c, _ := newTestCache(t, tier, time.Hour)
	cfg := jobspec.Fetch{Use: "local"}

	for range 2 {
		if _, kind, _ := c.Fetch(context.Background(), cfg, "https://a.test"); kind != fault.Blocked {
			t.Fatalf("kind = %q, want %q", kind, fault.Blocked)
		}
	}
	if tier.calls != 2 {
		t.Errorf("tier calls = %d, want 2 (a failure must never be served from cache)", tier.calls)
	}
}

func TestCacheExpiredEntryIsRefetched(t *testing.T) {
	tier := okTier()
	c, _ := newTestCache(t, tier, time.Nanosecond)
	cfg := jobspec.Fetch{Use: "local"}

	for range 2 {
		if _, _, err := c.Fetch(context.Background(), cfg, "https://a.test"); err != nil {
			t.Fatalf("fetch: %v", err)
		}
		time.Sleep(time.Millisecond)
	}
	if tier.calls != 2 {
		t.Errorf("tier calls = %d, want 2 (an expired entry must be refetched)", tier.calls)
	}
}

func TestCacheKey(t *testing.T) {
	base := jobspec.Fetch{Use: "proxy", Proxy: "http://user:secret-one@p.test:8080", Profile: "chrome", Timeout: "30s",
		Headers: map[string]string{"Authorization": "Bearer secret-one", "X-Custom": "1"}}

	cases := []struct {
		name    string
		mutate  func(cfg *jobspec.Fetch)
		url     string
		wantHit bool
	}{
		{name: "identical settings", mutate: func(*jobspec.Fetch) {}, wantHit: true},
		{name: "different timeout", mutate: func(cfg *jobspec.Fetch) { cfg.Timeout = "5s" }, wantHit: true},
		{name: "different cache ttl", mutate: func(cfg *jobspec.Fetch) { cfg.Cache = &jobspec.FetchCache{TTL: "1h"} }, wantHit: true},
		{name: "different proxy password", mutate: func(cfg *jobspec.Fetch) { cfg.Proxy = "http://user:secret-two@p.test:8080" }, wantHit: true},
		{name: "different authorization value", mutate: func(cfg *jobspec.Fetch) {
			cfg.Headers = map[string]string{"Authorization": "Bearer secret-two", "X-Custom": "1"}
		}, wantHit: true},
		{name: "different url", mutate: func(*jobspec.Fetch) {}, url: "https://a.test/other", wantHit: false},
		{name: "different proxy user", mutate: func(cfg *jobspec.Fetch) { cfg.Proxy = "http://user-country-us:secret-one@p.test:8080" }, wantHit: false},
		{name: "different profile", mutate: func(cfg *jobspec.Fetch) { cfg.Profile = "firefox" }, wantHit: false},
		{name: "different header", mutate: func(cfg *jobspec.Fetch) {
			cfg.Headers = map[string]string{"Authorization": "Bearer secret-one", "X-Custom": "2"}
		}, wantHit: false},
		{name: "authorization header removed", mutate: func(cfg *jobspec.Fetch) { cfg.Headers = map[string]string{"X-Custom": "1"} }, wantHit: false},
		{name: "different tier", mutate: func(cfg *jobspec.Fetch) { cfg.Use, cfg.Proxy, cfg.Profile, cfg.Name = "provider", "", "", "acme" }, wantHit: false},
		{name: "different provider option", mutate: func(cfg *jobspec.Fetch) { cfg.Options = map[string]any{"render": true} }, wantHit: false},
		{name: "ladder on", mutate: func(cfg *jobspec.Fetch) { cfg.Ladder = true }, wantHit: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tier := okTier()
			c, dir := newTestCache(t, tier, time.Hour)
			if _, _, err := c.Fetch(context.Background(), base, "https://a.test"); err != nil {
				t.Fatalf("priming fetch: %v", err)
			}

			cfg := base
			cfg.Headers = map[string]string{"Authorization": "Bearer secret-one", "X-Custom": "1"}
			tc.mutate(&cfg)
			url := tc.url
			if url == "" {
				url = "https://a.test"
			}
			got, _, err := c.Fetch(context.Background(), cfg, url)
			if err != nil {
				t.Fatalf("second fetch: %v", err)
			}
			if got.Cached != tc.wantHit {
				t.Errorf("cached = %v, want %v", got.Cached, tc.wantHit)
			}

			entries, err := os.ReadDir(dir)
			if err != nil {
				t.Fatalf("read cache dir: %v", err)
			}
			for _, e := range entries {
				data, err := os.ReadFile(filepath.Join(dir, e.Name())) //nolint:gosec // a file this test's own cache just wrote under t.TempDir()
				if err != nil {
					t.Fatalf("read entry: %v", err)
				}
				if strings.Contains(string(data), "secret") {
					t.Errorf("cache entry %s contains a credential: %s", e.Name(), data)
				}
			}
		})
	}
}
