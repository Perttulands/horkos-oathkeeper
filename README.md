# ⚖️ Oathkeeper

![Oathkeeper](images/oathkeeper.jpg)


*Spartan red cloak. Binding chain. The Styx brand on his palm. He checks the receipts.*

---

The Greeks swore their most terrible oaths by the River Styx. Break one and even the gods suffered — nine years of silence, cast out from Olympus. The punishment wasn't death. It was accountability.

Oathkeeper has the same philosophy, minus the river. He wears the red cloak — long, dramatic, unmistakable. Nobody else in the Agora wears red. A bronze breastplate with a ledger-bandolier strapped across it, because the book is part of him. A binding chain wrapped around his forearm, always ready. And in his other hand, a quill-tipped spear — the point writes and stabs with equal conviction.

When an agent says "I will," Oathkeeper writes it down. When the grace period expires and the promise is still floating, he brands it. The Styx brand on his palm glows when pressed onto the record. Burnt into the ledger. Permanent. He doesn't punish broken oaths. He just makes sure everyone knows about them.

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

## Operations

- Operational runbook: `docs/OPERATIONS_RUNBOOK.md`

## Part of the Agora

Oathkeeper was forged in **[Athena's Agora](https://github.com/Perttulands/athena-workspace)** — an autonomous coding system where AI agents build software and a Spartan in a red cloak makes sure they finish what they start.

[Argus](https://github.com/Perttulands/argus) watches the server. [Truthsayer](https://github.com/Perttulands/truthsayer) watches the code. [Relay](https://github.com/Perttulands/relay) carries the messages. Oathkeeper watches the promises. The chain is always ready.

The [mythology](https://github.com/Perttulands/athena-workspace/blob/main/mythology.md) has the full story.

## License

MIT
