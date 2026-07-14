# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.0] - 2026-07-13

### Added

- Cross-file discovery and classification of every external-code surface:
  GitHub workflows and composite actions, Dockerfiles/Containerfiles,
  docker-compose files, GitLab CI and CircleCI configs, shell scripts,
  Makefiles, and package.json scripts, with `--include`/`--exclude` globs
  supporting `*`, `?`, and `**`.
- One shared shell engine behind every surface: `curl|bash`-style
  pipe-to-shell (including `sudo`/`tee` laundering, `bash <(curl …)`,
  `eval "$(curl …)"`), package runners (`npx`, `npm exec`, `pnpm dlx`,
  `yarn dlx`, `bunx`, `uvx`, `pipx run`, `go run mod@ver`,
  `deno run <url>`), and remote downloads (`pip install <url|git+…>`,
  Dockerfile `ADD <url>`).
- Dockerfile scanning with stage-alias suppression, `COPY --from`
  cross-image detection, BuildKit flag stripping, exec-form and heredoc
  RUN bodies, and continuation-aware line numbers.
- A minimal purpose-built YAML line scanner (no YAML engine, no
  dependencies) covering `uses:`, `image:`, `run:` block scalars, and
  GitLab `script:` lists with exact line numbers.
- Pin grading for every source — `pinned` (SHA/digest/pseudo-version),
  `tag`, `floating` — with sources deduplicated across files and graded by
  their loosest occurrence.
- `scan` subcommand producing the census (totals, by-kind and by-host
  rollups, floating list) in text, stable JSON (`schema_version: 1`), and
  Markdown; `list` quoting file:line evidence for every occurrence.
- `check` subcommand enforcing `--max-sources`, `--max-floating`, and a
  `--allow-host` allowlist with exit code 1 on breach, for policy gates.
- Runnable examples (`examples/make-demo-repo.sh`,
  `examples/audit-gate.sh`) and a coverage reference
  (`docs/coverage.md`).
- 91 deterministic offline tests (unit + in-process CLI integration
  against fabricated repository trees) and `scripts/smoke.sh`.

[0.1.0]: https://github.com/JaydenCJ/xenolist/releases/tag/v0.1.0
