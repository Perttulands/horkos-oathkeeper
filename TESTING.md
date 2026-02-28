# Oathkeeper Test Quality Assessment

## What Oathkeeper Does

Oathkeeper is a commitment tracker for AI agents. It detects when an agent makes a
promise ("I'll check back in 5 minutes"), tracks it as a bead (via `br` CLI), applies
a grace period for verification, and resolves it when the agent follows through. It
runs as an HTTP server with a v2 API (analyze, commitments, stats, resolve) and also
has CLI subcommands (scan, list, stats, resolve, doctor).

## Rubric Scores — BEFORE (2026-02-28)

| Dimension                        | Score | Rationale |
|----------------------------------|-------|-----------|
| 1. E2E Realism                   | 4     | E2E covers detect → grace → bead → list → resolve → stats lifecycle, plus context analyzer fulfillment/escalation, concurrent requests, error cases, session isolation, category filtering, timing. Missing: CLI E2E (`oathkeeper scan`, `list`, `stats`, `resolve`, `doctor`). |
| 2. Unit Test Behaviour Focus     | 3     | Good behaviour-focused tests for parsers, flag parsing, stats building. But many core functions at 0% (filterByTags, hasAllTags, exitWithError, writeJSON, all run* functions in main.go). Tests don't cover the CLI dispatch layer at all. |
| 3. Edge Case & Error Path        | 2     | Parse error cases well-tested. But: no test for exitWithError JSON/text output, no test for run commands with missing files or bad configs, beads.run() error paths (timeout, stderr) untested, api.addContextResults half-tested. |
| 4. Test Isolation & Reliability  | 4     | E2E uses in-memory stores, temp dirs, random ports. Integration tests gated behind env var. Grace period tests use short durations. Minor: some tests sleep for timing. No shared global state. |
| 5. Regression Value              | 3     | Would catch commitment detection regressions, API contract changes, parse breakage. Would NOT catch CLI dispatch bugs (main.go run* at 0%), bead filtering bugs (filterByTags at 0%), or error output format changes (exitWithError at 0%). |

**Total: 16/25 — Grade C** (Functional but insufficient for a city tool)

## Rubric Scores — AFTER (2026-02-28)

| Dimension                        | Score | Delta | Rationale |
|----------------------------------|-------|-------|-----------|
| 1. E2E Realism                   | 4     | =     | Unchanged. E2E still excellent for HTTP API. CLI E2E still missing (would need exec.Command approach). |
| 2. Unit Test Behaviour Focus     | 4     | +1    | filterByTags, hasAllTags, matchesSession, beadCategory, formatAnalyzeCommitmentID, writeJSON all now tested. Core pure functions covered. Only run*/main/startServer at 0% (require os.Exit refactor). |
| 3. Edge Case & Error Path        | 4     | +2    | beads.run() timeout/stderr/failure paths tested via mock scripts. API nil-function paths, error mapping, method-not-allowed all covered. Stats zero-total edge case covered. addContextResults bead-closing path covered. |
| 4. Test Isolation & Reliability  | 4     | =     | Unchanged. New tests use temp dirs for mock scripts. No shared state. |
| 5. Regression Value              | 4     | +1    | Would now catch: bead CLI execution failures, tag filtering bugs, session matching bugs, API error mapping, context fulfillment bead-closing path. Remaining gap: CLI dispatch (run*) and startServer. |

**Total: 20/25 — Grade B** (Good, with known gaps)

### Known remaining gaps (not addressable without refactoring)

- `main()`, `runScan`, `runList`, `runStats`, `runResolve`, `runDoctor` — all call
  `exitWithError` which calls `os.Exit(1)`. Would need refactoring to a `run() error`
  pattern to test. This is a design issue, not a test gap per se.
- `startServer` — wires real dependencies including daemon signal handling. The serve
  tests already cover the HTTP wiring thoroughly via direct construction.
- `exitWithError` — calls `os.Exit(1)`. The formatting logic (`buildCLIErrorReport`)
  is fully tested. The JSON output path (`writeJSON`) is now tested.

## Gaps Identified (pre-fix)

### Critical (0% coverage, real regression risk)

1. **cmd/oathkeeper main.go — run* functions**: `runScan`, `runList`, `runStats`,
   `runResolve`, `runDoctor`, `runServe` — all 0%. These are the CLI entry points.
   Cannot be tested directly (call `os.Exit`), but the logic they wire is testable.

2. **cmd/oathkeeper main.go — filterByTags/hasAllTags**: Pure functions at 0%.
   Used by `runList` to filter beads by tag. Easy to unit test.

3. **cmd/oathkeeper main.go — exitWithError/writeJSON**: Output formatting at 0%.
   `exitWithError` calls `os.Exit` but `buildCLIErrorReport` is tested. The JSON
   output path via `writeJSON` is not tested at all.

4. **pkg/beads — run() at 15.8%**: The core CLI execution function. Timeout path,
   stderr capture, command failure paths all untested.

5. **pkg/beads — AutoResolve at 23.5%**: Only early-return paths tested. The actual
   resolution loop (list → filter by session → resolve each) is never exercised.

6. **pkg/api v2.go — addContextResults at 47.1%**: The bead-closing path for
   fulfilled commitments (listBeads + matchesSession + resolveBead) is untested.

7. **pkg/api v2.go — matchesSession at 0%**: Session tag matching logic never tested.

### Moderate (partially covered, some risk)

8. **pkg/beads — List/Get at 0-16%**: Depend on `run()` which depends on real `br`.
   Integration tests exist but are gated behind env var (never run in CI).

9. **pkg/api v2.go — handleStats at 63.2%**: Error path when listBeads fails untested.

10. **cmd/oathkeeper main.go — renderStatsConsoleDashboard at 87.5%**: The zero-total
    edge case (all dashes bar) is untested.

## What Tests Will Address

- Pure function tests for filterByTags, hasAllTags, writeJSON (cmd/oathkeeper)
- exitWithError behaviour via buildCLIErrorReport (already tested) + writeJSON
- matchesSession unit tests (pkg/api)
- addContextResults with bead-closing path (pkg/api)
- handleStats error path (pkg/api)
- renderStatsConsoleDashboard zero-total edge case (cmd/oathkeeper)
- beads.run() error paths via mock script (pkg/beads)
- AutoResolve full resolution loop via mock script (pkg/beads)
- parseStatsArgs dashboard + output conflict (cmd/oathkeeper)

---

## Changelog

### 2026-02-28 — Agent: Hephaestus
- Added: Unit tests for filterByTags, hasAllTags (pure functions at 0%)
- Added: Unit tests for writeJSON output formatting
- Added: Unit tests for matchesSession session-tag matching logic
- Added: Tests for addContextResults bead-closing path
- Added: handleStats error path test
- Added: renderStatsConsoleDashboard zero-total edge case
- Added: beads mock-script tests for run() error paths (timeout, stderr, success)
- Added: AutoResolve full loop test via mock script
- Added: parseStatsArgs additional edge cases
- Coverage delta: 74.3% → 81.0% overall
  - cmd/oathkeeper: 49.7% → 55.3% (+5.6%, 12 new tests covering filterByTags, hasAllTags, writeJSON, firstCategoryTag, sortedMapKeys, buildStatsSummary edge cases, renderStatsConsoleDashboard zero-total)
  - pkg/api: 74.7% → 89.7% (+15.0%, 19 new tests covering matchesSession, addContextResults bead-closing, handleStats errors, nil-function guards, beadCategory, formatAnalyzeCommitmentID, method-not-allowed, SetResolveBead, NewV2APIWithFuncs, auto-resolve error)
  - pkg/beads: 69.6% → 93.3% (+23.7%, 17 new tests covering run() timeout/stderr/failure/success, List/Get/Close/Create via mock scripts, AutoResolve full loop, SetTimeout, parseJSONTime, normalizeBead, CreateEmptyOutput)
