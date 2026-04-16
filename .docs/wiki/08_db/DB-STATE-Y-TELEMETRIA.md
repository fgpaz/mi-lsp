# DB-STATE-Y-TELEMETRIA

Volver a [08_modelo_fisico_datos.md](../08_modelo_fisico_datos.md).

## Summary

Describe el modelo fisico minimo de los stores locales de `mi-lsp`: indice repo-local y store global de daemon/telemetria.

## Owner, store y scope

- Owner logico: Workspace owner + Runtime supervision
- Stores:
  - `<repo>/.mi-lsp/index.db`
  - `~/.mi-lsp/daemon/daemon.db`
  - `~/.mi-lsp/daemon/state.json`
- Scope: catalogo, runtime snapshots, access events y metadata de bootstrap

## Data domains o schema groups

### `index.db`

Tablas canonicas minimas:

- `symbols`
- `files`
- `workspace_meta`

`workspace_meta` tambien puede persistir el snapshot repo-local de reentrada via claves `memory_snapshot_json` y `memory_snapshot_built_at`.

Campos duros esperados en `symbols`:

- `file_path`
- `name`
- `kind`
- `start_line`
- `end_line`
- `parent`
- `qualified_name`
- `signature`
- `signature_hash`
- `scope`
- `language`
- `file_hash`
- `repo_id`
- `repo_name`

Reglas:

- identidad endurecida por `qualified_name` y/o `signature_hash`
- no persistir ASTs
- refs/jerarquias profundas C# no viven aqui

### `daemon.db`

Tablas canonicas minimas:

- `daemon_runs`
- `runtime_snapshots`
- `access_events`

Campos recomendados:

`daemon_runs`
- `run_id`
- `started_at`
- `stopped_at`
- `protocol_version`
- `socket_or_pipe`
- `admin_url`

`runtime_snapshots`
- `runtime_key`
- `daemon_run_id`
- `workspace_root`
- `workspace_name`
- `backend_type`
- `pid`
- `memory_bytes`
- `started_at`
- `last_used_at`
- `status`

`access_events`
- `id`
- `occurred_at`
- `workspace` (display label)
- `workspace_input` (valor crudo recibido; puede venir vacio)
- `workspace_root` (clave canonica de analytics del workspace resuelto)
- `workspace_alias` (alias visible del workspace resuelto cuando exista)
- `operation`
- `backend`
- `route` (`direct`, `daemon`, `direct_fallback`)
- `format`
- `token_budget`
- `max_items`
- `max_chars`
- `compress`
- `repo`
- `entrypoint_id`
- `success`
- `latency_ms`
- `client_name`
- `session_id`
- `seq` (INTEGER DEFAULT 0; secuencia monotona dentro de `session_id`)
- `daemon_run_id`
- `warning_count`
- `pattern_mode` (`literal`, `regex`, `none`)
- `routing_outcome` (`direct`, `narrowed_repo`, `router_error`, `direct_fallback`)
- `failure_stage` (`none`, `selector_validation`, `router`, `backend`, `transport`)
- `hint_code`
- `truncation_reason` (`none`, `max_items`, `max_chars`, `token_budget`)
- `decision_json`
- `error_kind`
- `error_code`
- `truncated` (INTEGER DEFAULT 0) — 1 si la respuesta fue truncada por token/item budget
- `result_count` (INTEGER DEFAULT 0) — cantidad de items realmente emitidos en el envelope final
- `warning_count` se persiste en write-time y no debe reconstruirse unicamente desde `warnings_json`
- `decision_json` es un JSON compacto y sanitizado para debugging operacional; solo puede incluir longitud/patrones/hints/selectors/fallback/source y nunca `pattern`, argv ni payload completo
- `decision_json` puede incluir metadata derivada del bloque `coach` (`coach_present`, `coach_trigger`, `coach_action_count`) pero nunca su `message` ni los `command` sugeridos
- `decision_json` puede incluir metadata derivada de `continuation` y `memory_pointer` (`continuation_present`, `continuation_reason`, `continuation_op`, `memory_pointer_present`, `memory_stale`) y flags diagnosticos como `doc_ranker` / `intent_mode`, pero nunca `why`, `query`, `handoff` ni el snapshot repo-local completo
- `hint_code` puede caer al `coach.trigger` cuando no hubo `hint`/`next_hint` explicitos pero si existe guidance estructurado
- `workspace_input` no debe reescribirse con el alias resuelto; el export tiene que distinguir input vacio de alias/path explicito
- `workspace`, `workspace_alias` y `workspace_root` deben normalizarse desde el workspace resuelto, no desde el selector crudo
- `runtime_key` debe existir tanto en filas daemonizadas como en filas directas/direct_fallback para mantener attribution consistente
- `index.db` repo-local debe inicializarse con `PRAGMA journal_mode=WAL` y `PRAGMA busy_timeout`
- la escritura catalog/docs/file-symbols debe quedar serializada por workspace para que watcher e index manual no peleen la misma DB
- Nota: columnas agregadas via migration idempotente (`ALTER TABLE ... ADD COLUMN`); rows existentes quedan con DEFAULT 0 o `NULL` segun el schema de origen
- Lectores y exportadores deben usar lectura null-safe para columnas opcionales legacy (`repo`, `client_name`, `session_id`, `backend`, `runtime_key`, `entrypoint_id`, `error_text`, `workspace_root`, `workspace_alias`, `error_kind`, `error_code`)
- Lectores y exportadores deben usar lectura null-safe para columnas opcionales legacy (`route`, `format`, `token_budget`, `max_items`, `max_chars`, `compress`, `pattern_mode`, `routing_outcome`, `failure_stage`, `hint_code`, `truncation_reason`, `decision_json`) ademas de los campos previos
- `seq` debe round-trip en `RecentAccesses`, `admin export`, y CSV para que el orden intra-sesion no dependa solo de `occurred_at`

## Access patterns y operaciones sensibles

- `index.db` soporta lecturas frecuentes y escrituras incrementales por indexacion.
- `daemon.db` soporta escrituras append-heavy de telemetria local y replace liviano de `runtime_snapshots` por run activo.
- No registrar payloads completos de requests ni paths sensibles innecesarios en access events.
- Para `route=daemon`, el daemon es el writer canonico de `access_events`.
- La CLI solo debe persistir filas de `access_events` para `direct`, `direct_fallback` o fallas previas a la ejecucion remota.
- `admin export --summary` agrega sobre toda la ventana filtrada salvo `--limit` explicito; los breakdowns adicionales por route/client/hint/failure-stage son opcionales y no cambian esa base.

## Migracion, retencion y recovery

- Migraciones forward-only y automaticas.
- La apertura de `index.db` debe auto-migrar columnas `repo_id` y `repo_name` antes de crear indices dependientes para no romper workspaces legacy.
- `index.db` se puede recrear con `mi-lsp index --clean`.
- El snapshot de reentrada se reconstruye en `mi-lsp index`; si wiki/raw quedan mas nuevos que `memory_snapshot_built_at`, el runtime debe marcar `stale=true` y evitar recomputarlo en caliente.
- `daemon.db` puede purgarse para troubleshooting sin romper el repo.
- Access events deben tener retencion acotada configurable.
- Filas legacy de `access_events` con `repo = NULL`, `workspace_root = NULL` o metadata parcial no deben romper `recent-accesses`, `admin export` ni `/api/metrics`; la lectura debe derivar `workspace_root` y error typing cuando sea posible.
- Filas historicas duplicadas de requests daemonizadas pueden aparecer en ventanas recientes si fueron escritas antes del fix forward de ownership; deben interpretarse como legacy y no como comportamiento vigente del runtime.
- `runtime_snapshots` representa el estado observado del run activo; no es historico infinito.

## Related docs

- [TECH-DAEMON-GOBERNANZA.md](../07_tech/TECH-DAEMON-GOBERNANZA.md)
- [CT-CLI-DAEMON-ADMIN.md](../09_contratos/CT-CLI-DAEMON-ADMIN.md)
