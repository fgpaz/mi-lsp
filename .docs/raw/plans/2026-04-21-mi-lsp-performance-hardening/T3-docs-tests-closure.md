# T3 - Docs, tests, and closure

## Scope

- Update daemon flow, RF, TP, technical baseline, state/telemetry, and admin contract docs.
- Add tests for watch-mode parsing, lazy watcher dedupe/caps, status fields, backpressure, direct workspace-map policy, bounded reads, and perf harness command behavior.
- Run traceability closure commands when available.

## Acceptance

- Docs and tests describe the same contract exposed by code.
- Targeted Go test packages pass.
- Traceability closure is attempted and any unavailable tool is reported.
