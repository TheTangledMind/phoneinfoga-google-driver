package main

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

type chromeBrowser struct {
	ctx                     context.Context
	cancel, allocatorCancel context.CancelFunc
	profile                 string
	mu                      sync.Mutex
	frame                   cdp.FrameID
	status                  int
	once                    sync.Once
	closeErr                error
}

func findBrowser(explicit string) (string, error) {
	if explicit != "" {
		p, e := exec.LookPath(explicit)
		if e != nil {
			return "", errors.New("browser executable not found")
		}
		return p, nil
	}
	for _, name := range []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser"} {
		if p, e := exec.LookPath(name); e == nil {
			return p, nil
		}
	}
	return "", errors.New("install Chrome/Chromium or supply --browser")
}
func browserEnvironment() []string {
	// Do not pass API credentials or unrelated configuration into the browser.
	keys := []string{"HOME", "PATH", "DISPLAY", "WAYLAND_DISPLAY", "XAUTHORITY", "XDG_RUNTIME_DIR", "DBUS_SESSION_BUS_ADDRESS", "LANG", "LC_ALL", "XDG_CONFIG_HOME", "XDG_CACHE_HOME"}
	var env []string
	for _, key := range keys {
		if v, ok := os.LookupEnv(key); ok {
			env = append(env, key+"="+v)
		}
	}
	return env
}
func newBrowser(ctx context.Context, path string, startTimeout time.Duration) (*chromeBrowser, error) {
	if os.Geteuid() == 0 {
		return nil, errors.New("refusing root execution; Chrome sandbox must remain enabled")
	}
	profile, err := os.MkdirTemp("", "phoneinfoga-google-driver-")
	if err != nil {
		return nil, errors.New("cannot create temporary browser profile")
	}
	b := &chromeBrowser{profile: profile}
	// Deliberately do not inherit DefaultExecAllocatorOptions. No sandbox,
	// certificate, origin, fingerprint, or automation-evasion overrides.
	opts := []chromedp.ExecAllocatorOption{
		chromedp.ExecPath(path), chromedp.UserDataDir(profile), chromedp.ModifyCmdFunc(isolateBrowserCommand),
		chromedp.Flag("headless", false), chromedp.Flag("no-sandbox", false),
		chromedp.Flag("enable-automation", true),
		chromedp.Flag("no-first-run", true), chromedp.Flag("no-default-browser-check", true),
		chromedp.Flag("disable-background-networking", true), chromedp.Flag("disable-sync", true),
		chromedp.Flag("disable-extensions", true), chromedp.Flag("disable-breakpad", true),
		chromedp.Flag("disable-crash-reporter", true), chromedp.Flag("disable-logging", true),
		chromedp.Flag("password-store", "basic"), chromedp.Flag("remote-debugging-address", "127.0.0.1"),
	}
	// Cancellation stops operations first. Close then gets a live browser
	// context for graceful shutdown instead of killing Chrome mid-profile-write.
	allocator, ac := chromedp.NewExecAllocator(context.WithoutCancel(ctx), opts...)
	b.allocatorCancel = ac
	b.ctx, b.cancel = chromedp.NewContext(allocator, chromedp.WithLogf(func(string, ...interface{}) {}), chromedp.WithErrorf(func(string, ...interface{}) {}))
	chromedp.ListenTarget(b.ctx, func(event interface{}) {
		if e, ok := event.(*network.EventResponseReceived); ok && e.Type == network.ResourceTypeDocument {
			b.mu.Lock()
			if e.FrameID == b.frame {
				b.status = int(e.Response.Status)
			}
			b.mu.Unlock()
		}
	})
	// Startup must use the persistent context; a first Run with a short child
	// context would tie the whole browser lifetime to that child's cancellation.
	stopStartup := context.AfterFunc(ctx, b.cancel)
	timer := time.AfterFunc(startTimeout, b.cancel)
	err = chromedp.Run(b.ctx, network.Enable(), chromedp.ActionFunc(func(c context.Context) error {
		tree, e := page.GetFrameTree().Do(c)
		if e == nil {
			b.mu.Lock()
			b.frame = tree.Frame.ID
			b.mu.Unlock()
		}
		return e
	}))
	timer.Stop()
	stopStartup()
	if err != nil || ctx.Err() != nil {
		_ = b.Close()
		return nil, errors.New("browser startup failed (check display, browser installation and sandbox)")
	}
	return b, nil
}
func (b *chromeBrowser) Navigate(ctx context.Context, url string) (Page, error) {
	b.mu.Lock()
	b.status = 0
	b.mu.Unlock()
	// Preserve chromedp values while respecting the caller's deadline/cancellation.
	run, cancel := b.operationContext(ctx)
	defer cancel()
	if err := chromedp.Run(run, chromedp.Navigate(url)); err != nil {
		return Page{}, errors.New("navigation failed")
	}
	return b.Snapshot(ctx)
}
func (b *chromeBrowser) operationContext(ctx context.Context) (context.Context, context.CancelFunc) {
	c, cancel := context.WithCancel(b.ctx)
	stop := context.AfterFunc(ctx, cancel)
	return c, func() { stop(); cancel() }
}
func (b *chromeBrowser) Snapshot(ctx context.Context) (Page, error) {
	run, cancel := b.operationContext(ctx)
	defer cancel()
	var p Page
	err := chromedp.Run(run, chromedp.Location(&p.URL), chromedp.OuterHTML("html", &p.HTML, chromedp.ByQuery))
	if err != nil {
		return Page{}, errors.New("browser snapshot failed")
	}
	if strings.HasPrefix(p.URL, "chrome-error:") {
		return Page{}, errors.New("browser network error")
	}
	b.mu.Lock()
	p.HTTPStatus = b.status
	b.mu.Unlock()
	return p, nil
}
func (b *chromeBrowser) Close() error {
	b.once.Do(func() {
		if b.ctx != nil {
			ctx, cancel := context.WithTimeout(b.ctx, 3*time.Second)
			_ = chromedp.Cancel(ctx)
			cancel()
		}
		if b.cancel != nil {
			b.cancel()
		}
		if b.allocatorCancel != nil {
			b.allocatorCancel()
		}
		// Chrome helper processes can finish their final profile writes just
		// after the browser process exits. Bound this cleanup-only retry.
		for attempt := 0; attempt < 20; attempt++ {
			b.closeErr = os.RemoveAll(b.profile)
			if b.closeErr == nil {
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
	})
	return b.closeErr
}

var _ io.Closer = (*chromeBrowser)(nil)
