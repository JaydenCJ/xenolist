# xenolist examples

Two runnable scripts, both offline and self-contained.

## make-demo-repo.sh

Fabricates a small repository that touches every surface xenolist scans:
a GitHub workflow with pinned and floating actions, a multi-stage
Dockerfile with a `curl | bash` in a RUN, a digest-pinned compose service,
an installer script, a Makefile recipe, and package.json scripts.

```bash
bash examples/make-demo-repo.sh /tmp/xenolist-demo
xenolist scan /tmp/xenolist-demo
xenolist list /tmp/xenolist-demo
```

## audit-gate.sh

Shows `xenolist check` as a policy gate: a source-count budget, a
zero-floating rule, and a host allowlist — each exiting 1 on breach so
they can back a pre-push hook or any local automation.

```bash
bash examples/audit-gate.sh /tmp/xenolist-demo
```

Both scripts write fixed file contents, so their output is identical on
every machine.
