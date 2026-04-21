# T0 - Diagnosis gates

## Scope

- Add daemon process stats for working set, private bytes, handle count, and thread count.
- Add watcher stats for mode, active roots, watched directories, and pending debounce events.
- Add a perf smoke harness that can verify one daemon and 16 parallel callers.

## Acceptance

- `daemon status` includes `daemon_process` and `watchers`.
- `/api/status` includes the same fields.
- Smoke harness exits non-zero when duplicate daemon start, memory, or handle budgets fail.
