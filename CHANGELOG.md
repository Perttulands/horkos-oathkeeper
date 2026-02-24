# Changelog

All notable changes to Oathkeeper.

Format: [Keep a Changelog](https://keepachangelog.com/en/1.1.0/)

## [Unreleased]

### Changed
- README: restored mythology intro (The Spartan Controller), character sigil and visual items, "Part of the Agora" section
- `athena-792`: Hardened truthsayer-scan error paths by preserving CLI parse error context, adding fallback logging for recheck runtime errors, improving bead JSON parse error reporting, and tightening serve-time notification/shutdown error handling.

### Added
- `OK-008`: Global/configurable dry-run mode (`--dry-run` and `[general].dry_run`) that simulates mutating operations without creating/closing beads.
- `OK-017`: Added backend error classification helpers and API/CLI error report metadata (`detail`, `hint`) for clearer operational troubleshooting.
- `OK-018`: Added a consolidated test-suite runner (`tests/suite.sh`) with `quick` and `full` modes plus suite usage docs.
- `OK-019`: Added operational runbook documentation (`docs/OPERATIONS_RUNBOOK.md`) covering startup, health checks, incident triage, and upgrade/rollback procedures.

### Changed
- OK-002: Improved commitment detection accuracy with deadline/daypart temporal patterns (for example `by EOD`, `before 5 pm`, `tonight`) and added regression tests for non-agent/system phrasing.
- OK-003: Added `scheduled` commitment category detection (cron jobs, reminders, periodic tasks) with positive/negative detector coverage and explicit confidence scoring.
- OK-004: Hardened grace-period verification flow to safely handle nil verifier callbacks/outcomes without panics, with edge-case tests for graceful fallback behavior.
- OK-005: Improved automated bead creation by passing structured commitment metadata through grace callbacks and using original `session_key`/`detected_at` when creating unbacked tracking beads.
- OK-006: Extended Relay integration for `commitment.unbacked` with optional correlation metadata (`session_key`, `commitment_id`) and wired serve-time publishing to include that context, with schema/publisher tests.
- OK-007: Upgraded CLI stats output into a text dashboard with percentage bars and sorted status/category breakdowns while preserving `--json` and export/dashboard output modes.
- Tests: Updated E2E grace-callback wiring to use `api.GraceCallbackContext`, keeping `tests/suite.sh full` compatible with the current API callback signature.
- `OK-008`: `serve` and `resolve` now propagate dry-run behavior through bead operations and emit dry-run-safe responses.
- `OK-009`: Cron verification now supports configurable cron endpoint paths, filters disabled/paused cron jobs, and accepts alternate API response shapes (`crons` or `items`).
- `OK-010`: Relay publishing now uses explicit `RelayEvent` schemas with lifecycle event constants and payload validation before command dispatch.
- `OK-011`: Relay publishing now supports configurable retry attempts with backoff and is wired from config (`relay.retries`) for resilient event delivery.
- `OK-012`: Added dedicated resolution webhook routing (`alerts.resolution_webhook`, fallback to alert webhook) and included `resolved_at` timestamp in resolved webhook payloads.
- `OK-013`: Expanded `stats` output with status breakdowns, recent activity, oldest-open age, and richer JSON fields (`by_status`, `recent_24h`, `oldest_open_age_seconds`).
- `OK-014`: Added `stats` export pipeline with `--export json|csv` and optional `--output` file writing for machine-consumable reporting.
- `OK-015`: Added static HTML stats dashboard generation via `stats --dashboard <path>`.
- `OK-016`: Added multi-backend verification (cron + beads + state/memory file backends) with config-based verifier construction.
- `OK-017`: Improved runtime error handling for beads backend failures with status-aware API responses (`404`/`503`/`504`) and user-facing CLI hints for not-initialized workspaces, missing commands, timeouts, and missing bead IDs.
- `OK-018`: Hardened beads integration tests to skip cleanly in environments where temporary DB usage requires manual workspace initialization.
- `OK-019`: Linked operations documentation from `README.md` for faster operator discovery.

## [2026-02-20]

### Added
- Optional Relay publisher for commitment lifecycle events (`commitment.unbacked`, `commitment.resolved`) wired into `serve` callbacks with configurable relay command/route/timeout under `[relay]`.

### Changed
- Commitment detection now enforces `detector.min_confidence` (default `0.7`) across `serve` and `scan`, so detection sensitivity is configurable at runtime instead of being effectively hardcoded.
- Added detector/context/scanner tests for threshold filtering behavior (default threshold acceptance, stricter-threshold rejection, invalid-threshold fallback) to prevent regressions.
- Documented detector threshold semantics in `README.md` so operators can tune confidence behavior intentionally.

## [1.0.0] - 2026-02-13

### Added
- **Sprint 1**: Temporal commitment detection (US-001), system behavior exclusion (US-002), past vs future distinction (US-003), conditional commitment detection (US-004), cron job checking (US-005), 30-second grace period (US-006), OpenClaw wake events for unbacked commitments (US-007), Telegram notifications via Argus (US-008), periodic re-check of unresolved commitments (US-009)
- **Sprint 2**: SQLite storage layer (US-010), commitment formatter with mechanism display (US-011), commitment expiration with time extraction and auto-expire (US-012), commitment resolution with terminal state guards (US-013)
- **Sprint 3**: Doctor command for dependency health checks (US-014), daemon with systemd service support (US-015), TOML configuration (US-016), on-demand transcript scanning (US-017)
- **Sprint 4**: Bead creation tracking (US-018), memory file writing (US-019), JSON API server for commitment queries (US-020)
- Beads integration verified end-to-end with real `br` commands: `create` (tracking issue with `oathkeeper` label), `list --json` visibility, and `close` cleanup in integration tests
- Bead tracker CLI arguments aligned with current `br` interface (`--description`, `--labels`, `--silent`)
- All tests passing, clean build
