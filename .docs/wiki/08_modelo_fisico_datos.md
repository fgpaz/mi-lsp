# 08. Modelo fisico de datos

## Proposito y alcance

Este documento resume los stores fisicos de `mi-lsp`, su ownership y el ciclo de vida de los datos.
La novedad de v1.3 es que el store repo-local persiste tambien el grafo documental de `.docs/wiki` y el runtime considera cambios de docs, `00_gobierno_documental.md` o `read-model` como disparadores de full re-index.

## Inventario de stores

| Store | Ubicacion | Owner logico | Proposito |
|---|---|---|---|
| Workspace index DB | `<repo>/.mi-lsp/index.db` | Workspace owner | Catalogo repo-local de simbolos, archivos, repos, entrypoints y docs |
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
  - `workspace_meta` con `workspace_kind`, `default_repo`, `default_entrypoint`, `doc_count`
- `daemon.db`
  - `runtime_snapshots` con `repo_name`, `repo_root`, `entrypoint_id`, `entrypoint_path`, `entrypoint_type`
  - `access_events` con `client_name`, `session_id`, `seq INTEGER DEFAULT 0`, `workspace_input`, `workspace_root`, `workspace_alias`, `repo`, `entrypoint_id`, `route`, `format`, `token_budget`, `max_items`, `max_chars`, `compress`, `error_kind`, `error_code`, `truncated`, `result_count`, `warning_count`, `pattern_mode`, `routing_outcome`, `failure_stage`, `hint_code`, `truncation_reason`, `decision_json`

## Reglas de consistencia y retencion

- `index.db` debe tolerar reconstruccion completa con `mi-lsp index --clean`.
- Las migraciones aditivas de `index.db` deben crear `repo_id` y `repo_name` en `files`/`symbols` antes de crear indices que dependan de esas columnas.
- `doc_records`, `doc_edges` y `doc_mentions` deben refrescarse como un bloque consistente dentro de una sola transaccion.
- `project.toml` debe poder reescribirse al volver a detectar topologia del workspace.
- El `read-model.toml` no se copia a SQLite; se usa en lectura y sus cambios disparan re-index completo del corpus documental.
- `00_gobierno_documental.md` tampoco se persiste dentro de SQLite; su estado gobierna bloqueo/sync e invalida el indice cuando cambia.
- El store global del daemon no duplica el catalogo repo-local; solo guarda supervision y telemetria.
- `registry.toml` puede contener multiples aliases para un mismo root; `workspace list` debe preservar el alias registrado.
- `runtime_snapshots` representan solamente runtimes observables del `daemon_run_id` vigente.
- `access_events` registran metadata y nunca payloads completos.
- `decision_json` existe para debugging causal local y debe permanecer sanitizado: sin `pattern` crudo, sin argv y sin snapshot completo del request.
- `daemon.db` usa WAL mode para manejar escrituras concurrentes (daemon + CLI directo).
- Auto-purge elimina eventos y runs con mas de 30 dias en startup de CLI y daemon.
- La fila canonica de una request `route=daemon` la escribe el daemon.
- La CLI directa solo graba `access_events` cuando la request se sirve como `direct`, `direct_fallback` o falla antes de delegarse al daemon; esos eventos pueden llevar `daemon_run_id = NULL`.
- `result_count` representa los items emitidos en el envelope final; `warning_count` se persiste como contador explicito para que summary/CSV no dependan de re-hidratar `warnings_json`.
- Filas duplicadas historicas de requests daemonizadas pueden existir como artefactos previos al fix de ownership de telemetria y deben tratarse como legacy hasta que la retencion las purgue.

## Operaciones clave en `index.db`

### Queries

- `SymbolContainingLine(file, line)`: devuelve el simbolo mas chico que encierra un archivo + linea dados. Usado por `nav context` y `nav diff-context`.
- `ListDocRecords()`: devuelve el corpus documental ordenado por familia/capa.
- `DocEdgesFrom(path)`: devuelve relaciones explicitas salientes para priorizar supporting docs.
- `DocMentionsForPath(path)`: devuelve menciones a codigo o comandos derivadas de un documento.

### Transacciones

- `ReplaceFileSymbols(file_id, symbols)`: DELETE todos los simbolos del file, luego INSERT los nuevos. Usado para re-indexing incremental.
- `DeleteFileSymbols(file_id)`: DELETE simbolos y file record para archivos eliminados. Respeta `content_hash` para dedup.
- `ReplaceDocs(docs, edges, mentions)`: reemplaza el snapshot documental completo y actualiza `workspace_meta.doc_count`.

## Riesgos operativos observados

- Repos con `.docs` o templates que incluyen `.csproj` pueden aparecer como entrypoints visibles si no estan ignorados.
- La heuristica de bootstrap no debe elegir esos entrypoints auxiliares como `default_entrypoint` si existe una solucion o proyecto real fuera de `.docs/template(s)`.
- El mecanismo recomendado para ese ruido sigue siendo `.milspignore` o `[ignore].extra_patterns`.
- `.worktrees/` ya forma parte de los ignores internos y no debe volver a elegirse como entrypoint por defecto.
- Cambios parciales en docs pueden dejar ranking inconsistente si se mezclan con incremental por archivo; por eso el cambio de docs/profile fuerza full re-index.

## Documentos detalle

- [DB-STATE-Y-TELEMETRIA.md](08_db/DB-STATE-Y-TELEMETRIA.md)
- [DB-DOC-INDEX.md](08_db/DB-DOC-INDEX.md)
