# Oathkeeper PRD — Ralph Execution Format

**Tech Stack**: Go, SQLite, TOML config, claude -p --model haiku

**Repo**: /home/user/oathkeeper

**Reference**: docs/PRD.md (full PRD), docs/SPEC.md (spec)


## Sprint 1: Core Infrastructure
**Status:** COMPLETE

- [x] **US-001**: As an agent operator, I want Oathkeeper to detect when Athena says "I'll check back in 5 minutes", so that I can ensure a backing mechanism is created.
- [x] **US-002**: As an agent, I want Oathkeeper to ignore descriptions like "the script will monitor this process", so that I'm not falsely flagged for system behavior descriptions.
- [x] **US-003**: As an agent operator, I want Oathkeeper to distinguish between "I created a cron job" (past action) and "I'll create a cron job" (commitment), so that only future commitments are tracked.
- [x] **US-004**: As an agent, I want Oathkeeper to detect conditional commitments like "once the build finishes, I'll notify you", so that chained promises are not forgotten.
- [x] **US-005**: As an agent operator, I want Oathkeeper to check for recently created cron jobs after detecting a time-based commitment, so that I know if the promise is backed.
- [x] **US-006**: As an agent, I want a 30-second grace period after making a commitment, so that I have time to create the backing mechanism before being alerted.
- [x] **US-REVIEW-S1**: Review Sprint 1 — run tests, verify all tasks work together, fix integration issues.

## Sprint 2: Verification & Alerts
**Status:** COMPLETE

- [x] **US-007**: As an agent operator, I want Oathkeeper to send an OpenClaw wake event when an unbackered commitment is detected, so that the agent can address it immediately.
- [x] **US-008**: As a user, I want to receive a Telegram notification via Argus when critical commitments lack backing, so that I can intervene if needed.
- [x] **US-009**: As an agent operator, I want Oathkeeper to re-check commitments periodically until they are resolved or expire, so that late-created mechanisms are recognized.
- [x] **US-010**: As an agent operator, I want to list all tracked commitments and their statuses, so that I can audit what promises are outstanding.
- [x] **US-011**: As an agent operator, I want to see which mechanisms were found for each commitment (e.g., "cron:abc123", "bead:build-watcher"), so that I can verify correctness.
- [x] **US-012**: As an agent operator, I want commitments to expire after a reasonable time window (e.g., 24 hours for "I'll check tomorrow"), so that the database doesn't accumulate stale entries.
- [x] **US-013**: As an agent, I want Oathkeeper to mark a commitment as "resolved" when the backing mechanism completes or is manually confirmed, so that I'm not repeatedly alerted.
- [x] **US-REVIEW-S2**: Review Sprint 2 — run tests, verify all tasks work together, fix integration issues.

## Sprint 3: CLI & Operations
**Status:** COMPLETE

- [x] **US-014**: As a system administrator, I want to run `oathkeeper doctor` to verify that all dependencies (OpenClaw, beads, tmux, etc.) are accessible, so that I can diagnose issues.
- [x] **US-015**: As an agent operator, I want Oathkeeper to run as a systemd service that starts on boot and survives OpenClaw restarts, so that monitoring is always active.
- [x] **US-016**: As an agent operator, I want to configure detection sensitivity, grace periods, and alert destinations via a TOML config file, so that I can tune behavior without code changes.
- [x] **US-017**: As an agent operator, I want to scan a single transcript file on-demand with `oathkeeper scan <file>`, so that I can test detection logic before deploying the daemon.
- [x] **US-REVIEW-S3**: Review Sprint 3 — run tests, verify all tasks work together, fix integration issues.

## Sprint 4: Integration & Polish
**Status:** IN PROGRESS

- [x] **US-018**: As an agent operator, I want Oathkeeper to create a tracking bead for unresolved commitments, so that the commitment becomes part of the beads workflow.
- [x] **US-019**: As an agent operator, I want Oathkeeper to write detected commitments to a memory file, so that the agent can recall them in future sessions.
- [x] **US-020**: As a developer, I want Oathkeeper to expose commitment data via a JSON API or socket, so that other tools can query commitment status.
- [ ] **US-REVIEW-S4**: Review Sprint 4 — run tests, verify all tasks work together, fix integration issues.
