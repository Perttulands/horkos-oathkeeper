# Oathkeeper

Oathkeeper tracks agent commitments and enforces follow-through as part of the problem accountability system.

## Beads Integration (`bd`)

Oathkeeper creates tracking beads for unresolved commitments using the `bd` CLI.

Dependency:
- `bd` version **0.46.0** must be installed and accessible from `PATH` (or configured via `verification.beads_command` in `oathkeeper.toml`).
- Fork: [Perttulands/beads](https://github.com/Perttulands/beads) (branch `v0.46.0-stable`)

Flow:
1. A commitment is detected from agent output.
2. Oathkeeper waits for the configured grace period.
3. Oathkeeper checks for a backing mechanism (cron, bead, state file, etc.).
4. If no backing mechanism is found, Oathkeeper creates a tracking bead via `bd create`.
5. The bead is labeled/tagged with `oathkeeper` for traceability.

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

## Development Verification

```bash
go build ./...
go test ./...
```
