# Oathkeeper Code Review (`pkg/**/*.go`)

## Scope And Execution
- Reviewed every `.go` file under `pkg/` (30 files total, including tests).
- Requested commands executed:
  - `go test ./...`
  - `go vet ./...`
- Result: both failed in this environment with `/bin/bash: go: command not found`.

## Findings (Ordered By Severity)

### 1. High: Path traversal via commitment ID in memory file operations
- Location: `pkg/memory/memory.go:34`, `pkg/memory/memory.go:35`, `pkg/memory/memory.go:46`, `pkg/memory/memory.go:57`
- Issue: `FilePath` directly interpolates `id` into a path and uses `filepath.Join`. IDs containing `../` or path separators can escape `w.dir`.
- Impact: `WriteCommitment`/`RemoveCommitment` can write/delete files outside the intended memory directory.
- Recommendation: validate IDs against a strict allowlist (e.g. `^[a-zA-Z0-9_-]+$`) and enforce that cleaned paths stay within `w.dir`.

### 2. Medium: Duplicate scheduling bug in grace-period tracker
- Location: `pkg/grace/grace.go:49`, `pkg/grace/grace.go:82`, `pkg/grace/grace.go:83`, `pkg/grace/grace.go:71`
- Issue: scheduling the same `commitmentID` again overwrites `pending[commitmentID]` without canceling the old timer.
- Impact: duplicate callbacks can fire; old callback can remove the newer pending entry, causing incorrect `Pending()`/`Cancel()` behavior.
- Recommendation: on `Schedule`, cancel and remove any existing entry for that ID before registering the new timer.

### 3. Medium: Recheck flow ignores persistence/alert failures and can lose alerts
- Location: `pkg/recheck/recheck.go:96`, `pkg/recheck/recheck.go:113`, `pkg/recheck/recheck.go:125`, `pkg/recheck/recheck.go:128`
- Issue: return errors from `UpdateFunc` and `AlertFunc` are ignored. `IncrementAlert` is set from `shouldAlert` regardless of whether `AlertFunc` succeeded.
- Impact: commitments can be marked alerted/incremented even when notification failed; retries can be exhausted without any delivered alert.
- Recommendation: handle returned errors explicitly; only increment alert count when alert delivery succeeds.

### 4. Medium: Wake endpoint path is built from unescaped session/source value
- Location: `pkg/alerts/alerts.go:170`
- Issue: `commitment.Source` is interpolated directly into URL path via `fmt.Sprintf`.
- Impact: source values containing `/`, `?`, `#`, or `%` can alter routing and target wrong endpoints.
- Recommendation: use `url.PathEscape(commitment.Source)` when composing path segments.

### 5. Low: Cron verifier trusts remote filtering without local safety check
- Location: `pkg/verifier/verifier.go:62`, `pkg/verifier/verifier.go:80`, `pkg/verifier/verifier.go:81`
- Issue: all returned crons are accepted as backing mechanisms without checking `CreatedAt >= detectedAt` locally.
- Impact: if upstream API ignores/misapplies `since`, stale cron jobs can incorrectly mark commitments as backed.
- Recommendation: locally filter by `CreatedAt` as defense in depth.

### 6. Low: API JSON writer drops encode errors
- Location: `pkg/api/api.go:173`
- Issue: `json.NewEncoder(w).Encode(v)` return value is ignored.
- Impact: partial/failed writes are silent and hard to diagnose.
- Recommendation: check encode errors and log/handle appropriately.

## SQLite SQL Injection Assessment
- Reviewed all SQL call sites in `pkg/storage/storage.go`.
- No SQL injection vulnerability found.
- Dynamic filtering in `List` still uses placeholders (`?`) with bound args; untrusted values are not string-interpolated into SQL literals.

## Test Quality Assessment
- Good breadth across packages, including many success/failure paths and HTTP integration-style tests.
- Main gaps/risks:
  - Missing test for memory path traversal hardening (`pkg/memory/memory_test.go`).
  - Missing test for duplicate `Schedule` of same ID in grace scheduler (`pkg/grace/grace_test.go`).
  - Missing test that recheck does **not** increment alerts when `AlertFunc` fails (`pkg/recheck/recheck_test.go`).
  - Environment-coupled test: `pkg/doctor/doctor_test.go:80` assumes `go` binary is available.

## Idiomatic Go Assessment
- Positive: clear package boundaries, mostly small functions, use of wrapped errors (`%w`), table-driven tests in many places.
- Improvement areas:
  - Avoid silently ignoring returned errors where behavior depends on success (`recheck`, `api`).
  - Prefer standard library helpers (`strings.Contains`) over custom substring helpers in tests.

## Architecture Assessment
- Overall structure is modular and understandable (`detector`, `verifier`, `storage`, `api`, `alerts`, schedulers).
- Notable architecture risks:
  - State/status constants are duplicated across packages (e.g. `storage` and `recheck`), which risks drift.
  - Scheduling and retry paths lack explicit error channels/telemetry, making operational failures hard to observe.

## Personal Information Check
- No obvious personal/sensitive data found in `pkg/**/*.go`.
- Spot-check command run: `rg -n -i "(password|secret|token|api[_-]?key|ssn|social security|@gmail|@yahoo|phone|address|personal|email)" pkg -g '*.go'` (no matches).
