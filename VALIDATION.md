# Validation — 10 September 2026

## Compatibility and installation preservation

- Existing executable reported `PhoneInfoga 2.11.0-5f6156f` before editing.
- The local source's query generator and number formatter matched the `v2.11.0`
  tag. The companion pins the published library release, not the default branch.
- The installed executable's build metadata listed dorkgen v1.3.1 and
  phonenumbers v1.1.0. The offline golden fixture was captured with those versions
  before upgrading the parser; all 45 synthetic-number queries still match.
- The companion now uses phonenumbers v1.8.1. Its newer metadata may affect other
  numbers; tests do not establish identical formatting for every country/number.
- The installed executable and its private `.env` matched their pre-work SHA-256
  fingerprints after implementation. The configuration was not printed or used.
- No API server was started. No commit, push or installation replacement occurred.

## Tests actually run

With Go 1.27.1 on Linux and installed Google Chrome 152.0.7977.75:
The temporary Go toolchain for this review is `/tmp/phoneinfoga-driver-toolchain/go/bin/go`; no system Go installation was replaced.

- `DRIVER_BROWSER_TEST=1 go test -race ./...` — passed.
- `go vet ./...` — passed.
- `go mod verify` — passed.
- `CGO_ENABLED=0 go build -trimpath -o bin/phoneinfoga-google-driver .` — passed.
- `CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build` — passed (cross-build only).
- Built executable `--version` and one-query `--queries-only` — passed.
- `git diff --check` — passed for all three edited repositories.

Offline fixtures cover legacy and changed result layouts; titles and snippets;
Google redirect URLs; unsafe links; deduplication across queries; terminal control
characters; explicit empty results; unknown layouts; CAPTCHA including HTTP 429;
consent; blocks; delayed rendering; navigation/manual timeouts; pacing and query
limits; cancellation; private JSON permissions and refusal to overwrite/symlink.

The visible Chrome test uses a loopback HTTP fixture server only. It verifies DOM
extraction, HTTP 429 detection, navigation timeout, cancellation, browser-process
exit and removal of the temporary 0700 profile. A separate test checks profile
removal after browser startup failure. The child environment isolation test
checks that unrelated environment secrets are absent from a spawned process.

The first race-enabled browser test exposed profile removal racing Chrome's last
writes. Cancellation now stops operations first, leaving the browser context
available for bounded graceful shutdown. Cleanup retries only removal of the
owned profile for at most one second. The targeted test then passed three
consecutive race-enabled runs; the full suite also passed. No temporary driver
profiles remained after verification.

## Dependency vulnerability assessment

`govulncheck v1.8.0 -show verbose ./...` returned **zero affected symbols and zero
findings in imported packages** with the final dependencies.

The initial reachable finding was
[GO-2025-3987](https://pkg.go.dev/vuln/GO-2025-3987) in PhoneInfoga's old number
parser; this was resolved by upgrading phonenumbers. The initial unused-package
findings in Logrus, OAuth2, gRPC and protobuf were also removed through compatible
dependency updates. Query compatibility tests passed afterward.

One module-level advisory remains:
[GO-2026-5932](https://pkg.go.dev/vuln/GO-2026-5932), concerning the deprecated
`golang.org/x/crypto/openpgp` package. The module is required by other dependencies,
but `go list -deps .` confirms that **openpgp is not imported**, and govulncheck
reports no path to it. No OpenPGP functionality is used. The advisory has no fixed
version; this is not presented as a clean module-level scan. The current
`golang.org/x/crypto v0.56.0` also removes the two prior SSH module advisories.

## Not verified

No Google search was submitted, including for synthetic numbers. CAPTCHA and
consent transitions were fixture-tested, not completed against live Google.
Google may block automation or change markup/language; the driver reports an
unrecognized page as failure, not zero results. Live success is not guaranteed.
No result websites were visited. SIGINT/SIGTERM are wired into the cancellation
path; OS signal delivery itself was not exercised in a separate end-to-end test.
macOS runtime behavior and tagged GoReleaser publication were not exercised.

The existing PhoneInfoga installation has not received this companion's dependency
updates. It remains exactly as it was. This assessment covers the new companion,
not a fresh audit of that installed executable or the other repositories.

## Focused change map

- `query.go`: pinned structured 2.11.0 query interface, no private configuration.
- `browser.go`, `process*.go`: visible sandboxed browser, isolated environment and
  temporary profile, bounded startup/operations and cleanup.
- `parser.go`, `runner.go`, `main.go`: result normalization, status handling,
  manual waits, pacing, export and CLI/signal handling.
- Tests and `testdata/`: synthetic HTML and release-compatibility regression data.
- `go.mod`, `go.sum`: modern direct dependencies and fixed reachable dependencies.
- CI and `.goreleaser.yml`: current toolchain, offline tests, correct binary name;
  releases only on version tags.
- README: build, launch, stop, rollback, privacy and original-author credit.

The sibling PhoneInfoga and SearchPhone edits are limited to concise fork READMEs
and removal of inherited funding metadata. Existing local SearchPhone packaging
files and unrelated changes are preserved. Its Git origin still points upstream;
no remote was changed by this task.
