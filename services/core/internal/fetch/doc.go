// Package fetch dispatches a job's fetch to whichever tier fetch.use
// selects (local, proxy, or provider), and owns concurrency, rate
// limiting, and retrying that SAME tier on failure. Escalating to a
// different tier (local -> proxy -> provider) is NOT owned here — it's
// a CLI-level interactive flow, not a Core concern. See
// docs/PROJECT_STRUCTURE.md for the full rationale. Cache (fetch.cache)
// wraps the whole Fetcher, so a cache hit never reaches any of the above.
package fetch
