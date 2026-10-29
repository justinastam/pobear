package crawl_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Decodo/pobear/internal/testutil"
	"github.com/Decodo/pobear/services/core/internal/job/workers/crawl"
)

func TestHTTPRobotsCheckerAllowsWhenNoRobotsTxt(t *testing.T) {
	dir := t.TempDir()
	srv := testutil.NewFixtureServer(dir)
	defer srv.Close()

	c := crawl.NewHTTPRobotsChecker(srv.Client())
	allowed, err := c.Allowed(context.Background(), srv.URL+"/anything")

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !allowed {
		t.Fatal("expected allowed=true when robots.txt 404s (no robots.txt means no restrictions)")
	}
}

func TestHTTPRobotsCheckerRespectsDisallow(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "robots.txt", "User-agent: *\nDisallow: /private\n")
	srv := testutil.NewFixtureServer(dir)
	defer srv.Close()

	c := crawl.NewHTTPRobotsChecker(srv.Client())

	allowed, err := c.Allowed(context.Background(), srv.URL+"/private/page")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if allowed {
		t.Fatal("expected allowed=false for a Disallow-ed path")
	}

	allowed, err = c.Allowed(context.Background(), srv.URL+"/public/page")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !allowed {
		t.Fatal("expected allowed=true for a path not covered by Disallow")
	}
}

func TestHTTPRobotsCheckerFetchesRobotsTxtOnlyOncePerOrigin(t *testing.T) {
	var fetches int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			fetches++
		}
		_, _ = w.Write([]byte("User-agent: *\nDisallow:\n"))
	}))
	defer srv.Close()

	c := crawl.NewHTTPRobotsChecker(srv.Client())
	for i := 0; i < 5; i++ {
		if _, err := c.Allowed(context.Background(), srv.URL+"/page"); err != nil {
			t.Fatalf("Allowed call %d: %v", i, err)
		}
	}

	if fetches != 1 {
		t.Fatalf("robots.txt fetched %d times, want exactly 1 (cached per origin)", fetches)
	}
}

func TestHTTPRobotsCheckerFailsClosedOnServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := crawl.NewHTTPRobotsChecker(srv.Client())
	allowed, err := c.Allowed(context.Background(), srv.URL+"/page")

	if err != nil {
		t.Fatalf("expected no error (failure is encoded in the allowed result, not surfaced as an error), got %v", err)
	}
	if allowed {
		t.Fatal("expected allowed=false when robots.txt itself 500s -- fail closed, not open")
	}
}

func TestHTTPRobotsCheckerFailsClosedOnNetworkError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	unreachableURL := srv.URL + "/page"
	srv.Close()

	c := crawl.NewHTTPRobotsChecker(http.DefaultClient)
	allowed, err := c.Allowed(context.Background(), unreachableURL)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if allowed {
		t.Fatal("expected allowed=false when robots.txt can't be reached at all -- fail closed, not open")
	}
}

func TestHTTPRobotsCheckerFailsClosedOnMalformedRobotsTxt(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(": not a valid directive at all\n"))
	}))
	defer srv.Close()

	c := crawl.NewHTTPRobotsChecker(srv.Client())
	_, err := c.Allowed(context.Background(), srv.URL+"/page")
	if err != nil {
		t.Fatalf("expected no Go error even on a parse failure (it's encoded in the allowed result), got %v", err)
	}
}

func TestHTTPRobotsCheckerCrawlDelayReadsWildcardGroup(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "robots.txt", "User-agent: somebot\nCrawl-delay: 30\n\nUser-agent: *\nCrawl-delay: 2.5\nDisallow: /private\n")
	srv := testutil.NewFixtureServer(dir)
	defer srv.Close()

	c := crawl.NewHTTPRobotsChecker(srv.Client())
	if _, err := c.Allowed(context.Background(), srv.URL+"/page"); err != nil {
		t.Fatalf("Allowed: %v", err)
	}

	if got, want := c.CrawlDelay(srv.URL+"/other"), 2500*time.Millisecond; got != want {
		t.Fatalf("CrawlDelay = %v, want %v (the User-agent: * group's, not somebot's)", got, want)
	}
}

func TestHTTPRobotsCheckerCrawlDelayIsZeroWhenUnset(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "robots.txt", "User-agent: *\nDisallow: /private\n")
	srv := testutil.NewFixtureServer(dir)
	defer srv.Close()

	c := crawl.NewHTTPRobotsChecker(srv.Client())
	if _, err := c.Allowed(context.Background(), srv.URL+"/page"); err != nil {
		t.Fatalf("Allowed: %v", err)
	}

	if got := c.CrawlDelay(srv.URL + "/page"); got != 0 {
		t.Fatalf("CrawlDelay = %v, want 0 when robots.txt sets no Crawl-delay", got)
	}
}

func TestHTTPRobotsCheckerCrawlDelayNeverFetches(t *testing.T) {
	var fetches int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fetches++
		_, _ = w.Write([]byte("User-agent: *\nCrawl-delay: 5\n"))
	}))
	defer srv.Close()

	c := crawl.NewHTTPRobotsChecker(srv.Client())
	if got := c.CrawlDelay(srv.URL + "/page"); got != 0 {
		t.Fatalf("CrawlDelay before any Allowed call = %v, want 0 (nothing cached yet)", got)
	}
	if fetches != 0 {
		t.Fatalf("CrawlDelay made %d requests, want 0 -- it must only read the cache", fetches)
	}
}
