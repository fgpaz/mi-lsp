# Plan - mi-lsp daemon performance hardening

Date: 2026-04-21
Status: Execution plan

## Objective

Reduce daemon idle footprint and concurrency risk for agent use:

- Idle working set <= 250 MB.
- Idle private bytes <= 300 MB.
- Idle handles <= 5k.
- Exactly one reachable daemon per OS user.
- 16 parallel callers must not spawn duplicate daemons or grow daemon memory without bound.

## Root hypothesis

The primary risk is eager recursive file watching over every registered alias, amplified by duplicate aliases for the same root and concurrent auto-start paths. Worker runtime optimization is secondary until daemon lifecycle and watcher activation are bounded.

## Public contract changes

- Default watcher mode becomes `lazy`.
- `daemon start` and hidden `daemon serve` accept `--watch-mode off|lazy|eager` and `--max-watched-roots`.
- Environment variables:
  - `MI_LSP_WATCH_MODE`
  - `MI_LSP_WATCH_MAX_ROOTS`
  - `MI_LSP_DAEMON_MAX_INFLIGHT`
- `daemon status` and `/api/status` expose daemon process stats and watcher stats.
- Saturated daemon-served heavy operations return typed `daemon/backpressure_busy`.

## Waves

1. Diagnosis gates: add status/process/watcher evidence and a perf smoke harness.
2. Lifecycle/watchers: unify auto-start locking, validate PID and pipe health, lazy watcher activation by canonical root, dedupe aliases, cap watched roots with LRU.
3. Hot paths: bounded daemon inflight, direct/default `workspace-map`, bounded context/log/LSP reads.
4. Docs/tests: sync `FL/RF/TP/07/08/09` and add targeted tests.
5. Closure: run traceability and audit commands when available.

## Verification

- `go test ./internal/daemon ./internal/service ./internal/cli ./internal/worker`
- New perf smoke harness for idle daemon and 16 parallel callers.
- `mi-lsp daemon status --format json`

## Dirty worktree rule

The worktree already contains unrelated modifications. This plan and implementation must stage or commit only task-owned files, and must not revert or mix foreign dirty work.
