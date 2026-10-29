package workers

import (
	"net/url"
	"sort"
	"sync"
)

type Dedupe struct {
	mu   sync.Mutex
	seen map[string]struct{}
}

func NewDedupe() *Dedupe {
	return &Dedupe{seen: make(map[string]struct{})}
}

func (d *Dedupe) Claim(rawURL string) bool {
	key := dedupeKey(rawURL)

	d.mu.Lock()
	defer d.mu.Unlock()
	if _, ok := d.seen[key]; ok {
		return false
	}
	d.seen[key] = struct{}{}
	return true
}

func (d *Dedupe) Seen(rawURL string) bool {
	key := dedupeKey(rawURL)

	d.mu.Lock()
	defer d.mu.Unlock()
	_, ok := d.seen[key]
	return ok
}

func dedupeKey(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	u.Fragment = ""
	u.RawFragment = ""
	u.RawQuery = canonicalQuery(u.Query())
	return u.String()
}

func canonicalQuery(q url.Values) string {
	for _, values := range q {
		sort.Strings(values)
	}
	return q.Encode()
}
