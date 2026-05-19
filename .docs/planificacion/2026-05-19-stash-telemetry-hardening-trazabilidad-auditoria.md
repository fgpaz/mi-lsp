# Trazabilidad y auditoria - integracion selectiva de stash telemetry/logs

```yaml
harness_protocol: SDD-HARNESS-v1
id: "2026-05-19-stash-telemetry-hardening-trazabilidad-auditoria"
kind: "traceability-evidence"
audience: "dual"
imports:
  - '[[00_gobierno_documental]]'
  - '[[TECH-DAEMON-GOBERNANZA]]'
  - '[[DB-STATE-Y-TELEMETRIA]]'
  - '[[CT-CLI-DAEMON-ADMIN]]'
exports:
  - '2026-05-19-stash-telemetry-hardening-trazabilidad-auditoria'
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/07_tech/TECH-DAEMON-GOBERNANZA.md
  - .docs/wiki/08_db/DB-STATE-Y-TELEMETRIA.md
  - .docs/wiki/09_contratos/CT-CLI-DAEMON-ADMIN.md
  - .docs/planificacion/2026-05-19-stash-telemetry-hardening-trazabilidad-auditoria.md
agent_may_edit:
  - .docs/planificacion/2026-05-19-stash-telemetry-hardening-trazabilidad-auditoria.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - go test ./...
  - dotnet test worker-dotnet\MiLsp.Worker.sln
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-source --workspace mi-lsp --format toon
stop_if:
  - governance_blocked=true
  - go_test_failed=true
  - dotnet_test_failed=true
  - harness_verdict=BLOCKED
  - wiki_source_verdict=BLOCKED
evidence:
  - .docs/planificacion/2026-05-19-stash-telemetry-hardening-trazabilidad-auditoria.md
  - internal/daemon/export_test.go
  - internal/daemon/log_tail_test.go
```

Fecha: 2026-05-19

## Resultado

Veredicto: PASS

Se revisaron los stashes preservados al limpiar worktrees deprecados y se integro solo la parte vigente del stash `cleanup/deprecated-worktree-release-log-audit-hardening-2026-05-18`.

## Stashes revisados

- `cleanup/deprecated-worktree-release-log-audit-hardening-2026-05-18`
- `cleanup/deprecated-worktree-workspace-execution-review-2026-05-18`

## Integrado

- `admin export --summary` sin `--limit` explicito ahora calcula el resumen de telemetria en streaming desde SQLite, evitando materializar todos los `access_events`.
- `daemon.db` se abre con una conexion SQLite serializada, `busy_timeout`, `synchronous=NORMAL`, WAL y nuevos indices idempotentes para filtros calientes de export.
- `daemon logs` y `/api/logs` filtran ruido benigno de cierre normal de sockets/pipes junto con el bloque de ayuda asociado.
- Docs sincronizadas en `07`, `08` y `09` para baseline, DB fisico y contrato CLI/admin.

## Descartado

- Artifacts `artifacts/release-regression*`: eran reportes pesados de corrida vieja y no son canon ni evidencia durable para este cambio.
- Cambios viejos de `internal/service/search.go`: ya estaban superados por la implementacion actual de ignores/default globs.
- Cambios conflictivos de docs del stash: se reemplazaron por una actualizacion minima alineada al estado vigente de `main`.
- Cambios de `scripts/release/regression-smoke.ps1`: el script actual ya evoluciono hacia smokes federados y path-status; integrar el bloque viejo hubiera mezclado dos contratos de reporte.

## Evidencia

- `go test ./internal/daemon ./internal/cli ./internal/service`: PASS
- `go test ./...`: PASS
- `dotnet test worker-dotnet\MiLsp.Worker.sln`: PASS
- `go run ./cmd/mi-lsp admin export --since 30d --summary --by-backend --percentile --format toon`: PASS
- `mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon`: PASS
- `mi-lsp nav wiki validate-source --workspace mi-lsp --format toon`: PASS

## Riesgo residual

- La optimizacion reduce materializacion de filas completas, pero los percentiles siguen necesitando latencias por bucket para preservar el contrato actual.
- `nav wiki trace` para soporte `TECH/DB/CT` sigue reportando `partial`: el harness compila, pero la superficie de trace actual no promueve `implements/tests` de frontmatter tecnico fuera de RF salvo wiki-source explicito.
- Los stashes originales se mantienen sin borrar hasta que el usuario confirme que ya no hace falta conservarlos.
