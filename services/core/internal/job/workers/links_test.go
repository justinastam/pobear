package workers_test

import (
	"sort"
	"testing"

	"github.com/Decodo/pobear/services/core/internal/job/workers"
)

func TestHTMLLinksResolvesRelativeHrefsAgainstBaseURL(t *testing.T) {
	body := []byte(`<a href="/about">About</a><a href="page.html">Page</a>`)

	got := workers.HTMLLinks{}.Extract(body, "https://a.test/blog/index.html")

	sort.Strings(got)
	want := []string{"https://a.test/about", "https://a.test/blog/page.html"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestHTMLLinksResolvesAgainstFinalURLNotOriginalRequest(t *testing.T) {
	body := []byte(`<a href="sibling.html">Sibling</a>`)

	got := workers.HTMLLinks{}.Extract(body, "https://a.test/redirected-to/page.html")

	if len(got) != 1 || got[0] != "https://a.test/redirected-to/sibling.html" {
		t.Fatalf("got %v, want [https://a.test/redirected-to/sibling.html] (resolved against the final URL, not wherever the request originally targeted)", got)
	}
}

func TestHTMLLinksKeepsAlreadyAbsoluteHrefs(t *testing.T) {
	body := []byte(`<a href="https://other.test/page">Other</a>`)

	got := workers.HTMLLinks{}.Extract(body, "https://a.test/")

	if len(got) != 1 || got[0] != "https://other.test/page" {
		t.Fatalf("got %v, want [https://other.test/page]", got)
	}
}

func TestHTMLLinksDropsNonHTTPSchemes(t *testing.T) {
	body := []byte(`<a href="mailto:a@b.test">Mail</a><a href="tel:+1234">Tel</a><a href="javascript:void(0)">JS</a><a href="/real">Real</a>`)

	got := workers.HTMLLinks{}.Extract(body, "https://a.test/")

	if len(got) != 1 || got[0] != "https://a.test/real" {
		t.Fatalf("got %v, want only [https://a.test/real] -- mailto/tel/javascript hrefs were never crawlable candidates", got)
	}
}

func TestHTMLLinksIgnoresAnchorsWithoutHref(t *testing.T) {
	body := []byte(`<a name="section">No href</a><a href="/real">Real</a>`)

	got := workers.HTMLLinks{}.Extract(body, "https://a.test/")

	if len(got) != 1 || got[0] != "https://a.test/real" {
		t.Fatalf("got %v, want only [https://a.test/real]", got)
	}
}

func TestHTMLLinksReturnsNilOnUnparsableBaseURL(t *testing.T) {
	body := []byte(`<a href="/real">Real</a>`)

	got := workers.HTMLLinks{}.Extract(body, "://not a url")

	if got != nil {
		t.Fatalf("got %v, want nil for an unparsable baseURL", got)
	}
}

func TestHTMLLinksReturnsNilWhenNoLinksPresent(t *testing.T) {
	got := workers.HTMLLinks{}.Extract([]byte(`<p>no links here</p>`), "https://a.test/")

	if got != nil {
		t.Fatalf("got %v, want nil", got)
	}
}

func TestFollowLinkResolvesTheFirstMatchAgainstBaseURL(t *testing.T) {
	body := []byte(`<li class="next"><a href="page-2.html">next</a></li><li class="next"><a href="page-9.html">next</a></li>`)

	got, ok := workers.FollowLink(body, "https://a.test/list/page-1.html", "li.next a@href")

	if !ok || got != "https://a.test/list/page-2.html" {
		t.Fatalf("got %q (ok=%v), want https://a.test/list/page-2.html", got, ok)
	}
}

func TestFollowLinkDefaultsTheAttributeToHref(t *testing.T) {
	body := []byte(`<a rel="next" href="?page=2">next</a>`)

	got, ok := workers.FollowLink(body, "https://a.test/list?page=1", `a[rel=next]`)

	if !ok || got != "https://a.test/list?page=2" {
		t.Fatalf("got %q (ok=%v), want https://a.test/list?page=2", got, ok)
	}
}

func TestFollowLinkReadsANamedAttribute(t *testing.T) {
	body := []byte(`<button class="more" data-url="/list/3">more</button>`)

	got, ok := workers.FollowLink(body, "https://a.test/list/2", "button.more@data-url")

	if !ok || got != "https://a.test/list/3" {
		t.Fatalf("got %q (ok=%v), want https://a.test/list/3", got, ok)
	}
}

func TestFollowLinkSkipsNonHTTPMatchesAndReportsNoneWhenNothingUsable(t *testing.T) {
	body := []byte(`<a class="next" href="javascript:void(0)">next</a>`)

	if got, ok := workers.FollowLink(body, "https://a.test/", "a.next"); ok {
		t.Fatalf("got %q, want no next link (javascript: isn't followable)", got)
	}
	if got, ok := workers.FollowLink([]byte(`<p>last page</p>`), "https://a.test/", "a.next"); ok {
		t.Fatalf("got %q, want no next link on a page with no match", got)
	}
}
