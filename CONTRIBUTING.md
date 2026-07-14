# Contributing to xenolist

Issues, discussions and pull requests are all welcome.

## Getting started

You need Go ≥1.22; nothing else — no runtime dependencies, no services.

```bash
git clone https://github.com/JaydenCJ/xenolist && cd xenolist
go build ./...
go test ./...
bash scripts/smoke.sh
```

`scripts/smoke.sh` builds the binary, fabricates a repository with every
external-code surface in a temp dir, and asserts on real CLI output across
every subcommand; it must finish by printing `SMOKE OK`.

## Before you open a pull request

1. `gofmt -l .` reports nothing (formatting is enforced).
2. `go vet ./...` passes with no findings.
3. `go test ./...` passes (91 deterministic tests, no network).
4. `bash scripts/smoke.sh` prints `SMOKE OK`.
5. Add tests for behavior changes; keep logic in pure, unit-testable
   modules (scanners and aggregation never touch the network — nothing
   here does).

## Ground rules

- Keep dependencies at zero; adding one needs strong justification in the
  PR. xenolist audits supply chains — it must not grow one of its own.
- No network calls, ever. The census is produced entirely from bytes on
  disk. No telemetry.
- Detection rules are data plus one shared shell engine: new runners and
  installer shapes go into `internal/scan/shell.go` with a test
  reproducing the real command line, and a row in `docs/coverage.md`.
- Prefer honest skips over guesses: if a reference cannot be audited
  statically (variables, templates), it must not appear in the census.
- Code comments and doc comments are written in English.
- Determinism first: identical input must produce byte-identical reports,
  including all orderings.

## Reporting bugs

Include the output of `xenolist version`, the full command you ran, the
report output (redact paths if needed), and — for misdetections — the
exact source line from the scanned file, since that is exactly what the
scanner sees. For "xenolist missed X", a minimal file reproducing the
shape is perfect.

## Security

Please do not open public issues for security problems; use GitHub's
private vulnerability reporting on this repository instead.
