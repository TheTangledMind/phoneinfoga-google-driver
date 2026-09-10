package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fixture(t *testing.T, name string) string {
	t.Helper()
	b, e := os.ReadFile("testdata/" + name + ".html")
	if e != nil {
		t.Fatal(e)
	}
	return string(b)
}
func TestParseFixtures(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status Status
		count  int
	}{
		{"results", Completed, 1}, {"changed-layout", Completed, 1}, {"empty", NoResults, 0},
		{"captcha", CaptchaRequired, 0}, {"blocked", Blocked, 0}, {"consent", ConsentRequired, 0}, {"unknown", ParseFailed, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := parsePage(Page{URL: "https://www.google.com/search?q=synthetic", HTML: fixture(t, tc.name), HTTPStatus: 200})
			if p.Status != tc.status || len(p.Results) != tc.count {
				t.Fatalf("got %+v", p)
			}
		})
	}
	p := parsePage(Page{URL: "https://www.google.com/search", HTML: fixture(t, "results"), HTTPStatus: 200})
	if p.Results[0].URL != "https://example.org/entry" || p.Results[0].Title != "Example listing" || !strings.Contains(p.Results[0].Snippet, "synthetic") {
		t.Fatalf("bad result: %+v", p.Results[0])
	}
	for _, code := range []int{403, 429} {
		if p := parsePage(Page{HTTPStatus: code, HTML: fixture(t, "results")}); p.Status != Blocked {
			t.Fatal(p)
		}
	}
}
func TestNormalizeURL(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"/url?q=https%3A%2F%2FExample.org%2Fx%3Fb%3D2%26a%3D1%23frag", "https://example.org/x?a=1&b=2"},
		{"https://www.google.com/url?url=https%3A%2F%2Fexample.org%2F", "https://example.org/"},
		{"javascript:alert(1)", ""}, {"file:///etc/passwd", ""}, {"https://user:pass@example.org/", ""},
		{"/search?q=x", ""}, {"https://www.google.com.evil.test/url?q=https://example.org/", "https://www.google.com.evil.test/url?q=https%3A%2F%2Fexample.org%2F"},
	} {
		if got := normalizeURL(tc.in); got != tc.want {
			t.Errorf("%q => %q, want %q", tc.in, got, tc.want)
		}
	}
}
func TestTerminalText(t *testing.T) {
	got := safeText("hello\x1b[31m\nworld\u202e\x07")
	if strings.ContainsAny(got, "\x1b\n\u202e\x07") {
		t.Fatalf("unsafe %q", got)
	}
}
func TestQueries2110(t *testing.T) {
	qs, e := generateQueries("+12025550123")
	if e != nil {
		t.Fatal(e)
	}
	counts := map[string]int{}
	for _, q := range qs {
		counts[q.Category]++
		if !strings.HasPrefix(q.URL, "https://www.google.com/search?q=") {
			t.Fatal(q)
		}
	}
	for k, n := range map[string]int{"general": 2, "individuals": 7, "reputation": 10, "social_media": 5, "disposable_providers": 21} {
		if counts[k] != n {
			t.Fatalf("%s: %d", k, counts[k])
		}
	}
	want := `intext:"12025550123" | intext:"+12025550123" | intext:"2025550123" | intext:"(202) 555-0123"`
	if qs[0].Text != want {
		t.Fatalf("query differs from 2.11.0: %s", qs[0].Text)
	}
	for _, bad := range []string{"", "abc", "+123", "+12025550123;echo x"} {
		if _, e := generateQueries(bad); e == nil {
			t.Errorf("accepted %q", bad)
		}
	}
}

type fakeBrowser struct {
	pages       []Page
	err         error
	navs, snaps int
	wait        bool
}

func (f *fakeBrowser) Navigate(ctx context.Context, _ string) (Page, error) {
	f.navs++
	if f.wait {
		<-ctx.Done()
		return Page{}, ctx.Err()
	}
	return f.next()
}
func (f *fakeBrowser) Snapshot(ctx context.Context) (Page, error) {
	f.snaps++
	if f.wait {
		<-ctx.Done()
		return Page{}, ctx.Err()
	}
	return f.next()
}
func (f *fakeBrowser) next() (Page, error) {
	if f.err != nil {
		return Page{}, f.err
	}
	if len(f.pages) == 0 {
		return Page{}, errors.New("exhausted")
	}
	p := f.pages[0]
	if len(f.pages) > 1 {
		f.pages = f.pages[1:]
	}
	return p, nil
}
func testOptions() RunOptions {
	return RunOptions{NavigationTimeout: 50 * time.Millisecond, ManualTimeout: 100 * time.Millisecond, Pace: time.Millisecond, Poll: time.Millisecond, MaxQueries: 3}
}
func TestBlockedStopsSession(t *testing.T) {
	f := &fakeBrowser{pages: []Page{{HTTPStatus: 429}}}
	r := runQueries(context.Background(), f, []Query{{Text: "a"}, {Text: "b"}}, testOptions(), &bytes.Buffer{})
	if f.navs != 1 || len(r.Queries) != 1 || r.Queries[0].Status != Blocked {
		t.Fatalf("requests continued: %+v", r)
	}
}
func TestManualCaptchaResumesAndDeduplicates(t *testing.T) {
	f := &fakeBrowser{pages: []Page{{HTML: fixture(t, "captcha")}, {HTML: fixture(t, "results")}, {HTML: fixture(t, "results")}}}
	r := runQueries(context.Background(), f, []Query{{Text: "a", Category: "general"}, {Text: "b", Category: "reputation"}}, testOptions(), &bytes.Buffer{})
	if len(r.Results) != 1 || len(r.Results[0].Queries) != 2 || f.snaps == 0 || r.Queries[0].Status != Completed {
		t.Fatalf("bad resume/dedupe: %+v", r)
	}
}
func TestTimeoutsAndCancellation(t *testing.T) {
	f := &fakeBrowser{wait: true}
	r := runQueries(context.Background(), f, []Query{{Text: "a"}, {Text: "b"}}, testOptions(), &bytes.Buffer{})
	if r.Queries[0].Status != BrowserFailed || f.navs != 1 {
		t.Fatal(r)
	}
	f = &fakeBrowser{pages: []Page{{HTML: fixture(t, "captcha")}}}
	r = runQueries(context.Background(), f, []Query{{Text: "a"}, {Text: "b"}}, testOptions(), &bytes.Buffer{})
	if r.Queries[0].Status != CaptchaRequired || f.navs != 1 {
		t.Fatal(r)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	f = &fakeBrowser{}
	r = runQueries(ctx, f, []Query{{Text: "a"}}, testOptions(), &bytes.Buffer{})
	if f.navs != 0 || !r.Interrupted {
		t.Fatal(r)
	}
}
func TestMaxQueriesAndPacingCancellation(t *testing.T) {
	f := &fakeBrowser{pages: []Page{{HTML: fixture(t, "empty")}}}
	o := testOptions()
	o.MaxQueries = 1
	r := runQueries(context.Background(), f, []Query{{Text: "a"}, {Text: "b"}}, o, &bytes.Buffer{})
	if len(r.Queries) != 1 {
		t.Fatal(r)
	}
	o.MaxQueries = 3
	o.Pace = time.Second
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	r = runQueries(ctx, f, []Query{{Text: "a"}, {Text: "b"}}, o, &bytes.Buffer{})
	if !r.Interrupted || len(r.Queries) != 1 {
		t.Fatal(r)
	}
}
func TestPrivateExport(t *testing.T) {
	p := filepath.Join(t.TempDir(), "out.json")
	if e := exportJSON(p, Report{}); e != nil {
		t.Fatal(e)
	}
	st, _ := os.Stat(p)
	if st.Mode().Perm() != 0600 {
		t.Fatal(st.Mode())
	}
	b, _ := os.ReadFile(p)
	if !json.Valid(b) {
		t.Fatal("invalid json")
	}
	if e := exportJSON(p, Report{}); e == nil {
		t.Fatal("overwrote existing file")
	}
	target := filepath.Join(t.TempDir(), "target")
	os.WriteFile(target, []byte("keep"), 0600)
	link := filepath.Join(t.TempDir(), "link")
	os.Symlink(target, link)
	if exportJSON(link, Report{}) == nil {
		t.Fatal("followed symlink")
	}
}

// Golden captured offline with PhoneInfoga v2.11.0, dorkgen v1.3.1 and
// phonenumbers v1.1.0, matching the installed release's dependency versions.
func TestAllQueriesMatchReleaseGolden(t *testing.T) {
	var out, errOut bytes.Buffer
	code := runCLI(context.Background(), []string{"--queries-only", "--number", "+12025550123", "--max-queries", "45"}, &out, &errOut)
	golden, e := os.ReadFile("testdata/queries-2.11.0.txt")
	if e != nil {
		t.Fatal(e)
	}
	if code != 0 || out.String() != string(golden) {
		t.Fatalf("2.11.0 compatibility changed: exit %d, %s", code, errOut.String())
	}
}

func TestCaptchaOnRateLimitPageNeedsManualAction(t *testing.T) {
	p := parsePage(Page{HTTPStatus: 429, URL: "https://www.google.com/sorry/index", HTML: fixture(t, "captcha")})
	if p.Status != CaptchaRequired {
		t.Fatalf("challenge incorrectly discarded: %s", p.Status)
	}
}
func TestConsentResumesAndUnknownLayoutRemainsFailure(t *testing.T) {
	f := &fakeBrowser{pages: []Page{{HTML: fixture(t, "consent")}, {HTML: fixture(t, "empty")}}}
	r := runQueries(context.Background(), f, []Query{{Text: "a"}}, testOptions(), &bytes.Buffer{})
	if r.Queries[0].Status != NoResults {
		t.Fatal(r)
	}
	f = &fakeBrowser{pages: []Page{{HTML: fixture(t, "captcha")}, {HTML: fixture(t, "unknown")}}}
	r = runQueries(context.Background(), f, []Query{{Text: "a"}}, testOptions(), &bytes.Buffer{})
	if r.Queries[0].Status != ParseFailed {
		t.Fatalf("unknown post-challenge layout misreported: %+v", r)
	}
}
func TestDelayedResultsAreNotParsingFailure(t *testing.T) {
	f := &fakeBrowser{pages: []Page{{HTML: fixture(t, "unknown")}, {HTML: fixture(t, "results")}}}
	r := runQueries(context.Background(), f, []Query{{Text: "a"}}, testOptions(), &bytes.Buffer{})
	if r.Queries[0].Status != Completed {
		t.Fatalf("did not await rendering: %+v", r)
	}
}
