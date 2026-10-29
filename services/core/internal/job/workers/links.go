package workers

import (
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/andybalholm/cascadia"
)

type HTMLLinks struct{}

func (HTMLLinks) Extract(body []byte, baseURL string) []string {
	base, err := url.Parse(baseURL)
	if err != nil {
		return nil
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
	if err != nil {
		return nil
	}

	var links []string
	doc.Find("a[href]").Each(func(_ int, s *goquery.Selection) {
		href, ok := s.Attr("href")
		if !ok {
			return
		}
		if resolved, ok := resolveLink(base, href); ok {
			links = append(links, resolved)
		}
	})
	return links
}

// FollowLink returns the next-page link a source.follow selector points at:
// the first element matching selector[@attr] (attr defaults to href) whose
// value resolves, against baseURL, to an http(s) URL.
func FollowLink(body []byte, baseURL, selector string) (string, bool) {
	base, err := url.Parse(baseURL)
	if err != nil {
		return "", false
	}

	cssSelector, attr, _ := strings.Cut(selector, "@")
	if attr == "" {
		attr = "href"
	}
	matcher, err := cascadia.Compile(cssSelector)
	if err != nil {
		return "", false
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
	if err != nil {
		return "", false
	}

	var next string
	doc.FindMatcher(matcher).EachWithBreak(func(_ int, s *goquery.Selection) bool {
		v, ok := s.Attr(attr)
		if !ok {
			return true
		}
		resolved, ok := resolveLink(base, strings.TrimSpace(v))
		if !ok {
			return true
		}
		next = resolved
		return false
	})
	return next, next != ""
}

func resolveLink(base *url.URL, href string) (string, bool) {
	ref, err := url.Parse(href)
	if err != nil {
		return "", false
	}
	resolved := base.ResolveReference(ref)
	if resolved.Scheme != "http" && resolved.Scheme != "https" {
		return "", false
	}
	return resolved.String(), true
}
