package extractors_test

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/Decodo/pobear/pkg/jobspec"
	"github.com/Decodo/pobear/services/core/internal/stage/extractors"
)

func runMeta(t *testing.T, body string) map[string]any {
	t.Helper()
	results, err := extractors.Meta{}.Run(context.Background(), []byte(body), jobspec.Stage{Use: "meta"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected exactly 1 result, got %d", len(results))
	}
	if results[0].Key != "meta" {
		t.Errorf("key = %q, want \"meta\"", results[0].Key)
	}
	meta, ok := results[0].Value.(map[string]any)
	if !ok {
		t.Fatalf("value is %T, want map[string]any", results[0].Value)
	}
	return meta
}

func TestMetaRunFullPage(t *testing.T) {
	body := `<!doctype html>
<html lang="en">
<head>
  <title> The Go Programming Language </title>
  <meta name="Description" content="Go is an open source programming language.">
  <link rel="alternate canonical" href="https://go.dev/">
  <meta property="og:title" content="The Go Programming Language">
  <meta property="og:image" content="https://go.dev/first.png">
  <meta property="og:image" content="https://go.dev/second.png">
  <meta name="twitter:card" content="summary">
  <meta name="twitter:site" content="@golang">
  <script type="application/ld+json">{"@type": "WebSite", "name": "Go"}</script>
  <script type="application/ld+json">[{"@type": "Person"}]</script>
  <script type="text/javascript">var x = 1;</script>
</head>
<body><svg><title>icon</title></svg></body>
</html>`

	want := map[string]any{
		"title":       "The Go Programming Language",
		"description": "Go is an open source programming language.",
		"canonical":   "https://go.dev/",
		"lang":        "en",
		"og": map[string]any{
			"title": "The Go Programming Language",
			"image": "https://go.dev/first.png",
		},
		"twitter": map[string]any{
			"card": "summary",
			"site": "@golang",
		},
		"jsonld": []any{
			map[string]any{"@type": "WebSite", "name": "Go"},
			[]any{map[string]any{"@type": "Person"}},
		},
	}

	if got := runMeta(t, body); !reflect.DeepEqual(got, want) {
		t.Errorf("meta =\n%#v\nwant\n%#v", got, want)
	}
}

func TestMetaRunNoMetadataIsEmptyMap(t *testing.T) {
	got := runMeta(t, "<p>just text</p>")
	if len(got) != 0 {
		t.Errorf("meta = %#v, want an empty map", got)
	}
}

func TestMetaRunUnparsableLDJSONIsRecordedNotFatal(t *testing.T) {
	body := `<html><head>
<script type="application/ld+json">{"@type": "Broken",</script>
<script type="application/ld+json">  </script>
<script type="application/ld+json; charset=utf-8">{"@type": "Ok"}</script>
</head></html>`

	got := runMeta(t, body)

	wantLD := []any{map[string]any{"@type": "Ok"}}
	if !reflect.DeepEqual(got["jsonld"], wantLD) {
		t.Errorf("jsonld = %#v, want %#v", got["jsonld"], wantLD)
	}
	errs, ok := got["jsonldErrors"].([]any)
	if !ok || len(errs) != 1 {
		t.Fatalf("jsonldErrors = %#v, want exactly one entry", got["jsonldErrors"])
	}
	if msg, _ := errs[0].(string); !strings.HasPrefix(msg, "block 0: ") {
		t.Errorf("jsonldErrors[0] = %q, want it to name block 0", msg)
	}
}

func TestMetaRunOGUnderNameAttribute(t *testing.T) {
	got := runMeta(t, `<meta name="og:site_name" content="Example"><meta property="twitter:title" content="T">`)
	want := map[string]any{
		"og":      map[string]any{"site_name": "Example"},
		"twitter": map[string]any{"title": "T"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("meta = %#v, want %#v", got, want)
	}
}
