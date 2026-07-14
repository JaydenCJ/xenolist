#!/usr/bin/env bash
# Fabricates a small repository whose files touch every surface xenolist
# scans: a GitHub workflow, a multi-stage Dockerfile, a compose file, a
# shell script, a Makefile, and package.json scripts. Offline and
# deterministic: the same tree is produced on every machine.
set -euo pipefail

DEST="${1:?usage: make-demo-repo.sh <dest-dir>}"
mkdir -p "$DEST/.github/workflows" "$DEST/scripts"

cat > "$DEST/.github/workflows/ci.yml" <<'EOF'
name: CI
on: [push]
jobs:
  build:
    runs-on: ubuntu-latest
    services:
      db:
        image: postgres:16.3
    steps:
      - uses: actions/checkout@8f4b7f84864484a7bf31766abe9204da3cbe65b3
      - uses: actions/setup-go@v5
      - name: install tools
        run: |
          curl -fsSL https://get.example.test/install.sh | bash
          npx cowsay@5.0.0 "hi"
          go run golang.org/x/tools/cmd/stringer@latest -h
  release:
    uses: octo-org/workflows/.github/workflows/release.yml@main
EOF

cat > "$DEST/Dockerfile" <<'EOF'
FROM golang:1.22-alpine AS build
COPY . .
RUN go build -o /app ./cmd/app

FROM alpine:latest
COPY --from=build /app /app
ADD https://example.test/tools/jq /usr/bin/jq
RUN wget -qO- https://sh.example.test | sh
EOF

cat > "$DEST/docker-compose.yml" <<'EOF'
services:
  cache:
    image: redis:7.2.5@sha256:1b503bb77079ba644371969e06e1a6a1670bb34c2251107c0fc3a21ef9fdaeca
  web:
    build: .
EOF

cat > "$DEST/scripts/setup.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
curl -sSf https://static.example.test/v1.4.2/tool.sh | sudo bash -s -- --yes
pipx run ruff==0.4.4 check .
pip install git+https://github.com/example/lib@0123456789abcdef0123456789abcdef01234567
EOF

cat > "$DEST/Makefile" <<'EOF'
lint:
	npx --yes eslint .
	curl https://get.example.test/lint.sh | sh
EOF

cat > "$DEST/package.json" <<'EOF'
{
  "name": "demo",
  "scripts": {
    "docs": "npx typedoc@0.25.13",
    "fmt": "prettier -w ."
  }
}
EOF

echo "demo repository written to $DEST"
