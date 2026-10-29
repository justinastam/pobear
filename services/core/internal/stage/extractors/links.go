package extractors

import (
	"context"

	"github.com/Decodo/pobear/pkg/jobspec"
	"github.com/Decodo/pobear/services/core/internal/links"
)

// Links writes data.links: every http(s) <a href> on the page, resolved against
// pageURL, deduplicated in document order and filtered by the stage's own
// include/exclude globs -- the same discovery and glob rules a crawl's frontier
// applies (JOB_SPEC_REFERENCE.md §3.2, §9).
type Links struct{}

func (Links) Run(ctx context.Context, body []byte, pageURL string, cfg jobspec.Stage) ([]Result, error) {
	found := links.HTML{}.Extract(body, pageURL)

	seen := make(map[string]bool, len(found))
	out := []string{}
	for _, u := range found {
		if seen[u] || !links.MatchesIncludeExclude(u, cfg.Include, cfg.Exclude) {
			continue
		}
		seen[u] = true
		out = append(out, u)
	}
	return []Result{{Key: "links", Value: out}}, nil
}
