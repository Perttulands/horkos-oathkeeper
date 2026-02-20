# Test Suite

Run the consolidated suite from repository root:

```bash
tests/suite.sh full
```

Fast local pass (no race/e2e):

```bash
tests/suite.sh quick
```

Notes:
- Uses `GO_BIN` if set; otherwise prefers `/usr/local/go/bin/go` and falls back to `go`.
- Tests that require a real beads CLI write path are opt-in via `OATHKEEPER_RUN_BR_INTEGRATION=1`.
