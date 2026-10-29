package sink

import (
	"context"
	"sync"

	"github.com/Decodo/pobear/services/core/internal/sink/sinks"
)

type EmptyCounter struct {
	inner      ItemWriter
	hasStages  bool
	mu         sync.Mutex
	emptyItems int
}

type ItemWriter interface {
	Write(ctx context.Context, item sinks.Item) error
	Close() error
}

func NewEmptyCounter(inner ItemWriter, hasStages bool) *EmptyCounter {
	return &EmptyCounter{inner: inner, hasStages: hasStages}
}

func (c *EmptyCounter) Write(ctx context.Context, item sinks.Item) error {
	if c.hasStages && !hasExtractedValue(item.Data) {
		c.mu.Lock()
		c.emptyItems++
		c.mu.Unlock()
	}
	return c.inner.Write(ctx, item)
}

func (c *EmptyCounter) Close() error { return c.inner.Close() }

func (c *EmptyCounter) Empty() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.emptyItems
}

func hasExtractedValue(data map[string]any) bool {
	for _, v := range data {
		switch value := v.(type) {
		case nil:
			continue
		case string:
			if value != "" {
				return true
			}
		case []string:
			if len(value) > 0 {
				return true
			}
		case []any:
			if len(value) > 0 {
				return true
			}
		default:
			return true
		}
	}
	return false
}

type CachedCounter struct {
	inner       ItemWriter
	mu          sync.Mutex
	cachedItems int
}

func NewCachedCounter(inner ItemWriter) *CachedCounter {
	return &CachedCounter{inner: inner}
}

func (c *CachedCounter) Write(ctx context.Context, item sinks.Item) error {
	if item.Cached {
		c.mu.Lock()
		c.cachedItems++
		c.mu.Unlock()
	}
	return c.inner.Write(ctx, item)
}

func (c *CachedCounter) Close() error { return c.inner.Close() }

func (c *CachedCounter) Cached() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.cachedItems
}
