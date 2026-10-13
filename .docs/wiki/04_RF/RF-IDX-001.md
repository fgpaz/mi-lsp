---
id: RF-IDX-001
title: Construir y refrescar el indice repo-local
implements:
  - internal/cli/index.go
  - internal/service/index_jobs.go
  - internal/indexer/indexer.go
  - internal/indexer/graph_pipeline.go
  - internal/indexer/extractor_go.go
  - internal/store/schema.go
  - internal/indexer/incremental.go
  - internal/store/index_jobs.go
  - internal/store/index_publish.go
  - internal/service/autoindex.go
tests:
  - internal/service/app_test.go
  - internal/store/store_test.go
  - internal/store/index_jobs_test.go
  - internal/store/index_lock_test.go
  - internal/indexer/indexer_progress_test.go
  - internal/indexer/incremental_graph_repair_test.go
  - internal/workspace/gitignore_test.go
  - internal/indexer/extractor_python_test.go
  - internal/indexer/extractor_rust_test.go
  - internal/workspace/rust_topology_test.go
  - internal/service/autoindex_test.go
---

```yaml
harness_protocol: SDD-HARNESS-v1
id: "RF-IDX-001"
kind: "support-doc"
audience: "llm-first"
imports:
  - '[[00_gobierno_documental]]'
  - '[[RF-IDX-001]]'
exports:
  - 'RF-IDX-001'
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/04_RF/RF-IDX-001.md
agent_may_edit:
  - .docs/wiki/04_RF/RF-IDX-001.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
evidence:
  - .docs/wiki/04_RF/RF-IDX-001.md
```

# RF-IDX-001 - Construir y refrescar el indice repo-local

## 1. Execution Sheet

| Campo | Valor |
|---|---|
| ID | RF-IDX-001 |
| Titulo | Construir y refrescar el indice repo-local |
| Actores | Desarrollador, Skill, CLI/Core, Indexer |
| Prioridad | alta |
| Severidad | alta |
| FL origen | FL-IDX-01 |

## 2. Detailed Preconditions

| Condicion | Tipo | Estado requerido |
|---|---|---|
| Workspace resoluble por path o alias | funcional | obligatorio |
| `<repo>/.mi-lsp/` escribible | tecnica | obligatorio |
| Reglas de ignore disponibles | operativa | obligatorio |

## 3. Process Steps (Happy Path)

1. La CLI resuelve el workspace objetivo y carga `project.toml`.
2. El indexer obtiene ignores desde defaults internos, `.gitignore`, `.milspignore` y `[ignore].extra_patterns`, respetando el orden del archivo y los re-includes negados (`!pattern`) sobre paths normalizados con `/`.
   Los ignores operacionales hard-coded, incluyendo `.mi-lsp/` y `**/.mi-lsp/**`, se aplican al final y no pueden ser re-habilitados por negaciones del repo.
3. La CLI crea un job durable y una generacion candidata en `index.db`.
4. Si `--clean` esta activo, fuerza recomposicion completa del modo elegido, sin borrar `index.db` antes de construir el nuevo resultado.
5. El walker enumera archivos y asigna ownership por `repo_id`.
6. El extractor prepara `workspace_repos`, `workspace_entrypoints`, `FileRecord`, `SymbolRecord` y `WorkspaceMeta`, reportando progreso vivo durante loops largos de catalogo/docs. En Go, conserva la ruta exacta del entrypoint relativa al workspace (incluido `runtime/go.mod`). El selector que consume el observador puede ser una ruta de módulo repo-local explícita (`go.mod`) o un ID de `WorkspaceEntrypoint`; un ID se resuelve con coincidencia exacta de repo y entrypoint, se rebasa su ruta workspace-relative al root del repo seleccionado y se revalida como `go.mod` repo-local seguro y regular. Si el selector repo-local está vacío, usa el fallback al `go.mod` del root; un ID desconocido o malformado, una selección explícita insegura, inexistente o no regular, incluido un symlink final, falla cerrado y queda como omisión/diagnóstico (`""`), sin activar el fallback. `go.work` no tiene extracción de grafo en este slice y un selector explícito `go.work` se rechaza.
7. El indexador documental prepara `DocRecord`, `DocEdge` y `DocMention` a partir de `.docs/wiki`, `README*`, `docs/` y `.docs/`.
8. El runtime publica catalogo, docs y memoria de reentrada en una unica transaccion SQLite para `mode=full`.
9. La CLI devuelve job status, stats y warnings no fatales si encontro ruido, archivos sin match de repo o problemas de docs.

## 4. Outputs

| Campo | Tipo | Destino | Efecto observable |
|---|---|---|---|
| `index.db` | archivo | repo local | catalogo derivado listo para discovery |
| `stats.files` | numero | usuario/skill | total procesado |
| `stats.symbols` | numero | usuario/skill | total persistido |
| `warnings` | lista | usuario/skill | sugerencias de limpieza, ownership dudoso o problemas de docs |
| `job_id` | string | usuario/skill | identificador durable para status/cancel |
| `generation_id` | string | workspace_meta | generacion publicada o candidata |
| `current_stage` / `current_path` / `files_total` | campos de job | usuario/skill | progreso vivo mientras el job corre |

## 5. Typed Errors

| Codigo | Causa | Trigger | Respuesta esperada |
|---|---|---|---|
| `IDX_WORKSPACE_NOT_FOUND` | workspace no resoluble | alias/path invalido | abortar sin crear indice |
| `IDX_WALK_FAILED` | fallo de exploracion | error de lectura en repo | abortar con mensaje y contexto |
| `IDX_DB_WRITE_FAILED` | fallo SQLite | error de apertura o escritura | abortar con error sin ocultar causa |

## 6. Special Cases and Variants

- El indice guarda ownership por repo incluso en `container`.
- Si el indice detecta ruido en `.docs`, `old/`, `temp/` u otros paths no ignorados, debe sugerir `.milspignore`.
- Si la wiki canonica existe en disco pero `doc_records` quedo solo con docs `generic`, un `index` incremental sin cambios detectados debe degradar a full re-index en vez de responder `no changes detected`.
- Si el índice incremental encuentra un grafo stale o inválido sin cambios de catálogo, intenta repararlo; cuando Roslyn no aporta un batch completo, publica un grafo documental si hay Markdown canónico e identidad de repositorio. Si no puede restaurar la frescura, el job falla con un error tipado y no responde `no changes detected` como éxito.
- Si el proceso cae antes del commit de publicacion, SQLite debe conservar el estado previo y el job queda diagnosticable como stale/failed en la siguiente inspeccion.
- Los entrypoints auxiliares bajo `.docs/` o `template(s)` no deben convertirse en el default semantico solo por estar presentes en el repo.
- El indice documental resuelve primero links markdown y doc IDs explicitos; las heuristicas solo completan contexto, no reemplazan trazabilidad explicita.
- Para readiness de `nav.find`, son válidas una generación de catálogo activa (`active_catalog_generation_id`) o el par de metadata `indexed_at` + `total_files`, escrito atómicamente por `ReplaceCatalog` aun cuando `GenerationID` esté vacío. `total_files=0` puede representar un catálogo completo vacío. Un esquema sin estos campos o metadata parcial no basta; no se inspeccionan filas para decidir completitud. Sin ambas señales, o si no se puede abrir/leer el estado del workspace, `nav.find` no falla en vacío: responde desde texto con `degraded=true` y `reason=index_not_ready` (o `index_schema_broken` si la base está corrupta o con esquema roto) y dispara la autosanación de la sección 6.1. Ninguna de estas señales garantiza frescura frente a cambios posteriores en disco.
- El indice nunca persiste refs profundas ni ASTs.
- Para Go (`.go`), el extractor usa `go/parser`/AST nativo para catalogar funciones, métodos, tipos, structs, interfaces, consts y vars. Un módulo anidado puede conservarse como entrypoint con ruta exacta relativa al workspace (por ejemplo, `runtime/go.mod`); el selector del observador puede ser la ruta repo-local explícitamente configurada (`go.mod`) o un ID de `WorkspaceEntrypoint`. Un ID se resuelve con coincidencia exacta de repo y entrypoint, se rebasa la ruta workspace-relative al root del repo seleccionado y se revalida como `go.mod` repo-local seguro y regular. IDs desconocidos o malformados, rutas inseguras, absolutas, con `..`, fuera del root, inexistentes, no regulares o con symlink final fallan cerrado y producen omisión/diagnóstico (`""`) sin activar el fallback; `go.work` se rechaza como selector.
- Si el selector Go repo-local está vacío, la selección permanece determinista en `go.mod` del root. Un root solo con `go.work` no selecciona Go ni se envía al parser de `go.mod`; el contenido raíz no relacionado sigue indexándose y conserva su ownership; el indexador no sustituye este fallback por descubrimiento recursivo arbitrario.
- Para Python (`.py`, `.pyi`), el extractor usa una pasada lexical acotada por lineas e indentacion. Prioriza catalogo navegable y cancelabilidad por archivo; formas no reconocidas quedan cubiertas por `nav search` textual o Pyright opcional.
- Para Rust (`.rs`), el extractor lexical acotado cataloga funciones, métodos, tipos, traits, módulos, constantes, estáticos y macros comunes sin compilar crates; formas no reconocidas conservan `FileRecord` y `nav search` textual.
- `index cancel` sin `--force` marca `requested_cancel`; el worker debe detenerse cooperativamente al volver al loop de indexacion. `--force` queda como escape para procesos colgados y debe limpiar el lock asociado si el PID ya no vive.
- `.mi-lsp/**` es estado operacional del workspace, no input indexable. El walker y los matchers deben excluirlo tambien cuando aparece anidado dentro de repos hijos o cuando `.gitignore` intenta re-incluirlo con `!`.
### 6.1 Autosanación del índice desde lecturas (`nav.find`)

- Disparadores: `nav.find` sin catálogo publicado, con `index.db` ilegible o con esquema roto (`index_not_ready` o `index_schema_broken`). La lectura responde de inmediato desde texto (`ok=true`, `degraded=true`, items `origin=text`; ver [[RF-QRY-001]]) y no espera al reindex.
- Un único job completo en segundo plano: se crea con el mecanismo de jobs durables existente y la deduplicación por ownership; si ya hay un job activo, el warning indica `background reindex already running` y no se lanza otro. El resultado se informa en `warnings` (`background reindex started|already running|skipped`).
- Recursos acotados: el proceso hijo corre con `GOMAXPROCS=2`, prioridad baja (`nice 19` en Unix; sin efecto en Windows) y un tope de pared dado por `MI_LSP_AUTOINDEX_TIMEOUT` (duración Go, default `10m`); un valor inválido usa el default. Un watchdog termina el proceso 30 s después del tope y registra el fallo en el marcador. El job se selecciona por root, sin depender de un alias del registry.
- Backoff: tras un fallo se escribe `<root>/.mi-lsp/autoindex.last` (`at`, `status`) y no se reintenta durante 10 minutos; en ese lapso la lectura sigue degradada a texto con `background reindex skipped`.
- Lecturas cross-workspace: si un caller harness lee un workspace distinto del que contiene su `caller_cwd` (RF-WKS-005), el reindex en segundo plano se omite (`background reindex skipped (cross-workspace read; ...)`) salvo `--allow-cross-workspace`, cuyo uso queda registrado en `~/.mi-lsp/overrides.log` como `index.autoindex`. La lectura igual responde degradada a texto.
- Override: `MI_LSP_AUTOINDEX=0` (también `false`, `off`, `no`) desactiva el job en segundo plano; las lecturas siguen cayendo a texto. Igual que `MI_LSP_REFS_TIMEOUT` y `MI_LSP_AUTOINDEX_TIMEOUT`, se lee en el proceso que ejecuta la consulta: si la consulta pasa por el daemon, cuenta el entorno con el que arrancó el daemon; para aplicarlo desde el cliente, usar `--no-daemon` o reiniciar el daemon con la variable.
- Nunca escribe el registry (`registry.toml`): la autosanación solo toca `<root>/.mi-lsp/`.
- Cuarentena: si `index.db` está corrupta, el write-side del índice la renombra a `index.db.corrupt-<unixts>` junto con sus `-wal` y `-shm`, crea una base nueva y reconstruye; conserva como máximo las 2 cuarentenas más recientes y solo opera dentro de `<root>/.mi-lsp/`. Un fallo al marcar el job como fallido se registra aunque el contexto haya vencido.
- Concurrencia: solo la corrupción real (`malformed`, `not a database` y similares) justifica la cuarentena; una tabla faltante se repara con `EnsureSchema`. El rename se serializa con `<root>/.mi-lsp/quarantine.lock` (creación exclusiva, caduca a 30 s) y se vuelve a sondear la base dentro del lock; un proceso sin lock nunca renombra. Errores transitorios de apertura o un job ya activo no escriben el marcador de backoff. Una `index.db` ausente, de 0 bytes o sin tablas se informa como `index_not_ready`, no como `index_schema_broken`.

### 6.2 Observación Go local explícita (RF-IDX-001-GO-OFFLINE)

La autoridad normativa de este slice está en el bloque siguiente; su evidencia manual está en [[TP-IDX]] / TC-IDX-029.

```toon
doc_id: RF-IDX-001
block_id: RF-IDX-001-GO-OFFLINE
kind: normative
source_of_truth: normative
contract_id: CT-CLI-DAEMON-ADMIN-GO-OFFLINE
opt_in: "--go-offline en index e index start; ausente conserva defaults, go_offline durable booleano"
public_routes: "wrapper direct wait, start direct wait y start detached con recarga durable en run-job"
daemon_route: "CLI index-daemon unsupported/noapplicable actual; App/server comparten dispatch interno, no paridad pública ni routing nuevo"
compatibility: "q-v1, init, autoindex y consultas gopls intactos; sin configurar el entorno del usuario"
process_environment: "El proceso Go recibe `GOTOOLCHAIN=local`, `GOPROXY=off`, `GOSUMDB=off`, `GOVCS=*:off`, `GOFLAGS` con un espacio (cero flags; anula también el valor persistido por `go env -w`) y `GOWORK=off`. Se conservan el resto del entorno y los caches locales existentes. Se ignoran todos los GOFLAGS heredados/configurados (incluidos tags, overlays, toolexec y modfile); los tags de la petición interna explícita siguen vigentes."
local_metadata: "`go list -buildvcs=false -deps -export -compiled -json -e -mod=readonly` puede compilar localmente y escribir cache de compilación, pero no descargar toolchain/módulos ni contactar sumdb o VCS. El módulo seleccionado se observa fuera de modo workspace para evitar escrituras de `go.work.sum`; reemplazos disponibles únicamente por `go.work` no se incorporan. No escribe `go.mod`, `go.sum` ni `go.work`: usa copias temporales propias de `go.mod`/`go.sum` mediante `-modfile` (readonly por sí solo no impide nuevas sumas). Esas copias se descartan al terminar; los reemplazos relativos conservan la base del módulo seleccionado."
importer: "El importador consume solo exports obtenidos con esa política y paquetes locales tipados; no usa el importador gc ambiental que podría lanzar otro Go. Falta de exports/dependencias implica parcialidad, nunca facts de compilador sustituidos por regex/AST."
missing_metadata: "Sin metadata utilizable: GPH_GO_OFFLINE_UNAVAILABLE y mensaje sanitizado. Con metadata parcial, algunos faltantes quedan en diagnóstico genérico/index_failed, type_check_error o external_target; go_offline_metadata_unavailable no es universal. Precisión específica offline explicit_incomplete, sin retry online, instalación, bootstrap ni facts completos falsos."
public_status: "El job expone `go_offline`; la observación efectiva agrega warning `go_offline=true` y fingerprint de configuración distinto. Se mantienen las capabilities de declaraciones/contains/imports/references/calls y su degradación real, sin una capability inventada de completitud offline."
scope: "Full offline fuerza recomposición en vez de aceptar el no-op incremental de una generación creada con otro modo. Docs/catalog y backends no Go no cambian: el flag no promete ausencia de red de embeddings u otros backends, ni sandbox de compiladores externos/cgo personalizados."
deduplication: "La deduplicación compara el `go_offline` solicitado con el persistido del job activo. Si difieren, rechaza con `ok=false` y `error.code=index_job_policy_mismatch`, mostrando job_id, active_go_offline y requested_go_offline sin secretos. El rechazo es simétrico (online→offline y offline→online), también en docs/catalog aunque no ejecuten Go: la política durable no equivale a una observación efectiva. No transforma ni cancela el job, no crea otro runner ni reintenta automáticamente; el caller puede reintentar manualmente después de su estado terminal. Si la política coincide, conserva la deduplicación existente y los defaults de los demás parámetros; no añade rechazo por mode/clean/entrypoint."
verification_basis: "FINAL_VERIFY1 FAIL histórico; RECHECK1 PASS policy / PARTIAL global; audit1 Changes requested, reparación editorial sin aceptación global"
evidence_scope: "Rechazo simétrico, dedupe coincidente, aislamiento durable y retry manual terminal comprobados; stdlib succeeded en bwrap con namespace distinto, solo lo y sin rutas"
evidence_limits: "No cero intentos de syscall, ejecución normal fuera de namespace, salud global ni precisión/capabilities externas completas"
status: explicit_incomplete
owner: coord-milsp
disclosure_due: antes de publicación acotada
followup_due: 2026-10-20
followup_authority: "Precisión y evidencia futura solo con objetivo separado; sin renovar presupuesto de este objetivo"
budget: "FINAL_VERIFY1/audit1/RECHECK1 agotado; sin recheck2 ni verificador pendiente"
preexisting_debt: "Harness anidado preexistente no migrado; sin PASS de Harness/wiki-source/P6/Graph ni guard global de push"
```

## 7. Data Model Impact

- `WorkspaceRepo`
- `WorkspaceEntrypoint`
- `SymbolRecord`
- `FileRecord`
- `DocRecord`
- `DocEdge`
- `DocMention`
- `WorkspaceMeta`
- `IndexJob`
- `IndexGeneration`

## 8. Test Traceability

- Manual con evidencia acotada: `TP-IDX / TC-IDX-029` (RF-IDX-001-GO-OFFLINE); FINAL_VERIFY1 FAIL histórico, RECHECK1 PASS policy / PARTIAL global, sin aceptación completa.
- Positivo: `TP-IDX / TC-IDX-001`
- Positivo: `TP-IDX / TC-IDX-012`
- Positivo: `TP-IDX / TC-IDX-025`
- Positivo: `TP-IDX / TC-IDX-026`
- Negativo: `TP-IDX / TC-IDX-027`
- Positivo: `TP-QRY / TC-QRY-172`
- Positivo: `TP-QRY / TC-QRY-173`
- Positivo: `TP-QRY / TC-QRY-174`
- Negativo: `TP-IDX / TC-IDX-003`
