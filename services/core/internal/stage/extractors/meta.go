package extractors

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/PuerkitoBio/goquery"

	"github.com/Decodo/pobear/pkg/jobspec"
)

// Meta pulls a page's metadata and structured data into one map: title,
// description, canonical, lang, the og:* and twitter:* tags, and every
// application/ld+json block. A key is only present when the page has it, so a
// page with no metadata at all yields an empty map.
type Meta struct{}

func (Meta) Run(ctx context.Context, body []byte, cfg jobspec.Stage) ([]Result, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}

	meta := map[string]any{}
	if title := strings.TrimSpace(doc.Find("head > title").First().Text()); title != "" {
		meta["title"] = title
	}
	if lang := strings.TrimSpace(doc.Find("html").AttrOr("lang", "")); lang != "" {
		meta["lang"] = lang
	}
	if canonical := canonicalHref(doc); canonical != "" {
		meta["canonical"] = canonical
	}

	og := map[string]any{}
	twitter := map[string]any{}
	doc.Find("meta").Each(func(_ int, s *goquery.Selection) {
		content, ok := s.Attr("content")
		if !ok {
			return
		}
		content = strings.TrimSpace(content)
		for _, attr := range []string{"property", "name"} {
			name := strings.ToLower(strings.TrimSpace(s.AttrOr(attr, "")))
			switch {
			case name == "description":
				setFirst(meta, "description", content)
			case strings.HasPrefix(name, "og:"):
				setFirst(og, strings.TrimPrefix(name, "og:"), content)
			case strings.HasPrefix(name, "twitter:"):
				setFirst(twitter, strings.TrimPrefix(name, "twitter:"), content)
			}
		}
	})
	if len(og) > 0 {
		meta["og"] = og
	}
	if len(twitter) > 0 {
		meta["twitter"] = twitter
	}

	jsonld := []any{}
	jsonldErrors := []any{}
	doc.Find("script").FilterFunction(isLDJSONScript).Each(func(i int, s *goquery.Selection) {
		text := strings.TrimSpace(s.Text())
		if text == "" {
			return
		}
		var v any
		if err := json.Unmarshal([]byte(text), &v); err != nil {
			jsonldErrors = append(jsonldErrors, fmt.Sprintf("block %d: %v", i, err))
			return
		}
		jsonld = append(jsonld, v)
	})
	if len(jsonld) > 0 {
		meta["jsonld"] = jsonld
	}
	if len(jsonldErrors) > 0 {
		meta["jsonldErrors"] = jsonldErrors
	}

	return []Result{{Key: "meta", Value: meta}}, nil
}

// setFirst keeps the first non-empty value seen for key, so a repeated tag
// (several og:image, say) resolves to the one that appears first in the page.
func setFirst(m map[string]any, key, value string) {
	if key == "" || value == "" {
		return
	}
	if _, ok := m[key]; !ok {
		m[key] = value
	}
}

func canonicalHref(doc *goquery.Document) string {
	href := ""
	doc.Find("link[href]").EachWithBreak(func(_ int, s *goquery.Selection) bool {
		for _, rel := range strings.Fields(strings.ToLower(s.AttrOr("rel", ""))) {
			if rel == "canonical" {
				href = strings.TrimSpace(s.AttrOr("href", ""))
				return href == ""
			}
		}
		return true
	})
	return href
}

func isLDJSONScript(_ int, s *goquery.Selection) bool {
	mediaType, _, _ := strings.Cut(s.AttrOr("type", ""), ";")
	return strings.EqualFold(strings.TrimSpace(mediaType), "application/ld+json")
}
