package seen_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Decodo/pobear/services/core/internal/job/seen"
	"github.com/Decodo/pobear/services/core/internal/job/workers"
)

func writeFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "prev.ndjson")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
	return path
}

func TestLoadEmptyPathIsANilEmptySet(t *testing.T) {
	s, err := seen.Load("", nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if s.Has("https://a.test") || s.Len() != 0 {
		t.Fatalf("expected an empty set, got Len=%d", s.Len())
	}
}

func TestLoadMissingFileIsAnEmptySet(t *testing.T) {
	s, err := seen.Load(filepath.Join(t.TempDir(), "nope.ndjson"), nil)
	if err != nil {
		t.Fatalf("expected a missing file to mean nothing seen yet, got %v", err)
	}
	if s.Len() != 0 {
		t.Fatalf("Len = %d, want 0", s.Len())
	}
}

func TestLoadReadsEveryEnvelopeURL(t *testing.T) {
	path := writeFile(t, `{"url":"https://a.test/1","body":"x"}`+"\n"+
		"\n"+
		`{"url":"https://a.test/2","body":"y"}`+"\n")

	s, err := seen.Load(path, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !s.Has("https://a.test/1") || !s.Has("https://a.test/2") {
		t.Fatal("expected both envelope URLs to be seen")
	}
	if s.Has("https://a.test/3") {
		t.Fatal("expected a URL not in the file to be unseen")
	}
}

func TestLoadToleratesATruncatedLastLine(t *testing.T) {
	path := writeFile(t, `{"url":"https://a.test/1","body":"x"}`+"\n"+`{"url":"https://a.test/2","bo`)

	s, err := seen.Load(path, nil)
	if err != nil {
		t.Fatalf("expected a killed run's half-written last line to be ignored, got %v", err)
	}
	if !s.Has("https://a.test/1") || s.Has("https://a.test/2") || s.Len() != 1 {
		t.Fatalf("expected only the complete record to be seen, got Len=%d", s.Len())
	}
}

func TestLoadAcceptsACompleteLastLineWithoutNewline(t *testing.T) {
	path := writeFile(t, `{"url":"https://a.test/1","body":"x"}`)

	s, err := seen.Load(path, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !s.Has("https://a.test/1") {
		t.Fatal("expected the parseable last line to count")
	}
}

func TestLoadRejectsAMalformedLineBeforeTheEnd(t *testing.T) {
	path := writeFile(t, "[\n"+`{"url":"https://a.test/1"}`+"\n")

	_, err := seen.Load(path, nil)
	if err == nil {
		t.Fatal("expected an error for a file that isn't pobear NDJSON")
	}
	if !strings.Contains(err.Error(), ":1: not a pobear NDJSON result envelope") {
		t.Fatalf("error = %q, want it to name line 1", err)
	}
}

func TestHasMatchesCanonicalURLForms(t *testing.T) {
	path := writeFile(t, `{"url":"https://a.test/p?b=2&a=1#top"}`+"\n")

	s, err := seen.Load(path, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !s.Has("https://a.test/p?a=1&b=2") {
		t.Fatal("expected query order and fragment not to matter")
	}
	if s.Has("https://a.test/p/?a=1&b=2") {
		t.Fatal("expected a trailing slash to still be a different URL")
	}
}

func TestLinksAreExtractedFromTheStoredBodyAgainstFinalURL(t *testing.T) {
	path := writeFile(t, `{"url":"https://a.test/old","finalUrl":"https://a.test/dir/new","body":"<a href=\"child\">c</a><a href=\"mailto:x@a.test\">m</a>"}`+"\n")

	s, err := seen.Load(path, workers.HTMLLinks{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	links, ok := s.Links("https://a.test/old")
	if !ok {
		t.Fatal("expected the requested url to be seen")
	}
	if want := []string{"https://a.test/dir/child"}; !slices.Equal(links, want) {
		t.Fatalf("links = %v, want %v", links, want)
	}
}
