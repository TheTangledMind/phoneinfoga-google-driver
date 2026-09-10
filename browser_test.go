package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

func TestBrowserCommandEnvironment(t *testing.T) {
	t.Setenv("DRIVER_TEST_SECRET", "must-not-inherit")
	cmd := exec.Command("/bin/sh", "-c", `test -z "$DRIVER_TEST_SECRET" && printf ok`)
	isolateBrowserCommand(cmd)
	out, e := cmd.Output()
	if e != nil || string(out) != "ok" {
		t.Fatalf("environment leaked or command failed: %v", e)
	}
}
func TestCLIRejectsInvalidSettings(t *testing.T) {
	for _, args := range [][]string{{"--number", "invalid"}, {"--number", "+12025550123", "--pace", "0s"}, {"--number", "+12025550123", "--max-queries", "0"}} {
		if code := runCLI(context.Background(), args, &bytes.Buffer{}, &bytes.Buffer{}); code != 2 {
			t.Fatalf("accepted invalid options: %d", code)
		}
	}
}
func TestBrowserLocalFixtures(t *testing.T) {
	if os.Getenv("DRIVER_BROWSER_TEST") != "1" {
		t.Skip("opt in to visible local-fixture browser test with DRIVER_BROWSER_TEST=1")
	}
	path, e := findBrowser("")
	if e != nil {
		t.Fatal(e)
	}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/slow" {
			select {
			case <-r.Context().Done():
				return
			case <-time.After(time.Second):
			}
		}
		if r.URL.Path == "/blocked" {
			w.WriteHeader(429)
		}
		name := strings.TrimPrefix(r.URL.Path, "/")
		if name == "" || name == "slow" || name == "blocked" {
			name = "results"
		}
		b, e := os.ReadFile(filepath.Join("testdata", name+".html"))
		if e != nil {
			http.NotFound(w, r)
			return
		}
		w.Write(b)
	})
	server := httptest.NewServer(handler)
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	b, e := newBrowser(ctx, path, 20*time.Second)
	if e != nil {
		t.Fatal(e)
	}
	defer b.Close()
	profile := b.profile
	pid := chromedp.FromContext(b.ctx).Browser.Process().Pid
	st, _ := os.Stat(profile)
	if st.Mode().Perm() != 0700 {
		t.Fatal(st.Mode())
	}
	operation, done := context.WithTimeout(ctx, 5*time.Second)
	p, e := b.Navigate(operation, server.URL+"/results")
	done()
	if e != nil || parsePage(p).Status != Completed {
		t.Fatalf("local parsing: %v %+v", e, p)
	}
	operation, done = context.WithTimeout(ctx, 5*time.Second)
	p, e = b.Navigate(operation, server.URL+"/blocked")
	done()
	if e != nil || p.HTTPStatus != 429 || parsePage(p).Status != Blocked {
		t.Fatalf("status capture: %v %+v", e, p)
	}
	operation, done = context.WithTimeout(ctx, 20*time.Millisecond)
	_, e = b.Navigate(operation, server.URL+"/slow")
	done()
	if e == nil {
		t.Fatal("navigation ignored timeout")
	}
	cancel()
	if e = b.Close(); e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(profile); !os.IsNotExist(e) {
		t.Fatal("profile remains")
	}
	if e = syscall.Kill(pid, 0); e == nil {
		t.Fatal("driver browser process remains")
	}
}

func TestStartupFailureRemovesProfile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TMPDIR", dir)
	b, e := newBrowser(context.Background(), "/nonexistent-driver-test-browser", time.Second)
	if e == nil {
		b.Close()
		t.Fatal("unexpected startup success")
	}
	entries, e := os.ReadDir(dir)
	if e != nil {
		t.Fatal(e)
	}
	if len(entries) != 0 {
		t.Fatal("startup failure left private browser state")
	}
}
