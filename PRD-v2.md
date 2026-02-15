# Oathkeeper v2 PRD — Live Commitment Guard

## Vision

Oathkeeper v2 transforms from a batch transcript scanner into a **live commitment guard**. It analyzes agent messages in real-time, maintains a persistent commitment registry, and exposes an HTTP API for integration with OpenClaw and athena-web.

## Architecture

```
Agent reply → HTTP POST /api/analyze → Detector + Verifier → Response
                                              ↓
                                    SQLite commitment store
                                              ↓
                              HTTP GET /api/commitments → Dashboard
```

## Existing Code to Preserve

The v1 codebase is solid. Preserve and extend:
- `pkg/detector/` — commitment detection (temporal, conditional, speculative, untracked)
- `pkg/verifier/` — backing mechanism verification
- `pkg/storage/` — SQLite storage
- `pkg/config/` — TOML config
- `pkg/beadtracker/` — bead creation for unresolved commitments
- `pkg/alerts/` — alert sending
- All existing tests must continue to pass.

## Tasks

### Phase 1: Commitment Registry (SQLite)

**Task 1: Define Commitment model and migration**
- File: `pkg/registry/model.go`
- Commitment struct: ID (uuid), SessionKey, Text, Category, Confidence, BackingMechanism (nullable), Status (open|backed|resolved|expired|violated), DetectedAt, ResolvedAt, ExpiresAt, BeadID (nullable)
- Status enum with valid transitions: open→backed, open→resolved, open→expired, open→violated, backed→resolved, backed→expired
- Test: model validation, status transitions
- `go test ./pkg/registry/...`

**Task 2: Registry storage layer**
- File: `pkg/registry/store.go`
- SQLite table `commitments` with all model fields
- CRUD: Create, GetByID, List (with filters: status, session, category), Update, Count
- Filter struct: Status []string, SessionKey string, Category string, Since time.Time, Limit int
- Auto-migrate on open (CREATE TABLE IF NOT EXISTS)
- Test: full CRUD cycle, filtering, concurrent access
- `go test ./pkg/registry/...`

**Task 3: Expiry worker**
- File: `pkg/registry/expiry.go`
- Background goroutine that periodically checks open commitments past ExpiresAt
- Transitions them to `expired` status
- Calls BeadTracker.Create for expired commitments without a BeadID
- Test: expiry transitions, bead creation on expiry
- `go test ./pkg/registry/...`

### Phase 2: Live Analysis API

**Task 4: Analyze endpoint**
- File: `pkg/api/v2.go`
- `POST /api/v2/analyze` accepts JSON: `{"session_key": "...", "message": "...", "role": "assistant"}`
- Runs Detector.DetectCommitment on message
- If commitment detected: runs Verifier, stores in registry, returns `{"commitment": true, "category": "...", "confidence": 0.95, "backed": false, "id": "..."}`
- If no commitment: returns `{"commitment": false}`
- Only analyzes role=assistant messages
- Test: commitment detected and stored, non-commitment passthrough, role filtering
- `go test ./pkg/api/...`

**Task 5: Commitments list endpoint**
- File: `pkg/api/v2.go` (extend)
- `GET /api/v2/commitments` with query params: status, session_key, category, limit
- Returns JSON array of commitments from registry
- `GET /api/v2/commitments/:id` returns single commitment
- `PATCH /api/v2/commitments/:id` allows manual resolution (status=resolved)
- Test: list filtering, single fetch, manual resolution
- `go test ./pkg/api/...`

**Task 6: Stats endpoint**
- File: `pkg/api/v2.go` (extend)
- `GET /api/v2/stats` returns: total, open, backed, resolved, expired, violated counts
- Per-category breakdown
- Test: stats accuracy after various operations
- `go test ./pkg/api/...`

### Phase 3: Enhanced Detection

**Task 7: Expand commitment patterns**
- File: `pkg/detector/detector.go`
- Add patterns for:
  - "I'll monitor/watch/keep an eye on" → followup category
  - "I'll report back/update you/let you know" → followup category
  - "after X finishes, I'll Y" → conditional category (already partial)
  - "I need to X" / "I should X" → weak commitment, lower confidence (0.70)
  - "TODO" / "FIXME" / "HACK" markers in code context → untracked category
- Test: each new pattern with positive and negative cases
- `go test ./pkg/detector/...`

**Task 8: Message context window**
- File: `pkg/detector/context.go`
- ContextAnalyzer that takes last N messages (not just one)
- Detects fulfilled commitments: if "I'll check X" is followed by "I checked X, here's the result" → auto-resolve
- Detects repeated commitments (same promise made twice = escalate confidence)
- Test: fulfillment detection, duplicate detection
- `go test ./pkg/detector/...`

### Phase 4: Integration Hooks

**Task 9: OpenClaw webhook format**
- File: `pkg/hooks/webhook.go`
- Webhook client that POSTs to a configurable URL when:
  - New commitment detected (with no backing)
  - Commitment expired
  - Commitment violated (expired + no resolution)
- Payload: `{"event": "commitment.unbacked", "commitment": {...}}`
- Retry with backoff (max 3 attempts)
- Test: webhook fires on events, retry on failure, respects backoff
- `go test ./pkg/hooks/...`

**Task 10: Bead auto-creation for violations**
- File: `pkg/beadtracker/v2.go`
- When commitment transitions to `violated`:
  - Create bead via `br create --title "Violated: <text>" --priority 2 --tag oathkeeper`
  - Update commitment with BeadID
- When commitment transitions to `expired` (no bead yet):
  - Create bead via `br create --title "Expired: <text>" --priority 3 --tag oathkeeper`
- Test: bead creation on violation, bead creation on expiry, idempotency
- `go test ./pkg/beadtracker/...`

### Phase 5: CLI + Daemon

**Task 11: CLI commands**
- File: `cmd/oathkeeper/main.go` (create if missing)
- Subcommands:
  - `oathkeeper serve` — start HTTP server (uses existing daemon pkg)
  - `oathkeeper scan <file>` — batch scan (existing functionality, preserved)
  - `oathkeeper list` — list open commitments (calls API)
  - `oathkeeper stats` — show stats (calls API)
  - `oathkeeper resolve <id>` — manually resolve a commitment
- Use stdlib `flag` package only (no external CLI deps)
- Test: CLI argument parsing, subcommand routing
- `go test ./cmd/oathkeeper/...`

**Task 12: Config v2**
- File: `pkg/config/config.go` (extend)
- New config fields:
  - `[server]` section: port, bind address
  - `[registry]` section: db_path, expiry_check_interval
  - `[webhooks]` section: urls (array), enabled
  - `[detection]` section: context_window_size, min_confidence
- Backward-compatible: v1 config still works
- Test: v1 config loads, v2 config loads, defaults applied
- `go test ./pkg/config/...`

### Phase 6: Hardening

**Task 13: Graceful shutdown**
- File: `pkg/daemon/daemon.go` (extend)
- On SIGINT/SIGTERM: stop accepting new requests, finish in-flight, close DB, exit
- Drain timeout configurable (default 5s)
- Test: shutdown completes in-flight requests, DB closes cleanly
- `go test ./pkg/daemon/...`

**Task 14: Health and readiness endpoints**
- File: `pkg/api/health.go`
- `GET /healthz` — returns 200 if process alive
- `GET /readyz` — returns 200 if DB is accessible and writable
- Test: health OK, readiness fails when DB is unavailable
- `go test ./pkg/api/...`

**Task 15: Integration test suite**
- File: `pkg/integration_test.go`
- End-to-end test: start server → POST analyze with commitment → GET commitments → verify stored → wait for expiry → verify bead created
- Test with concurrent requests (10 goroutines)
- Test: full lifecycle, concurrency safety
- `go test ./pkg/...`

## Constraints

- Go only. No external HTTP frameworks (stdlib net/http + modernc.org/sqlite only).
- All existing v1 tests must pass throughout.
- TDD: write test first, then implementation. Every task has a test command.
- No `cmd/` binary required to exist for tests — all logic in `pkg/`.
- SQLite for storage (already a dependency).
- Config via TOML (already a dependency via BurntSushi/toml).

## Verification

```bash
go build ./...
go test ./... -count=1
go vet ./...
```

All three must pass with zero failures.
