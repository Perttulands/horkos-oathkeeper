# Oathkeeper — Commitment Accountability Watchdog

## Name
Oathkeeper (Νέμεσις) — Greek goddess of retribution against those who succumb to hubris. In our context: the force that ensures AI agent promises are backed by action.

## Purpose
Oathkeeper watches outgoing messages from AI agents (primarily Athena) and detects commitment language — promises, follow-ups, scheduled actions. When a commitment is detected, Oathkeeper verifies that a backing mechanism exists (cron job, bead, state file, watcher). If no mechanism is found, Oathkeeper alerts the agent to fulfill or retract the promise.

## Architecture

### Core Components

1. **Transcript Watcher** — Tails OpenClaw session transcripts for new assistant messages
2. **Commitment Detector** — Identifies commitment language in outgoing messages using pattern matching + LLM classification
3. **Mechanism Verifier** — Checks for backing mechanisms (crons, beads, state files, dispatched agents)
4. **Alert System** — Notifies via OpenClaw wake event or Telegram when unbackered commitments are found

### Detection Patterns
- Temporal promises: "I will", "I'll", "I'm going to", "let me", "I'll check"
- Scheduled actions: "at X o'clock", "in N minutes", "tomorrow", "after the agent finishes"
- Follow-up language: "I'll follow up", "I'll report back", "I'll monitor", "I'll let you know"
- Conditional futures: "once X completes, I'll Y", "when done, I'll Z"

### Verification Checks
- Cron jobs: query OpenClaw cron API for recently created jobs
- Beads: check `br list` for new/active beads
- State files: scan `state/` directory for recent writes
- Dispatched agents: check tmux sessions and dispatch socket
- Memory writes: check `memory/` for recent entries

### Non-Commitments (Exclude)
- Descriptions of system behavior: "the agent will run", "dispatch.sh will monitor"
- User instructions: "you can run", "this will work"
- Conditional/hypothetical: "if we did X, it would Y"
- Past tense: "I checked", "I created"

## Tech Stack
- **Language**: Go (CLI tool, like dcg)
- **LLM**: `claude -p --model haiku` for classification (no API key needed)
- **Runtime**: systemd service (independent of OpenClaw, like Argus)
- **Config**: TOML (like dcg)
- **Alerts**: OpenClaw cron wake events + optional Telegram via Argus bot

## Interface

```
oathkeeper watch          # Start watching transcripts (daemon mode)
oathkeeper scan <file>    # One-shot scan of a transcript file
oathkeeper check          # Verify all open commitments have mechanisms
oathkeeper list           # Show tracked commitments and their status
oathkeeper doctor         # Check installation and dependencies
oathkeeper --version      # Print version
```

## Data Model

```
Commitment {
  id:          string     # hash of message content + timestamp
  detected_at: timestamp  # when detected
  source:      string     # session key
  message_id:  string     # original message identifier
  text:        string     # the commitment text
  category:    string     # temporal|scheduled|followup|conditional
  backed_by:   []string   # mechanisms found (cron:id, bead:id, file:path)
  status:      string     # unverified|backed|alerted|resolved|expired
  expires_at:  timestamp  # when the commitment should have been fulfilled
}
```

## Storage
- SQLite database at `~/.local/share/oathkeeper/commitments.db`
- Lightweight, no external dependencies

## Alert Flow
1. New assistant message detected in transcript
2. Commitment detector classifies → commitment or not
3. If commitment: wait 30 seconds (grace period for mechanism creation)
4. Mechanism verifier checks all sources
5. If no mechanism found → alert via OpenClaw wake event
6. Alert includes: the promise text, what was checked, suggested actions
7. Track commitment as "alerted" — re-check on next scan

## Integration Points
- **OpenClaw**: Wake events to nudge the agent session
- **Argus**: Can use Argus Telegram bot for direct user notification
- **Beads**: Creates tracking beads for unresolved commitments
- **DCG pattern**: Similar architecture — Go binary, systemd service, TOML config

## Success Criteria
- Detects >90% of genuine commitments (low false-negative rate)
- <20% false positive rate (descriptions vs promises)
- Verification completes within 5 seconds
- Zero impact on agent performance (read-only observation)
- Works independently of OpenClaw (survives gateway restart)
