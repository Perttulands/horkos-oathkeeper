# Oathkeeper

Oathkeeper tracks agent commitments and enforces follow-through as part of the problem accountability system.

## Beads Integration (`bd`)

Oathkeeper creates tracking beads for unresolved commitments using the `bd` CLI.

Dependency:
- `bd` must be installed and accessible from `PATH` (or configured via `verification.beads_command` in `oathkeeper.toml`).

Flow:
1. A commitment is detected from agent output.
2. Oathkeeper waits for the configured grace period.
3. Oathkeeper checks for a backing mechanism (cron, bead, state file, etc.).
4. If no backing mechanism is found, Oathkeeper creates a tracking bead via `bd create`.
5. The bead is labeled/tagged with `oathkeeper` for traceability.

## Development Verification

```bash
go build ./...
go test ./...
```
