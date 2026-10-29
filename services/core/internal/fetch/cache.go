package fetch

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"maps"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/Decodo/pobear/pkg/fault"
	"github.com/Decodo/pobear/pkg/jobspec"
	"github.com/Decodo/pobear/services/core/internal/fetch/tiers"
)

// cacheKeyVersion is folded into every key, so a change to what a key covers
// (or to the entry format) orphans old entries instead of misreading them.
const cacheKeyVersion = 1

type Fetching interface {
	Fetch(ctx context.Context, cfg jobspec.Fetch, url string) (*tiers.Result, fault.Kind, error)
}

// Cache is fetch.cache: an on-disk response cache in front of a Fetcher. A
// hit returns before next is ever called, so it costs no semaphore slot, no
// throttle wait and no retry; a miss falls through to next, and only a
// successful result is written back.
type Cache struct {
	next Fetching
	dir  string
	ttl  time.Duration
	now  func() time.Time
}

type cacheEntry struct {
	URL      string    `json:"url"`
	StoredAt time.Time `json:"storedAt"`
	Status   int       `json:"status"`
	FinalURL string    `json:"finalUrl"`
	Tier     string    `json:"tier"`
	Body     []byte    `json:"body"`
}

func NewCache(next Fetching, dir string, ttl time.Duration) (*Cache, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return &Cache{next: next, dir: dir, ttl: ttl, now: time.Now}, nil
}

func (c *Cache) Fetch(ctx context.Context, cfg jobspec.Fetch, rawURL string) (*tiers.Result, fault.Kind, error) {
	path := filepath.Join(c.dir, cacheKey(cfg, rawURL)+".json")
	if result, ok := c.load(path, rawURL); ok {
		return result, "", nil
	}

	result, kind, err := c.next.Fetch(ctx, cfg, rawURL)
	if err == nil && kind == "" && result != nil {
		c.store(path, rawURL, result)
	}
	return result, kind, err
}

func (c *Cache) load(path, rawURL string) (*tiers.Result, bool) {
	data, err := os.ReadFile(path) //nolint:gosec // path is the cache dir joined with a hex digest, never user-shaped
	if err != nil {
		return nil, false
	}
	var e cacheEntry
	if err := json.Unmarshal(data, &e); err != nil {
		return nil, false
	}
	if e.URL != rawURL || c.now().Sub(e.StoredAt) > c.ttl {
		return nil, false
	}
	return &tiers.Result{Status: e.Status, Body: e.Body, FinalURL: e.FinalURL, Tier: e.Tier, Cached: true}, true
}

// store is best effort: a cache that can't be written is a slower next run,
// never a failed fetch. The entry is written to a temp file and renamed into
// place, so a concurrent reader never sees half of one.
func (c *Cache) store(path, rawURL string, result *tiers.Result) {
	data, err := json.Marshal(cacheEntry{
		URL: rawURL, StoredAt: c.now(), Status: result.Status, FinalURL: result.FinalURL, Tier: result.Tier, Body: result.Body,
	})
	if err != nil {
		return
	}
	tmp, err := os.CreateTemp(c.dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return
	}
	_, writeErr := tmp.Write(data)
	closeErr := tmp.Close()
	if writeErr != nil || closeErr != nil || os.Rename(tmp.Name(), path) != nil {
		_ = os.Remove(tmp.Name())
	}
}

// cacheKey digests the URL plus every fetch setting that can change the
// response. timeout only decides whether a fetch finishes, not what it
// returns, so it's left out; credentials are never part of the key -- a
// proxy URL's password and the values of credential-bearing headers are
// blanked first, and provider credentials never reach cfg at all.
func cacheKey(cfg jobspec.Fetch, rawURL string) string {
	cfg.Timeout = ""
	cfg.Cache = nil
	cfg.Proxy = redactProxyPassword(cfg.Proxy)
	cfg.Headers = redactCredentialHeaders(cfg.Headers)

	data, _ := json.Marshal(struct {
		V     int           `json:"v"`
		URL   string        `json:"url"`
		Fetch jobspec.Fetch `json:"fetch"`
	}{cacheKeyVersion, rawURL, cfg})
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func redactProxyPassword(proxy string) string {
	u, err := url.Parse(proxy)
	if err != nil || u.User == nil {
		return proxy
	}
	if _, hasPassword := u.User.Password(); hasPassword {
		u.User = url.User(u.User.Username())
	}
	return u.String()
}

var credentialHeaders = map[string]bool{
	"Authorization":       true,
	"Proxy-Authorization": true,
	"Cookie":              true,
}

func redactCredentialHeaders(headers map[string]string) map[string]string {
	out := maps.Clone(headers)
	for name := range out {
		if credentialHeaders[http.CanonicalHeaderKey(name)] {
			out[name] = ""
		}
	}
	return out
}
