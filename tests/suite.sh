#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
MODE="${1:-full}"

GO_BIN="${GO_BIN:-}"
if [[ -z "${GO_BIN}" ]]; then
  if [[ -x "/usr/local/go/bin/go" ]]; then
    GO_BIN="/usr/local/go/bin/go"
  else
    GO_BIN="go"
  fi
fi

run() {
  echo "+ $*"
  "$@"
}

cd "${ROOT_DIR}"

case "${MODE}" in
  quick)
    run "${GO_BIN}" test ./cmd/oathkeeper ./pkg/api ./pkg/beads -count=1
    ;;
  full)
    run "${GO_BIN}" test ./cmd/oathkeeper ./pkg/... -count=1
    run "${GO_BIN}" test ./tests/e2e -count=1
    run "${GO_BIN}" test ./pkg/api -race -count=1
    ;;
  *)
    echo "Usage: tests/suite.sh [quick|full]" >&2
    exit 2
    ;;
esac
