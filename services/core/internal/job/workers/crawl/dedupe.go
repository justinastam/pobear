package crawl

import (
	"sync"

	"github.com/Decodo/pobear/services/core/internal/job/seen"
)

type dedupe struct {
	mu   sync.Mutex
	seen map[string]struct{}
}

func newDedupe() *dedupe {
	return &dedupe{seen: make(map[string]struct{})}
}

func (d *dedupe) claim(rawURL string) bool {
	key := seen.Key(rawURL)

	d.mu.Lock()
	defer d.mu.Unlock()
	if _, ok := d.seen[key]; ok {
		return false
	}
	d.seen[key] = struct{}{}
	return true
}
