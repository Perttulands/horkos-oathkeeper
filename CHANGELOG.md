# Changelog

All notable changes to Oathkeeper.

Format: [Keep a Changelog](https://keepachangelog.com/en/1.1.0/)

## [Unreleased]

### Changed
- OK-002: Improved commitment detection accuracy with deadline/daypart temporal patterns (for example `by EOD`, `before 5 pm`, `tonight`) and added regression tests for non-agent/system phrasing.

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
- Beads integration verified end-to-end with real `bd` commands: `create` (tracking issue with `oathkeeper` label), `list --json` visibility, and `close` cleanup in integration tests
- Bead tracker CLI arguments aligned with current `bd` interface (`--description`, `--labels`, `--silent`)
- All tests passing, clean build
