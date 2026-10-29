// Package seen loads source.skipSeen: the set of URLs an earlier run already
// emitted, read back from that run's NDJSON output, so a resumed or
// incremental run neither re-fetches nor re-emits them.
package seen

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"sort"
)

type LinkExtractor interface {
	Extract(body []byte, baseURL string) []string
}

// Set is the loaded skip-seen set. A nil *Set is valid and empty, so callers
// never need to special-case "no skipSeen configured".
type Set struct {
	links map[string][]string
}

type envelope struct {
	URL      string `json:"url"`
	FinalURL string `json:"finalUrl"`
	Body     string `json:"body"`
}

// Load reads every envelope's `url` from the NDJSON file at path. A missing
// file is an empty set (nothing has been seen yet), so the same command line
// works for a first run and for every resume after it. A final line with no
// trailing newline that fails to parse is the half-written record of a killed
// run and is ignored; any other unparsable line is an error. When links is
// non-nil, each envelope's body is also walked for links (resolved against its
// finalUrl), so a crawl can traverse past a seen page without re-fetching it.
func Load(path string, links LinkExtractor) (*Set, error) {
	if path == "" {
		return nil, nil
	}
	f, err := os.Open(path) //nolint:gosec // path is the spec's own source.skipSeen, the user's documented input file
	if errors.Is(err, fs.ErrNotExist) {
		return &Set{links: map[string][]string{}}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("skipSeen: %w", err)
	}
	defer f.Close()

	s := &Set{links: map[string][]string{}}
	r := bufio.NewReader(f)
	for lineNo := 1; ; lineNo++ {
		line, readErr := r.ReadBytes('\n')
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return nil, fmt.Errorf("skipSeen: %s: %w", path, readErr)
		}
		truncated := errors.Is(readErr, io.EOF)
		if len(line) > 0 && !isBlank(line) {
			var env envelope
			if err := json.Unmarshal(line, &env); err != nil || env.URL == "" {
				if truncated {
					break
				}
				return nil, fmt.Errorf("skipSeen: %s:%d: not a pobear NDJSON result envelope (no `url` field)", path, lineNo)
			}
			s.add(env, links)
		}
		if truncated {
			break
		}
	}
	return s, nil
}

func (s *Set) add(env envelope, links LinkExtractor) {
	var found []string
	if links != nil {
		base := env.FinalURL
		if base == "" {
			base = env.URL
		}
		found = links.Extract([]byte(env.Body), base)
	}
	s.links[Key(env.URL)] = found
}

// Has reports whether rawURL was already emitted by the earlier run.
func (s *Set) Has(rawURL string) bool {
	_, ok := s.Links(rawURL)
	return ok
}

// Links returns the links found on a seen page's stored body (nil when Load
// wasn't asked to extract them), and whether rawURL was seen at all.
func (s *Set) Links(rawURL string) ([]string, bool) {
	if s == nil {
		return nil, false
	}
	links, ok := s.links[Key(rawURL)]
	return links, ok
}

// Len is how many distinct URLs the set holds.
func (s *Set) Len() int {
	if s == nil {
		return 0
	}
	return len(s.links)
}

// Key is the canonical form two URLs are compared by: the fragment dropped and
// the query string canonicalized (key order and each key's repeated values
// sorted). Shared with the crawl's own dedupe, so "seen" and "already visited
// this run" mean the same URL.
func Key(rawURL string) string {
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

func isBlank(line []byte) bool {
	for _, b := range line {
		if b != ' ' && b != '\t' && b != '\r' && b != '\n' {
			return false
		}
	}
	return true
}
