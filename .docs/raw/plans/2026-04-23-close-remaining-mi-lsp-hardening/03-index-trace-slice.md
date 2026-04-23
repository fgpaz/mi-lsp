# Wave 2 - Index jobs / trace / runtime slice

## Ownership

- `internal/cli/index.go`
- `internal/service/index_jobs.go`
- `internal/store/index_jobs.go`
- `internal/service/trace.go`
- `internal/service/trace_test.go`
- `internal/store/index_jobs_test.go`
- `internal/store/process_terminate_unix.go`
- `internal/store/process_terminate_windows.go`

## Required outcomes

- finish `index cancel --force` end to end:
  - CLI flag
  - payload propagation
  - service/store termination
  - warning surface
  - durable `canceled` result when a live PID exists
- preserve `phase=indexing` during heavy work
- reserve `publishing` only for final publish/close
- keep `nav trace` disk fallback for:
  - `.docs/wiki/04_RF.md`
  - `.docs/wiki/04_RF/*.md`
  - legacy `.docs/wiki/RF/*.md`
  - legacy root `.docs/wiki/RF.md`
- do not reopen `nav wiki`; only verify that `RF-QRY-016` behavior still holds

## Stop condition

Stop if:

- any edit spills outside the owned files
- `nav wiki trace RF-QRY-016` regresses
