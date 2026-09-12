# PhoneInfoga Google Driver — fork
 
We forked the previously abandoned [PhoneInfoga Google Driver by Sundowndev](https://github.com/sundowndev/phoneinfoga-google-driver)
with the intention of restoring compatibility with modern PhoneInfoga releases
and improving browser search, manual CAPTCHA handling, and result processing.
This companion currently targets **PhoneInfoga 2.11.0**.

Credit for the original driver belongs to Sundowndev, and for PhoneInfoga and its
query generator to [Sundowndev and contributors](https://github.com/sundowndev/phoneinfoga).
Original GPL-3.0 licensing is preserved; see [LICENSE](LICENSE).

## Integration

The companion calls the structured `NewGoogleSearchScanner().Run` library API
from `github.com/sundowndev/phoneinfoga/v2 v2.11.0`. It generates queries locally;
it does not parse CLI output, invoke other scanners, load `.env`, start an API
server, or alter an installed PhoneInfoga executable. Google browser searching
requires no API key. The former v2.0.8 dependency is removed.

The 2.11.0 query templates and dorkgen v1.3.1 are retained. The number parser and
other transitive dependencies are updated for security. All 45 queries for the
synthetic regression number match the release baseline. Updated phone metadata
may validate/format some numbers differently from the original 2023 metadata;
byte-for-byte compatibility for every international number is not claimed.

## Build

Use [Go **1.27.1 or newer**](https://go.dev/dl/) and an installed, updated Google Chrome or Chromium.
Linux is locally tested; macOS builds are supported but browser behavior is not
locally verified. A graphical desktop is required. Root execution is refused.

```bash
cd /opt/github_repo/phoneinfoga-google-driver
go mod download
go test ./...
go build -trimpath -o bin/phoneinfoga-google-driver .
./bin/phoneinfoga-google-driver --version
```

This creates only the companion under this repo's ignored `bin/` directory.
It does not install anything into PATH or replace PhoneInfoga.

## Launch

First inspect queries offline (the example number is synthetic):

```bash
./bin/phoneinfoga-google-driver --queries-only --number '+12025550123' --max-queries 5
```

To search a number you are authorized to investigate, substitute it below:

```bash
./bin/phoneinfoga-google-driver --number '+COUNTRY_CODE_AND_NUMBER' \
  --browser /usr/bin/google-chrome --max-queries 5 --pace 5s \
  --navigation-timeout 30s --manual-timeout 5m --overall-timeout 15m
```

The browser is visible from launch, with its sandbox enabled and an isolated
0700 temporary profile. On CAPTCHA or consent, complete the action yourself in
that window; the driver observes the page and resumes automatically when results
or an explicit no-results page appears. It does not solve challenges, accept
consent for you, rotate proxies, hide automation, or retry blocked searches.

Only the first results page is parsed; result websites are **not** opened.
Titles, HTTP/HTTPS destination links and available snippets are extracted and
deduplicated. Each exported result retains every matching query/category.

Defaults: five queries, five seconds between queries, 30 seconds per navigation,
five minutes per manual wait, 15 minutes overall including all manual waits.
The query limit is 1–45 and pacing is at least one second. A blocked, failed or
unresolved challenge stops the remaining queries. The overall limit always wins.

## Status and limitations

- `completed`: recognized results were extracted.
- `no_results`: an explicit no-results message was recognized.
- `captcha_requires_user_action`: a CAPTCHA needs manual completion.
- `consent_requires_user_action`: consent needs a manual choice.
- `blocked`: a block page or HTTP 403/429 without a CAPTCHA was received; no repeated requests.
- `page_parsing_failed`: the page layout was not recognized; not zero results.
- `browser_or_network_failed`: browser, navigation or network failure/timeout.

Exit 0 means selected queries completed (including explicit no-results); 1 means
an incomplete/failed/interrupted run; 2 means invalid options or setup.
Google layout and language changes may require new fixtures. CAPTCHA acceptance
and search availability are controlled by Google; manual completion is not a
guarantee that a session can continue. Never treat a matching page as verified
ownership, precise location, or proof that a number is currently assigned.

## Privacy and optional export

**Searches disclose the queried phone number to Google.** Command arguments and
terminal output may also be visible in shell history/scrollback or process lists.
No persistent debug logs or reports are written by default. Browser state is
temporary and removed on normal exit, Ctrl+C or SIGTERM. Browser subprocesses
receive only desktop/runtime environment variables, not API keys. Keep private
configuration outside Git; the existing PhoneInfoga `.env` is not needed here.

To save results deliberately, add `--json /path/outside/repos/results.json`.
The destination must not already exist; files are created with mode 0600 and
existing files/symlinks are refused. Export contains the number in query text and
URLs. Partial results and statuses are exported too when a running session stops.

## Stop and rollback

For a foreground run, press **Ctrl+C**. For a background run, keep its exact PID:

```bash
./bin/phoneinfoga-google-driver --number '+COUNTRY_CODE_AND_NUMBER' &
driver_pid=$!
# Stop only this driver and the browser it created:
kill -TERM "$driver_pid"
wait "$driver_pid"
```

The driver closes only its own browser and removes its temporary profile. Avoid
SIGKILL: it prevents Go cleanup and may leave a `phoneinfoga-google-driver-*`
directory under the system temporary directory. Do not remove an active profile.

To remove the built companion, after stopping it:

```bash
cd /opt/github_repo/phoneinfoga-google-driver
rm -f -- bin/phoneinfoga-google-driver
```

The existing PhoneInfoga installation remains usable throughout. No server,
service, private-config change or installation replacement needs rolling back.
Source changes remain uncommitted for review; do not use a broad reset to discard
unrelated local work.

## Verification

```bash
go test -race ./...
go vet ./...
go install golang.org/x/vuln/cmd/govulncheck@v1.8.0
govulncheck ./...
# Optional: opens Chrome against loopback fixtures only, never Google:
DRIVER_BROWSER_TEST=1 go test -run TestBrowserLocalFixtures -v -timeout 60s
```

See [VALIDATION.md](VALIDATION.md) for observed results and limits. Automated tests
use saved synthetic HTML and a fictional number. No real phone numbers are sent
to Google during tests. Fixture behavior must not be described as live-Google
verification.

## Editable query lists

`--search-list /path/to/queries.json` replaces built-in queries for this run.
The format is shared with our PhoneInfoga fork:

```json
{"version":1,"queries":[{"category":"general","query":"\"{number}\"","enabled":true}]}
```

Placeholders: `{number}` (E.164 including `+`) and `{national}` (national digits).
Categories: `general`, `individuals`, `reputation`, `social_media`, `disposable_providers`.
Only explicitly enabled entries run. Lists are bounded to 100 entries/64 KiB,
validated as data only, and still subject to `--max-queries` and `--pace`.
An empty enabled list exits without starting Chrome. With no list supplied,
default query generation remains pinned to PhoneInfoga 2.11.0.

```bash
./bin/phoneinfoga-google-driver --number '+12025550123' \
  --search-list /path/to/queries.json --queries-only
```

Keep `internal/searchlist` identical to the fork's `lib/searchlist` until a shared
versioned module is published. Compatibility and expansion tests are offline;
this change has not been verified against live Google.
