package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func runCLI(ctx context.Context, args []string, out, errOut io.Writer) (code int) {
	flags := flag.NewFlagSet("phoneinfoga-google-driver", flag.ContinueOnError)
	flags.SetOutput(errOut)
	number := flags.String("number", "", "international phone number (searches disclose it to Google)")
	searchList := flags.String("search-list", "", "version 1 JSON Google query list; defaults to PhoneInfoga 2.11.0 queries")
	browser := flags.String("browser", "", "installed Chrome/Chromium executable")
	jsonPath := flags.String("json", "", "optional new JSON export file (0600; never overwrite)")
	dry := flags.Bool("queries-only", false, "print locally generated queries without launching a browser")
	version := flags.Bool("version", false, "show query compatibility version")
	nav := flags.Duration("navigation-timeout", 30*time.Second, "timeout for each navigation/browser operation")
	overall := flags.Duration("overall-timeout", 15*time.Minute, "overall session limit including manual waits")
	manual := flags.Duration("manual-timeout", 5*time.Minute, "limit for each manual CAPTCHA/consent wait")
	pace := flags.Duration("pace", 5*time.Second, "minimum pause between queries")
	max := flags.Int("max-queries", 5, "maximum queries (1-45; no retries or result-page crawling)")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if *version {
		fmt.Fprintf(out, "phoneinfoga-google-driver (PhoneInfoga queries %s)\n", phoneinfogaVersion)
		return 0
	}
	if os.Geteuid() == 0 {
		fmt.Fprintln(errOut, "Refusing root execution; browser sandbox must remain enabled.")
		return 2
	}
	if flags.NArg() != 0 || *max < 1 || *max > 45 || *nav <= 0 || *overall <= 0 || *manual <= 0 || *pace < time.Second {
		fmt.Fprintln(errOut, "Invalid options: use --help; pacing must be at least 1s and max-queries 1-45.")
		return 2
	}
	queries, err := generateQueriesFromList(*number, *searchList)
	if err != nil {
		fmt.Fprintln(errOut, err)
		return 2
	}
	if len(queries) == 0 {
		fmt.Fprintln(out, "No enabled queries; no searches submitted.")
		return 0
	}
	if len(queries) > *max {
		queries = queries[:*max]
	}
	fmt.Fprintln(out, resultNotice)
	if *dry {
		for _, q := range queries {
			fmt.Fprintf(out, "%s: %s\n%s\n", q.Category, safeText(q.Text), safeText(q.URL))
		}
		return 0
	}
	path, err := findBrowser(*browser)
	if err != nil {
		fmt.Fprintln(errOut, err)
		return 2
	}
	ctx, cancel := context.WithTimeout(ctx, *overall)
	defer cancel()
	b, err := newBrowser(ctx, path, *nav)
	if err != nil {
		fmt.Fprintln(errOut, err)
		return 1
	}
	defer func() {
		if err := b.Close(); err != nil {
			fmt.Fprintln(errOut, "Temporary browser profile cleanup failed; remove only the phoneinfoga-google-driver-* profile created by this run.")
			code = 1
		}
	}()
	report := runQueries(ctx, b, queries, RunOptions{NavigationTimeout: *nav, ManualTimeout: *manual, Pace: *pace, Poll: 500 * time.Millisecond, MaxQueries: *max}, out)
	if *jsonPath != "" {
		if err := exportJSON(*jsonPath, report); err != nil {
			fmt.Fprintln(errOut, err)
			return 1
		}
		fmt.Fprintln(out, "Private JSON export written.")
	}
	if report.Interrupted {
		fmt.Fprintln(errOut, "Session interrupted or overall timeout reached.")
		return 1
	}
	for _, q := range report.Queries {
		if q.Status != Completed && q.Status != NoResults {
			return 1
		}
	}
	return 0
}
func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := runCLI(ctx, os.Args[1:], os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}
