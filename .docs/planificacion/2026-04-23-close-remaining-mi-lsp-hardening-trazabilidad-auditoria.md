# Close Remaining `mi-lsp` Hardening - Trazabilidad y Auditoria

Fecha de ejecucion: `2026-04-23`

## Alcance cerrado

Se cerraron los dos slices pendientes definidos por el plan `2026-04-23-close-remaining-mi-lsp-hardening`:

1. docs/ranking/tokenization/fallback documentation
2. index jobs cancelation force path + `nav trace` disk fallback

Guardrails respetados durante la ejecucion:

- gobernanza `mi-lsp` valida (`spec_backend`, projection in sync)
- worktree limitado al dirty set congelado de 18 paths mas:
  - `.docs/raw/plans/2026-04-23-close-remaining-mi-lsp-hardening/*`
  - este artefacto de cierre
- smokes de Windows usando `C:\Users\fgpaz\bin\mi-lsp.exe` desde cwd neutral
- smokes de WSL usando rutas explicitas
- sin inventar canon `RF-GAS-09/10` dentro de `mi-lsp`

## Cambios implementados

### Slice 1 - Docs/ranking

- `internal/docgraph/docgraph.go`
  - la tokenizacion conserva tokens canonicos cortos: `RF`, `FL`, `TP`, `CT`, `DB`, `API`, `SDK`, `UX`, `UI`, `OIDC`
- `internal/service/doc_ranking.go`
  - cuando existe un candidato canonico positivo bajo `.docs/wiki/`, el scorer penaliza artefactos de soporte bajo `.docs/raw/` para que no ganen el documento primario
- `internal/service/owner_ranking_test.go`
  - cobertura para `nav ask`, `nav route`, `nav pack` y `nav.intent` demostrando que `.docs/raw/*` no gana sobre `.docs/wiki/*`
  - cobertura unitaria del tokenizer para tokens cortos canonicos
- docs sincronizadas:
  - `.docs/wiki/04_RF/RF-QRY-010.md`
  - `.docs/wiki/06_pruebas/TP-QRY.md`
  - `.docs/wiki/07_baseline_tecnica.md`
  - `.docs/wiki/09_contratos_tecnicos.md`

### Slice 2 - Index jobs / trace

- `internal/cli/index.go`
  - `index cancel` ahora expone `--force`
- `internal/service/index_jobs.go`
  - el payload `force` se propaga al servicio
  - la surface devuelve warning explicito cuando se usa force cancel
  - `phase=indexing` se mantiene durante el trabajo pesado y `publishing` se mueve al cierre final
- `internal/store/index_jobs.go`
  - `CancelIndexJob(..., force)` termina el PID vivo cuando existe y marca el job como `canceled`
- `internal/store/process_terminate_unix.go`
- `internal/store/process_terminate_windows.go`
  - terminacion de proceso platform-specific
- `internal/store/index_jobs_test.go`
  - prueba end-to-end del force cancel con proceso vivo
- `internal/service/trace.go`
  - `nav trace` hace fallback a disco para:
    - `.docs/wiki/04_RF.md`
    - `.docs/wiki/04_RF/*.md`
    - `.docs/wiki/RF/*.md`
    - `.docs/wiki/RF.md`
- `internal/service/trace_test.go`
  - cobertura para fallback `04_RF/*.md`
  - cobertura para fallback legacy `RF/*.md`
  - cobertura explicita para fallback legacy root `RF.md`

## Verificacion

### Repo base

- `mi-lsp workspace status mi-lsp --format toon`: PASS
  - `governance_blocked=false`
  - `governance_profile=spec_backend`
  - `docs_index_ready=true`
- `mi-lsp nav governance --workspace mi-lsp --format toon`: PASS

### Test suite

- `go test ./internal/service`: PASS
- `go test ./internal/store`: PASS
- `go test ./...`: PASS

### Git hygiene

- `git diff --check`: WARN no bloqueante
  - solo aparecen warnings LF/CRLF en archivos ya modificados
  - no se reportaron whitespace errors bloqueantes

### CLI / runtime checks

- `C:\Users\fgpaz\bin\mi-lsp.exe index cancel --help`: PASS
  - muestra `--force`
- `C:\Users\fgpaz\bin\mi-lsp.exe --workspace multi-tedi index status --format toon`: PASS
  - ultimo job `docs`, `status=succeeded`, `phase=done`
- `C:\Users\fgpaz\bin\mi-lsp.exe nav wiki trace RF-QRY-016 --workspace mi-lsp --format toon`: PASS
  - `status=implemented`
  - links explicitos y tests verificados
- `C:\Users\fgpaz\bin\mi-lsp.exe nav wiki search "RF IDX" --workspace mi-lsp --format toon`: PASS
  - top results permanecen bajo `.docs/wiki/*`

### External fallback proof

- `C:\Users\fgpaz\bin\mi-lsp.exe --workspace gastos nav trace RF-GAS-10 --format json`: WARN esperado
  - respuesta `ok=true` con warning `RF "RF-GAS-10" not found in doc index`
  - verificacion filesystem en `C:\repos\mios\gastos\.docs`: `NO_MATCHES`
  - conclusion: no habia canon externo presente para resolver en ese workspace; no se invento canon nuevo

### Docs-first reranking proof

- Windows
  - `C:\Users\fgpaz\bin\mi-lsp.exe --workspace interbancarizacion_coelsa nav ask "...wiki-to-code parity audit..." --format json`: PASS
  - `primary_doc=.docs/wiki/07_tech/TECH-CONFIG.md`
  - el primer `doc_evidence` tambien queda bajo `.docs/wiki/*`
- WSL
  - `/home/fgpaz/.local/bin/mi-lsp --workspace interbancarizacion_coelsa nav ask "...wiki-to-code parity audit..." --format json`: PASS
  - `/home/fgpaz/bin/mi-lsp --workspace interbancarizacion_coelsa nav ask "...wiki-to-code parity audit..." --format json`: PASS
  - una corrida paralela inicial devolvio `disk I/O error (4618)` desde `~/.local/bin`; el rerun aislado paso, por lo que se clasifica como contencion transitoria y no como regression deterministica

## Distribucion baseline verificada

### Windows

- `C:\Users\fgpaz\bin\mi-lsp.exe`
  - revision `335388d8c767c882e686d04a1b71231876a68e11`
- `C:\repos\mios\mi-lsp\mi-lsp.exe`
  - revision `888baa181baafcd22ee94eaa5a417127ea24bf4c`
  - stale, shadow risk desde repo root
- `C:\repos\mios\mi-lsp\dist\win-arm64\mi-lsp.exe`
  - revision `335388d8c767c882e686d04a1b71231876a68e11`

### WSL

- `/home/fgpaz/.local/bin/mi-lsp`
  - revision `335388d8c767c882e686d04a1b71231876a68e11`
- `/home/fgpaz/bin/mi-lsp`
  - revision `335388d8c767c882e686d04a1b71231876a68e11`
- `/home/fgpaz/go/bin/mi-lsp`
  - revision `97433cf59020948139b6407dd97e8e19863fd64d`
  - stale

## Estado de cierre

Resultado tecnico: PASS con un WARN externo no bloqueante (`gastos` sin canon `RF-GAS-09/10`) y un WARN operativo transitorio no reproducible (`disk I/O error (4618)` en corrida WSL paralela).

Listo para:

- branch `hardening/close-remaining-doc-ranking-index-trace`
- commit del dirty set permitido
- PR unico
- merge a `main`
- refresh post-merge de binarios/daemon desde un arbol limpio
