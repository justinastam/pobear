package extractors_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/Decodo/pobear/pkg/jobspec"
	"github.com/Decodo/pobear/services/core/internal/stage/extractors"
)

func TestLinksRun(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		pageURL string
		include []string
		exclude []string
		want    []string
	}{
		{
			name:    "resolves-against-page-url-in-document-order",
			body:    `<a href="b.html">b</a><a href="/a">a</a><a href="https://other.test/x">x</a>`,
			pageURL: "https://a.test/dir/index.html",
			want:    []string{"https://a.test/dir/b.html", "https://a.test/a", "https://other.test/x"},
		},
		{
			name:    "dedupes-keeping-first-occurrence",
			body:    `<a href="/a">1</a><a href="/b">2</a><a href="https://a.test/a">3</a>`,
			pageURL: "https://a.test/",
			want:    []string{"https://a.test/a", "https://a.test/b"},
		},
		{
			name:    "drops-non-http-schemes",
			body:    `<a href="mailto:x@a.test">m</a><a href="javascript:void(0)">j</a><a href="/ok">ok</a>`,
			pageURL: "https://a.test/",
			want:    []string{"https://a.test/ok"},
		},
		{
			name:    "include-keeps-only-matches",
			body:    `<a href="/blog/1">1</a><a href="/about">a</a><a href="/blog/2">2</a>`,
			pageURL: "https://a.test/",
			include: []string{"**/blog/**"},
			want:    []string{"https://a.test/blog/1", "https://a.test/blog/2"},
		},
		{
			name:    "exclude-wins-over-include",
			body:    `<a href="/blog/1">1</a><a href="/blog/tag/x">t</a>`,
			pageURL: "https://a.test/",
			include: []string{"**/blog/**"},
			exclude: []string{"**/tag/**"},
			want:    []string{"https://a.test/blog/1"},
		},
		{
			name:    "no-links-is-an-empty-list-not-nil",
			body:    `<p>nothing here</p>`,
			pageURL: "https://a.test/",
			want:    []string{},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			results, err := extractors.Links{}.Run(context.Background(), []byte(c.body), c.pageURL, jobspec.Stage{Use: "links", Include: c.include, Exclude: c.exclude})
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if len(results) != 1 {
				t.Fatalf("expected exactly 1 result, got %d", len(results))
			}
			if results[0].Key != "links" {
				t.Errorf("key = %q, want \"links\"", results[0].Key)
			}
			got, ok := results[0].Value.([]string)
			if !ok {
				t.Fatalf("value type = %T, want []string", results[0].Value)
			}
			if got == nil || !reflect.DeepEqual(got, c.want) {
				t.Errorf("value = %#v, want %#v", got, c.want)
			}
		})
	}
}
