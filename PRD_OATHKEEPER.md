# Oathkeeper v2 PRD — Beads-Native Live Commitment Guard

## Vision

Oathkeeper v2 uses **beads as the commitment store** — not a separate SQLite database. Every detected commitment becomes a bead. Resolution closes the bead. The bead system IS the registry.

The live analysis API lets OpenClaw POST every assistant message for real-time commitment detection.

## Principles

- **No mocks in production code.** No mock interfaces, no stub implementations, no "fallback" modes that silently degrade.
- **No SQLite commitment registry.** Beads are the single source of truth. The existing `pkg/storage` becomes unnecessary for commitment tracking.
- **Real integration tests.** Tests that exercise actual `br` CLI (or skip if unavailable). No fake shell scripts pretending to be `br`.
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

## What Exists (preserve)

- `pkg/detector/` — solid pattern matching, keep and extend
- `pkg/verifier/` — backing mechanism checks, keep
- `pkg/grace/` — grace period scheduler, keep
- `pkg/alerts/` — wake + telegram alerting, keep
- `pkg/config/` — TOML config, keep and extend
- `pkg/daemon/` — lifecycle management, keep
- `pkg/scanner/` — batch file scanning, keep
- `pkg/expiry/` — time extraction, keep
- `pkg/formatter/` — display formatting, keep
- `pkg/doctor/` — dependency health checks, keep

All existing tests must continue passing.

## Tasks

### Phase 1: Beads-Native Store

- [x] **US-001** Task 1: Bead-backed commitment tracker
- File: `pkg/beads/beads.go`, `pkg/beads/beads_test.go`
- New package `pkg/beads` (replace `pkg/beadtracker` which is too limited)
- `BeadStore` struct wraps `br` CLI calls for commitment lifecycle:
  - `Create(commitment CommitmentInfo) (string, error)` — runs `br create --title "oathkeeper: <text>" --priority 2 --tag oathkeeper --tag <category>` returns bead ID
  - `Close(beadID string, reason string) error` — runs `br close <beadID>` with reason
  - `List(filter Filter) ([]Bead, error)` — runs `br list --tag oathkeeper --format json` and parses output
  - `Get(beadID string) (Bead, error)` — runs `br show <beadID> --format json`
- CommitmentInfo: Text, Category, SessionKey, DetectedAt, ExpiresAt
- Filter: Status (open/closed), Category, Since
- Bead struct: ID, Title, Status, Tags, CreatedAt, ClosedAt
- **No fallback mode.** If `br` is not in PATH, return error.
- Test: use real `br` if available, skip with `t.Skip("br not in PATH")` if not. No mock scripts.
- Verify: `go test ./pkg/beads/...`

- [x] **US-002** Task 2: Remove SQLite commitment storage dependency
- File: `pkg/storage/storage.go`
- Keep pkg/storage but add a deprecation comment at package level: `// Deprecated: Use pkg/beads for commitment tracking. This package remains for migration.`
- Remove `FallbackEnabled` from config. Remove any code path that silently falls back to local storage when beads are unavailable.
- File: `pkg/config/config.go` — remove `fallback_enabled` field from LLM config section
- Test: config still loads without `fallback_enabled`, no panic
- Verify: `go test ./pkg/config/... ./pkg/storage/...`

- [x] **US-003** Task 3: Commitment resolution via bead close
- File: `pkg/beads/resolve.go`, `pkg/beads/resolve_test.go`
- `Resolve(beadID string, evidence string) error` — closes the bead with evidence as the close reason
- `AutoResolve(sessionKey string, message string) ([]string, error)` — scans message for resolution indicators ("I checked X", "done", "completed", "here are the results") and closes matching open oathkeeper beads for that session
- Returns list of resolved bead IDs
- Test: resolution with evidence, auto-resolve detection, no false auto-resolve on unrelated messages
- Verify: `go test ./pkg/beads/...`

### Phase 2: Live Analysis API

- [x] **US-004** Task 4: POST /api/v2/analyze endpoint
- File: `pkg/api/v2.go`, `pkg/api/v2_test.go`
- Accepts JSON: `{"session_key": "main", "message": "I'll check on that in 10 minutes", "role": "assistant"}`
- Only processes role=assistant
- Runs Detector.DetectCommitment
- If commitment: starts grace period timer, returns `{"commitment": true, "category": "temporal", "confidence": 0.95, "text": "..."}`
- If no commitment: runs AutoResolve to check if message resolves existing commitments, returns `{"commitment": false, "resolved": ["bd-xxx"]}`
- No commitment: returns `{"commitment": false, "resolved": []}`
- Test: POST with commitment, POST with non-commitment, POST with resolution, role filtering, invalid JSON
- Verify: `go test ./pkg/api/...`

- [x] **US-005** Task 5: GET /api/v2/commitments endpoint
- File: `pkg/api/v2.go` (extend)
- Queries `BeadStore.List` with filters from query params: `?status=open&category=temporal`
- Returns JSON array of beads tagged `oathkeeper`
- `GET /api/v2/commitments/:id` — single bead detail
- `POST /api/v2/commitments/:id/resolve` — manual resolution with `{"reason": "..."}`
- Test: list open commitments, filter by category, resolve via API, 404 on unknown ID
- Verify: `go test ./pkg/api/...`

- [x] **US-006** Task 6: GET /api/v2/stats endpoint
- File: `pkg/api/v2.go` (extend)
- Returns: `{"total": N, "open": N, "resolved": N, "by_category": {"temporal": N, "conditional": N, ...}}`
- Computed from `BeadStore.List` (all oathkeeper-tagged beads)
- Test: stats with mixed states, empty stats
- Verify: `go test ./pkg/api/...`

### Phase 3: Enhanced Detection

- [x] **US-007** Task 7: Expand commitment patterns
- File: `pkg/detector/detector.go`, `pkg/detector/detector_test.go`
- Add patterns:
  - "I'll monitor/watch/keep an eye on" → followup
  - "I'll report back/update you/let you know" → followup  
  - "I need to X" / "I should X" → weak commitment, confidence 0.70
  - "TODO" / "FIXME" / "HACK" in code context → untracked
- Each pattern must have at least 3 positive and 2 negative test cases
- Test: new patterns don't break existing 64 tests
- Verify: `go test ./pkg/detector/...`

- [x] **US-008** Task 8: Context-aware auto-resolution
- File: `pkg/detector/context.go`, `pkg/detector/context_test.go`
- `ContextAnalyzer` takes last N messages (configurable, default 5)
- Detects fulfilled commitments: "I'll check X" followed by "I checked X" → marks as fulfilled
- Detects repeated promises: same commitment type made twice → escalate confidence
- Returns list of resolved commitment texts and list of escalated commitments
- Test: fulfillment across messages, repetition detection, no false positives on unrelated messages
- Verify: `go test ./pkg/detector/...`

### Phase 4: CLI

- [x] **US-009** Task 9: CLI entry point
- File: `cmd/oathkeeper/main.go`
- Subcommands using stdlib `flag` only:
  - `oathkeeper serve` — start HTTP server + daemon
  - `oathkeeper scan <file>` — batch scan transcript (existing scanner pkg)
  - `oathkeeper list` — list open oathkeeper beads (calls BeadStore.List)
  - `oathkeeper stats` — show commitment stats
  - `oathkeeper resolve <bead-id> [reason]` — resolve a commitment
  - `oathkeeper doctor` — run health checks
- Config loaded from `~/.config/oathkeeper/oathkeeper.toml` or `--config` flag
- Test: subcommand routing, help text, missing required args
- Verify: `go test ./cmd/oathkeeper/...`

- [x] **US-010** Task 10: Wire serve command
- File: `cmd/oathkeeper/serve.go`
- Wires together: config → BeadStore → Detector → Verifier → GracePeriod → API server → Daemon
- Registers v2 API routes
- Starts grace period expiry checker as background goroutine  
- Graceful shutdown closes all components
- Test: server starts and responds to health check, graceful shutdown
- Verify: `go test ./cmd/oathkeeper/...`

### Phase 5: Hardening

- [x] **US-011** Task 11: Integration test — full lifecycle
- File: `pkg/integration_test.go`
- Requires `br` in PATH (skip otherwise)
- Full cycle: start server → POST analyze with commitment → wait grace period → verify bead created → POST resolve → verify bead closed
- Concurrent test: 5 goroutines posting commitments simultaneously
- Test: lifecycle correctness, no races under concurrency
- Verify: `go test ./pkg/ -run Integration -count=1 -race`

- [x] **US-012** Task 12: Webhook on violation
- File: `pkg/hooks/webhook.go`, `pkg/hooks/webhook_test.go`
- When grace period expires and commitment is unbacked:
  - POST to configurable webhook URL with `{"event": "commitment.unbacked", "bead_id": "...", "text": "...", "category": "..."}`
  - Retry with exponential backoff (max 3 attempts, 1s/2s/4s)
- When bead auto-resolved:
  - POST with `{"event": "commitment.resolved", "bead_id": "...", "evidence": "..."}`  
- Test: webhook fires, retry on 500, gives up after 3 attempts, resolved event
- Verify: `go test ./pkg/hooks/...`

- [ ] **US-013** Task 13: Health and readiness
- File: `pkg/api/health.go`, `pkg/api/health_test.go`
- `GET /healthz` → 200 if alive
- `GET /readyz` → 200 if `br` is accessible (runs `br version`)
- Test: health OK, readiness when br available, readiness fails when br missing
- Verify: `go test ./pkg/api/...`

## Constraints

- Go stdlib only for HTTP. No gin/echo/chi.
- modernc.org/sqlite stays as dep but NOT used for commitment storage.
- BurntSushi/toml for config.
- `br` CLI is a hard dependency, not optional.
- All existing tests must pass. New tests must use real integrations where possible, `t.Skip` when external deps unavailable.
- TDD: test first, implement second.

## Verification

```bash
/usr/local/go/bin/go build ./...
/usr/local/go/bin/go test ./... -count=1
/usr/local/go/bin/go vet ./...
```

All three must pass with zero failures.
