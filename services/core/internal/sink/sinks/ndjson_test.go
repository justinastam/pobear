package sinks_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Decodo/pobear/services/core/internal/sink/sinks"
)

func assertNDJSONLineURLs(t *testing.T, content []byte, wantURLs []string) {
	t.Helper()
	lines := strings.Split(string(content), "\n")
	i := 0
	for _, line := range lines {
		if line == "" {
			continue
		}
		if i >= len(wantURLs) {
			t.Fatalf("more non-empty lines than expected %d:\n%s", len(wantURLs), content)
		}
		var got sinks.Item
		if err := json.Unmarshal([]byte(line), &got); err != nil {
			t.Fatalf("line %d: invalid JSON: %v", i, err)
		}
		if got.URL != wantURLs[i] {
			t.Errorf("line %d: url = %q, want %q", i, got.URL, wantURLs[i])
		}
		i++
	}
	if i != len(wantURLs) {
		t.Fatalf("got %d non-empty lines, want %d:\n%s", i, len(wantURLs), content)
	}
}

func TestNDJSONWritesEachItemAsItCompletes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.ndjson")
	n := &sinks.NDJSON{Path: path}

	if err := n.Write(context.Background(), sinks.Item{URL: "https://a.test"}); err != nil {
		t.Fatalf("Write 1: expected no error, got %v", err)
	}

	got, err := os.ReadFile(path) //nolint:gosec // path is a t.TempDir() path this test constructed itself
	if err != nil {
		t.Fatalf("expected the file to already contain the first item after one Write, but it doesn't exist: %v", err)
	}
	if strings.Count(string(got), "\n") != 1 {
		t.Fatalf("expected exactly 1 line after 1 Write, got:\n%s", got)
	}
	assertNDJSONLineURLs(t, got, []string{"https://a.test"})

	if err := n.Write(context.Background(), sinks.Item{URL: "https://b.test"}); err != nil {
		t.Fatalf("Write 2: expected no error, got %v", err)
	}
	got, err = os.ReadFile(path) //nolint:gosec // path is a t.TempDir() path this test constructed itself
	if err != nil {
		t.Fatalf("read after 2nd write: %v", err)
	}
	if strings.Count(string(got), "\n") != 2 {
		t.Fatalf("expected exactly 2 lines after 2 Writes, got:\n%s", got)
	}
	assertNDJSONLineURLs(t, got, []string{"https://a.test", "https://b.test"})

	if err := n.Close(); err != nil {
		t.Fatalf("Close: expected no error, got %v", err)
	}
}

func TestNDJSONWriteAfterCloseErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.ndjson")
	n := &sinks.NDJSON{Path: path}

	_ = n.Close()

	if err := n.Write(context.Background(), sinks.Item{URL: "https://a.test"}); err == nil {
		t.Fatal("Write after Close: expected an error, got nil")
	}
}

func TestNDJSONCloseWithNoWritesCreatesEmptyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.ndjson")
	n := &sinks.NDJSON{Path: path}

	_ = n.Close()

	got, err := os.ReadFile(path) //nolint:gosec // path is a t.TempDir() path this test constructed itself
	if err != nil {
		t.Fatalf("expected the file to exist after Close with no Writes, got: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected an empty (0-byte) file, got %d bytes:\n%s", len(got), got)
	}
}

func TestNDJSONAppendKeepsExistingLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.ndjson")
	if err := os.WriteFile(path, []byte(`{"url":"https://a.test"}`+"\n"), 0o600); err != nil {
		t.Fatalf("seeding file: %v", err)
	}
	n := &sinks.NDJSON{Path: path, Append: true}

	if err := n.Write(context.Background(), sinks.Item{URL: "https://b.test"}); err != nil {
		t.Fatalf("Write: expected no error, got %v", err)
	}
	if err := n.Close(); err != nil {
		t.Fatalf("Close: expected no error, got %v", err)
	}

	got, err := os.ReadFile(path) //nolint:gosec // path is a t.TempDir() path this test constructed itself
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	assertNDJSONLineURLs(t, got, []string{"https://a.test", "https://b.test"})
}

func TestNDJSONAppendDropsATruncatedLastLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.ndjson")
	seed := `{"url":"https://a.test"}` + "\n" + `{"url":"https://half` + strings.Repeat("x", 5000)
	if err := os.WriteFile(path, []byte(seed), 0o600); err != nil {
		t.Fatalf("seeding file: %v", err)
	}
	n := &sinks.NDJSON{Path: path, Append: true}

	if err := n.Write(context.Background(), sinks.Item{URL: "https://b.test"}); err != nil {
		t.Fatalf("Write: expected no error, got %v", err)
	}
	_ = n.Close()

	got, err := os.ReadFile(path) //nolint:gosec // path is a t.TempDir() path this test constructed itself
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	assertNDJSONLineURLs(t, got, []string{"https://a.test", "https://b.test"})
}

func TestNDJSONAppendCloseWithNoWritesLeavesFileIntact(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.ndjson")
	seed := `{"url":"https://a.test"}` + "\n"
	if err := os.WriteFile(path, []byte(seed), 0o600); err != nil {
		t.Fatalf("seeding file: %v", err)
	}
	n := &sinks.NDJSON{Path: path, Append: true}

	if err := n.Close(); err != nil {
		t.Fatalf("Close: expected no error, got %v", err)
	}

	got, err := os.ReadFile(path) //nolint:gosec // path is a t.TempDir() path this test constructed itself
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != seed {
		t.Fatalf("file = %q, want it untouched %q", got, seed)
	}
}

func TestNDJSONAppendCreatesAMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.ndjson")
	n := &sinks.NDJSON{Path: path, Append: true}

	if err := n.Write(context.Background(), sinks.Item{URL: "https://a.test"}); err != nil {
		t.Fatalf("Write: expected no error, got %v", err)
	}
	_ = n.Close()

	got, err := os.ReadFile(path) //nolint:gosec // path is a t.TempDir() path this test constructed itself
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	assertNDJSONLineURLs(t, got, []string{"https://a.test"})
}
