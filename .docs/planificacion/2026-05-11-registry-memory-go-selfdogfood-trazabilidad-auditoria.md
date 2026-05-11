# Trazabilidad y auditoria - registry, memoria y Go self-dogfood

```yaml
harness_protocol: SDD-HARNESS-v1
id: "2026-05-11-registry-memory-go-selfdogfood-trazabilidad-auditoria"
kind: "traceability-evidence"
audience: "dual"
imports:
  - '[[00_gobierno_documental]]'
  - '[[RF-WKS-004]]'
  - '[[RF-WKS-005]]'
  - '[[RF-IDX-001]]'
  - '[[RF-QRY-003]]'
  - '[[RF-QRY-007]]'
exports:
  - '2026-05-11-registry-memory-go-selfdogfood-trazabilidad-auditoria'
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/04_RF/RF-WKS-004.md
  - .docs/wiki/04_RF/RF-WKS-005.md
  - .docs/wiki/04_RF/RF-IDX-001.md
  - .docs/wiki/04_RF/RF-QRY-003.md
  - .docs/wiki/04_RF/RF-QRY-007.md
  - .docs/planificacion/2026-05-11-registry-memory-go-selfdogfood-trazabilidad-auditoria.md
agent_may_edit:
  - .docs/planificacion/2026-05-11-registry-memory-go-selfdogfood-trazabilidad-auditoria.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
  - .docs/raw/**
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - go test ./...
  - mi-lsp workspace prune --stale --dry-run --format toon
  - mi-lsp workspace status mi-lsp --full --format toon
  - mi-lsp nav workspace-map --workspace mi-lsp --axi --full --format toon
  - mi-lsp nav service internal/service --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-source --workspace mi-lsp --format toon
stop_if:
  - governance_blocked=true
  - go_test_failed=true
  - registry_prune_candidates_after_apply>0
  - harness_verdict=BLOCKED
  - wiki_source_verdict=BLOCKED
evidence:
  - .docs/planificacion/2026-05-11-registry-memory-go-selfdogfood-trazabilidad-auditoria.md
```

Fecha: 2026-05-11
Worktree: `C:\repos\mios\mi-lsp`
Branch: `feature/registry-memory-go-selfdogfood`
Base: `origin/main` / `d50bdbe3a823cb74687fbffdee533daed5917d7b`
Disposition: `pr-open` por regla del repo; no push directo a `main`.

## Scope cerrado

- Limpieza segura de registry/worktrees: `workspace prune --stale` con dry-run por default y apply explicito, registry-only.
- Memoria de reentrada: `workspace status --full` refresca snapshots stale cuando `auto_sync` esta habilitado y la gobernanza no esta bloqueada.
- Self-dogfood Go: deteccion de topologia Go, extractor AST Go, indexacion de `.go`, `workspace-map` con paquetes Go y `nav service` con `profile=go-package`.
- Binario instalado y daemon reiniciado: `C:\Users\fgpaz\bin\mi-lsp.exe`, daemon run `69`.

## Cadena SDD revisada

- Gobernanza: `.docs/wiki/00_gobierno_documental.md`, `.docs/wiki/_mi-lsp/read-model.toml`.
- WKS: `.docs/wiki/04_RF/RF-WKS-004.md`, `.docs/wiki/04_RF/RF-WKS-005.md`, `.docs/wiki/06_pruebas/TP-WKS.md`.
- IDX: `.docs/wiki/04_RF/RF-IDX-001.md`, `.docs/wiki/06_pruebas/TP-IDX.md`.
- QRY: `.docs/wiki/04_RF/RF-QRY-003.md`, `.docs/wiki/04_RF/RF-QRY-007.md`, `.docs/wiki/06_pruebas/TP-QRY.md`.
- Tecnica/contratos/datos: `.docs/wiki/07_baseline_tecnica.md`, `.docs/wiki/08_modelo_fisico_datos.md`, `.docs/wiki/09_contratos_tecnicos.md`.

## Evidencia

- `go test ./...`: PASS en todos los paquetes.
- `go build -o C:\Users\fgpaz\bin\mi-lsp.exe ./cmd/mi-lsp`: PASS.
- `mi-lsp workspace prune --stale --dry-run --format toon`: antes del apply encontro `candidates[25]`; despues del apply `candidates[0]`.
- `mi-lsp workspace prune --stale --apply --format toon`: PASS, `removed_count: 25`, `skipped[0]`, warning `no files or git worktrees were deleted`.
- `git worktree prune --dry-run -v` y `git worktree prune -v`: PASS idempotente, sin metadata stale que reportar.
- `mi-lsp workspace doctor --format toon`: PASS, `stale_paths[0]`; quedan duplicados de alias y familias de worktrees existentes como diagnostico no bloqueante.
- `mi-lsp workspace status mi-lsp --full --format toon`: PASS, `governance_blocked=false`, `governance_sync=in_sync`, `governance_index_sync=current`, `languages[2]: csharp,go`, `index_files: 174`, `index_symbols: 1979`.
- Primer `workspace status --full` tras el cambio: PASS, warning `refreshed stale reentry memory snapshot from docs index`.
- `mi-lsp index --workspace mi-lsp --clean --format toon`: PASS, `status=succeeded`, `docs=84`, `files=174`, `symbols=1979`.
- `mi-lsp nav workspace-map --workspace mi-lsp --axi --full --format toon`: PASS, `service_count: 16`, `total_symbols: 2005`, paquetes Go `internal/cli`, `internal/daemon`, `internal/indexer`, `internal/service`, `internal/workspace`, etc. con `profile: go-package`.
- `mi-lsp nav search "func New" --workspace mi-lsp --include-content --format toon`: PASS, contenido inline de funciones Go.
- `mi-lsp nav find New --workspace mi-lsp --format toon`: PASS, simbolos Go catalogados como `function`.
- `mi-lsp nav context internal/service/workspace_map.go 333 --workspace mi-lsp --format toon`: PASS, slice textual directo.
- `mi-lsp nav service internal/service --workspace mi-lsp --format toon`: PASS despues de reiniciar daemon; `backend: catalog`, `profile: go-package`, `sources[1]: catalog`, sin falsos endpoints .NET.
- `mi-lsp daemon restart --format toon`: PASS, daemon nuevo `pid=23424`, `run_id=69`.
- `mi-lsp nav governance --workspace mi-lsp --format toon`: PASS, `blocked=false`, `sync=in_sync`, `index_sync=current`.
- `mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon`: PASS, `harness_verdict: PASS`, `harness_contracts_reviewed: 83`.
- `mi-lsp nav wiki validate-source --workspace mi-lsp --format toon`: PASS, `wiki_source_verdict: PASS`, `navigation_readiness: ready`.

## Implementacion trazada

- Registry cleanup: `internal/cli/workspace.go`, `internal/workspace/registry.go`, `internal/service/workspace_ops.go`.
- Cross-workspace stale skip: `internal/service/app.go`.
- Memoria auto-refresh: `internal/service/workspace_ops.go`, `internal/service/workspace_memory_test.go`.
- Go catalog: `internal/indexer/extractor_go.go`, `internal/indexer/extractor_ts.go`, `internal/indexer/walker.go`, `internal/workspace/topology.go`.
- Go workspace-map/service: `internal/service/workspace_map.go`, `internal/service/service_exploration.go`.
- Cobertura: `internal/workspace/registry_test.go`, `internal/workspace/topology_test.go`, `internal/indexer/extractor_go_test.go`, `internal/service/workspace_memory_test.go`, `internal/service/workspace_map_test.go`, `internal/service/service_exploration_test.go`.

## Hallazgos de ps-trazabilidad

- PASS: gobernanza valida y proyeccion sincronizada antes y despues del reindex.
- PASS: cambios runtime/contrato sincronizados en `07`, `08`, `09`, RF y TP.
- PASS: registry cleanup no borra filesystem; solo muta `~/.mi-lsp/registry.toml` para roots inexistentes.
- PASS: memoria stale queda recuperable desde `workspace status --full` sin exigir reindex full manual.
- PASS: self-dogfood Go navega por `status`, `index`, `workspace-map`, `find`, `search`, `service` y `context`.
- WARN no bloqueante: `workspace doctor` conserva aliases duplicados para roots existentes; son diagnostico operacional, no drift de este scope.
- WARN no bloqueante: no hay issue/card asociado; waiver registrado por ejecucion directa en chat.

## Auditoria de ps-auditar-trazabilidad

- Findings criticos: 0.
- Findings high: 0.
- Drift spec-vs-code: no encontrado en el scope auditado.
- Harness: PASS.
- Wiki Source: PASS.
- Source/mirror skills: no aplica; no se editaron skills bajo `C:\Users\fgpaz\.agents\skills`.
- Binarios: actualizado `C:\Users\fgpaz\bin\mi-lsp.exe` y reiniciado daemon para evitar runtime viejo.
- Pre-push: requiere PR flow; no push directo a `main`.

## No-drift snapshot

- `mi-lsp nav governance --workspace mi-lsp --format toon`: PASS.
- `mi-lsp workspace prune --stale --dry-run --format toon`: PASS, `candidates[0]`.
- `git status --short --branch`: rama `feature/registry-memory-go-selfdogfood`, cambios solo dentro del scope esperado.

## Verdict

`Approved with PR-flow follow-up`: la implementacion, docs, binario instalado, daemon y evidencia local estan cerrados. La integracion a `origin/main` debe hacerse por branch/PR segun `AGENTS.md`.
