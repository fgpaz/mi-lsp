# 08. Modelo fisico de datos

```yaml
harness_protocol: SDD-HARNESS-v1
id: "08_modelo_fisico_datos"
kind: "support-doc"
audience: "dual"
imports:
  - '[[00_gobierno_documental]]'
  - '.docs/wiki/08_modelo_fisico_datos.md'
exports:
  - '08_modelo_fisico_datos'
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/08_modelo_fisico_datos.md
agent_may_edit:
  - .docs/wiki/08_modelo_fisico_datos.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
evidence:
  - .docs/wiki/08_modelo_fisico_datos.md
```

## Proposito y alcance

Este documento resume los stores fisicos de `mi-lsp`, su ownership y el ciclo de vida de los datos.
El store repo-local persiste catálogo y grafo documental; el slice graph-native v1 ya agrega y usa generations, adjacency, evidence, unresolved, migration/rollback y analysis cache sin crear otro store autoritativo. El detalle físico vive en [[DB-SYMBOL-EDGE-GRAPH]].

## Inventario de stores

| Store | Ubicacion | Owner logico | Proposito |
|---|---|---|---|
| Workspace index DB | `<repo>/.mi-lsp/index.db` | Workspace owner | Catálogo, docgraph y graph-native v1 repo-local; SQLite es autoridad de adjacency |
| Workspace index lock | `<repo>/.mi-lsp/index.lock` | Workspace owner | Lock interproceso con PID owner para evitar dos indexaciones simultaneas y recuperar locks stale |
| Workspace index job logs | `<repo>/.mi-lsp/index-jobs/*.log` | Workspace owner | stdout/stderr de procesos detached de `index start` |
| Workspace config | `<repo>/.mi-lsp/project.toml` | Workspace owner | Overrides locales, ignores y topologia `single|container` |
| Workspace ignore file | `<repo>/.milspignore` | Workspace owner | Exclusiones repo-locales adicionales para el catalogo |
| Docs read model | `<repo>/.docs/wiki/_mi-lsp/read-model.toml` | Maintainer de wiki | Perfil de lectura y ranking docs-first por proyecto |
| Governance source | `<repo>/.docs/wiki/00_gobierno_documental.md` | Maintainer de wiki | Fuente humana del perfil y de la proyeccion ejecutable |
| Global registry | `~/.mi-lsp/registry.toml` | Core runtime | Aliases registrados, roots conocidos y `kind` |
| Daemon state | `~/.mi-lsp/daemon/state.json` | Runtime supervision | PID, pipe/socket, admin URL y protocolo |
| Daemon telemetry DB | `~/.mi-lsp/daemon/daemon.db` | Runtime supervision | Runs, runtime snapshots y access events locales |

## Estructuras fisicas relevantes

- `index.db`
  - `workspace_repos`
  - `workspace_entrypoints`
  - `files` con `repo_id`, `repo_name`, `content_hash`
  - `symbols` con `repo_id`, `repo_name`
  - `graph_generations` con identity/schema/fingerprints/digest/status/counts y prior pointer
  - `graph_nodes` con surrogate local, `NodeKey` BLOB(32), identity fields, owner, status y cross-RID
  - `graph_edges` con endpoints de la misma generation, relation, status, owner/backend y cross-RID
  - `graph_evidence` con subject node/edge, source range/digest, backend/version, claim y cross-RID
  - `graph_unresolved` con reason, selector digest, candidatos bounded (máximo 64 y 4096 bytes), recovery hint y contexto diagnóstico nullable (`source_document` repo-relative, `source_block`, `target_kind`, `target_value`) con máximo total de 4096 bytes
  - `graph_migrations` y `graph_analysis` para rollback durable y cache derivativo; no autoridad
  - `doc_records` con `path`, `doc_id`, `layer`, `family`, `search_text`, `content_hash`, `indexed_at`
  - `doc_edges` con `from_path`, `to_path`, `to_doc_id`, `kind` (`markdown_link`, `wikilink`, `embed`, `hierarchy`, doc-id), `label`
  - `doc_mentions` con `doc_path`, `mention_type`, `mention_value` y `source_block` opcional
  - `doc_source_blocks` con `doc_path` (`source_document`), `block_id` (`source_block`), `doc_id`, `kind`, `source_format`, `ordinal`, `start_line`, `end_line`, `content_hash`, `indexed_at`
  - `doc_source_records` con `doc_path` (`source_document`), `block_id` (`source_block`), `record_id`, `record_type`, `ordinal`, `start_line`, `end_line`, `content_hash`, `indexed_at`
  - `wiki_chunk_embeddings` con `doc_path`, `chunk_id`, `start_line`, `end_line`, `heading_text`, `snippet`, `content_hash`, `embedding` (BLOB float32 LE), `embedding_model`, `embedding_dim`, `indexed_at`
  - `index_jobs` con `job_id`, `generation_id`, workspace, `mode`, `status`, `phase`, `current_stage`, `current_path`, `files_total`, `pid`, `requested_cancel`, `error`, contadores y timestamps
  - `index_generations` con `generation_id`, `job_id`, workspace, `mode`, `status`, contadores, `created_at`, `published_at` y `error`
  - `workspace_meta` con `workspace_kind`, `default_repo`, `default_entrypoint`, `doc_count`, `memory_snapshot_json`, `memory_snapshot_built_at`, `active_*_generation_id`
- `index.lock`
  - JSON efimero con `pid`, `operation`, `started_at`; se crea con semantica exclusiva y se elimina al terminar la indexacion
- `daemon.db`
  - `runtime_snapshots` con `repo_name`, `repo_root`, `entrypoint_id`, `entrypoint_path`, `entrypoint_type`
  - `access_events` con `client_name`, `session_id`, `seq INTEGER DEFAULT 0`, `workspace_input`, `workspace_root`, `workspace_alias`, `repo`, `entrypoint_id`, `route`, `format`, `token_budget`, `max_items`, `max_chars`, `compress`, `error_kind`, `error_code`, `truncated`, `result_count`, `warning_count`, `pattern_mode`, `routing_outcome`, `failure_stage`, `hint_code`, `truncation_reason`, `decision_json`, `decision_hash` (v0.5.0+)
- `daemon status` / `/api/status`
  - `daemon_process` y `watchers` son snapshots derivados del proceso vivo; no se persisten como tablas nuevas.
- Rerank extension
  - no crea tablas nuevas; `[recall.rerank_extension]` vive en `.mi-lsp/project.toml`
  - no persiste query, snippets, stdin/stdout, stderr, provider responses, tokens ni secretos

## Cierre físico del puente wiki ↔ código

```toon
doc_id: 08_modelo_fisico_datos
block_id: 08_modelo_fisico_datos.wiki-code-binding-state
kind: physical-schema-contract
source_of_truth: this
status: implemented_slice
bindings:
  table: doc_artifact_bindings
  owner: doc_path
  identity: [doc_path, block_id, relation, target_path, target_symbol, target_kind, binding_ref]
  provenance: [authoring_origin, binding_status, doc_lifecycle, superseded_by, source_content_hash, start_line, end_line]
  indexes:
    - idx_doc_artifact_bindings_doc: [doc_path, block_id]
    - idx_doc_artifact_bindings_target: [target_path, target_symbol]
    - idx_doc_artifact_bindings_relation: [relation, target_kind]
    - idx_doc_artifact_bindings_ref: [binding_ref, unique]
artifact_states:
  table: doc_artifact_states
  primary_key: path
  fields: [size, mtime_nsec, content_sha256, parser_version, authority_config_hash, indexed_at, last_docs_gen_at, lifecycle]
  lifecycle_index: lifecycle
ownership:
  document_owned_tables: [doc_artifact_bindings, doc_source_records, doc_source_blocks, doc_mentions, doc_edges, doc_records, doc_artifact_states]
  replace_delete_scope: explicit_repo_relative_path
  shrink_guard: unrelated_owner_rows_must_remain_unchanged
generations:
  per_domain:
    docs: active_docs_generation_id
    memory: active_memory_generation_id
    catalog: active_catalog_generation_id
    graph: active_graph_generation_id
    graph_rollback: previous_graph_generation_id
  docs_refresh: [docs, memory]
  docs_refresh_preserves: [catalog]
  docs_refresh_marks: [graph_stale]
deletions:
  positive_proof: [disk_absent, scope_changed]
  excluded_alive: omission_without_delete
  retired: historical_state_preserved
query_access:
  snapshot: fixed_at_request_start
  sqlite: read_only
  writes: forbidden
  migrations_repairs_publication: forbidden
  derived_views: [catalog, graph, cache]
verify:
  - "FINAL_VERIFY e1835ee: go test ./... PASS (28 paquetes)"
  - "wiki-code-bridge-runner/v1: no_query_writes PASS"
evidence:
  - internal/store/schema.go
  - internal/store/doc_artifact_state.go
  - internal/store/queries_incremental.go
  - internal/store/index_publish.go
  - .docs/wiki/06_pruebas/TP-GPH.md
```

## Reglas de consistencia y retencion

- `index.db` debe tolerar reconstruccion completa con `mi-lsp index --clean` sin borrar el DB antes del publish.
- `index`, `index start`, `index run-job` y el auto-index de `init/add` deben tomar `.mi-lsp/index.lock` antes de caminar archivos, para cubrir tambien trabajos colgados antes de la transaccion SQLite.
- Si `index.lock` apunta a un PID inexistente, el siguiente index puede removerlo y continuar; si el PID sigue vivo, la operacion falla con owner visible.
- `index_jobs` es durable para observabilidad operacional; solo puede existir un job activo por workspace (`queued`, `running`, `publishing`, `cancel_requested`), y los jobs largos deben mantener fresco `updated_at` con `current_stage`, `current_path`, `files_total` y contadores parciales.
- `index_generations` registra el candidato de publish. Los punteros activos viven en `workspace_meta`: `active_catalog_generation_id`, `active_docs_generation_id`, `active_memory_generation_id` y `last_index_generation_id`.
- El graph-native v1 agrega `graph_schema_version`, `active_graph_generation_id` y `previous_graph_generation_id`; cada reader fija una generation y nunca mezcla snapshots.
- Staging graph-native es invisible; valida NodeKey, colisiones, endpoints, evidence, digests y cross-RID antes del compare-and-swap atómico del pointer. `graph_unresolved` conserva `source_document`, `source_block`, `target_kind` y `target_value` como contexto diagnóstico bounded/sanitized, excluido de `unresolved_key`, `Cross-RID` y del ordinal local.
- Aunque son diagnósticos, `source_document`, `source_block`, `target_kind` y `target_value` participan, con framing determinista y orden estable, en los digests de contenido y facts que alimentan los fingerprints de source/config/backend y el `generation_id`; permanecen fuera de `UnresolvedKey` y `CrossRID`.
- La observación Go en una topología `single` o repo seleccionada usa `DefaultEntrypoint` como un selector de módulo repo-local explícito (`go.mod`) o como un ID de `WorkspaceEntrypoint`. Un ID se resuelve por coincidencia exacta de repo y entrypoint, se rebasa desde su ruta workspace-relative al root del repo seleccionado y se revalida como un `go.mod` repo-local, seguro y regular. Un selector explícito inseguro (absoluto, con `\`, `..`, basename inválido, symlink, directorio, archivo inexistente u otra entrada no regular), o un ID desconocido o malformado, falla cerrado y omite Go sin activar el fallback. El fallback a `go.mod` en la raíz del repo solo aplica cuando el selector está vacío; un root solo con `go.work` no selecciona módulo ni intenta extracción. `go.work` se rechaza como selector de grafo. En `container` se consulta la raíz del workspace con ese fallback y no se elige un módulo por timestamp ni se acepta una ruta absoluta o fuera del root.
- Un selector Go explícito con `\` se rechaza antes de cualquier normalización de separadores; solo el selector vacío habilita el fallback a `go.mod` raíz.
- La migración graph-native es aditiva y transaccional. Dual-read/write existe solo durante una ventana explícita; query-time migration y conversión destructiva están prohibidas.
- Crash/cancel limpia solo staging incompleto y conserva/restaura la generation válida anterior. Retención mantiene active + rollback probado durante la ventana de release.
- `graph_analysis` es cache derivativo keyeado por generation/extension/params/authority digest; MILX y packs no escriben nodes/edges primarios.
- La ruta full fenced (`ReplaceWorkspaceIndexForJob`) reemplaza catálogo, grafo documental y memoria de reentrada en una única transacción SQLite cuando recibe la publicación graph; la ruta foreground publica el grafo con `PublishGraphObservationBatches` antes de `ReplaceWorkspaceIndex`, por lo que sus transacciones son separadas. Un crash antes de cada commit conserva la generación activa previa.
- La publicación `docs` reemplaza docs + memoria en una única transacción y no toca `files`, `symbols`, `workspace_repos` ni `workspace_entrypoints`.
- La publicación `catalog` reemplaza solo catálogo de código y no toca docs ni memoria.
- Las migraciones aditivas de `index.db` deben crear `repo_id` y `repo_name` en `files`/`symbols` antes de crear índices que dependan de esas columnas.
- `doc_records`, `doc_edges`, `doc_mentions`, `doc_source_blocks`, `doc_source_records` y `doc_artifact_bindings` deben refrescarse como un bloque consistente dentro de una sola transacción; `source_block` conserva la proveniencia del bloque que declaró la mención.
- `mi-lsp index --docs-only` puede ejecutar `ReplaceWorkspaceDocs` sin tocar `files`, `symbols`, `workspace_repos` ni `workspace_entrypoints`.
- Las tablas `doc_source_*` son aditivas y reconstruibles; no requieren migracion destructiva ni `PRAGMA user_version`.
- El snapshot repo-local de reentrada (`memory_snapshot_json`) se reconstruye en `mi-lsp index`; `workspace status --full` puede refrescarlo con una pasada docs-only solamente cuando el snapshot esta stale, `auto_sync` esta habilitado y la gobernanza no esta bloqueada.
- `project.toml` debe poder reescribirse al volver a detectar topologia del workspace.
- El `read-model.toml` no se copia a SQLite; se usa en lectura y sus cambios disparan re-index completo del corpus documental.
- `00_gobierno_documental.md` tampoco se persiste dentro de SQLite; su estado gobierna bloqueo/sync e invalida el indice cuando cambia.
- El store global del daemon no duplica el catalogo repo-local; solo guarda supervision y telemetria.
- `registry.toml` puede contener multiples aliases para un mismo root; `workspace list` debe preservar el alias registrado y `workspace list --group-by-root` solo debe agruparlos para diagnostico.
- `workspace prune --stale --apply` puede remover del `registry.toml` solo aliases cuyo `root` ya no existe en disco; si el alias removido era `defaults.last_workspace`, tambien limpia ese puntero. La operacion no elimina worktrees, carpetas, `.mi-lsp/index.db` ni otros archivos repo-locales.
- Worktrees de un mismo repositorio comparten `git common dir` pero tienen `workspace_root` fisico distinto; cada worktree mantiene su propio `.mi-lsp/index.db`, watcher y runtime identity.
- `runtime_snapshots` representan solamente runtimes observables del `daemon_run_id` vigente.
- `access_events` registran metadata y nunca payloads completos.
- Las fallas del hook de rerank solo pueden aumentar contadores/warnings sanitizados; no deben copiar payloads del hook a `decision_json`, logs o tablas.
- `workspace_input` guarda el selector crudo recibido; `workspace`, `workspace_alias` y `workspace_root` representan la identidad resuelta del workspace y no deben degradarse a `unscoped` si la operacion eligio un alias real.
- `decision_json` existe para debugging causal local y debe permanecer sanitizado: sin `pattern` crudo, sin argv, sin snapshot completo del request y sin el texto/comandos completos del bloque `coach`.
- `decision_json` puede incluir solo derivaciones de `continuation` y `memory_pointer` (`continuation_present`, `continuation_reason`, `continuation_op`, `memory_pointer_present`, `memory_stale`) y metadatos diagnosticos como `doc_ranker` / `intent_mode`; nunca `why`, `query`, `handoff` ni el contenido completo del snapshot repo-local.
- `decision_json` puede incluir campos backend/fallback derivados (`requested_backend`, `result_backend`, `backend_fallback_taken`, `fallback_from`, `fallback_to`, `runtime_error_code`) para diagnostico de harnesses, pero nunca paths crudos, `slice_text`, errores raw de worker, query ni payload completo.
- `decision_json` puede incluir metadatos derivados del safe-degrade planner (`planner_path`, `planner_outcome`, `safe_degrade_reason`, `guardrail_trigger`) para explicar por que una request eligio preview, expandio con `--full`, cambio a busqueda literal/regex, degrado de daemon/semantica a texto o pidio reindex. Estos campos son diagnosticos, no payload.
- `daemon.db` usa WAL mode, `busy_timeout` y retry/backoff breve en escrituras para manejar contencion transitoria entre daemon y CLI directo.
- `.mi-lsp/index.db` repo-local tambien usa WAL mode + `busy_timeout`, y las escrituras se serializan por workspace para evitar contencion entre watcher e index manual.
- Las lecturas documentales criticas (`ListDocRecords`, `FindDocRecordsBySourceID`, `ListDocSourceBlocks`, `ListDocSourceRecords`, FTS doc search y helpers relacionados) aplican retry/backoff breve ante `database is locked` / `SQLITE_BUSY`; si el lock persiste, el error sigue visible.
- Ante corrupcion de `index.db`, el runtime debe cuarentenar el archivo previo y reconstruir uno nuevo en el mismo workspace.
- Auto-purge elimina eventos y runs con mas de 30 dias en startup de CLI y daemon; debe ejecutarse con `VACUUM` para recuperar espacio en `daemon.db`.
- La fila canonica de una request `route=daemon` la escribe el daemon.
- Backpressure daemon-aware usa la telemetria existente: `error_kind=daemon`, `error_code=backpressure_busy`, `success=false` y warning `daemon/backpressure_busy`.
- Permisos de archivo: `~/.mi-lsp/daemon/daemon.db` y `state.json` deben crearse con permisos 0o600 (rw--- para owner); `~/.mi-lsp/daemon/` debe tener 0o700 (rwx--- para owner). Esto protege exposicion de `admin_url` y `PID` a otros usuarios del mismo host.
- `decision_hash` es un campo v0.5.0+ que guarda un hash corto del JSON `decision_json` para tracking de patrones de decision y deduplicacion sin overhead de almacenamiento directo del JSON; es opcional en migrations legacy.
- La CLI directa solo graba `access_events` cuando la request se sirve como `direct`, `direct_fallback` o falla antes de delegarse al daemon; esos eventos pueden llevar `daemon_run_id = NULL`.
- `runtime_key` debe persistirse tambien en filas `route=direct` o `route=direct_fallback` para que `admin export` pueda atribuir uso de queries directas a un workspace/backend/entrypoint estable; la clave usa `(workspace_root, backend_type, entrypoint_id)` y preserva alias/input en campos separados para display y forensics.
- `result_count` representa los items emitidos en el envelope final; `warning_count` se persiste como contador explicito para que summary/CSV no dependan de re-hidratar `warnings_json`.
- Filas duplicadas historicas de requests daemonizadas pueden existir como artefactos previos al fix de ownership de telemetria y deben tratarse como legacy hasta que la retencion las purgue.

## Operaciones clave en `index.db`

### Queries

- `SymbolContainingLine(file, line)`: devuelve el símbolo más chico que encierra un archivo + línea dados. Usado por `nav context` y `nav diff-context`.
- Graph selectors: lookup por `node_key`/cross-RID y scoped-name dentro de una generation fija.
- Graph adjacency: frontiers inbound/outbound por `(generation, endpoint, relation)` con depth/result/token bounds.
- Graph evidence/unresolved: lookup por subject u owner/reason; siempre generation-aware. Las omisiones unresolved se limitan a 50 filas y exponen contexto tipado saneado cuando está disponible.
- La propuesta `symbol_edges` queda supersedida por `graph_generations/nodes/edges/evidence/unresolved`; no se usa como fuente autoritativa de migración.
- `ListDocRecords()`: devuelve el corpus documental ordenado por familia/capa.
- `DocEdgesFrom(path)`: devuelve relaciones explicitas salientes para priorizar supporting docs.
- `DocMentionsForPath(path)`: devuelve menciones a codigo o comandos derivadas de un documento.
- `CountDocRecords()`: expone `doc_count` para `workspace status`.
- `FindDocRecordsByMention(type, value)`: permite resolver IDs embebidos dentro de documentos agregados bajo `04_RF/`.
- Las queries documentales anteriores deben preservar fallo visible si agotan el retry de lock; no deben ocultar corrupcion, SQL invalido ni errores permanentes como si fueran contencion transitoria.

### Transacciones

- `ReplaceFileSymbols(file_id, symbols)`: DELETE todos los simbolos del file, luego INSERT los nuevos. Usado para re-indexing incremental.
- `DeleteFileSymbols(file_id)`: DELETE simbolos y file record para archivos eliminados. Respeta `content_hash` para dedup.
- `ReplaceDocs(docs, edges, mentions)`: reemplaza el snapshot documental completo y actualiza `workspace_meta.doc_count`.
- `SaveReentrySnapshot(snapshot)`: persiste `memory_snapshot_json` y `memory_snapshot_built_at` en `workspace_meta` al final de una indexacion exitosa.
- `ReplaceWorkspaceIndex(generation_id, ...)`: publica catalogo, docs, memoria y punteros de generacion en una unica transaccion.
- `ReplaceWorkspaceDocs(generation_id, ...)`: publica docs, memoria y punteros docs/memory en una unica transaccion.
- `ReplaceWorkspaceCatalog(generation_id, ...)`: publica solo catalogo y puntero catalog.
- `StageGraphGeneration(ctx, db, bundle)` y `StageGraphGenerationTx(ctx, tx, bundle)`: validan y persisten un bundle graph-native sellado; solo el refresco gráfico incremental y los cambios de código reensamblan el bundle completo antes de staging.
- `PublishIncrementalDocsGeneration(...)` y su variante fenced reemplazan solo los owners documentales explícitos, avanzan docs/memoria, marcan el grafo stale y no hacen staging de una generación graph-native.
- `ValidateGraphGeneration(...)`: sella counts/digest y rechaza colisión, dangling, stale evidence, contexto diagnóstico inválido o conflicto de cross-RID.
- `ActivateGraphGeneration(expected_prior, next)`: compare-and-swap atómico; actualiza active/previous y estados.
- `RecoverGraphState()`: resuelve migration/staging/pointer no terminal sin elegir por timestamp y conserva el contexto tipado de las filas válidas.
- `index_jobs`: `CreateIndexJob`, `MarkIndexJobRunning`, `MarkIndexJobProgress`, `MarkIndexJobSucceeded`, `MarkIndexJobFailed`, `RequestIndexJobCancel`, `CancelIndexJob`.

## Riesgos operativos observados

- Repos con `.docs` o templates que incluyen `.csproj` pueden aparecer como entrypoints visibles si no estan ignorados.
- La heuristica de bootstrap no debe elegir esos entrypoints auxiliares como `default_entrypoint` si existe una solucion o proyecto real fuera de `.docs/template(s)`.
- El mecanismo recomendado para ese ruido sigue siendo `.milspignore` o `[ignore].extra_patterns`.
- `.worktrees/` y `.docs/temp/worktrees/` ya forman parte de los ignores internos y no deben volver a elegirse como entrypoint ni corpus documental del workspace activo.
- Cambios parciales en docs pueden dejar ranking inconsistente si se mezclan con incremental por archivo; por eso el cambio de docs/profile fuerza full re-index.
- Si `doc_count=0` pero existe `.docs/wiki`, las consultas docs-first pueden anclarse por Tier 1, pero el workspace debe considerarse documentalmente incompleto hasta ejecutar `index --docs-only` o un full index exitoso.
- `daemon.db` es append-heavy y de lectura diagnostica; debe sostener exports de ventanas completas con indices y agregacion streaming, evitando materializar todos los eventos cuando el usuario pide summary sin `--limit`.

## Documentos detalle

- [DB-STATE-Y-TELEMETRIA.md](08_db/DB-STATE-Y-TELEMETRIA.md)
- [DB-DOC-INDEX.md](08_db/DB-DOC-INDEX.md)
- [DB-SYMBOL-EDGE-GRAPH.md](08_db/DB-SYMBOL-EDGE-GRAPH.md)
- [DB-WIKI-EMBEDDINGS.md](08_db/DB-WIKI-EMBEDDINGS.md)

## Change triggers graph-native

Actualizar `08`, [[DB-SYMBOL-EDGE-GRAPH]], `05_modelo_datos.md`, RF/TP y contratos cuando cambien NodeKey/edge identity, schema/indexes, publication visibility, migration/rollback, retention, query access pattern, global snapshot cache o MILX analysis persistence.
