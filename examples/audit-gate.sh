#!/usr/bin/env bash
# Shows `xenolist check` as a supply-chain gate: it exits non-zero when the
# tree pulls executable code from more sources than your team agreed on,
# when unpinned sources appear, or when any source lives outside the host
# allowlist — ready for a pre-push hook or a release checklist.
set -euo pipefail

REPO="${1:?usage: audit-gate.sh <repo-dir>}"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# prefer an explicit override, then a repo-root build, then PATH
if [[ -z "${XENOLIST:-}" ]]; then
  if [[ -x "$ROOT/xenolist" ]]; then XENOLIST="$ROOT/xenolist"; else XENOLIST="xenolist"; fi
fi
if ! command -v "$XENOLIST" >/dev/null; then
  echo "audit-gate.sh: xenolist binary not found — build it first:" >&2
  echo "  (cd \"$ROOT\" && go build -o xenolist ./cmd/xenolist)" >&2
  exit 1
fi

echo "== budget gate: at most 20 sources, none floating =="
"$XENOLIST" check --max-sources 20 --max-floating 0 "$REPO" || true

echo
echo "== provenance gate: only well-known hosts allowed =="
"$XENOLIST" check \
  --allow-host github.com \
  --allow-host docker.io \
  --allow-host registry.npmjs.org \
  --allow-host proxy.golang.org \
  --allow-host pypi.org \
  "$REPO" || true

echo
echo "(each gate exits 1 on breach; '|| true' keeps the demo running)"
