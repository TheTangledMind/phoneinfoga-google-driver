package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"
)

type SearchBrowser interface {
	Navigate(context.Context, string) (Page, error)
	Snapshot(context.Context) (Page, error)
}
type RunOptions struct {
	NavigationTimeout, ManualTimeout, Pace, Poll time.Duration
	MaxQueries                                   int
}
type QueryOutcome struct {
	Query   Query  `json:"query"`
	Status  Status `json:"status"`
	Message string `json:"message,omitempty"`
	Count   int    `json:"result_count"`
}
type Report struct {
	PhoneInfogaVersion string         `json:"phoneinfoga_query_version"`
	Notice             string         `json:"notice"`
	Queries            []QueryOutcome `json:"queries"`
	Results            []Result       `json:"results"`
	Interrupted        bool           `json:"interrupted"`
}

const resultNotice = "Search matches are leads, not verified ownership. Searches disclose the queried number to Google."

func waitFor(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
func runQueries(ctx context.Context, b SearchBrowser, qs []Query, o RunOptions, out io.Writer) Report {
	report := Report{PhoneInfogaVersion: phoneinfogaVersion, Notice: resultNotice, Queries: []QueryOutcome{}, Results: []Result{}}
	seen := map[string]int{}
	for i, q := range qs {
		if i >= o.MaxQueries {
			break
		}
		if ctx.Err() != nil {
			report.Interrupted = true
			break
		}
		if i > 0 && !waitFor(ctx, o.Pace) {
			report.Interrupted = true
			break
		}
		nav, cancel := context.WithTimeout(ctx, o.NavigationTimeout)
		p, err := b.Navigate(nav, q.URL)
		parsed := ParsedPage{Status: BrowserFailed}
		message := ""
		if err == nil {
			parsed = parsePage(p)
			// Wait for late-rendered results within the same navigation budget.
			for parsed.Status == ParseFailed && waitFor(nav, o.Poll) {
				p, err = b.Snapshot(nav)
				if err != nil {
					if nav.Err() == nil {
						parsed = ParsedPage{Status: BrowserFailed}
					}
					break
				}
				parsed = parsePage(p)
			}
		} else {
			message = "Navigation failed or timed out; session stopped."
		}
		cancel()
		if parsed.Status == CaptchaRequired || parsed.Status == ConsentRequired {
			fmt.Fprintf(out, "%s: complete the challenge/consent in the driver browser; waiting up to %s.\n", parsed.Status, o.ManualTimeout)
			manual, stop := context.WithTimeout(ctx, o.ManualTimeout)
			for waitFor(manual, o.Poll) {
				snap, done := context.WithTimeout(manual, o.NavigationTimeout)
				p, err = b.Snapshot(snap)
				done()
				if err != nil {
					if manual.Err() == nil {
						parsed = ParsedPage{Status: BrowserFailed}
						message = "Browser failed while waiting for user action."
					}
					break
				}
				parsed = parsePage(p)
				// Allow rendering to settle, but retain an honest parsing failure
				// if the challenge disappears and the final layout is unknown.
				if parsed.Status == ParseFailed {
					continue
				}
				if parsed.Status != CaptchaRequired && parsed.Status != ConsentRequired {
					break
				}
			}
			if parsed.Status == CaptchaRequired || parsed.Status == ConsentRequired {
				message = "User action was not completed within the manual timeout; session stopped."
			}
			stop()
		}
		outcome := QueryOutcome{Query: q, Status: parsed.Status, Message: message, Count: len(parsed.Results)}
		report.Queries = append(report.Queries, outcome)
		fmt.Fprintf(out, "[%d] %s: %s (%d results)\n", i+1, safeText(q.Category), parsed.Status, len(parsed.Results))
		for _, r := range parsed.Results {
			if index, ok := seen[r.URL]; ok {
				report.Results[index].Queries = append(report.Results[index].Queries, q)
				continue
			}
			r.Queries = []Query{q}
			seen[r.URL] = len(report.Results)
			report.Results = append(report.Results, r)
			fmt.Fprintf(out, "  %s\n  %s\n", safeText(r.Title), safeText(r.URL))
			if r.Snippet != "" {
				fmt.Fprintf(out, "  %s\n", safeText(r.Snippet))
			}
		}
		if ctx.Err() != nil {
			report.Interrupted = true
			break
		}
		if parsed.Status != Completed && parsed.Status != NoResults {
			break
		}
	}
	return report
}
func exportJSON(path string, r Report) (err error) {
	// O_EXCL refuses existing files and symlinks; mode is restrictive from creation.
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fmt.Errorf("cannot create private JSON export (destination must not exist)")
	}
	defer func() {
		if err != nil {
			_ = os.Remove(path)
		}
	}()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err = enc.Encode(r); err != nil {
		_ = f.Close()
		return fmt.Errorf("cannot write JSON export")
	}
	if err = f.Close(); err != nil {
		return fmt.Errorf("cannot close JSON export")
	}
	return nil
}
