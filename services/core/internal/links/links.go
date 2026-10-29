package links

import (
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

type HTML struct{}

func (HTML) Extract(body []byte, baseURL string) []string {
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
		ref, err := url.Parse(href)
		if err != nil {
			return
		}
		resolved := base.ResolveReference(ref)
		if resolved.Scheme != "http" && resolved.Scheme != "https" {
			return
		}
		links = append(links, resolved.String())
	})
	return links
}
