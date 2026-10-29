package extractors_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/Decodo/pobear/pkg/jobspec"
	"github.com/Decodo/pobear/services/core/internal/stage/extractors"
)

func TestCSSRun(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		fields  map[string]string
		want    []extractors.Result
		wantErr bool
	}{
		{
			name:   "text-selector",
			body:   "<h1>Hello</h1>",
			fields: map[string]string{"title": "h1"},
			want:   []extractors.Result{{Key: "title", Value: []string{"Hello"}}},
		},
		{
			name:   "attr-selector",
			body:   `<a href="https://x.test">text</a>`,
			fields: map[string]string{"link": "a@href"},
			want:   []extractors.Result{{Key: "link", Value: []string{"https://x.test"}}},
		},
		{
			name:   "multiple-fields",
			body:   `<h1>Hello</h1><a href="https://x.test">text</a>`,
			fields: map[string]string{"title": "h1", "link": "a@href"},
			want: []extractors.Result{
				{Key: "title", Value: []string{"Hello"}},
				{Key: "link", Value: []string{"https://x.test"}},
			},
		},
		{
			name:   "no-match-is-empty-list",
			body:   "<p>nothing here</p>",
			fields: map[string]string{"title": "h1"},
			want:   []extractors.Result{{Key: "title", Value: []string{}}},
		},
		{
			name:   "multiple-matches-returns-all-in-document-order",
			body:   "<li>one</li><li>two</li>",
			fields: map[string]string{"items": "li"},
			want:   []extractors.Result{{Key: "items", Value: []string{"one", "two"}}},
		},
		{
			name:   "nested-markup-is-text-only",
			body:   "<h1>Hello <em>world</em></h1>",
			fields: map[string]string{"title": "h1"},
			want:   []extractors.Result{{Key: "title", Value: []string{"Hello world"}}},
		},
		{
			name:   "attr-present-element-missing-attr",
			body:   "<a>text</a>",
			fields: map[string]string{"link": "a@href"},
			want:   []extractors.Result{{Key: "link", Value: []string{}}},
		},
		{
			name:    "invalid-selector-errors",
			body:    "<a>text</a>",
			fields:  map[string]string{"bad": "[["},
			wantErr: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			css := extractors.CSS{}
			results, err := css.Run(context.Background(), []byte(c.body), jobspec.Stage{Use: "css", Fields: c.fields})
			if c.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if len(results) != len(c.want) {
				t.Fatalf("got %d results, want %d: %+v", len(results), len(c.want), results)
			}
			byKey := make(map[string]any, len(results))
			for _, r := range results {
				byKey[r.Key] = r.Value
			}
			for _, w := range c.want {
				got, ok := byKey[w.Key]
				if !ok {
					t.Errorf("missing result for key %q", w.Key)
					continue
				}
				if !reflect.DeepEqual(got, w.Value) {
					t.Errorf("key %q: value = %v, want %v", w.Key, got, w.Value)
				}
			}
		})
	}
}

func TestCSSRunItems(t *testing.T) {
	const products = `<article class="p"><h3><a title="Book A" href="/a">A…</a></h3><p class="price">£1</p></article>` +
		`<article class="p"><h3><a title="Book B" href="/b">B…</a></h3><p class="price">£2</p><p class="price">£3</p></article>` +
		`<article class="p" data-id="c"><h3>no link</h3></article>`

	cases := []struct {
		name    string
		body    string
		items   string
		fields  map[string]string
		want    []map[string]any
		wantErr bool
	}{
		{
			name:   "one-record-per-item-in-document-order",
			body:   products,
			items:  "article.p",
			fields: map[string]string{"title": "h3 a@title", "price": ".price"},
			want: []map[string]any{
				{"title": "Book A", "price": "£1"},
				{"title": "Book B", "price": "£2"},
				{"title": nil, "price": nil},
			},
		},
		{
			name:   "empty-selector-is-the-item-itself",
			body:   `<a class="x" href="/1">one</a><a class="x" href="/2">two</a>`,
			items:  "a.x",
			fields: map[string]string{"url": "@href", "label": ""},
			want: []map[string]any{
				{"url": "/1", "label": "one"},
				{"url": "/2", "label": "two"},
			},
		},
		{
			name:   "first-match-that-has-the-attr-wins",
			body:   `<div><a>no href</a><a href="/y">y</a></div>`,
			items:  "div",
			fields: map[string]string{"link": "a@href"},
			want:   []map[string]any{{"link": "/y"}},
		},
		{
			name:   "fields-do-not-leak-across-items",
			body:   `<li><b>1</b></li><li></li><b>outside</b>`,
			items:  "li",
			fields: map[string]string{"n": "b"},
			want:   []map[string]any{{"n": "1"}, {"n": nil}},
		},
		{
			name:   "no-item-match-is-empty-list",
			body:   "<p>nothing here</p>",
			items:  "article",
			fields: map[string]string{"title": "h1"},
			want:   []map[string]any{},
		},
		{
			name:    "invalid-items-selector-errors",
			body:    products,
			items:   "[[",
			fields:  map[string]string{"title": "h1"},
			wantErr: true,
		},
		{
			name:    "invalid-field-selector-errors",
			body:    products,
			items:   "article",
			fields:  map[string]string{"bad": "[["},
			wantErr: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			css := extractors.CSS{}
			results, err := css.Run(context.Background(), []byte(c.body), jobspec.Stage{Use: "css", Items: c.items, Fields: c.fields})
			if c.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			want := []extractors.Result{{Key: "items", Value: c.want}}
			if !reflect.DeepEqual(results, want) {
				t.Errorf("results = %#v, want %#v", results, want)
			}
		})
	}
}
