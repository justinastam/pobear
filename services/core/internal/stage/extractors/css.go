package extractors

import (
	"context"
	"sort"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/andybalholm/cascadia"

	"github.com/Decodo/pobear/pkg/jobspec"
)

type CSS struct{}

type cssField struct {
	name    string
	matcher goquery.Matcher
	attr    string
}

func (CSS) Run(ctx context.Context, body []byte, cfg jobspec.Stage) ([]Result, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}

	fields, err := compileCSSFields(cfg.Fields, cfg.Items != "")
	if err != nil {
		return nil, err
	}

	if cfg.Items != "" {
		itemMatcher, err := cascadia.Compile(cfg.Items)
		if err != nil {
			return nil, err
		}
		records := []map[string]any{}
		doc.FindMatcher(itemMatcher).Each(func(_ int, item *goquery.Selection) {
			records = append(records, extractRecord(item, fields))
		})
		return []Result{{Key: jobspec.ItemsDataKey, Value: records}}, nil
	}

	results := make([]Result, 0, len(fields))
	for _, f := range fields {
		values := []string{}
		doc.FindMatcher(f.matcher).Each(func(_ int, s *goquery.Selection) {
			if v, ok := selectionValue(s, f.attr); ok {
				values = append(values, v)
			}
		})
		results = append(results, Result{Key: f.name, Value: values})
	}
	return results, nil
}

// compileCSSFields compiles every field's selector, in name order. Inside an
// item an empty selector (`@attr`, or nothing at all) means the item element
// itself, so allowSelf leaves its matcher nil instead of rejecting it.
func compileCSSFields(raw map[string]string, allowSelf bool) ([]cssField, error) {
	names := make([]string, 0, len(raw))
	for name := range raw {
		names = append(names, name)
	}
	sort.Strings(names)

	fields := make([]cssField, 0, len(names))
	for _, name := range names {
		cssSelector, attr, _ := strings.Cut(raw[name], "@")
		f := cssField{name: name, attr: attr}
		if cssSelector != "" || !allowSelf {
			matcher, err := cascadia.Compile(cssSelector)
			if err != nil {
				return nil, err
			}
			f.matcher = matcher
		}
		fields = append(fields, f)
	}
	return fields, nil
}

// extractRecord evaluates every field relative to one item element: each is
// the first descendant match that yields a value, or nil when none does.
func extractRecord(item *goquery.Selection, fields []cssField) map[string]any {
	record := make(map[string]any, len(fields))
	for _, f := range fields {
		var value any
		candidates := item
		if f.matcher != nil {
			candidates = item.FindMatcher(f.matcher)
		}
		candidates.EachWithBreak(func(_ int, s *goquery.Selection) bool {
			if v, ok := selectionValue(s, f.attr); ok {
				value = v
				return false
			}
			return true
		})
		record[f.name] = value
	}
	return record
}

func selectionValue(s *goquery.Selection, attr string) (string, bool) {
	if attr == "" {
		return s.Text(), true
	}
	return s.Attr(attr)
}
