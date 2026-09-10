package main

import (
	"net/url"
	"strings"
	"unicode"

	"github.com/PuerkitoBio/goquery"
)

type Status string

const (
	Completed       Status = "completed"
	NoResults       Status = "no_results"
	CaptchaRequired Status = "captcha_requires_user_action"
	ConsentRequired Status = "consent_requires_user_action"
	Blocked         Status = "blocked"
	ParseFailed     Status = "page_parsing_failed"
	BrowserFailed   Status = "browser_or_network_failed"
)

type Page struct {
	URL, HTML  string
	HTTPStatus int
}
type Result struct {
	Title   string  `json:"title"`
	URL     string  `json:"url"`
	Snippet string  `json:"snippet,omitempty"`
	Queries []Query `json:"queries"`
}
type ParsedPage struct {
	Status  Status
	Results []Result
}

// Strip ANSI escape/control characters, bidi controls and other invisible format
// characters. Collapse whitespace so a result cannot forge extra terminal lines.
func safeText(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			if unicode.IsSpace(r) {
				return ' '
			}
			return -1
		}
		return r
	}, s)
	s = strings.Join(strings.Fields(s), " ")
	runes := []rune(s)
	if len(runes) > 2000 {
		s = string(runes[:2000]) + "…"
	}
	return s
}
func googleHost(host string) bool {
	return host == "google.com" || strings.HasSuffix(host, ".google.com")
}
func normalizeURL(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	// Only unwrap redirects from the Google origin, never lookalike hosts.
	if (u.Host == "" || googleHost(strings.ToLower(u.Hostname()))) && u.Path == "/url" {
		dest := u.Query().Get("q")
		if dest == "" {
			dest = u.Query().Get("url")
		}
		u, err = url.Parse(dest)
		if err != nil {
			return ""
		}
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil {
		return ""
	}
	if googleHost(strings.ToLower(u.Hostname())) {
		return ""
	}
	u.Host = strings.ToLower(u.Host)
	u.Fragment = ""
	u.RawFragment = ""
	if u.Path == "" {
		u.Path = "/"
	}
	u.RawQuery = u.Query().Encode()
	return u.String()
}
func parsePage(p Page) ParsedPage {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(p.HTML))
	if err != nil {
		return ParsedPage{Status: ParseFailed}
	}
	body := strings.ToLower(doc.Find("body").Text())
	u, _ := url.Parse(p.URL)
	host := ""
	path := ""
	if u != nil {
		host = strings.ToLower(u.Hostname())
		path = u.Path
	}
	if doc.Find(`#recaptcha, .g-recaptcha, iframe[src*="/recaptcha/"], form[action*="/sorry/"]`).Length() > 0 {
		return ParsedPage{Status: CaptchaRequired}
	}
	if p.HTTPStatus == 403 || p.HTTPStatus == 429 {
		return ParsedPage{Status: Blocked}
	}
	if p.HTTPStatus >= 400 {
		return ParsedPage{Status: BrowserFailed}
	}
	if strings.HasPrefix(host, "consent.google.") || doc.Find(`form[action*="consent.google.com"]`).Length() > 0 || strings.Contains(body, "before you continue to google") {
		return ParsedPage{Status: ConsentRequired}
	}
	if strings.HasPrefix(path, "/sorry") || strings.Contains(body, "unusual traffic") || strings.Contains(body, "automated queries") || strings.Contains(body, "access denied") {
		return ParsedPage{Status: Blocked}
	}
	var results []Result
	seen := map[string]bool{}
	// A heading inside a link is more stable than any single Google CSS class.
	doc.Find("a:has(h3)").Each(func(_ int, a *goquery.Selection) {
		href, _ := a.Attr("href")
		dest := normalizeURL(href)
		if dest == "" || seen[dest] {
			return
		}
		title := safeText(a.Find("h3").First().Text())
		if title == "" {
			return
		}
		snippet := ""
		for box, depth := a.Parent(), 0; box.Length() > 0 && depth < 5; box, depth = box.Parent(), depth+1 {
			if box.Find("h3").Length() > 1 {
				break
			}
			snippet = safeText(box.Find(".VwiC3b, .IsZvec, .aCOpRe, [data-sncf], p").First().Text())
			if snippet != "" {
				break
			}
		}
		seen[dest] = true
		results = append(results, Result{Title: title, URL: dest, Snippet: snippet})
	})
	if len(results) > 0 {
		return ParsedPage{Status: Completed, Results: results}
	}
	// An empty container alone is not evidence of zero matches.
	if strings.Contains(body, "did not match any documents") || strings.Contains(body, "no results found for") || strings.Contains(body, "no results containing all your search terms") {
		return ParsedPage{Status: NoResults}
	}
	return ParsedPage{Status: ParseFailed}
}
