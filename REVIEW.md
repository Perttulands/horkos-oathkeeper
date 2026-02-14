# Oathkeeper Reliability and Performance Review

## Scope
Reviewed and updated:
- `pkg/detector/detector.go` + `pkg/detector/detector_test.go`
- `pkg/scanner/scanner.go`
- `pkg/verifier/verifier.go`
- `pkg/daemon/daemon.go`
- Related failure-path code/tests in `pkg/recheck` and `pkg/beadtracker`

## Test Execution Notes
I followed the TDD flow (added tests before each fix), but this environment does not have Go installed.
- Attempted command after each change: `go test ./...`
- Result each time: `/bin/bash: line 1: go: command not found`

## Findings (Ordered by Severity)

### 1. High: Daemon shutdown race could make shutdown impossible
- Area: `pkg/daemon/daemon.go`
- Problem: `Shutdown()` called before `Run()` used a `sync.Once` path that consumed shutdown without a valid cancel func. Later `Run()` could not be stopped via `Shutdown()`.
- Impact: daemon hang/race in lifecycle control.
- Status: **Fixed**
- Fix:
  - Replaced `sync.Once` shutdown gating with mutex-protected `cancel` + `shutdownRequested` state.
  - Ensured pre-run shutdown cancels immediately once run starts.
  - Added regression test: `TestDaemonShutdownBeforeRun`.

### 2. High: Scanner failed on long JSONL lines
- Area: `pkg/scanner/scanner.go`
- Problem: default `bufio.Scanner` token limit (~64 KiB) caused `ScanFile` failures for long transcript lines.
- Impact: commitment detection misses and scan errors on realistic large assistant messages.
- Status: **Fixed**
- Fix:
  - Increased scanner buffer cap to 2 MiB.
  - Added regression test: `TestScanFile_LongLine`.

### 3. High: Alert-send failure consumed alert state incorrectly
- Area: `pkg/recheck/recheck.go`
- Problem: failed alert sends still marked commitments as `alerted` and incremented alert count.
- Impact: silent lost alerts and exhausted alert budget without actual notification.
- Status: **Fixed**
- Fix:
  - Alert count increments only on successful alert send.
  - On alert failure, status remains previous state instead of forcing `alerted`.
  - Added regression test: `TestRecheckAlertFailureDoesNotConsumeAlert`.

### 4. Medium: Storage/update errors were silent in recheck loop
- Area: `pkg/recheck/recheck.go`
- Problem: `FetchFunc`/`UpdateFunc`/`VerifyFunc`/`AlertFunc` errors were swallowed.
- Impact: operational blind spots when storage or transports fail.
- Status: **Fixed (observability layer)**
- Fix:
  - Added `ErrorFunc` callback to report non-fatal runtime errors.
  - Wired error reporting for fetch/verify/update/alert failures.
  - Added test: `TestRecheckReportsUpdateErrors`.

### 5. Medium: Detector missed common commitment forms
- Area: `pkg/detector/detector.go`
- Problem: missed phrases such as `I'm going to ... in 5 minutes` and `let me ... in 30 seconds`.
- Impact: false negatives in real commitments.
- Status: **Fixed**
- Fix:
  - Expanded temporal patterns for `I'm going to`, `I am going to`, and `let me`.
  - Added tests: `TestDetectTemporalCommitmentVariants`.

### 6. Medium: Untracked-problem detection false negatives
- Area: `pkg/detector/detector.go`
- Problem: tracking marker regex treated any `tracked` word as positive tracking reference (e.g. `not tracked yet`).
- Impact: missed untracked-problem detections.
- Status: **Fixed**
- Fix:
  - Replaced broad marker with explicit positive references (`bd-123`, `tracked in ...`, `created/logged/filed issue/bead`).
  - Added regression test: `TestDetectUntrackedProblemNotTrackedYet`.

### 7. Medium: Cron verifier trusted upstream filtering too much
- Area: `pkg/verifier/verifier.go`
- Problem: accepted all API-returned crons without local `CreatedAt` guard.
- Impact: stale cron jobs could incorrectly mark commitments as backed.
- Status: **Fixed**
- Fix:
  - Added local timestamp filtering (`CreatedAt >= detectedAt`).
  - Added regression test: `TestCronChecker_FiltersStaleCronJobsLocally`.

### 8. Low: `br` unavailable path lacked typed error
- Area: `pkg/beadtracker/beadtracker.go`
- Problem: missing `br` command was only a generic wrapped exec error.
- Impact: callers cannot branch cleanly for dependency failures.
- Status: **Fixed**
- Fix:
  - Added sentinel `ErrCommandUnavailable` and explicit classification using `errors.Is(err, exec.ErrNotFound)`.
  - Added test: `TestCreateBeadCommandNotFoundErrorType`.

### 9. Performance: regex compilation overhead per detector instance
- Area: `pkg/detector/detector.go`
- Problem: regexes were compiled each `NewDetector()` call.
- Impact: avoidable startup overhead in repeated detector instantiation.
- Status: **Fixed**
- Fix:
  - Moved regex compilation to package-level shared vars.
  - `NewDetector()` now reuses compiled regexes.
  - Added test: `TestNewDetectorReusesCompiledRegexes`.
  - Added benchmarks: `pkg/detector/benchmark_test.go`.

## Fixed in This Review
- Daemon pre-run shutdown race.
- Scanner long-line reliability.
- Detector coverage for common temporal phrasing.
- Detector untracked-problem tracking-reference precision.
- Shared precompiled regexes in detector.
- Local stale-cron filtering in verifier.
- Typed error for missing `br` command.
- Recheck error reporting hook and alert-failure semantics.

## Needs Architectural Discussion
- Recheck/storage resilience: errors are now surfaced via `ErrorFunc`, but there is still no built-in retry/backoff/persistent dead-letter strategy for storage transport failures.
- Detector recall strategy: regex coverage improved, but achieving sustained >90% recall likely needs configurable regex packs and optional second-stage classifier (as spec hints).
- Verification breadth: verifier currently checks only crons in code; beads/state/tmux checks remain an architecture/implementation gap relative to spec.
