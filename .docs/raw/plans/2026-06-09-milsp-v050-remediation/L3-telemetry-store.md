# Task L3: Telemetry & state store

## Shared Context
**Goal:** Telemetría sin contención (cola async), perms 0o600, redacción de paths, decision_json→hash, dedup workspace_root, implementar PurgeAndVacuum.
**Stack:** Go, SQLite.
**Architecture:** Worktree `C:/wt/v050-l3-telemetry-store`, branch `v050/l3-telemetry-store`. Único dueño de `state_store.go`, `telemetry/access_diagnostics.go`, `telemetry/access_events.go`. Implementa el stub `PurgeAndVacuum` de T2.

> **CORREGIDO por discovery.yaml:** el tipo es `TelemetryStore` (state_store.go:143), no `StateStore`. Métodos existentes: `PurgeOldEvents`:230, `PurgeOldRuns`:238, `RecentAccesses`:566, `RecordAccess`:512, `NextSeq`:444, `ComputeMetrics`:611. El campo `DecisionHash` en `model.AccessEvent` ya viene de T2.

## Locked Decisions
- Escrituras de telemetría a una cola async (canal con worker único) para eliminar "database is locked" de raíz; el retry queda como red de seguridad.
- `state.json` y `start.lock`: modo 0o600; dir `~/.mi-lsp/daemon` 0o700 (no-op en Windows, aplicar en Unix).
- `decision_json`: persistir igual, pero en la salida de `recent_accesses` reemplazar por `DecisionHash` (sha256 corto 12 chars). El JSON completo solo en export admin.
- `workspace_root`/`entrypoint_path` en respuesta multi-evento: emitir una vez a nivel envelope, no por evento (TOK-03).
- `PurgeAndVacuum(retentionDays, maxBytes)`: borra eventos > retentionDays; si DB > maxBytes, baja a 7 días; corre VACUUM; devuelve (purged, vacuumed, err).

## Task Metadata
```yaml
id: L3
depends_on: [T2]
agent_type: general-purpose
goal_id: G3
github_issues: []
expected_outcome: "telemetría async sin locks; perms restrictivos; salida con hash y paths deduplicados; purga+vacuum funcional."
files:
  - modify: internal/daemon/state_store.go
  - modify: internal/telemetry/access_diagnostics.go
  - modify: internal/telemetry/access_events.go
complexity: high
done_when:
  - "go build ./... exits 0"
  - "go test ./internal/daemon/... ./internal/telemetry/... passes"
  - "PurgeAndVacuum implemented (no longer returns not-implemented error)"
evidence_expected:
  - ".docs/auditoria/2026-06-09-milsp-v050-remediation/L3-verdict.yaml"
stop_if:
  - "async queue can drop events silently under load — must block or buffer, not drop"
```

## Reference
`discovery.yaml` PERF-06(19-20,449-628)/SEC-04(74,129)/SEC-05(274-479)/TOK-02/TOK-03. `internal/telemetry/access_events.go` para el shape de AccessEvent.

## Prompt
Editá SOLO tu set. No cambies `model/types.go` (L7 lo posee; el campo `DecisionHash` ya existe desde T2). Cambios:
1. PERF-06: introducí una cola async para writes de telemetría (un goroutine consumidor, canal bufferizado). Mantené el retry como respaldo. NO permitas drop silencioso: si el buffer se llena, bloqueá brevemente o crecé acotado.
2. SEC-04: `state.json`/`start.lock` a 0o600; dir daemon 0o700 (guardar Windows con build tags si aplica).
3. TOK-02: al construir `recent_accesses`, calculá `DecisionHash` (sha256 hex[:12] del decision_json) y dejá `decision_json` fuera de esa salida por defecto.
4. TOK-03: dedup de `workspace_root`/`entrypoint_path` — la función que arma la respuesta multi-evento debe emitirlos a nivel contenedor, no por item.
5. SEC-05: redacción opcional de paths en export (hash del workspace_root) detrás de un flag de export; documentá el default.
6. Implementá `PurgeAndVacuum` (reemplazá el stub de T2).

## Execution Procedure
1. `cd C:/wt/v050-l3-telemetry-store`; `git merge --no-edit main`.
2. Aplicá los cambios.
3. `go build ./... && go vet ./... && go test ./internal/daemon/... ./internal/telemetry/...`.
4. Commit. `L3-verdict.yaml`.

## Skeleton
```go
func (s *TelemetryStore) PurgeAndVacuum(retentionDays int, maxBytes int64) (int, bool, error) {
    days := retentionDays
    if sz, _ := s.dbSize(); sz > maxBytes { days = 7 }
    purged, err := s.purgeOlderThan(days)
    if err != nil { return purged, false, err }
    _, verr := s.db.Exec("VACUUM")
    return purged, verr == nil, verr
}
```

## Verify
`go test ./internal/daemon/... ./internal/telemetry/...` → PASS

## Commit
`feat(telemetry): async writes, 0o600 perms, decision hash, path dedup, purge+vacuum (PERF-05/06 SEC-04/05 TOK-02/03)`
