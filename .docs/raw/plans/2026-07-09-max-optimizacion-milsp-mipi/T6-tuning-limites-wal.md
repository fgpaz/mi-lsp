# Task T6: Tuning de límites del daemon + checkpoint WAL + techos de memoria

## Shared Context
**Goal:** Que los defaults del daemon soporten 50 clientes (hoy: inflight 16, pool 3, WAL sin checkpoint, un runtime llegó a 779 MB).
**Stack:** Go, repo `C:/repos/mios/mi-lsp`, paquetes `internal/daemon` + `internal/cli`.
**Architecture:** `maxInflight=16` (`internal/daemon/options.go:16,29`, env `MI_LSP_DAEMON_MAX_INFLIGHT`), `maxWorkers=3` (`internal/cli/daemon.go:62`), reapLoop cada 1 min (`lifecycle.go`), telemetría SQLite con WAL sin checkpoint programado (`state_store.go`), techos de memoria opt-in por env (`lifecycle.go:115-117, 471-495`).

## Locked Decisions
- `maxInflight` default: 16 → **48** (env override sigue mandando).
- `maxWorkers` default: 3 → **6** (flag CLI sigue mandando). Documentar en el help del flag que cada worker Roslyn puede usar cientos de MB.
- Techo de memoria por runtime DEFAULT: si `MI_LSP_DAEMON_MAX_RUNTIME_MEMORY_MB` no está seteado, aplicar **1024 MB** (hoy es opt-in sin default → el runtime de 779 MB creció libre). Mantener env override.
- Checkpoint WAL: en el reapLoop (cada 1 min ya existe), cada **30 iteraciones** (~30 min) ejecutar `PRAGMA wal_checkpoint(TRUNCATE)` sobre la DB de telemetría, con log a debug y error no fatal.
- Idle timeout de runtimes queda en 30 min (no cambiar: el thrash se resuelve con pool 6, no con TTL más largo).
- Tests: ajustar los tests existentes que asuman los defaults viejos (correr `go test ./internal/...` y arreglar SOLO asserts de defaults; cualquier otro fallo = STOP).
- Registrar los nuevos defaults en el output de `system.status` si ya expone config (si no la expone, no agregar superficie nueva — eso es de T10 docs).

## Task Metadata
```yaml
id: T6
depends_on: []
agent_type: general-purpose
goal_id: G2
github_issues: []
expected_outcome: "Daemon default soporta 48 inflight / 6 runtimes, WAL acotada, runtimes con techo de 1 GB"
files:
  - modify: C:/repos/mios/mi-lsp/internal/daemon/options.go
  - modify: C:/repos/mios/mi-lsp/internal/cli/daemon.go:62
  - modify: C:/repos/mios/mi-lsp/internal/daemon/lifecycle.go
  - modify: C:/repos/mios/mi-lsp/internal/daemon/state_store.go
complexity: low
done_when:
  - "go build ./... && go test ./internal/... exit 0"
  - "grep de los nuevos defaults (48, 6, 1024) presente en el código con sus env/flag overrides intactos"
evidence_expected:
  - "Salida de build/test + diff de defaults"
stop_if:
  - "tests no relacionados con defaults fallan"
```

## Reference
`internal/daemon/options.go:15-29`, `internal/cli/daemon.go:61-62`, `internal/daemon/lifecycle.go:114-117,356-362,443-495`, `internal/daemon/state_store.go:176-232`.

## Prompt
Protocolo de despacho del plan aplica: micro-lane pi read-only para confirmar los 4 anclajes (defaults actuales y el reapLoop). Después aplicá los cambios de Locked Decisions. El checkpoint WAL va como método del TelemetryStore (`Checkpoint()`), llamado desde el reapLoop con un contador de iteraciones; usá `db.Exec("PRAGMA wal_checkpoint(TRUNCATE)")` y tragá el error con log. Para el techo de memoria default, buscá dónde se lee `MI_LSP_DAEMON_MAX_RUNTIME_MEMORY_MB` y agregá el default 1024 cuando la env está vacía.

## Execution Procedure
1. Lane pi de anclajes.
2. Edits en los 4 archivos.
3. `go build ./... && go test ./internal/...` (arreglar solo asserts de defaults).
4. NO commitees. Reportá diffs + salidas.

## Skeleton
```go
const defaultMaxInflight = 48        // options.go (era 16)
const defaultMaxWorkers = 6          // daemon.go flag default (era 3)
const defaultMaxRuntimeMemoryMB = 1024
func (s *TelemetryStore) Checkpoint() { _, _ = s.db.Exec("PRAGMA wal_checkpoint(TRUNCATE)") }
```

## Verify
`cd C:/repos/mios/mi-lsp && go build ./... && go test ./internal/...` → exit 0

## Commit
`perf(daemon): defaults for 50-client scale (inflight 48, pool 6, 1GB runtime ceiling, WAL checkpoint)`
