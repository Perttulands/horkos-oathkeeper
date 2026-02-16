# Oathkeeper v2 PRD — Beads-Native Live Commitment Guard

## Vision

Oathkeeper v2 uses **beads as the commitment store** — not a separate SQLite database. Every detected commitment becomes a bead. Resolution closes the bead. The bead system IS the registry.

The live analysis API lets OpenClaw POST every assistant message for real-time commitment detection.

## Principles

- **No mocks in production code.** No mock interfaces, no stub implementations, no "fallback" modes that silently degrade.
- **No SQLite commitment registry.** Beads are the single source of truth.
- **Real integration tests.** Tests that exercise actual `br` CLI (or skip if unavailable).
- **Fail loud.** If `br` isn't available, error. Don't silently continue without tracking.

## Architecture

```
POST /api/v2/analyze  →  Detector  →  Commitment?
                                           ↓ yes
                                     Grace period (30s)
                                           ↓
                                     Verifier (check cron/beads/state)
                                           ↓ unbacked
                                     br create --tag oathkeeper
                                           ↓
                                     Webhook alert (OpenClaw wake)
```

## Constraints

- Go stdlib only for HTTP. No gin/echo/chi.
- BurntSushi/toml for config.
- `br` CLI is a hard dependency, not optional.
- All existing tests must pass. Zero regressions.
- TDD: test first, implement second.
- Run with `-race` for anything touching shared state.

## Global Verification

```bash
/usr/local/go/bin/go build ./...
/usr/local/go/bin/go test ./... -count=1 -race
/usr/local/go/bin/go vet ./...
```

All three must pass with zero failures.

---

## Sprint 1: Beads-Native Store
**Status:** COMPLETE

- [x] **US-001** Bead-backed commitment tracker
  - File: `pkg/beads/beads.go`, `pkg/beads/beads_test.go`
  - `BeadStore` wraps `br` CLI: Create, Close, List, Get
  - CommitmentInfo: Text, Category, SessionKey, DetectedAt, ExpiresAt
  - No fallback mode — if `br` not in PATH, return error
  - Verify: `go test ./pkg/beads/...`

- [x] **US-002** Remove SQLite commitment storage dependency
  - File: `pkg/storage/storage.go`, `pkg/config/config.go`
  - pkg/storage has deprecation comment, `FallbackEnabled` removed from config
  - Verify: `go test ./pkg/config/... ./pkg/storage/...`

- [x] **US-003** Commitment resolution via bead close
  - File: `pkg/beads/resolve.go`, `pkg/beads/resolve_test.go`
  - `Resolve(beadID, evidence)` closes bead with evidence
  - `AutoResolve(sessionKey, message)` scans for resolution indicators
  - Verify: `go test ./pkg/beads/...`

---

## Sprint 2: Live Analysis API
**Status:** COMPLETE

- [x] **US-004** POST /api/v2/analyze endpoint
  - File: `pkg/api/v2.go`, `pkg/api/v2_test.go`
  - Accepts JSON: `{"session_key": "main", "message": "...", "role": "assistant"}`
  - Only processes role=assistant; runs Detector + AutoResolve
  - Verify: `go test ./pkg/api/...`

- [x] **US-005** GET /api/v2/commitments endpoint
  - File: `pkg/api/v2.go`
  - List, detail, and manual resolve endpoints for oathkeeper beads
  - Verify: `go test ./pkg/api/...`

- [x] **US-006** GET /api/v2/stats endpoint
  - File: `pkg/api/v2.go`, `pkg/api/v2_test.go`
  - Returns `{"total": N, "open": N, "resolved": N, "by_category": {...}}`
  - Computed from `BeadStore.List(Filter{})` — all oathkeeper beads
  - Tests: `TestV2StatsMixedStates`, `TestV2StatsEmpty` passing
  - Verify: `/usr/local/go/bin/go test ./pkg/api/... -run Stats -count=1 -v`

---

## Sprint 3: Enhanced Detection
**Status:** IN PROGRESS

- [x] **US-007** Expand commitment patterns
  - File: `pkg/detector/detector.go`, `pkg/detector/detector_test.go`
  - followupPatterns: monitor, watch, keep an eye on, report back, update you, let you know
  - weakCommitmentPatterns: "I need to X", "I should X" (confidence 0.70)
  - codeUntrackedPatterns: TODO, FIXME, HACK
  - Each pattern has ≥3 positive and ≥2 negative test cases
  - Verify: `/usr/local/go/bin/go test ./pkg/detector/... -count=1 -v`

- [ ] **US-008** Context-aware auto-resolution — INTEGRATE ContextAnalyzer into V2API
  - **Problem**: `pkg/detector/context.go` has a full ContextAnalyzer with tests — but it's never used. `handleAnalyze` is stateless per-message. Fulfilled commitments are never detected from conversation flow. Escalation is dead code.
  - File: `pkg/api/v2.go` — add session buffer, integrate ContextAnalyzer
  - File: `pkg/api/v2_test.go` — new tests for context integration
  - File: `pkg/detector/context.go` — no changes needed (already complete)
  - **Implementation**:
    - Add to V2API struct: `mu sync.Mutex`, `sessions map[string]*sessionBuffer`, `contextWindowSize int`
    - `sessionBuffer` holds `messages []string` per session_key
    - In `handleAnalyze` after role check: append message to session buffer (under lock), copy buffer, unlock
    - After detectCommitment + autoResolve: run `ContextAnalyzer.Analyze(buffer)`
    - For each fulfilled: close matching open beads for that session
    - For each escalated: include in response
    - Trim buffer to window size after append
  - **Response additions**: `"fulfilled": [{"text": "...", "fulfilled_by": "..."}]` and `"escalated": [{"category": "...", "count": N}]`
  - **Acceptance criteria**:
    1. V2API maintains in-memory per-session message buffer (last N messages, configurable, default 5)
    2. Every POST /api/v2/analyze with role=assistant appends to session buffer
    3. ContextAnalyzer.Analyze runs on the session buffer after detection + auto-resolve
    4. Fulfilled commitments close matching open oathkeeper beads
    5. Escalated commitments included in analyze response
    6. Buffer bounded: oldest messages dropped when exceeding window size
    7. Per session_key — no cross-contamination between sessions
    8. Thread-safe: concurrent requests don't interfere (use sync.Mutex, narrow lock scope)
  - **Tests**:
    - POST 3 messages to same session (commitment → unrelated → fulfillment), verify fulfilled in response
    - POST 2 commitments of same type, verify escalated in response
    - Different sessions don't cross-contaminate
    - Buffer trimming beyond window size
    - 3 goroutines posting to same session simultaneously, no race (`-race`)
    - Non-assistant messages don't affect buffer
  - Verify: `/usr/local/go/bin/go test ./pkg/api/... -count=1 -race -v`

---

## Sprint 4: CLI
**Status:** IN PROGRESS

- [x] **US-009** CLI entry point
  - File: `cmd/oathkeeper/main.go`, `cmd/oathkeeper/main_test.go`
  - Subcommands: serve, scan, list, stats, resolve, doctor
  - `--config PATH` override, `--help`, `--version`
  - Verify: `/usr/local/go/bin/go test ./cmd/oathkeeper/... -count=1 -v`

- [ ] **US-010** Wire serve command — add ContextAnalyzer wiring and resolved webhook
  - **Problem**: serve.go wires most components but ContextAnalyzer is not instantiated. Resolved webhook (`NotifyResolved`) never fires. Config lacks `context_window_size`.
  - File: `cmd/oathkeeper/serve.go` — wire ContextAnalyzer, resolve callback
  - File: `cmd/oathkeeper/serve_test.go` — test resolved webhook fires
  - File: `pkg/config/config.go` — add `ContextWindowSize int` to GeneralConfig, `ServerAddr` field
  - File: `pkg/config/config_test.go` — test new config fields
  - File: `pkg/api/v2.go` — add `SetResolveCallback`, call it on resolve
  - **Implementation**:
    1. Config additions: `[general] context_window_size = 5`, `[server] addr = ":9876"`
    2. Create ContextAnalyzer in `startServer`: `context.NewContextAnalyzer(cfg.General.ContextWindowSize)`
    3. Pass to V2API via constructor or `SetContextAnalyzer()`
    4. Add resolve callback to V2API: `SetResolveCallback(func(beadID, evidence string))`
    5. V2API calls resolve callback from `handleResolveCommitment` and `AutoResolve`
    6. In serve.go, wire: `v2.SetResolveCallback(func(beadID, evidence string) { webhook.NotifyResolved(beadID, evidence) })`
  - **Acceptance criteria**:
    1. `startServer` wires: config → BeadStore → Detector → ContextAnalyzer → Verifier → GracePeriod → V2API → Webhook → Daemon
    2. V2API receives `contextWindowSize` from config (default 5)
    3. Bead resolution (API or auto-resolve) triggers `webhook.NotifyResolved(beadID, evidence)`
    4. Resolve callback is fire-and-forget (`go v2.onResolve(...)`)
    5. Server addr configurable in config (currently hardcoded `:9876`)
    6. Graceful shutdown stops all components
  - **Tests**:
    - Resolve via API triggers resolve callback
    - Auto-resolve triggers resolve callback for each resolved bead
    - Config loads context_window_size with default fallback
    - Start server, resolve commitment, verify callback was called
  - Verify: `/usr/local/go/bin/go test ./cmd/oathkeeper/... ./pkg/api/... ./pkg/config/... -count=1 -v`

---

## Sprint 5: Hardening
**Status:** IN PROGRESS

- [ ] **US-011** Integration test — full bead lifecycle
  - **Problem**: `pkg/integration_test.go` exists but is shallow. `TestIntegrationFullLifecycle` never verifies bead creation after grace period. `TestIntegrationConcurrentCommitments` uses nil beadStore. The PRD-specified lifecycle (create → verify → resolve → verify closed) isn't tested.
  - File: `pkg/integration_test.go` — rewrite lifecycle and concurrent tests
  - **Implementation**:
    1. Create isolated BeadStore using `br-wrapper.sh` pattern from `pkg/beads/beads_test.go` (each test gets own `t.TempDir()`)
    2. Wire graceCallback in test server that actually calls `beadStore.Create(CommitmentInfo{...})` for unbacked commitments
    3. After posting commitment and waiting for grace: query `beadStore.List` to verify bead created
    4. Resolve bead via API, then verify closure via `beadStore.Get`
    5. For concurrent test: use real BeadStore, verify 5 distinct beads created
    6. Use `t.Cleanup` to close/clean up beads
  - **Acceptance criteria**:
    1. **Full lifecycle** (requires `br`): start server → POST analyze (temporal commitment) → assert `commitment: true` → wait grace (100ms in test) → GET commitments?status=open (verify bead) → POST resolve → GET commitment (verify closed) → GET stats (verify resolved count)
    2. **Concurrent** (requires `br`): 5 goroutines POST different commitments → all 200 → no data races (`-race`) → verify distinct beads created
    3. Test isolation: each test gets its own bead database, no pollution of real DB
    4. Skip with `t.Skip("br not in PATH")` if br unavailable
  - Verify: `/usr/local/go/bin/go test ./pkg/ -run Integration -count=1 -race -v -timeout 30s`

- [x] **US-012** Webhook on violation — wire resolved webhook into live flow
  - **Problem**: `pkg/hooks/webhook.go` has `NotifyResolved()` with tests passing, but it's never called. `NotifyUnbacked` is wired (grace callback in serve.go), but resolved events are missing from both AutoResolve and manual resolve paths.
  - File: `pkg/api/v2.go` — add `onResolve func(beadID, evidence string)` callback to V2API
  - File: `pkg/api/v2_test.go` — test callback fires on manual resolve and auto-resolve
  - File: `cmd/oathkeeper/serve.go` — wire `webhook.NotifyResolved` into onResolve
  - File: `pkg/hooks/webhook.go` — no changes needed
  - **Implementation**:
    1. Add to V2API: `onResolve func(beadID string, evidence string)`
    2. Add setter: `func (v2 *V2API) SetResolveCallback(fn func(beadID, evidence string))`
    3. In `handleResolveCommitment`, after successful `resolveBead`: `go v2.onResolve(beadID, req.Reason)`
    4. In `handleAnalyze`, after successful `autoResolve`: `go v2.onResolve(resolvedID, req.Message)` for each resolved ID
    5. In serve.go: `v2.SetResolveCallback(func(beadID, evidence string) { webhook.NotifyResolved(beadID, evidence) })`
  - **Acceptance criteria**:
    1. Unbacked commitment → POST `{"event": "commitment.unbacked", ...}` ✅ already done
    2. Auto-resolved bead (during analyze) → POST `{"event": "commitment.resolved", "bead_id": "...", "evidence": "..."}`
    3. Manual resolve (POST /:id/resolve) → POST `{"event": "commitment.resolved", "bead_id": "...", "evidence": "..."}`
    4. Retry 5xx with backoff (1s/2s/4s, max 3) ✅ already done
    5. No retry on 4xx ✅ already done
    6. Webhook URL optional — empty means no webhook
    7. Webhook failures don't block API response (fire-and-forget, log errors)
  - **Tests**:
    - Manual resolve via POST /:id/resolve triggers onResolve with correct beadID and reason
    - Auto-resolve in handleAnalyze triggers onResolve for each resolved bead ID
    - onResolve=nil doesn't panic
  - Verify: `/usr/local/go/bin/go test ./pkg/api/... ./pkg/hooks/... -count=1 -v`

- [x] **US-013** Health and readiness
  - File: `pkg/api/health.go`, `pkg/api/health_test.go`
  - `GET /healthz` → 200 always; `GET /readyz` → 200 if `br version` succeeds, 503 otherwise
  - 3-second timeout on readiness check; non-GET → 405
  - Verify: `/usr/local/go/bin/go test ./pkg/api/... -run "Health|Ready" -count=1 -v`

---

## Implementation Order (dependency-aware)

```
Phase 1 — Verify only (no code changes):
  US-006, US-007, US-009, US-013 → run tests, confirm passing

Phase 2 — Code changes (in order):
  US-012 → add onResolve callback to V2API + wire in serve.go
  US-008 → integrate ContextAnalyzer into V2API (depends on US-012)
  US-010 → wire ContextAnalyzer + config additions (depends on US-008, US-012)

Phase 3 — Integration (last):
  US-011 → rewrite integration tests with real bead lifecycle (depends on all above)
```

## Dependency Graph

```
US-006 (stats)     ─── done
US-007 (patterns)  ─── done
US-009 (CLI)       ─── done
US-013 (health)    ─── done

US-012 (webhook)   ─── add resolve callback to V2API
        ↓
US-008 (context)   ─── integrate ContextAnalyzer into V2API
        ↓
US-010 (wiring)    ─── wire ContextAnalyzer + resolve webhook in serve.go
        ↓
US-011 (integ.)    ─── rewrite tests with full bead lifecycle
```
