# Oathkeeper — Horkos (Ὅρκος)

![Oathkeeper Banner](banner.png)

*Standing in the Styx. Ledger-bandolier across his chest. The brand glows. He checks the receipts.*

---

Oathkeeper watches what agents promise and whether they follow through. When an AI agent says "I will do X," Oathkeeper writes it down, waits, checks whether anything actually happened, and if not, creates a visible tracking record. It is not a punishment system. It is an accountability system — the difference matters. Built in Go, it runs as a CLI tool and an HTTP daemon, and it integrates with the rest of the Polis agent infrastructure.

---

The Greeks swore their most terrible oaths by the River Styx. Break one and even the gods suffered — nine years of silence, cast out from Olympus. The punishment wasn't death. It was accountability.

Horkos stands in the river. Always in the river, water up to his shins. The cold is permanent — he does not react to it. His cloak is the color of the Styx itself: deep shifting dark blue, dark green, near-black. Long, immediately recognizable. His one piece of drama.

The ledger-bandolier hangs across his chest — active records of oaths made, not yet fulfilled, not yet broken. Heavy, because the city runs on commitments. The binding chain connects agents to their commitments in the Styx. Not restrictive — the agent moves freely. The chain simply exists so Horkos knows where to look.

When an agent says "I will," Horkos writes it down. When the grace period expires and the promise is still floating, he reaches into the water and pulls it out. The Styx brand on his palm glows when pressed onto the record — verification before branding, care not ceremony. Then the glow fades to a permanent mark. Unfulfilled oaths pulled from the water become beads — visible, tracked. He doesn't punish broken oaths. He just makes sure everyone knows about them.

Oathkeeper tracks agent commitments and enforces follow-through as part of the Polis accountability system.

---

## Current Status

| Area | Status | Notes |
|------|--------|-------|
| Core CLI (`serve`, `scan`, `list`, `stats`, `resolve`, `doctor`) | ✅ Working | All six subcommands implemented and tested |
| HTTP API (v2) | ✅ Working | Health, readiness, analyze, commitments CRUD, stats |
| Commitment detection (regex + confidence) | ✅ Working | Categories: temporal, scheduled, followup, conditional, and more |
| Grace period + deferred verification | ✅ Working | Configurable, with duplicate cancellation |
| Beads integration (`br`) | ✅ Working | Create, list, resolve via beads backend |
| Relay integration | ✅ Working | Publishes `commitment.unbacked` and `commitment.resolved` events |
| Transcript polling | ✅ Working | Auto-tails transcript files when `monitor_transcripts` is enabled |
| Session context (fulfillment detection) | ✅ Working | Rolling buffer detects past-tense completion, auto-resolves beads |
| Stats dashboard (console + HTML + CSV/JSON export) | ✅ Working | `--dashboard`, `--export`, console bar charts |
| Legacy `watch` / `check` naming | ⚠️ Retired | Live CLI uses `serve`, `scan`, `list`, `stats`, `resolve`, and `doctor` |
| Systemd unit (`deploy/oathkeeper.service`) | ✅ Ready | Uses `oathkeeper serve`; install the binary and config before enabling |
| LLM-based detection | ⚠️ Config only | `[llm]` section exists in config but detection is regex-based; LLM path not wired |

---

## CLI

```
oathkeeper [--config <path>] [--dry-run] <command> [flags]
```

### Commands

| Command | Description |
|---------|-------------|
| `serve [--tag a,b,c]` | Start HTTP server + daemon lifecycle + periodic recheck loop + optional transcript poller |
| `scan <file> [--format text\|json] [--json]` | Scan one transcript JSONL file for commitments |
| `list [flags]` | List commitments from the beads backend |
| `stats [flags]` | Compute commitment statistics; supports console dashboard, JSON/CSV export, HTML dashboard |
| `resolve <bead-id> [reason] [flags]` | Resolve one commitment bead |
| `doctor [--json]` | Run dependency checks |
| `version` | Print version (`oathkeeper v2.0.0`) |
| `help` | Print top-level usage and command list |

### Global Flags

| Flag | Description |
|------|-------------|
| `--config <path>` | Config file path (default: `~/.config/oathkeeper/oathkeeper.toml`) |
| `--dry-run` | Simulate mutating operations without backend writes |

### `serve` Flags

| Flag | Description |
|------|-------------|
| `--tag a,b,c` | Comma-separated extra tags appended to created beads |

### `scan` Flags

| Flag | Description |
|------|-------------|
| `--format text\|json` | Output format (default: `text`) |
| `--json` | Force JSON output |

### `list` Flags

| Flag | Description |
|------|-------------|
| `--status open\|closed\|all` | Filter by status (default: `open`) |
| `--category <tag>` | Filter by a single category/tag |
| `--since <duration>` | Include only commitments newer than duration |
| `--tag a,b,c` | AND filter on tags |
| `--json` | Machine-readable output `{"commitments": [...], "count": N}` |

### `stats` Flags

| Flag | Description |
|------|-------------|
| `--json` | Machine-readable output |
| `--export json\|csv` | Export format |
| `--output <path>` | Write export to file (requires `--export`) |
| `--dashboard <path>` | Write HTML dashboard file (cannot combine with `--export`) |

### `resolve` Flags

| Flag | Description |
|------|-------------|
| `--reason <text>` | Resolution reason (default: `resolved via CLI`) |
| `--json` | Machine-readable output |

### Known Limitations

Some older notes may still refer to pre-v2 `watch` / `check` naming. The live CLI entrypoints are `serve`, `scan`, `list`, `stats`, `resolve`, and `doctor`.

---

## HTTP API (when `serve` is running)

Default server address: `:9876`

### Health and Readiness

| Endpoint | Method | Response |
|----------|--------|----------|
| `GET /healthz` | `200 {"status":"ok"}` |
| `GET /readyz` | `200` if ready; `503 {"status":"not ready","error":"..."}` if degraded |

### v2 API

| Endpoint | Method | Description |
|----------|--------|-------------|
| `POST /api/v2/analyze` | POST | Analyze a message for commitment language; body: `{"session_key","message","role"}` |
| `GET /api/v2/commitments` | GET | List commitments; query params: `status`, `category` |
| `GET /api/v2/commitments/{id}` | GET | Get one commitment by ID |
| `POST /api/v2/commitments/{id}/resolve` | POST | Resolve a commitment; body: `{"reason":"..."}` (required) |
| `GET /api/v2/stats` | GET | `{"total","open","resolved","by_category":{...}}` |

---

## How It Works

```
Agent output → detect commitment language → wait grace period → verify backing mechanism → bead if unbacked
```

1. **Detection**: Commitment language in agent transcripts is matched against regex patterns with confidence scores. Categories: `temporal`, `scheduled`, `followup`, `conditional`, `untracked_problem`, `speculative`, `weak_commitment`. Only detections at or above `detector.min_confidence` (default `0.7`) proceed.

2. **Grace period**: After detection, a deferred verification is scheduled (default 30s). If a duplicate commitment arrives, the prior pending entry is cancelled and rescheduled.

3. **Verification**: Three backends check for backing mechanisms:
   - **Cron backend**: queries OpenClaw API for recent enabled cron jobs
   - **Bead backend**: checks `br list` for recent open `oathkeeper`-tagged beads
   - **File backend**: checks modified files in configured `state_dirs` and `memory_dirs`

4. **Bead creation**: If no backing mechanism is found, Oathkeeper creates a tracking bead via `br create` and publishes a `commitment.unbacked` event (webhook and/or Relay).

5. **Recheck loop**: Open commitments are rechecked periodically (default every 300s). Stale commitments expire after `auto_expire_hours` (default 168h / 7 days).

6. **Session context**: A per-session rolling message buffer detects fulfillment (past-tense completion) and escalation (repeated unresolved categories). On fulfillment, matching open beads are auto-resolved.

---

## Configuration

Default: `~/.config/oathkeeper/oathkeeper.toml`

```toml
[general]
grace_period = 30               # seconds before verification check
recheck_interval = 300          # seconds between periodic rechecks
max_alerts = 3                  # max recheck alerts per commitment
verbose = false
context_window_size = 5         # session message buffer size
dry_run = false
monitor_transcripts = true      # tail transcript files for auto-analyze
transcript_poll_interval = 3    # seconds between transcript poll cycles
readiness_error_threshold = 5   # failures before /readyz degrades
readiness_error_window = 300    # window for readiness error counting (seconds)

[server]
addr = ":9876"

[openclaw]
api_url = "http://localhost:8080"
transcript_dir = "~/.openclaw/sessions"
cron_endpoint = "/api/v1/crons"
wake_endpoint = "/api/v1/sessions/{session}/wake"

[llm]
command = "claude"
args = ["-p", "--model", "haiku"]
timeout = 10

[verification]
state_dirs = ["~/.openclaw/state"]
memory_dirs = ["~/.openclaw/memory"]
beads_command = "br"
tmux_command = "tmux"

[alerts]
openclaw_enabled = true
telegram_enabled = false
telegram_webhook = "http://localhost:9090/webhook/telegram"
resolution_webhook = ""         # falls back to telegram_webhook
throttle_window = 3600

[relay]
enabled = false
command = "relay"
to = "athena"
from = "oathkeeper"
timeout = 5
retries = 2

[storage]
db_path = "~/.local/share/oathkeeper/commitments.db"
auto_expire_hours = 168

[detector]
min_confidence = 0.7
pattern_matching_enabled = true
categories = ["temporal","scheduled","followup","conditional"]
```

---

## Beads Integration (`br`)

Oathkeeper creates tracking beads for unresolved commitments using the `br` (beads-polis) CLI.

**Note:** `br` is non-invasive and never executes git commands. After `br sync --snapshot`, you must manually run `git add .beads/ && git commit`.

Dependency:
- `br` must be installed and accessible from `PATH` (or configured via `verification.beads_command` in `oathkeeper.toml`).

Flow:
1. A commitment is detected from agent output.
2. Oathkeeper waits for the configured grace period.
3. Oathkeeper checks for a backing mechanism (cron, bead, state file, etc.).
4. If no backing mechanism is found, Oathkeeper creates a tracking bead via `br create`.
5. The bead is labeled/tagged with `oathkeeper` for traceability.

---

## Relay Integration

Oathkeeper can publish commitment events to Relay in addition to webhooks.

```toml
[relay]
enabled = true
command = "relay"
to = "athena"
from = "oathkeeper"
timeout = 5
```

Published events:
- `commitment.unbacked` when a bead is created for an unbacked commitment
- `commitment.resolved` when a tracked commitment is resolved

Events include retry logic (up to 2 retries with exponential backoff).

---

## Detector Confidence Threshold

The commitment detector applies a minimum confidence threshold from config:

```toml
[detector]
min_confidence = 0.7
```

Threshold behavior:
- A detection is considered a commitment only when `confidence >= min_confidence`.
- Default is `0.7`.
- Raising the threshold (for example, `0.8`) filters lower-confidence matches like weak commitments (`"I need to ..."`, confidence `0.70`).

---

## Doctor

```bash
oathkeeper doctor
oathkeeper doctor --json
```

Checks: Oathkeeper version, config file accessibility, OpenClaw API reachability, `br` binary, `tmux`, Claude CLI, and optional Argus webhook. Reports pass/fail/warn counts and an overall summary.

---

## Dependencies

**Required:** `br` CLI — beads backend for all list/show/create/close operations.

**Optional:**
- `relay` — publishes `commitment.unbacked` and `commitment.resolved` events to other agents
- `openclaw` HTTP API — cron verification backend, wake alerts
- `tmux` — checked by doctor
- `claude` CLI — checked by doctor; used for LLM config if configured

---

## Development

```bash
go build ./...
go test ./...
tests/suite.sh          # quick test suite
tests/suite.sh full     # full test suite including E2E
```

## Operations

- Operational runbook: `docs/OPERATIONS_RUNBOOK.md`

---

## Part of Polis

Horkos stands in the Styx at the edge of the city. He is one piece of a larger system.

- [Ergon](https://github.com/Perttulands/ergon-work-orchestration) — work orchestration
- [Hermes Relay](https://github.com/Perttulands/hermes-relay) — inter-agent messaging
- [Cerberus Gate](https://github.com/Perttulands/cerberus-gate) — access control
- [Chiron](https://github.com/Perttulands/chiron-trainer) — agent training
- [Learning Loop](https://github.com/Perttulands/learning-loop) — feedback and adaptation
- [Senate](https://github.com/Perttulands/senate) — governance and decision-making
- [Beads](https://github.com/Perttulands/beads-polis) — commitment tracking primitives
- [Truthsayer](https://github.com/Perttulands/truthsayer) — code analysis
- [UBS](https://github.com/Perttulands/ultimate_bug_scanner) — bug scanning
- [Argus](https://github.com/Perttulands/argus-watcher) — server monitoring
- [Polis Utils](https://github.com/Perttulands/polis-utils) — shared utilities

The chain descends into the river.

## License

MIT
