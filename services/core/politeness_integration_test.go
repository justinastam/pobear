package core_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Decodo/pobear/pkg/jobspec"
	"github.com/Decodo/pobear/services/core"
	"github.com/Decodo/pobear/services/core/internal/job"
)

// requestLog records when each non-robots.txt request reached a server.
type requestLog struct {
	mu    sync.Mutex
	times []time.Time
}

func (l *requestLog) record() {
	l.mu.Lock()
	l.times = append(l.times, time.Now())
	l.mu.Unlock()
}

func (l *requestLog) minGap() time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	ts := slices.Clone(l.times)
	slices.SortFunc(ts, func(a, b time.Time) int { return a.Compare(b) })
	gap := time.Duration(1<<63 - 1)
	for i := 1; i < len(ts); i++ {
		gap = min(gap, ts[i].Sub(ts[i-1]))
	}
	return gap
}

func (l *requestLog) count() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.times)
}

func newPoliteServer(t *testing.T, robots string, links []string) (*httptest.Server, *requestLog) {
	t.Helper()
	log := &requestLog{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			if robots == "" {
				http.NotFound(w, r)
				return
			}
			_, _ = w.Write([]byte(robots))
			return
		}
		log.record()
		w.Header().Set("Content-Type", "text/html")
		var body strings.Builder
		body.WriteString("<html><body>")
		if r.URL.Path == "/" {
			for _, l := range links {
				fmt.Fprintf(&body, `<a href="%s">%s</a>`, l, l)
			}
		}
		body.WriteString("</body></html>")
		_, _ = w.Write([]byte(body.String()))
	}))
	t.Cleanup(srv.Close)
	return srv, log
}

func runSpec(t *testing.T, spec string) job.Result {
	t.Helper()
	svc := core.New(t.TempDir(), filepath.Join(t.TempDir(), "credentials.env"), io.Discard)
	result, _, err := svc.Run(context.Background(), []byte(spec), jobspec.ModePlain)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	return result
}

func TestRPSPerHostPacesEachHostIndependently(t *testing.T) {
	a, aLog := newPoliteServer(t, "", nil)
	b, bLog := newPoliteServer(t, "", nil)

	var urls []string
	for i := 0; i < 4; i++ {
		urls = append(urls, fmt.Sprintf("%q", fmt.Sprintf("%s/p%d", a.URL, i)), fmt.Sprintf("%q", fmt.Sprintf("%s/p%d", b.URL, i)))
	}
	spec := fmt.Sprintf(`
version: 1
source: {urls: [%s]}
fetch: {use: local}
limits: {concurrency: 8, rps: 0, rpsPerHost: 5}
`, strings.Join(urls, ", "))

	start := time.Now()
	result := runSpec(t, spec)
	elapsed := time.Since(start)

	if result.Succeeded != 8 {
		t.Fatalf("result = %+v, want 8 succeeded", result)
	}
	for name, log := range map[string]*requestLog{"a": aLog, "b": bLog} {
		if gap := log.minGap(); gap < 150*time.Millisecond {
			t.Fatalf("host %s: closest two requests were %v apart, want >= ~200ms at rpsPerHost=5", name, gap)
		}
	}
	// 4 requests per host at 200ms apart is ~600ms; serializing the two hosts
	// behind one limiter would take ~1.4s.
	if elapsed > 1200*time.Millisecond {
		t.Fatalf("job took %v, want the two hosts paced in parallel (~600ms), not one after the other", elapsed)
	}
}

func TestCrawlHonorsRobotsCrawlDelay(t *testing.T) {
	srv, log := newPoliteServer(t, "User-agent: *\nCrawl-delay: 0.3\n", []string{"/a", "/b", "/c"})

	spec := fmt.Sprintf(`
version: 1
source: {crawl: {seeds: [%q], depth: 1, robots: respect}}
fetch: {use: local}
limits: {concurrency: 8}
`, srv.URL+"/")
	result := runSpec(t, spec)

	if result.Succeeded != 4 {
		t.Fatalf("result = %+v, want 4 succeeded (seed + 3 links)", result)
	}
	if gap := log.minGap(); gap < 250*time.Millisecond {
		t.Fatalf("closest two page requests were %v apart, want >= ~300ms from robots.txt's Crawl-delay", gap)
	}
}

func TestCrawlIgnoreRobotsSkipsCrawlDelay(t *testing.T) {
	srv, log := newPoliteServer(t, "User-agent: *\nCrawl-delay: 5\n", []string{"/a", "/b", "/c"})

	spec := fmt.Sprintf(`
version: 1
source: {crawl: {seeds: [%q], depth: 1, robots: ignore}}
fetch: {use: local}
limits: {concurrency: 8}
`, srv.URL+"/")

	start := time.Now()
	result := runSpec(t, spec)

	if result.Succeeded != 4 || log.count() != 4 {
		t.Fatalf("result = %+v (%d page requests), want 4 succeeded", result, log.count())
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("crawl took %v, want no Crawl-delay pacing with robots: ignore", elapsed)
	}
}

func TestCrawlRPSPerHostWinsWhenStricterThanCrawlDelay(t *testing.T) {
	srv, log := newPoliteServer(t, "User-agent: *\nCrawl-delay: 0.05\n", []string{"/a", "/b"})

	spec := fmt.Sprintf(`
version: 1
source: {crawl: {seeds: [%q], depth: 1, robots: respect}}
fetch: {use: local}
limits: {concurrency: 8, rpsPerHost: 4}
`, srv.URL+"/")
	result := runSpec(t, spec)

	if result.Succeeded != 3 {
		t.Fatalf("result = %+v, want 3 succeeded", result)
	}
	if gap := log.minGap(); gap < 200*time.Millisecond {
		t.Fatalf("closest two page requests were %v apart, want >= ~250ms from rpsPerHost=4 (stricter than the 50ms Crawl-delay)", gap)
	}
}
