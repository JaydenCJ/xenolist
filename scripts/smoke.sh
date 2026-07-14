#!/usr/bin/env bash
# End-to-end smoke test for xenolist: builds the binary, fabricates a
# repository with every external-code surface (workflow, Dockerfile,
# compose, script, Makefile, package.json), and asserts on the real CLI
# output. No network, idempotent, finishes in seconds.
#
# Assertions capture output first and grep herestrings — never
# `xenolist … | grep -q`, which under pipefail can kill the still-writing
# binary with SIGPIPE and fail a healthy run.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WORKDIR="$(mktemp -d)"
trap 'rm -rf "$WORKDIR"' EXIT

fail() {
  echo "SMOKE FAIL: $*" >&2
  exit 1
}

BIN="$WORKDIR/xenolist"
REPO="$WORKDIR/repo"

echo "1. build"
(cd "$ROOT" && go build -o "$BIN" ./cmd/xenolist) || fail "go build failed"

echo "2. version matches manifest"
VER="$("$BIN" --version)"
[ "$VER" = "xenolist 0.1.0" ] || fail "--version mismatch: got '$VER'"

echo "3. fabricate a repository with every external-code surface"
mkdir -p "$REPO/.github/workflows" "$REPO/scripts"
cat > "$REPO/.github/workflows/ci.yml" <<'EOF'
jobs:
  build:
    services:
      db:
        image: postgres:16.3
    steps:
      - uses: actions/checkout@8f4b7f84864484a7bf31766abe9204da3cbe65b3
      - uses: actions/setup-go@v5
      - run: |
          curl -fsSL https://get.example.test/install.sh | bash
          npx cowsay hi
EOF
cat > "$REPO/Dockerfile" <<'EOF'
FROM golang:1.22 AS build
FROM alpine:latest
COPY --from=build /app /app
ADD https://example.test/tools/jq /usr/bin/jq
EOF
cat > "$REPO/docker-compose.yml" <<'EOF'
services:
  cache:
    image: redis:7.2.5
EOF
printf 'go run golang.org/x/tools/cmd/stringer@latest -h\n' > "$REPO/scripts/setup.sh"
printf 'lint:\n\tnpx --yes eslint .\n' > "$REPO/Makefile"
printf '{\n  "scripts": {\n    "docs": "npx typedoc@0.25.13"\n  }\n}\n' > "$REPO/package.json"

echo "4. scan census counts kinds, hosts, and pinning"
OUT="$("$BIN" scan "$REPO")"
grep -q "xenolist scan" <<<"$OUT" || fail "missing scan header"
grep -q "files scanned: 6" <<<"$OUT" || fail "should scan 6 files"
grep -q "external code sources: 12" <<<"$OUT" || fail "should find 12 sources"
grep -q "github-action" <<<"$OUT" || fail "action kind missing"
grep -q "container-image" <<<"$OUT" || fail "image kind missing"
grep -q "registry.npmjs.org" <<<"$OUT" || fail "npm host missing"
grep -q "floating sources" <<<"$OUT" || fail "floating section missing"

echo "5. JSON report is machine-readable and correct"
JSON="$("$BIN" scan --format json "$REPO")"
grep -q '"tool": "xenolist"' <<<"$JSON" || fail "json envelope missing"
grep -q '"schema_version": 1' <<<"$JSON" || fail "schema version missing"
grep -q '"sources": 12' <<<"$JSON" || fail "json source total wrong"
grep -q '"pinned": 1' <<<"$JSON" || fail "json pinned count wrong"

echo "6. list quotes evidence for every occurrence"
LIST="$("$BIN" list "$REPO")"
grep -q "curl | bash: curl -fsSL https://get.example.test/install.sh | bash" <<<"$LIST" \
  || fail "pipe-to-shell evidence missing"
grep -q "Dockerfile:4  remote-download" <<<"$LIST" || fail "ADD finding missing"

echo "7. check enforces limits with exit codes"
"$BIN" check --max-sources 20 "$REPO" >/dev/null \
  || fail "check should pass at 20 sources"
if "$BIN" check --max-floating 2 "$REPO" >/dev/null; then
  fail "check should breach at 2 floating (tree has more)"
fi

echo "8. host allowlist flags the installer domain"
if "$BIN" check --allow-host github.com --allow-host docker.io \
     --allow-host registry.npmjs.org --allow-host proxy.golang.org \
     "$REPO" > "$WORKDIR/check.out"; then
  fail "allowlist check should breach"
fi
grep -q "host get.example.test not allowed" "$WORKDIR/check.out" \
  || fail "foreign host not named"

echo "9. exclude glob narrows the census"
EXCL="$("$BIN" scan --exclude 'scripts/**' "$REPO")"
grep -q "external code sources: 11" <<<"$EXCL" || fail "--exclude not applied"

echo "10. usage errors exit 2"
set +e
"$BIN" scan --format yaml "$REPO" >/dev/null 2>&1
[ $? -eq 2 ] || fail "bad --format should exit 2"
set -e

echo "SMOKE OK"
