# 08. Modelo fisico de datos

## Proposito y alcance

Este documento resume los stores fisicos de `mi-lsp`, su ownership y el ciclo de vida de los datos.
La novedad de v1.3 es que el store repo-local persiste tambien el grafo documental de `.docs/wiki` y el runtime considera cambios de docs, `00_gobierno_documental.md` o `read-model` como disparadores de full re-index.

## Inventario de stores

| Store | Ubicacion | Owner logico | Proposito |
|---|---|---|---|
| Workspace index DB | `<repo>/.mi-lsp/index.db` | Workspace owner | Catalogo repo-local de simbolos, archivos, repos, entrypoints y docs |
| Workspace index lock | `<repo>/.mi-lsp/index.lock` | Workspace owner | Lock interproceso para evitar dos indexaciones simultaneas |
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
  - `doc_records` con `path`, `doc_id`, `layer`, `family`, `search_text`, `content_hash`, `indexed_at`
  - `doc_edges` con `from_path`, `to_path`, `to_doc_id`, `kind`, `label`
  - `doc_mentions` con `doc_path`, `mention_type`, `mention_value`
  - `workspace_meta` con `workspace_kind`, `default_repo`, `default_entrypoint`, `doc_count`, `memory_snapshot_json`, `memory_snapshot_built_at`
- `index.lock`
  - JSON efimero con `pid`, `operation`, `started_at`; se crea con semantica exclusiva y se elimina al terminar la indexacion
- `daemon.db`
  - `runtime_snapshots` con `repo_name`, `repo_root`, `entrypoint_id`, `entrypoint_path`, `entrypoint_type`
  - `access_events` con `client_name`, `session_id`, `seq INTEGER DEFAULT 0`, `workspace_input`, `workspace_root`, `workspace_alias`, `repo`, `entrypoint_id`, `route`, `format`, `token_budget`, `max_items`, `max_chars`, `compress`, `error_kind`, `error_code`, `truncated`, `result_count`, `warning_count`, `pattern_mode`, `routing_outcome`, `failure_stage`, `hint_code`, `truncation_reason`, `decision_json`
- `daemon status` / `/api/status`
  - `daemon_process` y `watchers` son snapshots derivados del proceso vivo; no se persisten como tablas nuevas.

## Reglas de consistencia y retencion

- `index.db` debe tolerar reconstruccion completa con `mi-lsp index --clean`.
- `index.run` y el auto-index de `init/add` deben tomar `.mi-lsp/index.lock` antes de caminar archivos, para cubrir tambien trabajos colgados antes de la transaccion SQLite.
- Las migraciones aditivas de `index.db` deben crear `repo_id` y `repo_name` en `files`/`symbols` antes de crear indices que dependan de esas columnas.
- `doc_records`, `doc_edges` y `doc_mentions` deben refrescarse como un bloque consistente dentro de una sola transaccion.
- `mi-lsp index --docs-only` puede ejecutar `ReplaceDocs` y `SaveReentrySnapshot` sin tocar `files`, `symbols`, `workspace_repos` ni `workspace_entrypoints`.
- El snapshot repo-local de reentrada (`memory_snapshot_json`) se reconstruye en `mi-lsp index`, no en cada query interactiva.
- `project.toml` debe poder reescribirse al volver a detectar topologia del workspace.
- El `read-model.toml` no se copia a SQLite; se usa en lectura y sus cambios disparan re-index completo del corpus documental.
- `00_gobierno_documental.md` tampoco se persiste dentro de SQLite; su estado gobierna bloqueo/sync e invalida el indice cuando cambia.
- El store global del daemon no duplica el catalogo repo-local; solo guarda supervision y telemetria.
- `registry.toml` puede contener multiples aliases para un mismo root; `workspace list` debe preservar el alias registrado.
- `runtime_snapshots` representan solamente runtimes observables del `daemon_run_id` vigente.
- `access_events` registran metadata y nunca payloads completos.
- `workspace_input` guarda el selector crudo recibido; `workspace`, `workspace_alias` y `workspace_root` representan la identidad resuelta del workspace y no deben degradarse a `unscoped` si la operacion eligio un alias real.
- `decision_json` existe para debugging causal local y debe permanecer sanitizado: sin `pattern` crudo, sin argv, sin snapshot completo del request y sin el texto/comandos completos del bloque `coach`.
- `decision_json` puede incluir solo derivaciones de `continuation` y `memory_pointer` (`continuation_present`, `continuation_reason`, `continuation_op`, `memory_pointer_present`, `memory_stale`) y metadatos diagnosticos como `doc_ranker` / `intent_mode`; nunca `why`, `query`, `handoff` ni el contenido completo del snapshot repo-local.
- `daemon.db` usa WAL mode para manejar escrituras concurrentes (daemon + CLI directo).
- `.mi-lsp/index.db` repo-local tambien usa WAL mode + `busy_timeout`, y las escrituras se serializan por workspace para evitar contencion entre watcher e index manual.
- Ante corrupcion de `index.db`, el runtime debe cuarentenar el archivo previo y reconstruir uno nuevo en el mismo workspace.
- Auto-purge elimina eventos y runs con mas de 30 dias en startup de CLI y daemon.
- La fila canonica de una request `route=daemon` la escribe el daemon.
- Backpressure daemon-aware usa la telemetria existente: `error_kind=daemon`, `error_code=backpressure_busy`, `success=false` y warning `daemon/backpressure_busy`.
- La CLI directa solo graba `access_events` cuando la request se sirve como `direct`, `direct_fallback` o falla antes de delegarse al daemon; esos eventos pueden llevar `daemon_run_id = NULL`.
- `runtime_key` debe persistirse tambien en filas `route=direct` o `route=direct_fallback` para que `admin export` pueda atribuir uso de queries directas a un workspace/backend/entrypoint estable.
- `result_count` representa los items emitidos en el envelope final; `warning_count` se persiste como contador explicito para que summary/CSV no dependan de re-hidratar `warnings_json`.
- Filas duplicadas historicas de requests daemonizadas pueden existir como artefactos previos al fix de ownership de telemetria y deben tratarse como legacy hasta que la retencion las purgue.

## Operaciones clave en `index.db`

### Queries

- `SymbolContainingLine(file, line)`: devuelve el simbolo mas chico que encierra un archivo + linea dados. Usado por `nav context` y `nav diff-context`.
- `ListDocRecords()`: devuelve el corpus documental ordenado por familia/capa.
- `DocEdgesFrom(path)`: devuelve relaciones explicitas salientes para priorizar supporting docs.
- `DocMentionsForPath(path)`: devuelve menciones a codigo o comandos derivadas de un documento.
- `CountDocRecords()`: expone `doc_count` para `workspace status`.
- `FindDocRecordsByMention(type, value)`: permite resolver IDs embebidos dentro de documentos agregados bajo `04_RF/`.

### Transacciones

- `ReplaceFileSymbols(file_id, symbols)`: DELETE todos los simbolos del file, luego INSERT los nuevos. Usado para re-indexing incremental.
- `DeleteFileSymbols(file_id)`: DELETE simbolos y file record para archivos eliminados. Respeta `content_hash` para dedup.
- `ReplaceDocs(docs, edges, mentions)`: reemplaza el snapshot documental completo y actualiza `workspace_meta.doc_count`.
- `SaveReentrySnapshot(snapshot)`: persiste `memory_snapshot_json` y `memory_snapshot_built_at` en `workspace_meta` al final de una indexacion exitosa.

## Riesgos operativos observados

- Repos con `.docs` o templates que incluyen `.csproj` pueden aparecer como entrypoints visibles si no estan ignorados.
- La heuristica de bootstrap no debe elegir esos entrypoints auxiliares como `default_entrypoint` si existe una solucion o proyecto real fuera de `.docs/template(s)`.
- El mecanismo recomendado para ese ruido sigue siendo `.milspignore` o `[ignore].extra_patterns`.
- `.worktrees/` ya forma parte de los ignores internos y no debe volver a elegirse como entrypoint por defecto.
- Cambios parciales en docs pueden dejar ranking inconsistente si se mezclan con incremental por archivo; por eso el cambio de docs/profile fuerza full re-index.
- Si `doc_count=0` pero existe `.docs/wiki`, las consultas docs-first pueden anclarse por Tier 1, pero el workspace debe considerarse documentalmente incompleto hasta ejecutar `index --docs-only` o un full index exitoso.

## Documentos detalle

- [DB-STATE-Y-TELEMETRIA.md](08_db/DB-STATE-Y-TELEMETRIA.md)
- [DB-DOC-INDEX.md](08_db/DB-DOC-INDEX.md)
