# Oathkeeper Operational Runbook

## Scope
This runbook covers production operation of Oathkeeper in `serve` mode:
- startup and shutdown
- health verification
- routine maintenance
- incident triage
- upgrade and rollback

## Prerequisites
- Oathkeeper binary available on host (`oathkeeper`)
- Config file present (`~/.config/oathkeeper/oathkeeper.toml` by default)
- Beads CLI configured in `verification.beads_command` (typically `br`)
- Network access to configured dependencies (OpenClaw, optional webhooks/Relay)

## Startup
Foreground run:

```bash
oathkeeper serve --config ~/.config/oathkeeper/oathkeeper.toml
```

Dry-run startup (safe verification path):

```bash
oathkeeper --dry-run serve --config ~/.config/oathkeeper/oathkeeper.toml
```

Expected startup signal:
- stdout prints `Oathkeeper listening on <addr>`

## Shutdown
- `Ctrl+C` in foreground mode
- If supervised by service manager, stop via manager command (for example `systemctl stop oathkeeper`)

Expected shutdown signal:
- stdout prints `Oathkeeper stopped.`

## Health Checks
Service liveness:

```bash
curl -fsS http://127.0.0.1:9876/healthz
```

Service readiness:

```bash
curl -fsS http://127.0.0.1:9876/readyz
```

Dependency diagnostics:

```bash
oathkeeper doctor --json
```

## Routine Operations
List open commitments:

```bash
oathkeeper list --status open
```

Resolve a commitment:

```bash
oathkeeper resolve <bead-id> --reason "manual resolution"
```

Generate stats exports:

```bash
oathkeeper stats --export json --output /tmp/oathkeeper-stats.json
oathkeeper stats --export csv --output /tmp/oathkeeper-stats.csv
oathkeeper stats --dashboard /tmp/oathkeeper-dashboard.html
```

## Incident Triage
### API returns `503` with not-initialized hint
Symptom:
- `list`/`resolve`/`stats` responses show beads workspace not initialized.

Action:
1. Confirm the configured beads command and DB/workspace location.
2. Have a human initialize the target workspace (for example `br init` in the intended repo).
3. Re-run `oathkeeper doctor` and retry request.

### API returns `503` command unavailable
Symptom:
- Error indicates beads command unavailable.

Action:
1. Validate `verification.beads_command` in config.
2. Confirm command exists on PATH for the service user.
3. Restart service after fixing path/install.

### API returns `504` timeout
Symptom:
- Backend beads command timed out.

Action:
1. Check host load and disk pressure.
2. Manually run the equivalent beads command to confirm responsiveness.
3. Retry request; if persistent, investigate beads DB state and lock contention.

### Webhook or Relay notification failures
Symptom:
- Logs show webhook/relay publish failures while service stays healthy.

Action:
1. Verify endpoint URL, credentials, and network egress.
2. Validate Relay command availability and routing config.
3. Use `--dry-run` to verify commitment lifecycle behavior independent of notification transport.

## Upgrade Procedure
1. Capture current version and config:
```bash
oathkeeper --version
cp ~/.config/oathkeeper/oathkeeper.toml /tmp/oathkeeper.toml.backup
```
2. Deploy new binary.
3. Run preflight diagnostics:
```bash
oathkeeper doctor
```
4. Restart service.
5. Verify `/healthz`, `/readyz`, and a test `POST /api/v2/analyze`.

## Rollback Procedure
1. Stop service.
2. Restore previous known-good binary.
3. Restore prior config if changed.
4. Start service and validate health/readiness endpoints.

## Test Validation Before Deploy
Run repository suite:

```bash
tests/suite.sh full
```

Optional beads CLI integration tests:

```bash
OATHKEEPER_RUN_BR_INTEGRATION=1 tests/suite.sh full
```
