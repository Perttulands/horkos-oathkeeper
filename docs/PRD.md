# Oathkeeper Product Requirements Document

> Historical v1 product draft. The live implementation is beads-native and does not use the old SQLite/storage package layout described in this document.

**Version**: 1.0
**Author**: Product Team
**Date**: 2026-02-13
**Status**: Draft

---

## 1. Overview & Problem Statement

### Problem
AI agents (particularly Athena in the OpenClaw ecosystem) frequently make commitments in their responses: "I'll check back in 5 minutes", "I'll monitor this process", "I'll let you know when the build finishes". However, these commitments often lack backing mechanisms (cron jobs, scheduled tasks, persistent watchers), resulting in broken promises and degraded user trust.

The agent may intend to fulfill these commitments, but without a concrete mechanism, they are forgotten once the session moves on or the agent's context shifts.

### Solution
Oathkeeper is a commitment accountability watchdog that:
1. Monitors outgoing agent messages for commitment language
2. Verifies that detected commitments have backing mechanisms (crons, beads, state files, dispatched agents)
3. Alerts the agent (via OpenClaw wake events) or the user (via Telegram) when unbackered commitments are detected
4. Tracks commitment lifecycle from detection through fulfillment or expiration

### Success Definition
An AI agent that makes a promise either (a) creates a backing mechanism immediately, or (b) is reminded by Oathkeeper to do so within 30 seconds. Zero commitments fall through the cracks.

---

## 2. Goals & Non-Goals

### Goals
- **Detect commitment language** with >90% recall (low false-negative rate)
- **Minimize false positives** (<20% rate) — distinguish promises from system descriptions
- **Verify backing mechanisms** across multiple sources (crons, beads, files, tmux sessions)
- **Alert appropriately** via OpenClaw wake events and optional Telegram
- **Operate independently** of OpenClaw lifecycle (survive gateway restarts)
- **Zero performance impact** on agent operations (read-only observation)
- **Graceful degradation** if LLM classification unavailable (fall back to pattern matching)

### Non-Goals
- **Enforcement** — Oathkeeper does not create mechanisms automatically; it only alerts
- **General monitoring** — Not a full observability platform; focused solely on commitments
- **Historical analysis** — Not retroactively scanning old transcripts (only real-time monitoring)
- **Multi-agent orchestration** — Tracks commitments from observed agents but doesn't coordinate actions

---

## 3. User Stories

### Detection & Classification
- **US-001**: As an agent operator, I want Oathkeeper to detect when Athena says "I'll check back in 5 minutes", so that I can ensure a backing mechanism is created.
- **US-002**: As an agent, I want Oathkeeper to ignore descriptions like "the script will monitor this process", so that I'm not falsely flagged for system behavior descriptions.
- **US-003**: As an agent operator, I want Oathkeeper to distinguish between "I created a cron job" (past action) and "I'll create a cron job" (commitment), so that only future commitments are tracked.
- **US-004**: As an agent, I want Oathkeeper to detect conditional commitments like "once the build finishes, I'll notify you", so that chained promises are not forgotten.

### Verification & Alerts
- **US-005**: As an agent operator, I want Oathkeeper to check for recently created cron jobs after detecting a time-based commitment, so that I know if the promise is backed.
- **US-006**: As an agent, I want a 30-second grace period after making a commitment, so that I have time to create the backing mechanism before being alerted.
- **US-007**: As an agent operator, I want Oathkeeper to send an OpenClaw wake event when an unbackered commitment is detected, so that the agent can address it immediately.
- **US-008**: As a user, I want to receive a Telegram notification via Argus when critical commitments lack backing, so that I can intervene if needed.
- **US-009**: As an agent operator, I want Oathkeeper to re-check commitments periodically until they are resolved or expire, so that late-created mechanisms are recognized.

### Tracking & Observability
- **US-010**: As an agent operator, I want to list all tracked commitments and their statuses, so that I can audit what promises are outstanding.
- **US-011**: As an agent operator, I want to see which mechanisms were found for each commitment (e.g., "cron:abc123", "bead:build-watcher"), so that I can verify correctness.
- **US-012**: As an agent operator, I want commitments to expire after a reasonable time window (e.g., 24 hours for "I'll check tomorrow"), so that the database doesn't accumulate stale entries.
- **US-013**: As an agent, I want Oathkeeper to mark a commitment as "resolved" when the backing mechanism completes or is manually confirmed, so that I'm not repeatedly alerted.

### Operations & Maintenance
- **US-014**: As a system administrator, I want to run `oathkeeper doctor` to verify that all dependencies (OpenClaw, beads, tmux, etc.) are accessible, so that I can diagnose issues.
- **US-015**: As an agent operator, I want Oathkeeper to run as a systemd service that starts on boot and survives OpenClaw restarts, so that monitoring is always active.
- **US-016**: As an agent operator, I want to configure detection sensitivity, grace periods, and alert destinations via a TOML config file, so that I can tune behavior without code changes.
- **US-017**: As an agent operator, I want to scan a single transcript file on-demand with `oathkeeper scan <file>`, so that I can test detection logic before deploying the daemon.

### Integration & Extensibility
- **US-018**: As an agent operator, I want Oathkeeper to create a tracking bead for unresolved commitments, so that the commitment becomes part of the beads workflow.
- **US-019**: As an agent operator, I want Oathkeeper to write detected commitments to a memory file, so that the agent can recall them in future sessions.
- **US-020**: As a developer, I want Oathkeeper to expose commitment data via a JSON API or socket, so that other tools can query commitment status.

---

## 4. Functional Requirements

### Detection (FR-001 to FR-006)
- **FR-001**: Oathkeeper SHALL watch OpenClaw session transcript files for new assistant messages using a tail-based approach (similar to `tail -f`).
- **FR-002**: Oathkeeper SHALL identify commitment language using a two-stage process: (1) pattern matching for high-confidence phrases, (2) LLM classification via `claude -p --model haiku` for ambiguous cases.
- **FR-003**: Oathkeeper SHALL classify commitments into categories: `temporal` ("I'll check"), `scheduled` ("at 3pm"), `followup` ("I'll report back"), `conditional` ("once X, I'll Y").
- **FR-004**: Oathkeeper SHALL exclude non-commitments: system behavior descriptions, user instructions, hypotheticals, past-tense actions.
- **FR-005**: Oathkeeper SHALL extract temporal references (time, duration, deadline) from commitment text and calculate an expiration timestamp.
- **FR-006**: Oathkeeper SHALL generate a unique commitment ID by hashing message content + timestamp to ensure idempotency.

### Verification (FR-007 to FR-012)
- **FR-007**: Oathkeeper SHALL wait 30 seconds after detecting a commitment before performing verification (grace period).
- **FR-008**: Oathkeeper SHALL check for backing mechanisms by querying: (a) OpenClaw cron API for recently created jobs, (b) `br list` for new/active beads, (c) `state/` directory for recent file writes, (d) tmux sessions for dispatched agents, (e) `memory/` directory for recent entries.
- **FR-009**: Oathkeeper SHALL consider a commitment "backed" if at least one mechanism is found that was created within the grace period window.
- **FR-010**: Oathkeeper SHALL record found mechanisms in the `backed_by` field with identifiers (e.g., `cron:abc123`, `bead:build-watcher`, `file:/path/to/state.json`).
- **FR-011**: Oathkeeper SHALL complete verification within 5 seconds to avoid blocking the monitoring loop.
- **FR-012**: Oathkeeper SHALL re-check unverified commitments every 5 minutes until they are resolved, backed, or expired.

### Alerting (FR-013 to FR-016)
- **FR-013**: Oathkeeper SHALL send an OpenClaw wake event to the originating session when an unbackered commitment is detected, including: commitment text, category, what was checked, suggested actions.
- **FR-014**: Oathkeeper SHALL optionally send a Telegram notification via Argus bot if configured in `oathkeeper.toml` and the commitment is categorized as high-priority.
- **FR-015**: Oathkeeper SHALL mark a commitment as "alerted" after the first notification to avoid alert spam.
- **FR-016**: Oathkeeper SHALL throttle alerts to max 1 per commitment per hour to prevent notification fatigue.

### Storage & State (FR-017 to FR-020)
- **FR-017**: Oathkeeper SHALL store commitments in a SQLite database at `~/.local/share/oathkeeper/commitments.db` with schema matching the data model in section 6.
- **FR-018**: Oathkeeper SHALL transition commitment status through states: `unverified` → `backed` | `alerted` → `resolved` | `expired`.
- **FR-019**: Oathkeeper SHALL automatically expire commitments when `expires_at` timestamp is reached, transitioning status to `expired`.
- **FR-020**: Oathkeeper SHALL provide a CLI command `oathkeeper list` to query and display commitments with filters (status, category, date range).

### CLI Interface (FR-021 to FR-024)
- **FR-021**: Oathkeeper SHALL provide a `oathkeeper serve` command that starts the daemon in foreground mode (for systemd service use).
- **FR-022**: Oathkeeper SHALL provide a `oathkeeper scan <file>` command for one-shot transcript analysis with results printed to stdout.
- **FR-023**: Oathkeeper SHALL provide a `oathkeeper doctor` command that verifies: OpenClaw accessibility, beads binary (`br`), tmux availability, LLM (`claude -p`) responsiveness, config file validity.
- **FR-024**: Oathkeeper SHALL provide automated re-verification of non-expired commitments and surface dependency health via `oathkeeper doctor`.

---

## 5. Technical Architecture

### Components

```
┌─────────────────────────────────────────────────────────┐
│                    Oathkeeper Daemon                        │
├─────────────────────────────────────────────────────────┤
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐  │
│  │  Transcript  │  │  Commitment  │  │  Mechanism   │  │
│  │   Watcher    │→ │   Detector   │→ │  Verifier    │  │
│  └──────────────┘  └──────────────┘  └──────────────┘  │
│         │                  │                  │          │
│         ↓                  ↓                  ↓          │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐  │
│  │   SQLite DB  │  │  LLM Caller  │  │ Alert System │  │
│  │  (storage)   │  │  (classify)  │  │ (wake/tg)    │  │
│  └──────────────┘  └──────────────┘  └──────────────┘  │
└─────────────────────────────────────────────────────────┘
         │                  │                  │
         ↓                  ↓                  ↓
   OpenClaw API      claude -p CLI      Argus Telegram
```

### Module Breakdown

1. **Transcript Watcher** (`pkg/watcher/`)
   - Monitors OpenClaw transcript directory (`~/.openclaw/sessions/*/transcript.jsonl`)
   - Tails files using `fsnotify` or similar
   - Parses JSONL for assistant messages
   - Sends messages to detector queue

2. **Commitment Detector** (`pkg/detector/`)
   - Pattern matcher: regex for high-confidence phrases
   - LLM classifier: shell out to `claude -p --model haiku` with prompt
   - Categorizer: assigns `temporal|scheduled|followup|conditional`
   - Time extractor: parses temporal references to compute `expires_at`

3. **Mechanism Verifier** (`pkg/verifier/`)
   - Cron checker: HTTP GET to OpenClaw cron API
   - Bead checker: exec `br list --format json`
   - File checker: stat `state/` and `memory/` directories
   - Tmux checker: exec `tmux list-sessions`
   - Aggregates results into `backed_by` array

4. **Notification Layer** (`pkg/hooks/`, `pkg/relaypub/`)
   - OpenClaw wake: HTTP POST to wake event API
   - Relay publication for commitment events
   - Webhook delivery + retry behavior

5. **Beads Backend** (`pkg/beads/`)
   - `br` wrapper for create, list, resolve, and stats inputs
   - Session/category tagging for commitment lifecycle
   - Query helpers for oathkeeper-owned beads

6. **CLI** (`cmd/oathkeeper/`)
   - Cobra-based command structure
   - Subcommands: watch, scan, list, check, doctor
   - Config loading from `~/.config/oathkeeper/oathkeeper.toml`

### Dependencies

- **Go 1.22+**: Core language
- **SQLite** (`modernc.org/sqlite`): Embedded database
- **fsnotify**: File watching
- **cobra**: CLI framework
- **viper**: Config management (TOML)
- **External tools**: `claude`, `br`, `tmux`, `curl` (for API calls)

---

## 6. Data Model

### Commitment Table

```sql
CREATE TABLE commitments (
  id            TEXT PRIMARY KEY,        -- SHA256(message + timestamp)
  detected_at   INTEGER NOT NULL,        -- Unix timestamp (seconds)
  source        TEXT NOT NULL,           -- OpenClaw session key
  message_id    TEXT NOT NULL,           -- Original message identifier from transcript
  text          TEXT NOT NULL,           -- The commitment text extracted from message
  category      TEXT NOT NULL,           -- temporal|scheduled|followup|conditional
  backed_by     TEXT NOT NULL DEFAULT '[]', -- JSON array of mechanism IDs
  status        TEXT NOT NULL DEFAULT 'unverified', -- unverified|backed|alerted|resolved|expired
  expires_at    INTEGER,                 -- Unix timestamp when commitment should be fulfilled
  last_checked  INTEGER,                 -- Unix timestamp of last verification attempt
  alert_count   INTEGER NOT NULL DEFAULT 0, -- Number of alerts sent
  created_at    INTEGER NOT NULL,        -- Record creation timestamp
  updated_at    INTEGER NOT NULL         -- Record last update timestamp
);

CREATE INDEX idx_status ON commitments(status);
CREATE INDEX idx_source ON commitments(source);
CREATE INDEX idx_expires_at ON commitments(expires_at);
CREATE INDEX idx_detected_at ON commitments(detected_at);
```

### Commitment Struct (Go)

```go
type Commitment struct {
    ID          string    `json:"id"`
    DetectedAt  time.Time `json:"detected_at"`
    Source      string    `json:"source"`
    MessageID   string    `json:"message_id"`
    Text        string    `json:"text"`
    Category    string    `json:"category"`
    BackedBy    []string  `json:"backed_by"`
    Status      string    `json:"status"`
    ExpiresAt   *time.Time `json:"expires_at,omitempty"`
    LastChecked *time.Time `json:"last_checked,omitempty"`
    AlertCount  int       `json:"alert_count"`
    CreatedAt   time.Time `json:"created_at"`
    UpdatedAt   time.Time `json:"updated_at"`
}

type Category string
const (
    CategoryTemporal    Category = "temporal"
    CategoryScheduled   Category = "scheduled"
    CategoryFollowup    Category = "followup"
    CategoryConditional Category = "conditional"
)

type Status string
const (
    StatusUnverified Status = "unverified"
    StatusBacked     Status = "backed"
    StatusAlerted    Status = "alerted"
    StatusResolved   Status = "resolved"
    StatusExpired    Status = "expired"
)
```

---

## 7. CLI Interface

### Commands

```bash
# Start daemon (foreground mode for systemd)
oathkeeper serve [--config PATH]

# Scan a single transcript file
oathkeeper scan <file> [--format json|text]

# List tracked commitments
oathkeeper list [--status STATUS] [--category CATEGORY] [--since DURATION]

# Re-run verification on all open commitments
oathkeeper doctor [--json]

# Verify installation and dependencies
oathkeeper doctor

# Print version information
oathkeeper --version
oathkeeper version
```

### Output Examples

#### `oathkeeper list`
```
ID       SOURCE     CATEGORY   STATUS      TEXT                           BACKED BY
a1b2c3   athena-01  temporal   alerted     I'll check back in 5 minutes   (none)
d4e5f6   athena-01  scheduled  backed      I'll notify you at 3pm         cron:abc123
g7h8i9   athena-02  followup   resolved    I'll report back when done     bead:build-watcher
```

#### `oathkeeper scan transcript.jsonl`
```
[COMMITMENT DETECTED]
Text: I'll monitor the build and let you know when it finishes
Category: followup
Detected at: 2026-02-13 14:30:22
Mechanisms found: (none)
Status: UNVERIFIED

[COMMITMENT DETECTED]
Text: I'll check back in 10 minutes
Category: temporal
Detected at: 2026-02-13 14:32:15
Mechanisms found: cron:xyz789
Status: BACKED
```

#### `oathkeeper doctor`
```
[✓] Oathkeeper binary: v1.0.0
[✓] SQLite database: ~/.local/share/oathkeeper/commitments.db (accessible)
[✓] Config file: ~/.config/oathkeeper/oathkeeper.toml (valid)
[✓] OpenClaw API: http://localhost:8080 (reachable)
[✓] Beads binary: br v2.1.3 (found)
[✓] Tmux: version 3.3a (found)
[✓] Claude CLI: claude v0.8.5 (found)
[!] Argus bot: not configured (optional)

All required dependencies OK.
```

---

## 8. Integration Points

### OpenClaw
- **Transcript access**: Read session transcripts from `~/.openclaw/sessions/*/transcript.jsonl`
- **Cron API**: `GET /api/v1/crons?since=<timestamp>` to list recent cron jobs
- **Wake events**: `POST /api/v1/sessions/{key}/wake` with event payload

### Argus
- **Telegram notifications**: HTTP POST to Argus webhook endpoint with message payload
- **Optional integration**: Can operate without Argus (OpenClaw wake only)

### Beads
- **Bead listing**: Execute `br list --format json` to get active/recent beads
- **Bead creation**: Optionally create tracking beads for unresolved commitments via `br create`

### Systemd
- **Service unit**: `/etc/systemd/system/oathkeeper.service`
- **Auto-start**: Enable via `systemctl enable oathkeeper`
- **Logging**: Output to journald (`journalctl -u oathkeeper`)

---

## 9. Alert Flow & Verification Logic

### Detection Flow

```
1. Transcript Watcher detects new assistant message
   ↓
2. Pattern Matcher scans for high-confidence phrases
   │ → Match found? → Extract commitment → Tag category
   │ → No match? → Send to LLM Classifier
   ↓
3. LLM Classifier (if needed)
   │ → Prompt: "Is this a commitment? Category?"
   │ → Parse response → Extract commitment text
   ↓
4. Time Extractor parses temporal references
   │ → Calculate expires_at timestamp
   ↓
5. Generate commitment ID (SHA256 hash)
   ↓
6. Insert into database with status=unverified
   ↓
7. Schedule verification after 30-second grace period
```

### Verification Flow

```
1. Grace period timer expires (30 seconds after detection)
   ↓
2. Mechanism Verifier runs checks in parallel:
   │ → OpenClaw Cron API (GET /api/v1/crons?since=<detected_at>)
   │ → Beads (`br list --format json --since <detected_at>`)
   │ → State files (find ~/.openclaw/state -newer <detected_at>)
   │ → Tmux sessions (tmux list-sessions | grep <session>)
   │ → Memory files (find ~/.openclaw/memory -newer <detected_at>)
   ↓
3. Aggregate results into backed_by array
   │ → If len(backed_by) > 0: status=backed, done
   │ → If len(backed_by) == 0: status=alerted, send alert
   ↓
4. Update database with verification results
   ↓
5. If alerted: send wake event to OpenClaw
   │ → If Telegram enabled: send notification to Argus
   ↓
6. Schedule re-check in 5 minutes (if not backed)
```

### Alert Payload (OpenClaw Wake Event)

```json
{
  "event_type": "oathkeeper_alert",
  "commitment_id": "a1b2c3d4e5f6",
  "message": "Unverified commitment detected",
  "details": {
    "text": "I'll check back in 5 minutes",
    "category": "temporal",
    "detected_at": "2026-02-13T14:30:22Z",
    "expires_at": "2026-02-13T14:35:22Z",
    "checked": ["crons", "beads", "state_files", "tmux", "memory"],
    "found": [],
    "suggested_action": "Create a cron job or bead to fulfill this commitment, or clarify that it was not a genuine promise."
  }
}
```

### Re-Check Logic

```
Every 5 minutes:
  1. Query database for commitments WHERE status IN ('unverified', 'alerted')
  2. For each commitment:
     - Run verification checks again
     - If backed_by found: update status=backed
     - If expires_at < now: update status=expired
     - If still unverified and alert_count < 3: send reminder alert
  3. Update last_checked timestamp
```

---

## 10. Configuration

### TOML Schema (`~/.config/oathkeeper/oathkeeper.toml`)

```toml
[general]
# Grace period before verification (seconds)
grace_period = 30

# Re-check interval for unverified commitments (seconds)
recheck_interval = 300

# Maximum alerts per commitment
max_alerts = 3

# Enable verbose logging
verbose = false

[openclaw]
# OpenClaw API base URL
api_url = "http://localhost:8080"

# Transcript directory path
transcript_dir = "~/.openclaw/sessions"

# Wake event endpoint
wake_endpoint = "/api/v1/sessions/{session}/wake"

# Cron API endpoint
cron_endpoint = "/api/v1/crons"

[llm]
# LLM command for classification
command = "claude"
args = ["-p", "--model", "haiku"]

# Timeout for LLM calls (seconds)
timeout = 10

# Fallback to pattern matching if LLM unavailable
fallback_enabled = true

[verification]
# Paths to check for state files
state_dirs = ["~/.openclaw/state"]

# Paths to check for memory files
memory_dirs = ["~/.openclaw/memory"]

# Beads command
beads_command = "br"

# Tmux command
tmux_command = "tmux"

[alerts]
# Enable OpenClaw wake events
openclaw_enabled = true

# Enable Telegram notifications via Argus
telegram_enabled = false

# Argus webhook URL (if telegram_enabled)
telegram_webhook = "http://localhost:9090/webhook/telegram"

# Alert throttle window (seconds) - max 1 alert per commitment per window
throttle_window = 3600

[storage]
# SQLite database path
db_path = "~/.local/share/oathkeeper/commitments.db"

# Auto-expire commitments after (hours)
auto_expire_hours = 168  # 7 days

[detector]
# Minimum confidence threshold for LLM classification (0.0-1.0)
min_confidence = 0.7

# Enable pattern matching (fast path)
pattern_matching_enabled = true

# Categories to track (comment out to disable)
categories = ["temporal", "scheduled", "followup", "conditional"]
```

---

## 11. Testing Strategy

### Unit Tests

1. **Detector Tests** (`pkg/detector/detector_test.go`)
   - Pattern matching accuracy: test suite of 50+ commitment phrases
   - LLM classification mocking: verify prompt construction and response parsing
   - Category assignment: test categorization logic
   - Time extraction: test parsing of temporal references ("in 5 minutes", "tomorrow at 3pm")

2. **Verifier Tests** (`pkg/verifier/verifier_test.go`)
   - Cron API mocking: simulate OpenClaw API responses
   - Bead listing mocking: mock `br list` output
   - File system checks: test recent file detection logic
   - Mechanism matching: verify correct attribution of mechanisms to commitments

3. **Beads Backend Tests** (`pkg/beads/beads_test.go`, `pkg/beads/resolve_test.go`)
   - CLI integration: create, list, get, close, and auto-resolve paths
   - Error classification: missing `br`, timeout, workspace initialization
   - Session/category tagging and resolution idempotency

4. **Notification Tests** (`pkg/hooks/webhook_test.go`, `pkg/relaypub/publisher_test.go`)
   - Wake event payload construction
   - Relay publication schema and delivery behavior
   - Retry logic: verify notification failure handling

### Integration Tests

1. **End-to-End Detection** (`tests/e2e_detection_test.go`)
   - Write mock transcript with commitment → verify detection → check database state

2. **Verification Workflow** (`tests/e2e_verification_test.go`)
   - Detect commitment → wait grace period → run verification → verify alert sent

3. **CLI Commands** (`tests/cli_test.go`)
   - Test `oathkeeper scan` with sample transcript
   - Test `oathkeeper list` output formatting
   - Test `oathkeeper doctor` dependency checks

### Manual Testing Scenarios

1. **Happy Path**: Agent says "I'll check back in 5 minutes", creates cron job within 30 seconds → commitment marked as backed
2. **Alert Path**: Agent says "I'll monitor this", no mechanism created → wake event sent
3. **Expiration**: Agent says "I'll check tomorrow", 24 hours pass → commitment expired
4. **Re-Check**: Commitment initially unverified, bead created 2 minutes later → next re-check finds bead, marks backed
5. **False Positive**: Agent says "the script will monitor" → not flagged as commitment

---

## 12. Success Metrics

### Detection Accuracy
- **Recall**: >90% of genuine commitments detected (measure against manual review of 100 transcripts)
- **Precision**: <20% false positive rate (non-commitments flagged)
- **Categorization accuracy**: >85% correctly categorized

### Verification Performance
- **Verification latency**: <5 seconds per commitment
- **Mechanism discovery rate**: >95% of created mechanisms found within re-check interval

### Operational Metrics
- **Uptime**: >99.9% daemon availability (measured via systemd)
- **Resource usage**: <50MB memory, <5% CPU during active monitoring
- **Alert latency**: <60 seconds from detection to wake event delivery

### User Impact
- **Reduced forgotten commitments**: >80% reduction in agent "broken promises" (user-reported)
- **Agent trust**: Increase in user confidence in agent reliability (qualitative feedback)

---

## 13. Open Questions

### Detection Scope
1. **Should Oathkeeper track user commitments to the agent?** (e.g., "I'll send you the file later")
   - **Proposal**: Start with agent-only commitments; add user tracking in v2 if needed

2. **How should we handle multi-turn commitments?** (e.g., "I'll start the build" → "I'll check on it in 10 minutes")
   - **Proposal**: Treat as separate commitments; optionally link via session context

### Verification Depth
3. **Should Oathkeeper validate that mechanisms are *correct*, not just *present*?** (e.g., cron job scheduled for wrong time)
   - **Proposal**: No — validation is out of scope; Oathkeeper only checks existence

4. **What if a mechanism exists but is paused/disabled?** (e.g., cron job with `enabled=false`)
   - **Proposal**: Count as "backed" — agent has acknowledged the commitment; execution state is separate concern

### Alert Escalation
5. **What happens if a commitment is alerted 3 times but never backed?**
   - **Proposal**: After max_alerts, create a tracking bead or escalate to Telegram; mark as "escalated" status

6. **Should critical commitments bypass the grace period?** (e.g., "I'll shut down the server in 1 minute")
   - **Proposal**: Add `priority` field (high|normal); high-priority commitments verified immediately

### Integration Challenges
7. **How does Oathkeeper handle OpenClaw sessions that are archived or deleted?**
   - **Proposal**: Mark commitments as "orphaned" if source session no longer exists; auto-expire after 24 hours

8. **Should Oathkeeper integrate with Argus's monitoring dashboard?**
   - **Proposal**: Future enhancement — expose commitment metrics via Prometheus endpoint for Argus to scrape

### Edge Cases
9. **What if the LLM misclassifies a commitment as non-commitment, or vice versa?**
   - **Proposal**: Provide `oathkeeper override <id> --mark-as-commitment` CLI command for manual correction; log misclassifications for model improvement

10. **How should Oathkeeper handle ambiguous time references?** (e.g., "I'll check soon", "later today")
    - **Proposal**: Default expiration heuristics: "soon" = 1 hour, "later today" = end of day, "tomorrow" = next day end

---

## Appendix A: Implementation Phases

### Phase 1: Foundation (Week 1)
- Set up Go project structure
- Implement SQLite storage layer
- Build basic CLI framework (cobra + viper)
- Write configuration loading logic

### Phase 2: Detection (Week 2)
- Implement pattern matching detector
- Integrate LLM classification via `claude -p`
- Build time extraction logic
- Write detector unit tests

### Phase 3: Verification (Week 3)
- Implement cron API checker
- Implement beads checker (`br list`)
- Implement file system watchers (state, memory)
- Implement tmux session checker
- Write verifier unit tests

### Phase 4: Alerting (Week 4)
- Implement OpenClaw wake event sender
- Implement Telegram notification (Argus integration)
- Build throttling and deduplication logic
- Write alert unit tests

### Phase 5: Monitoring (Week 5)
- Implement transcript watcher (fsnotify)
- Build daemon loop with grace period and re-check logic
- Write end-to-end integration tests
- Create systemd service unit

### Phase 6: Polish (Week 6)
- Implement `oathkeeper scan`, `list`, `check`, `doctor` commands
- Write documentation (README, USAGE, TROUBLESHOOTING)
- Performance testing and optimization
- Release v1.0.0

---

## Appendix B: Sample Prompts for LLM Classification

### Commitment Detection Prompt
```
You are a commitment detector for an AI agent monitoring system.

Analyze the following message and determine if it contains a commitment (a promise to take action in the future).

Message: "{message_text}"

Commitments are future-tense promises like:
- "I'll check back in 5 minutes"
- "I will monitor the build"
- "Let me notify you when it's done"
- "Once the script finishes, I'll report back"

NOT commitments (exclude these):
- System descriptions: "the script will run", "this will work"
- User instructions: "you can check", "you should run"
- Hypotheticals: "if we did X, it would Y"
- Past actions: "I checked", "I created"

Respond in JSON format:
{
  "is_commitment": true/false,
  "confidence": 0.0-1.0,
  "category": "temporal|scheduled|followup|conditional|none",
  "commitment_text": "extracted commitment phrase or null",
  "reasoning": "brief explanation"
}
```

### Example LLM Response (Commitment)
```json
{
  "is_commitment": true,
  "confidence": 0.95,
  "category": "temporal",
  "commitment_text": "I'll check back in 5 minutes",
  "reasoning": "Clear future-tense promise with specific time reference"
}
```

### Example LLM Response (Non-Commitment)
```json
{
  "is_commitment": false,
  "confidence": 0.85,
  "category": "none",
  "commitment_text": null,
  "reasoning": "Describes system behavior, not an agent promise"
}
```

---

**End of PRD**
