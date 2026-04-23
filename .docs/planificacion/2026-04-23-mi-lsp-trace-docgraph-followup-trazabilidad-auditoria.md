# mi-lsp trace/docgraph follow-up - Trazabilidad y auditoria

Fecha de ejecucion: `2026-04-23`

## Contexto y criterio de cierre

Este follow-up cerro el bug real reproducido desde `multi-tedi` donde `nav search`
y `nav trace` no encontraban IDs explicitos `RF-*` / `TP-*` aun despues de un
`index --docs-only` exitoso.

Decision de diseno reafirmada durante este cierre:

- la wiki no siempre vive bajo `.docs/wiki/*`
- `nav trace` no puede asumir rutas canonicas fijas
- el fallback a disco debe consultar primero las rutas gobernadas por
  `00_gobierno_documental.md` y `.docs/wiki/_mi-lsp/read-model.toml`
- los layouts legacy conocidos siguen vigentes solo como fallback de compatibilidad

Restricciones respetadas:

- no se edito canon de `multi-tedi`
- no se revirtieron cambios ajenos
- gobernanza `mi-lsp` valida antes de ejecutar cambios
- `ps-contexto` / `ps-trazabilidad` no estaban expuestos como comandos en esta
  sesion, asi que se uso equivalente manual con evidencia real

## Cambios implementados

### Search / trace runtime

- `internal/service/search.go`
  - el path de `rg` ahora incluye `--hidden`, con lo que `nav search` deja de
    perder docs repo-locales ocultos despues de un rebuild docs-only
- `internal/service/trace.go`
  - `TP-*` ya no cae por una ruta RF-only
  - `RF-*` puede tomar evidencia documental desde `06_pruebas`
  - `TP-*` resuelve bien titulos dentro de tablas/agregados TP
  - el fallback a disco para RF/TP consulta primero rutas funcionales gobernadas
    por `read-model`, y despues layouts legacy conocidos
- `internal/service/workspace_ops.go`
  - `workspace status` ahora advierte explicitamente el split state de
    `docs_index_ready=true` con `index_ready=false`

### Cobertura de regresion

- `internal/service/trace_test.go`
  - cobertura para `TP-*`
  - cobertura para usar evidencia TP en trazas RF
  - cobertura para fallback gobernado fuera de `.docs/wiki/*`
- `internal/service/app_test.go`
  - cobertura para el warning de `workspace status` despues de `--docs-only`

### Docs sincronizadas

- `.docs/wiki/06_pruebas/TP-QRY.md`
- `.docs/wiki/07_baseline_tecnica.md`
- `.docs/wiki/09_contratos_tecnicos.md`
- `.docs/wiki/09_contratos/CT-NAV-WIKI.md`

Los contratos y baseline dejaron de describir la solucion como si dependiera de
`.docs/wiki/*` fijo; ahora hablan de docs gobernados por `00` / `read-model` y
de fallback legacy solo cuando corresponde.

### Skill `mi-lsp` sincronizada

- `skills/mi-lsp/SKILL.md`
  - ahora declara explicitamente que `nav wiki search|route|pack|trace` es la
    superficie documental canonica
  - advierte que `nav search` es texto-first y puede devolver prompts,
    auditorias, `.docs/raw` u otros artefactos de soporte
  - agrega una regla operativa wiki-first para tareas documentales y de
    trazabilidad
- `skills/mi-lsp/references/quickstart.md`
  - agrega un loop corto de descubrimiento canonico usando
    `nav route -> nav wiki search -> nav wiki pack -> nav wiki trace`
  - deja explicito que `nav search` no decide autoridad documental
- `skills/mi-lsp/references/recipes.md`
  - agrega una receta de descubrimiento canonico/trazabilidad
- sincronizacion verificada en las tres superficies:
  - repo actual: `C:\repos\mios\mi-lsp\skills\mi-lsp`
  - source compartido local: `C:\Users\fgpaz\.agents\skills\mi-lsp`
  - mirror: `C:\repos\buho\assets\skills\mi-lsp`

## Verificacion

### Gobernanza

- `C:\Users\fgpaz\bin\mi-lsp.exe workspace status mi-lsp --format toon`: PASS
  - `governance_blocked=false`
  - `governance_sync=in_sync`
- `C:\Users\fgpaz\bin\mi-lsp.exe nav governance --workspace mi-lsp --format toon`: PASS

### Tests

- `go test ./internal/service -run "TestNavTraceFallsBackToGovernedRFPathOutsideDefaultWikiRoot|TestWorkspaceStatusWarnsWhenDocsOnlyRecoveryLeavesCodeCatalogAbsent|TestNavTraceUsesTPDocsAsCoverageEvidenceForRFAndTPIDs|TestSearchPatternRg_IncludesHiddenDocsPaths" -count=1`: PASS
- `go test ./...`: PASS
- `git diff --check`: PASS sin errores bloqueantes

### Repro en source

- `go run ./cmd/mi-lsp --workspace multi-tedi index --docs-only --format toon`: PASS
- `go run ./cmd/mi-lsp --workspace multi-tedi nav trace RF-GAS-10 --format toon`: PASS
  - `status: partial`
  - `title: RF-GAS-10 Dashboard de finanzas personales web`
- `go run ./cmd/mi-lsp --workspace multi-tedi nav trace TP-GAS-23 --format toon`: PASS
  - `status: partial`
  - `title: Ejecutar query first-party sin binding`

### Refresh de binario instalado

- se reconstruyo `dist\win-arm64\mi-lsp.exe`
- se reemplazo `C:\Users\fgpaz\bin\mi-lsp.exe`
- se reinicio el daemon
- `go version -m C:\Users\fgpaz\bin\mi-lsp.exe`: PASS
  - `GOARCH=arm64`
  - `vcs.revision=bb6ac9c72da9d80527eadbdd9878597d3d18b2d3`
  - `vcs.modified=true`
- `C:\Users\fgpaz\bin\mi-lsp.exe worker status --format toon`: PASS

### Repro desde binario instalado

- `C:\Users\fgpaz\bin\mi-lsp.exe workspace status multi-tedi --format json --full`: PASS
  - `docs_index_ready=true`
  - `index_ready=false`
  - warning visible:
    `code catalog is empty while documentation is ready; docs-only recovery rebuilt governed docs and memory_pointer, but nav.find/nav.symbols/semantic code features still need 'mi-lsp index --workspace multi-tedi'`
- `C:\Users\fgpaz\bin\mi-lsp.exe --workspace multi-tedi nav trace RF-GAS-10 --format toon`: PASS
- `C:\Users\fgpaz\bin\mi-lsp.exe --workspace multi-tedi nav trace TP-GAS-23 --format toon`: PASS
- `C:\Users\fgpaz\bin\mi-lsp.exe --workspace multi-tedi nav search RF-GAS-10 --include-content --format toon`: PASS
- `C:\Users\fgpaz\bin\mi-lsp.exe --workspace multi-tedi nav search TP-GAS-23 --include-content --format toon`: PASS

### Skill / mirror sync

- hash identico de `SKILL.md` entre:
  - `C:\Users\fgpaz\.agents\skills\mi-lsp\SKILL.md`
  - `C:\repos\mios\mi-lsp\skills\mi-lsp\SKILL.md`
  - `C:\repos\buho\assets\skills\mi-lsp\SKILL.md`
- hash identico de `references/quickstart.md` entre las tres superficies: PASS
- hash identico de `references/recipes.md` entre las tres superficies: PASS

### Push readiness manual-equivalente (`ps-pre-push`)

Contexto:

- branch actual: `main`
- `git fetch origin main`: PASS
- `git rev-list --left-right --count origin/main...HEAD`: `0 0`
  - no hay drift ni necesidad de force-push
- repo-local guard `infra/git/Invoke-PrePushGuard.ps1`: ausente
- board config `.pj-crear-tarjeta.conf`: ausente
- `.docs/raw/` tiene historico trackeado, pero en este slice no hay paths
  agregados/modificados bajo `.docs/raw/`: PASS
- `go test ./...`: PASS
- `git diff --check`: PASS sin errores bloqueantes; solo warnings LF/CRLF

Waiver operativa usada para el push directo a `main`:

- no hay issue/card ni board config en este repo
- el usuario pidio explicitamente subir directo a `main`
- se usa fallback manual del guard porque el script repo-local no existe

Nota de mirror:

- la sincronizacion source/mirror del skill `mi-lsp` esta hecha y validada por
  hash
- el repo `C:\repos\buho\assets` sigue teniendo cambios ajenos en `mi-key-cli`,
  asi que este artefacto no afirma push del mirror; solo afirma mirror sync
  local y que el push de `mi-lsp` no depende de mezclar esos cambios

## Trazabilidad manual-equivalente

Chequeos de cierre:

- Gobernanza: PASS
- Codigo y tests: PASS
- Docs `07/09`: PASS
- Regression multi-tedi docs-only: PASS
- Binario instalado refrescado y validado: PASS
- Compatibilidad con layouts no `.docs/wiki/*`: PASS por test y por contrato
- Skill `mi-lsp` alineada con wiki canonica: PASS
- Source/mirror sync del skill: PASS
- Push readiness manual-equivalente: PASS con waiver explicita por ausencia de
  board/guard repo-local

## Auditoria manual-equivalente

Hallazgos abiertos despues de este slice:

1. Bajo - `docs-only` sigue dejando `index_ready=false`
   - ya no se presenta como ambiguedad silenciosa
   - queda explicitado como comportamiento contractual y warning UX

2. Bajo - `nav search` sigue siendo texto-first
   - cumple el acceptance de dejar de devolver `0 matches`
   - no se trato en este slice reordenar resultados para que un doc gobernado
     gane siempre frente a prompts/auditorias no canonicas

3. Bajo - el mirror `C:\repos\buho\assets` no queda push-eado por este artefacto
   - hay cambios ajenos en `mi-key-cli` en ese repo
   - el sync del skill `mi-lsp` quedo validado localmente, pero el push del
     mirror requiere un cierre aislado de ese repo para no mezclar trabajo

Veredicto:

- `APPROVED`
- el bug pendiente de `multi-tedi` quedo cerrado en source, en tests y en el
  binario instalado de Windows
- el skill `mi-lsp` quedo sincronizado y endurecido para usar mejor la wiki
  canonica
- el push directo de este repo a `main` queda aprobado con waiver manual por
  ausencia de card/board y de guard repo-local
