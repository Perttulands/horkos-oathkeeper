# Oathkeeper v2 — Refactor Analysis

**Reviewer**: Claude (senior Go architect review)  
**Date**: 2026-02-16  
**Scope**: Full codebase — 4,214 lines of production Go, 8,870 lines of tests, 19 packages  
**Status**: All 19 packages pass `go test ./... -count=1 -race`, `go vet ./...` clean

---

## Executive Summary

Oathkeeper v2 is **solid for what it does**. The core v2 pipeline — detector → grace period → bead creation → webhook — works correctly end-to-end with proper integration tests. The code is well-structured, tests are behavioral, and the architecture is reasonable.

However, there is significant **dead weight from v1** that should be cleaned up, and a few **production-readiness gaps** that matter more than cosmetic refactoring.

**Priority order for any refactoring effort:**
1. Remove dead v1 packages (high value, low risk)
2. Fix the `beadtracker` vs `beads` duplication (actual code smell)
3. Evaluate the `br` CLI shelling approach (architectural decision)
4. Everything else is cosmetic

---

## 1. Dead Code

### Verdict: 5 packages are vestigial from v1 and should be removed or deprecated

| Package | Lines | Status | Recommendation |
|---------|-------|--------|----------------|
| `pkg/storage` | 237 | **Dead in v2 flow**. Only consumed by `pkg/api/api.go` (v1 server) and `pkg/formatter`. Marked deprecated. | **Remove**. No v2 code path uses SQLite storage. |
| `pkg/beadtracker` | 90 | **Superseded by `pkg/beads`**. Both shell out to `br create`. BeadTracker adds `--description` and a slightly different title format, but serve.go uses `beads.BeadStore.Create()`, not BeadTracker. | **Remove**. Merge any unique behavior into `beads`. |
| `pkg/memory` | 95 | **Orphaned**. Never imported by any non-test code. Writes markdown files for commitments — a v1 concept that beads replaced. | **Remove entirely**. |
| `pkg/recheck` | 190 | **Orphaned**. Never imported by serve.go or main.go. Was the v1 periodic re-checker. v2 uses grace period + auto-resolve instead. | **Remove entirely**. |
| `pkg/formatter` | 70 | **Vestigial**. Formats `storage.Commitment` (v1 type) into table/detail views. v2 CLI `list` command formats beads directly, not via this package. | **Remove**. |

Additionally within active packages:
- `pkg/api/api.go` (v1 Server, ListResponse, HealthResponse, unix socket logic) — **164 lines of dead code**. The v2 flow uses `V2API` from `v2.go`. The v1 `Server` struct is only tested in `api_test.go` but never used in production. Could be removed along with `api_test.go` (the v1-specific tests).
- `pkg/api/api.go` helper functions `writeJSON` and `writeError` are shared with v2 — those must stay. The `Server` struct and its handlers can go.

### Impact of removal
~850 lines of production code and ~2,500 lines of tests. The `modernc.org/sqlite` dependency could also be dropped from `go.mod` if storage is removed, saving ~20MB from the binary.

---

## 2. Architecture Smell

### 2.1 No circular dependencies ✅
The dependency graph is clean and acyclic:
```
cmd/oathkeeper → pkg/api, pkg/beads, pkg/config, pkg/daemon, pkg/detector, pkg/grace, pkg/hooks, pkg/verifier, pkg/scanner, pkg/doctor
pkg/api → pkg/beads, pkg/detector, pkg/grace
pkg/detector → (none)
pkg/beads → (none)
pkg/grace → (none)
pkg/hooks → (none)
```

### 2.2 No God objects ✅
`V2API` is the most complex struct (~11 fields), but they're all function references injected at construction time — clean dependency injection.

### 2.3 Package separation is mostly right ✅
- `detector` does detection, `beads` does bead lifecycle, `grace` does timing, `hooks` does webhooks. Clear responsibilities.
- `api/v2.go` is the composition root for HTTP handlers — appropriate place for wiring.

### 2.4 Minor smell: `V2API` uses function fields instead of interfaces
```go
detectCommitment  func(string) detector.DetectionResult
autoResolve       func(sessionKey string, message string) ([]string, error)
listBeads         func(filter beads.Filter) ([]beads.Bead, error)
```

This is a **pragmatic choice** for testability (no need to define mock interfaces), and it works. It's unconventional in Go but not wrong. The V2API struct is the only consumer, so interfaces would add abstraction without earning it. **Leave as-is.**

### 2.5 Config has v1 cruft
`config.Config` carries fields that v2 doesn't use:
- `LLM` section (claude command, args) — never referenced in v2 serve path
- `Storage.DBPath`, `Storage.AutoExpireHours` — SQLite-era settings
- `Alerts.ThrottleWindow` — v1 concept, v2 uses webhooks
- `OpenClaw.TranscriptDir`, `OpenClaw.WakeEndpoint` — v1 scan-mode fields

These should be cleaned up when removing v1 packages, but are **low priority** since TOML ignores unknown keys.

---

## 3. Test Quality

### Verdict: Tests are strong. Behavioral, not implementation-coupled.

**Strengths:**
- Tests verify behavior, not structure. E.g., detector tests check "this message IS/ISN'T a commitment" rather than internal regex details.
- Table-driven tests throughout (`detector_test.go` has 64+ cases).
- Real `br` integration tests with isolated databases (`newTestBeadStore` pattern with temp dirs and wrapper scripts).
- Concurrency tests run with `-race` flag.
- The `pkg/integration_test.go` does a real full-lifecycle test: POST analyze → wait grace → verify bead created → resolve → verify closed → check stats.
- Good negative tests (invalid JSON, empty IDs, missing commands, 4xx vs 5xx webhook behavior).

**Weaknesses:**
- `pkg/api/api_test.go` has 15 tests for the v1 Server that's dead code. These should go with the v1 server removal.
- `pkg/doctor/doctor_test.go` uses relative path `../../go.mod` for file existence checks — fragile if test working directory changes.
- Some test helpers reimplements `strings.Contains` manually (e.g., `containsSubstring` in alerts_test.go, `containsStr` in doctor_test.go). Trivial but odd.

**Coverage gaps (minor):**
- No test for `startServer()` in serve.go with a real config file. The serve tests wire dependencies manually rather than going through `startServer()`. This is fine for unit testing but means the actual wiring function is only tested implicitly.
- `extractCommitmentText` in detector.go always returns the full message — the TODO comment says "can be refined." Not a bug, but worth noting.

---

## 4. The v2 Wiring

### Verdict: Correctly wired. All 3 gaps from the PRD-v2-remaining are closed.

**ContextAnalyzer integration** ✅
- `serve.go` creates `detector.NewContextAnalyzer(cfg.General.ContextWindowSize)` and passes it via `v2.SetContextAnalyzer(ca, windowSize)`.
- `handleAnalyze` appends messages to per-session buffer, runs `ContextAnalyzer.Analyze()`, and closes matching beads for fulfilled commitments.
- Session isolation and buffer trimming work correctly (verified by tests).

**Resolve webhook** ✅
- `serve.go` sets `v2.SetResolveCallback(...)` that calls `webhook.NotifyResolved()`.
- Both manual resolve and auto-resolve paths fire the callback.
- Null-safe: no panic when callback is nil.

**Grace callback → bead creation** ✅
- `serve.go` wires `v2.SetGraceCallback(...)` that calls `beadStore.Create()` for unbacked commitments and `webhook.NotifyUnbacked()`.

**One subtle issue**: In `serve.go`, the resolve callback is only wired when `webhook != nil` (when TelegramWebhook is configured). If you want resolve callbacks for other purposes (logging, metrics), you'd need to restructure this. **Low priority** — the current design matches the PRD.

---

## 5. Production Readiness

### Verdict: Could run as a daemon today with caveats.

**What works:**
- `oathkeeper serve` starts an HTTP server with graceful shutdown (SIGINT/SIGTERM).
- Health endpoint (`/healthz`) and readiness check (`/readyz` verifies `br` is available).
- Grace period with proper cleanup on shutdown.
- Config file with sensible defaults.
- All tests pass including integration tests with real `br`.

**What's missing for production:**

1. **No structured logging.** Uses `log.Printf` (stdlib) — no levels, no JSON output, no correlation IDs. For a daemon processing live messages, you'd want at minimum `slog` (stdlib since Go 1.21).

2. **No metrics/observability.** No request counts, latency histograms, error rates, or bead creation counts. For a system that runs continuously and processes OpenClaw messages, this is a gap.

3. **No systemd unit file.** The daemon handles signals correctly, but there's no `oathkeeper.service` file for deployment.

4. **Session buffers are unbounded in session count.** `v2.sessions` is a `map[string]*sessionBuffer` that grows indefinitely. Each session is bounded in message count (window size), but the number of distinct session keys is not. For a long-running daemon, this is a slow memory leak. **Fix: add LRU eviction or periodic cleanup of stale sessions.**

5. **No request timeout on the HTTP server.** `http.Server` has no `ReadTimeout`, `WriteTimeout`, or `IdleTimeout` set. For a production server, these should be configured to prevent slowloris and similar attacks.

6. **The `br` CLI dependency is a single point of failure.** If `br` hangs, the goroutine calling `beadStore.Create()` blocks for 5 seconds (timeout), but the grace callback goroutine is also blocked. Multiple stuck grace callbacks could exhaust goroutines. The 5-second timeout mitigates this but doesn't eliminate it.

---

## 6. Simplification Opportunities

### Packages that could be merged (but probably shouldn't):

- **`expiry` into `detector`**: `ComputeExpiresAt` is a pure function that analyzes commitment text. It could live in detector. But it's small (50 lines), self-contained, and clearly named. **Leave as-is.**

- **`hooks` could absorb `alerts`**: Both deal with notifications. But `alerts` is v1-era (Argus/Telegram/Wake alerters) while `hooks` is v2-era (generic webhook). If v1 code is removed, `alerts` goes away and the question is moot.

### Abstractions that don't earn their keep:

- **`pkg/doctor` Config struct mirrors `pkg/config` Config**: The doctor has its own `Config` struct that's populated by extracting fields from the main config. This intermediate struct adds friction but keeps doctor testable without depending on config package internals. **Neutral — leave as-is.**

- **`GraceCallbackFunc` type alias**: Defined in v2.go as `func(commitmentID string, message string, category string, outcome grace.VerificationOutcome)`. This is used exactly once. The type alias adds readability though. **Leave as-is.**

---

## 7. The Beads Integration: `br` CLI vs Go Package

### Current approach: Shell out to `br` CLI

`pkg/beads/beads.go` runs `br create`, `br list --json`, `br show --json`, `br close` as child processes with a 5-second timeout.

### Assessment

**Pros of shelling out:**
- `br` is the authoritative implementation — no risk of divergence.
- Zero additional Go dependencies (just `os/exec`).
- If `br` gets new features (e.g., new list flags), oathkeeper gets them for free.
- The `--db` flag allows test isolation via wrapper scripts.

**Cons:**
- **Process overhead**: Each bead operation spawns a child process (~5ms per call). For the current use case (one bead per detected commitment), this is fine. At scale (hundreds per second), it would be a bottleneck.
- **Error handling is string-based**: Errors come from stderr parsing, not typed errors.
- **JSON parsing is fragile**: `parseBeadListJSON` handles both `tags` and `labels` fields, both `created_at` and `createdAt` field names — evidence that the `br` CLI output format isn't fully stable.
- **No transactional operations**: Can't atomically "create bead and tag it" — it's two separate exec calls (though `br create --labels` handles this currently).

**Recommendation**: **Keep shelling out for now.** The process overhead is negligible at oathkeeper's throughput (maybe 10-50 commitments per day). The fragile JSON parsing is the main risk, and that's mitigatable by pinning `br` versions. Switching to a Go package would only be worth it if:
1. Oathkeeper needs to process >100 bead operations per second, OR
2. The beads codebase publishes a stable Go API (e.g., `github.com/perttulands/beads/pkg/store`)

If/when the beads Go package exists, the migration is clean: replace the `run()` method in `BeadStore` with direct function calls, keeping the same `BeadStore` interface.

---

## Recommended Refactoring Plan

### Phase 1: Dead code removal (1-2 hours, high value)
1. Delete `pkg/storage/` and `pkg/storage/storage_test.go`
2. Delete `pkg/beadtracker/` and tests
3. Delete `pkg/memory/` and tests
4. Delete `pkg/recheck/` and tests
5. Delete `pkg/formatter/` and tests
6. Remove v1 `Server` struct and handlers from `pkg/api/api.go` (keep `writeJSON`, `writeError`)
7. Remove v1 tests from `pkg/api/api_test.go`
8. Remove `modernc.org/sqlite` from `go.mod` (run `go mod tidy`)
9. Clean v1-only fields from `pkg/config/config.go` (optional, low priority)

**Expected result**: ~850 fewer production lines, ~2,500 fewer test lines, smaller binary, no sqlite dependency.

### Phase 2: Production hardening (if deploying soon)
1. Add HTTP server timeouts to `serve.go` (`ReadTimeout: 10s`, `WriteTimeout: 10s`, `IdleTimeout: 60s`)
2. Add session buffer eviction (LRU or max-sessions cap with eviction of oldest)
3. Add a systemd unit file
4. Consider switching from `log.Printf` to `log/slog` for structured logging

### Phase 3: Optional improvements (low priority)
1. Remove redundant `containsSubstring` helpers from test files (use `strings.Contains`)
2. Fix `pkg/doctor` to not use relative paths in tests
3. Add `extractCommitmentText` refinement (extract just the commitment phrase, not full message)

---

## What NOT to Refactor

- **Package structure**: It's clean. Don't merge packages that are fine as-is.
- **Function-field dependency injection in V2API**: It works, it's testable, don't add interfaces for the sake of it.
- **The `br` shelling approach**: It's the right choice at current scale.
- **Test structure**: Tests are behavioral and well-organized. Don't restructure for coverage metrics.
- **Config file format**: TOML works, backward-compatible loading works. Don't switch to YAML/JSON.

---

## Final Assessment

Oathkeeper v2 is a **well-built, well-tested Go project**. The v2 pipeline is correctly wired, tests cover real behavior including integration with the `br` CLI, and the architecture is clean. The main cleanup needed is removing ~5 v1 packages that are no longer used — straightforward work with zero risk to the v2 functionality. After that cleanup and minor production hardening, this is ready to run as a daemon.
