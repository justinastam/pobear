package sinks_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Decodo/pobear/services/core/internal/sink/sinks"
)

func readCSVFile(t *testing.T, path string) string {
	t.Helper()
	got, err := os.ReadFile(path) //nolint:gosec // path is a t.TempDir() path this test constructed itself
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(got)
}

func TestCSVWritesEachRowAsItCompletes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.csv")
	c := &sinks.CSV{Path: path, DataKeys: []string{"price", "title"}}

	item := sinks.Item{
		URL: "https://a.test", Status: 200, Tier: "local",
		Data: map[string]any{"title": []string{"A Light in the Attic"}, "price": []string{"£51.77"}},
	}
	if err := c.Write(context.Background(), item); err != nil {
		t.Fatalf("Write 1: expected no error, got %v", err)
	}

	want := "url,status,tier,price,title\nhttps://a.test,200,local,£51.77,A Light in the Attic\n"
	if got := readCSVFile(t, path); got != want {
		t.Fatalf("expected the header and first row on disk after one Write, before Close:\ngot:\n%s\nwant:\n%s", got, want)
	}

	item = sinks.Item{URL: "https://b.test", Status: 200, Tier: "proxy", Data: map[string]any{"title": []string{}, "price": []string{"£1"}}}
	if err := c.Write(context.Background(), item); err != nil {
		t.Fatalf("Write 2: expected no error, got %v", err)
	}
	want += "https://b.test,200,proxy,£1,\n"
	if got := readCSVFile(t, path); got != want {
		t.Fatalf("after 2 Writes:\ngot:\n%s\nwant:\n%s", got, want)
	}

	if err := c.Close(); err != nil {
		t.Fatalf("Close: expected no error, got %v", err)
	}
}

func TestCSVCellValues(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  string
	}{
		{name: "missing key", value: nil, want: ``},
		{name: "no matches", value: []string{}, want: ``},
		{name: "one match is the plain value", value: []string{"Hello"}, want: `Hello`},
		{name: "several matches join with a newline, quoted", value: []string{"a", "b"}, want: "\"a\nb\""},
		{name: "comma and quote are escaped", value: `say "hi", then`, want: `"say ""hi"", then"`},
		{name: "generic list", value: []any{"a", float64(2)}, want: "\"a\n2\""},
		{name: "mapping is JSON-encoded", value: map[string]any{"k": "<v>"}, want: `"{""k"":""<v>""}"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "out.csv")
			c := &sinks.CSV{Path: path, DataKeys: []string{"f"}}
			data := map[string]any{}
			if tt.value != nil {
				data["f"] = tt.value
			}
			if err := c.Write(context.Background(), sinks.Item{URL: "https://a.test", Status: 200, Tier: "local", Data: data}); err != nil {
				t.Fatalf("Write: %v", err)
			}
			_ = c.Close()

			want := "url,status,tier,f\nhttps://a.test,200,local," + tt.want + "\n"
			if got := readCSVFile(t, path); got != want {
				t.Fatalf("got:\n%q\nwant:\n%q", got, want)
			}
		})
	}
}

func TestCSVDropsKeysOutsideDataKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.csv")
	c := &sinks.CSV{Path: path, DataKeys: []string{"title"}}

	item := sinks.Item{URL: "https://a.test", Status: 200, Tier: "local", Data: map[string]any{"title": "T", "surprise": "x"}}
	if err := c.Write(context.Background(), item); err != nil {
		t.Fatalf("Write: %v", err)
	}
	_ = c.Close()

	want := "url,status,tier,title\nhttps://a.test,200,local,T\n"
	if got := readCSVFile(t, path); got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestCSVWithNoDataKeysWritesBodyColumn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.csv")
	c := &sinks.CSV{Path: path}

	if err := c.Write(context.Background(), sinks.Item{URL: "https://a.test", Status: 200, Tier: "local", Body: "<h1>Hi</h1>"}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	_ = c.Close()

	want := "url,status,tier,body\nhttps://a.test,200,local,<h1>Hi</h1>\n"
	if got := readCSVFile(t, path); got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestCSVWriteAfterCloseErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.csv")
	c := &sinks.CSV{Path: path}

	_ = c.Close()

	if err := c.Write(context.Background(), sinks.Item{URL: "https://a.test"}); err == nil {
		t.Fatal("Write after Close: expected an error, got nil")
	}
}

func TestCSVCloseWithNoWritesWritesHeaderOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.csv")
	c := &sinks.CSV{Path: path, DataKeys: []string{"title"}}

	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	want := "url,status,tier,title\n"
	if got := readCSVFile(t, path); got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}
