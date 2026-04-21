# T1 - Lifecycle and watcher containment

## Scope

- Route `EnsureDaemon` through the same lock and health recheck path as `SpawnBackground`.
- Make watcher mode lazy by default.
- Support `off|lazy|eager` for CLI/env.
- Deduplicate aliases by canonical workspace root.
- Bound active watched roots with LRU.

## Acceptance

- Concurrent auto-start callers converge on one daemon.
- Starting the daemon does not recursively watch all registered aliases by default.
- Lazy activation starts at most one watcher per canonical root.
