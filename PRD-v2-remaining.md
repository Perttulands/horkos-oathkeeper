# Oathkeeper v2 — Remaining Work PRD (US-006 → US-013)

## Context

US-001 through US-005 are **complete and tested**:
- **US-001**: `pkg/beads/beads.go` — BeadStore wrapping `br` CLI with Create/Close/List/Get
- **US-002**: legacy `pkg/storage/` removed, `FallbackEnabled` removed from config
- **US-003**: `pkg/beads/resolve.go` — Resolve + AutoResolve with session-scoped resolution
- **US-004**: `pkg/api/v2.go` — POST `/api/v2/analyze` with grace period scheduling
- **US-005**: `pkg/api/v2.go` — GET `/api/v2/commitments`, GET `/:id`, POST `/:id/resolve`

All existing tests pass (`go test ./... -count=1` = 0 failures).

## Implementation Status Summary

| Story | Code Exists | Tests Exist | Wired | Gaps |
|-------|------------|-------------|-------|------|
| US-006 | ✅ | ✅ | ✅ | None — verify only |
| US-007 | ✅ | ✅ | ✅ | None — verify only |
| US-008 | ✅ | ✅ | ❌ | **ContextAnalyzer not integrated into V2API** |
| US-009 | ✅ | ✅ | ✅ | None — verify only |
| US-010 | ✅ | ✅ | ⚠️ | Missing ContextAnalyzer wiring; no resolved webhook |
| US-011 | ✅ | ⚠️ | ⚠️ | **Integration tests don't verify bead lifecycle** |
| US-012 | ✅ | ✅ | ⚠️ | **NotifyResolved never called from live flow** |
| US-013 | ✅ | ✅ | ✅ | None — verify only |

## Critical Gaps (What's Actually Missing)

### Gap 1: ContextAnalyzer is Orphaned (US-008 + US-010)

`pkg/detector/context.go` has a full `ContextAnalyzer` with fulfillment detection and
escalation logic. It has comprehensive tests. **But it's never used by the API or serve
command.** The `handleAnalyze` endpoint processes each message in isolation — it doesn't
maintain a message history or use `ContextAnalyzer.Analyze()`.

This means:
- Fulfilled commitments are never auto-detected from conversation flow
- Repeated promises are never escalated
- The entire "context-aware" capability is dead code

### Gap 2: Integration Tests Are Shallow (US-011)

`pkg/integration_test.go` exists but:
- `TestIntegrationFullLifecycle` never verifies that a bead was actually created after
  the grace period fires. It checks endpoints respond, not that the lifecycle works.
- `TestIntegrationConcurrentCommitments` uses `nil` beadStore (doesn't test real bead
  operations under concurrency).
- The PRD specified: "start server → POST analyze → wait grace period → **verify bead
  created** → POST resolve → **verify bead closed**". The bold parts aren't tested.

### Gap 3: Resolved Webhook Not Wired (US-012)

`pkg/hooks/webhook.go` has `NotifyResolved()` and it's tested. In `serve.go`, only
`NotifyUnbacked()` is wired into the grace callback. When a bead is auto-resolved
(via `AutoResolve` in the analyze endpoint) or manually resolved (via
`POST /:id/resolve`), no webhook fires. The PRD says: "When bead auto-resolved:
POST with `{"event": "commitment.resolved", ...}`".

---

## User Stories — Detailed Specifications

### US-006: GET /api/v2/stats endpoint

**Status**: ✅ Complete — verify only

**What exists**:
- `handleStats` in `pkg/api/v2.go` lines 157–191
- `statsAPIResponse` struct with Total, Open, Resolved, ByCategory
- `beadCategory` helper extracts category from tags (skips "oathkeeper" and "session-*")
- Tests: `TestV2StatsMixedStates`, `TestV2StatsEmpty` in `pkg/api/v2_test.go`

**Acceptance Criteria**:
1. `GET /api/v2/stats` returns 200 with JSON: `{"total": N, "open": N, "resolved": N, "by_category": {"temporal": N, ...}}`
2. Stats are computed from `BeadStore.List(Filter{})` — all oathkeeper beads
3. `by_category` excludes "oathkeeper" and "session-*" tags
4. Empty bead list returns `{"total": 0, "open": 0, "resolved": 0, "by_category": {}}`
5. Non-GET methods return 405
6. Bead store unavailable returns 500

**File paths**: `pkg/api/v2.go`, `pkg/api/v2_test.go`

**Test strategy**: Unit tests with mock `listBeads` function (already passing)

**Dependencies**: US-005 (listBeads function wired)

**Verification**:
```bash
/usr/local/go/bin/go test ./pkg/api/... -run Stats -count=1 -v
```

---

### US-007: Expand commitment patterns

**Status**: ✅ Complete — verify only

**What exists**:
- `followupPatterns` in `pkg/detector/detector.go`: monitor, watch, keep an eye on, report back, update you, let you know
- `weakCommitmentPatterns`: I need to X, I should X (confidence 0.70)
- `codeUntrackedPatterns`: TODO, FIXME, HACK with `:` or whitespace separator
- Category constants: `CategoryFollowup`, `CategoryWeakCommitment`, `CategoryUntracked`
- Tests: `TestDetectFollowupCommitments` (7 positive, 3 negative), `TestDetectWeakCommitments` (4 positive, 3 negative), `TestDetectCodeUntrackedMarkers` (4 positive, 3 negative)

**Acceptance Criteria**:
1. "I'll monitor/watch/keep an eye on" → `followup`, confidence 0.90
2. "I'll report back/update you/let you know" → `followup`, confidence 0.90
3. "I need to X" / "I should X" → `weak_commitment`, confidence 0.70
4. "TODO:" / "FIXME:" / "HACK:" without tracking reference → `untracked_problem`, confidence 0.90
5. Each pattern has ≥3 positive and ≥2 negative test cases
6. No regression on existing 64+ detector tests
7. Patterns support I'll/I will/I'm going to/I am going to prefixes
8. Past tense filter still blocks: "I monitored" not detected as commitment

**File paths**: `pkg/detector/detector.go`, `pkg/detector/detector_test.go`

**Test strategy**: Table-driven unit tests (already passing)

**Dependencies**: None (standalone patterns)

**Verification**:
```bash
/usr/local/go/bin/go test ./pkg/detector/... -count=1 -v
```

---

### US-008: Context-aware auto-resolution

**Status**: ⚠️ Code exists but NOT integrated — **requires wiring work**

**What exists** (standalone, untouched):
- `pkg/detector/context.go`: `ContextAnalyzer` with `Analyze(messages []string) ContextResult`
- `FulfilledCommitment` and `EscalatedCommitment` types
- Fulfillment: matches commitment verb → past tense in later message
- Escalation: ≥2 unfulfilled commitments of same category → escalated (0.95 for 2, 1.0 for 3+)
- 8 context tests all passing

**What's missing** (the actual integration):
- `V2API` has no message history — each `handleAnalyze` call is stateless
- `ContextAnalyzer` is never instantiated outside tests
- No API endpoint exposes context analysis results
- Fulfilled commitments from context analysis don't trigger bead auto-resolution
- Escalated commitments don't affect confidence or trigger alerts

**Acceptance Criteria**:
1. V2API maintains an in-memory per-session message buffer (last N messages, configurable, default 5)
2. On every `POST /api/v2/analyze` with role=assistant, the message is appended to the session buffer
3. After commitment detection AND after auto-resolve, run `ContextAnalyzer.Analyze(buffer)` on the session's message window
4. If ContextAnalyzer finds fulfilled commitments: close matching open oathkeeper beads for that session (same as AutoResolve but using verb-matching logic)
5. If ContextAnalyzer finds escalated commitments: include escalation info in the analyze response
6. The AnalyzeResponse gains optional fields: `"fulfilled": [{"text": "...", "fulfilled_by": "..."}]` and `"escalated": [{"category": "...", "count": N}]`
7. Message buffer is bounded: when buffer exceeds window size, oldest messages are dropped
8. Buffer is per session_key — different sessions don't share context
9. Thread-safe: concurrent requests to different sessions don't interfere

**Implementation plan**:

```
File: pkg/api/v2.go

Add to V2API struct:
  mu       sync.Mutex
  sessions map[string]*sessionBuffer

type sessionBuffer struct {
  messages []string
}

In handleAnalyze, after role check:
  1. Append message to session buffer (under lock)
  2. After detectCommitment + autoResolve logic, run ContextAnalyzer
  3. For each fulfilled: attempt to close matching beads
  4. For each escalated: include in response
  5. Trim buffer to window size

New constructor parameter or setter:
  contextWindowSize int  (default 5, from config)
```

**File paths**:
- `pkg/api/v2.go` — add session buffer, integrate ContextAnalyzer
- `pkg/api/v2_test.go` — new tests for context integration
- `pkg/detector/context.go` — no changes needed (already complete)

**Test strategy**:
- Unit test: POST 3 messages to same session (commitment → unrelated → fulfillment), verify fulfilled in response
- Unit test: POST 2 commitments of same type to same session, verify escalated in response
- Unit test: POST to different sessions, verify no cross-contamination
- Unit test: Verify buffer trimming when > window size
- Concurrency test: 3 goroutines posting to same session simultaneously, verify no race (run with `-race`)
- Negative test: POST non-assistant messages don't affect buffer

**Dependencies**: US-004 (handleAnalyze), US-007 (pattern detection)

**Verification**:
```bash
/usr/local/go/bin/go test ./pkg/api/... -count=1 -race -v
/usr/local/go/bin/go test ./pkg/detector/... -count=1 -v
```

---

### US-009: CLI entry point

**Status**: ✅ Complete — verify only

**What exists**:
- `cmd/oathkeeper/main.go`: subcommand routing for serve, scan, list, stats, resolve, doctor
- `extractConfigFlag` for `--config PATH` parsing
- `--help`, `--version` handled
- Tests: `TestUsageContainsAllSubcommands`, `TestExtractConfigFlag`, `TestVersionConstDefined`, `TestLoadConfigDefaultPath`

**Acceptance Criteria**:
1. `oathkeeper serve` starts HTTP server + daemon
2. `oathkeeper scan <file>` batch scans transcript (uses `pkg/scanner`)
3. `oathkeeper list` lists open oathkeeper beads (table format)
4. `oathkeeper stats` shows commitment statistics (JSON)
5. `oathkeeper resolve <bead-id> [reason]` resolves a commitment
6. `oathkeeper doctor` runs health checks on all dependencies
7. `--config PATH` overrides default config path
8. `--help` / `--version` work as top-level flags
9. Unknown commands print usage to stderr and exit 1
10. Missing required args (e.g., `scan` without file) print usage and exit 1

**File paths**: `cmd/oathkeeper/main.go`, `cmd/oathkeeper/main_test.go`

**Test strategy**: Unit tests for routing, flag parsing, config loading (already passing)

**Dependencies**: US-001 (BeadStore), US-004/005 (V2API)

**Verification**:
```bash
/usr/local/go/bin/go test ./cmd/oathkeeper/... -count=1 -v
```

---

### US-010: Wire serve command

**Status**: ⚠️ Mostly complete — needs ContextAnalyzer wiring and resolved webhook

**What exists**:
- `cmd/oathkeeper/serve.go`: wires config → BeadStore → Detector → Verifier → GracePeriod → V2API → Daemon
- Grace callback creates bead for unbacked commitments + fires NotifyUnbacked webhook
- Graceful shutdown with `gp.Stop()` in OnStop
- Tests: `TestServeStartsAndRespondsToHealth`, `TestServeGracefulShutdown`, `TestServeWiresV2APIRoutes`

**What's missing**:
- ContextAnalyzer not instantiated or wired (depends on US-008 integration)
- Resolved webhook: when a bead is auto-resolved or manually resolved, `webhook.NotifyResolved()` is never called
- Config doesn't expose `context_window_size` setting

**Acceptance Criteria**:
1. `startServer` wires: config → BeadStore → Detector → ContextAnalyzer → Verifier → GracePeriod → V2API → Webhook → Daemon
2. V2API receives a `contextWindowSize` parameter derived from config (default 5)
3. When a bead is resolved (via API or auto-resolve), `webhook.NotifyResolved(beadID, evidence)` fires
4. The resolved webhook is wired through V2API — V2API gets a `onResolve` callback
5. Grace callback on unbacked: creates bead + fires NotifyUnbacked (already done)
6. Server addr is configurable in config (currently hardcoded `:9876`)
7. Graceful shutdown: stops grace period, closes HTTP server, calls OnStop

**Implementation plan**:

```
File: cmd/oathkeeper/serve.go

Changes:
1. Add to config: [general] context_window_size = 5, [server] addr = ":9876"
2. Create ContextAnalyzer: context.NewContextAnalyzer(cfg.General.ContextWindowSize)
3. Pass to V2API: v2.SetContextAnalyzer(contextAnalyzer)  — or via constructor
4. Add resolve callback to V2API:
   v2.SetResolveCallback(func(beadID, evidence string) {
     if webhook != nil {
       webhook.NotifyResolved(beadID, evidence)
     }
   })
5. V2API calls the resolve callback from handleResolveCommitment and AutoResolve

File: pkg/config/config.go

Add to GeneralConfig:
  ContextWindowSize int `toml:"context_window_size"`

Add to DefaultConfig():
  ContextWindowSize: 5

Add new section or field for server addr:
  type ServerConfig struct {
    Addr string `toml:"addr"`
  }
```

**File paths**:
- `cmd/oathkeeper/serve.go` — wire ContextAnalyzer, resolve callback
- `cmd/oathkeeper/serve_test.go` — test resolved webhook fires
- `pkg/config/config.go` — add ContextWindowSize, ServerAddr
- `pkg/config/config_test.go` — test new config fields
- `pkg/api/v2.go` — add SetResolveCallback, call it on resolve

**Test strategy**:
- Unit test: resolve via API triggers resolve callback
- Unit test: auto-resolve triggers resolve callback for each resolved bead
- Unit test: config loads context_window_size with default fallback
- Integration-style: start server, resolve a commitment, verify callback was called

**Dependencies**: US-008 (ContextAnalyzer integration), US-012 (webhook)

**Verification**:
```bash
/usr/local/go/bin/go test ./cmd/oathkeeper/... -count=1 -v
/usr/local/go/bin/go test ./pkg/api/... -count=1 -v
/usr/local/go/bin/go test ./pkg/config/... -count=1 -v
```

---

### US-011: Integration test — full lifecycle

**Status**: ⚠️ Tests exist but are shallow — **need real bead lifecycle verification**

**What exists**:
- `pkg/integration_test.go`: `TestIntegrationFullLifecycle` and `TestIntegrationConcurrentCommitments`
- Both require `br` in PATH (skip otherwise)
- `startTestServer` helper spins up a test server with real detector and short grace period
- `freePort` helper finds available TCP port

**What's missing**:
- Full lifecycle doesn't verify bead creation after grace period expiry
- Full lifecycle doesn't test POST resolve → verify bead closed
- Concurrent test uses nil beadStore — doesn't test real bead operations under load
- No `-race` flag in the test command (PRD requires it)
- Grace callback in test server doesn't create beads (only detects → no bead store wired for writes)

**Acceptance Criteria**:
1. **Full lifecycle test** (requires `br`):
   - Start test server with real BeadStore (isolated test DB)
   - POST `/api/v2/analyze` with temporal commitment (role=assistant)
   - Assert response: `commitment: true, category: temporal`
   - Wait for grace period to expire (100ms in test)
   - Grace callback creates a bead via BeadStore
   - GET `/api/v2/commitments?status=open` — verify the new bead appears
   - POST `/api/v2/commitments/{id}/resolve` with reason
   - GET `/api/v2/commitments/{id}` — verify status=closed, close_reason matches
   - GET `/api/v2/stats` — verify resolved count increased
2. **Concurrent test** (requires `br`):
   - 5 goroutines each POST a different commitment to the same server
   - All return 200 with `commitment: true`
   - No data races detected with `-race`
   - If beadStore is wired: verify created beads are distinct
3. **No external dependencies beyond `br`**: test creates its own isolated bead database using the `newTestBeadStore` pattern from `beads_test.go`

**Implementation plan**:

```
File: pkg/integration_test.go

Key changes:
1. Create an isolated BeadStore using br-wrapper.sh (same pattern as beads_test.go)
2. Wire a graceCallback into the test server that actually creates beads:
   v2.SetGraceCallback(func(commitmentID, message, category string, outcome) {
     if !outcome.IsBacked {
       beadStore.Create(CommitmentInfo{...})
     }
   })
3. After posting commitment and waiting for grace: query beadStore.List to find created bead
4. Resolve the bead via API, then verify closure via beadStore.Get
5. For concurrent test: use the real BeadStore, verify 5 beads created
6. Use t.Cleanup to close/clean up beads
```

**File paths**:
- `pkg/integration_test.go` — rewrite lifecycle and concurrent tests

**Test strategy**: These ARE the tests. Run with:
```bash
/usr/local/go/bin/go test ./pkg/ -run Integration -count=1 -race -timeout 30s
```

**Dependencies**: US-004 (analyze API), US-005 (commitments API), US-010 (serve wiring)

**Verification**:
```bash
/usr/local/go/bin/go test ./pkg/ -run Integration -count=1 -race -v
```

---

### US-012: Webhook on violation

**Status**: ⚠️ Code complete, partially wired — **needs resolved webhook integration**

**What exists**:
- `pkg/hooks/webhook.go`: `Webhook` with `NotifyUnbacked` and `NotifyResolved`
- Exponential backoff retry: 1s, 2s, 4s (max 3 attempts)
- 4xx errors are not retried (only 5xx)
- Tests: 5 webhook tests all passing (unbacked, resolved, retry, give up, no-retry-on-4xx)
- Wired in `serve.go`: grace callback calls `NotifyUnbacked` for unbacked commitments

**What's missing**:
- `NotifyResolved` is never called from the live flow
- When `AutoResolve` closes beads in `handleAnalyze`, no webhook fires
- When `POST /api/v2/commitments/:id/resolve` closes a bead, no webhook fires

**Acceptance Criteria**:
1. When grace period expires and commitment is unbacked: POST `{"event": "commitment.unbacked", "bead_id": "...", "text": "...", "category": "..."}` to webhook URL ✅ (already done)
2. When bead is auto-resolved via `AutoResolve` during analyze: POST `{"event": "commitment.resolved", "bead_id": "...", "evidence": "..."}` to webhook URL
3. When bead is manually resolved via `POST /:id/resolve`: POST `{"event": "commitment.resolved", "bead_id": "...", "evidence": "..."}` to webhook URL
4. Retry on 5xx with exponential backoff (1s, 2s, 4s), max 3 attempts ✅ (already done)
5. No retry on 4xx ✅ (already done)
6. Webhook URL is optional — if empty/not configured, no webhook fires
7. Webhook failures don't block the API response (fire-and-forget, log errors)

**Implementation plan**:

```
File: pkg/api/v2.go

Add to V2API:
  onResolve func(beadID string, evidence string)

Add setter:
  func (v2 *V2API) SetResolveCallback(fn func(beadID, evidence string))

In handleResolveCommitment, after successful resolveBead:
  if v2.onResolve != nil {
    go v2.onResolve(beadID, req.Reason)
  }

In handleAnalyze, after successful autoResolve:
  if v2.onResolve != nil {
    for _, resolvedID := range resolved {
      go v2.onResolve(resolvedID, req.Message)
    }
  }

File: cmd/oathkeeper/serve.go

Wire the callback:
  v2.SetResolveCallback(func(beadID, evidence string) {
    if webhook != nil {
      if err := webhook.NotifyResolved(beadID, evidence); err != nil {
        log.Printf("resolved webhook failed for %s: %v", beadID, err)
      }
    }
  })
```

**File paths**:
- `pkg/api/v2.go` — add onResolve callback + invoke in resolve + auto-resolve paths
- `pkg/api/v2_test.go` — test callback fires on manual resolve and auto-resolve
- `cmd/oathkeeper/serve.go` — wire webhook.NotifyResolved into onResolve
- `pkg/hooks/webhook.go` — no changes needed
- `pkg/hooks/webhook_test.go` — no changes needed

**Test strategy**:
- Unit test: manual resolve via `POST /:id/resolve` triggers onResolve callback with correct beadID and reason
- Unit test: auto-resolve in handleAnalyze triggers onResolve for each resolved bead ID
- Unit test: onResolve=nil doesn't panic
- Integration test (in US-011): resolve a bead, verify webhook received the event

**Dependencies**: US-005 (resolve API), US-010 (serve wiring)

**Verification**:
```bash
/usr/local/go/bin/go test ./pkg/api/... -count=1 -v
/usr/local/go/bin/go test ./pkg/hooks/... -count=1 -v
```

---

### US-013: Health and readiness

**Status**: ✅ Complete — verify only

**What exists**:
- `pkg/api/health.go`: `HealthHandler` (GET /healthz → 200 + `{"status":"ok"}`) and `ReadinessHandler` (GET /readyz → runs `br version`)
- Readiness returns 503 with `{"status":"not ready","error":"..."}` when `br` is missing
- 3-second timeout on readiness check
- Tests: `TestHealthzReturnsOK`, `TestHealthzRejectsNonGet`, `TestReadyzWhenBRAvailable`, `TestReadyzFailsWhenBRMissing`, `TestReadyzRejectsNonGet`
- Wired in `serve.go`: `/healthz` and `/readyz` routes registered

**Acceptance Criteria**:
1. `GET /healthz` → 200 `{"status":"ok"}` (always, if process is alive)
2. `GET /readyz` → 200 `{"status":"ready"}` if `br version` succeeds
3. `GET /readyz` → 503 `{"status":"not ready","error":"..."}` if `br` is missing or fails
4. Non-GET methods → 405 on both endpoints
5. Readiness check has 3-second timeout (doesn't hang if br is slow)

**File paths**: `pkg/api/health.go`, `pkg/api/health_test.go`

**Test strategy**: Unit tests with real `br` (skip if unavailable) and fake missing binary (already passing)

**Dependencies**: None

**Verification**:
```bash
/usr/local/go/bin/go test ./pkg/api/... -run Health -count=1 -v
/usr/local/go/bin/go test ./pkg/api/... -run Ready -count=1 -v
```

---

## Implementation Order

The stories have real dependencies. Implement in this order:

```
Phase 1 (verify-only, no code changes):
  US-006 ──→ verify tests pass
  US-007 ──→ verify tests pass
  US-009 ──→ verify tests pass
  US-013 ──→ verify tests pass

Phase 2 (requires code changes):
  US-012 ──→ add onResolve callback to V2API + wire in serve.go
  US-008 ──→ integrate ContextAnalyzer into V2API (depends on US-012 for resolved webhook)
  US-010 ──→ wire ContextAnalyzer + config additions (depends on US-008, US-012)

Phase 3 (integration, last):
  US-011 ──→ rewrite integration tests with real bead lifecycle (depends on all above)
```

Stories US-006, US-007, US-009, US-013 need **zero code changes** — just run their tests
to confirm they pass. The real work is US-008 + US-012 + US-010 + US-011.

## Dependency Graph

```
US-006 (stats)     ─── standalone, done
US-007 (patterns)  ─── standalone, done
US-009 (CLI)       ─── standalone, done
US-013 (health)    ─── standalone, done

US-012 (webhook)   ─── add resolve callback to V2API
        ↓
US-008 (context)   ─── integrate ContextAnalyzer into V2API
        ↓
US-010 (wiring)    ─── wire ContextAnalyzer + resolve webhook in serve.go
        ↓
US-011 (integ.)    ─── rewrite tests with full bead lifecycle
```

## Constraints (inherited from v2 PRD)

- Go stdlib only for HTTP. No gin/echo/chi.
- `br` CLI is a hard dependency, not optional.
- BurntSushi/toml for config.
- All existing tests must continue passing. Zero regressions.
- New tests: real integrations where possible, `t.Skip` when external deps unavailable.
- TDD: test first, implement second.
- Run with `-race` for anything touching shared state.

## Global Verification

After all stories are complete:
```bash
/usr/local/go/bin/go build ./...
/usr/local/go/bin/go test ./... -count=1 -race
/usr/local/go/bin/go vet ./...
```

All three must pass with zero failures.

## Architecture Notes

### Session Buffer Design (US-008)

The session buffer for ContextAnalyzer should be a simple bounded slice per session key.
Don't over-engineer this — it's an in-memory cache, not persistent state. If the server
restarts, the buffer resets. That's fine.

```go
type sessionBuffer struct {
    messages []string
}

// In V2API
sessions map[string]*sessionBuffer  // protected by sync.Mutex
```

Use `sync.Mutex` (not `sync.RWMutex`) since every analyze call writes to the buffer.
The lock scope should be narrow: lock → append message → copy buffer → unlock → analyze.
Don't hold the lock during ContextAnalyzer.Analyze() — it's CPU-only, no I/O.

### Resolve Callback Design (US-012)

The `onResolve` callback should be fire-and-forget from the API handler's perspective.
Use `go v2.onResolve(...)` so the webhook retry logic doesn't block the HTTP response.
The webhook itself handles retries internally.

### Integration Test Isolation (US-011)

Use the same `br-wrapper.sh` pattern from `pkg/beads/beads_test.go` to create an
isolated bead database per test. This avoids polluting the real bead database and
makes tests repeatable. Each test gets its own `t.TempDir()` with a fresh `beads.db`.

The test server's grace callback must wire through to `beadStore.Create()` — the
current test server doesn't do this, which is why the lifecycle test is shallow.
