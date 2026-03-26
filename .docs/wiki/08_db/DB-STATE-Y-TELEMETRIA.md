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
- `workspace_input` (valor crudo recibido)
- `workspace_root` (clave canonica de analytics)
- `workspace_alias` (alias visible cuando exista)
- `operation`
- `backend`
- `repo`
- `entrypoint_id`
- `success`
- `latency_ms`
- `client_name`
- `session_id`
- `daemon_run_id`
- `warning_count`
- `error_kind`
- `error_code`
- `truncated` (INTEGER DEFAULT 0) — 1 si la respuesta fue truncada por token/item budget
- `result_count` (INTEGER DEFAULT 0) — numero de simbolos/items devueltos (de `Stats.Symbols`)
- Nota: columnas agregadas via migration idempotente (`ALTER TABLE ... ADD COLUMN`); rows existentes quedan con DEFAULT 0 o `NULL` segun el schema de origen
- Lectores y exportadores deben usar lectura null-safe para columnas opcionales legacy (`repo`, `client_name`, `session_id`, `backend`, `runtime_key`, `entrypoint_id`, `error_text`, `workspace_root`, `workspace_alias`, `error_kind`, `error_code`)

## Access patterns y operaciones sensibles

- `index.db` soporta lecturas frecuentes y escrituras incrementales por indexacion.
- `daemon.db` soporta escrituras append-heavy de telemetria local y replace liviano de `runtime_snapshots` por run activo.
- No registrar payloads completos de requests ni paths sensibles innecesarios en access events.

## Migracion, retencion y recovery

- Migraciones forward-only y automaticas.
- La apertura de `index.db` debe auto-migrar columnas `repo_id` y `repo_name` antes de crear indices dependientes para no romper workspaces legacy.
- `index.db` se puede recrear con `mi-lsp index --clean`.
- `daemon.db` puede purgarse para troubleshooting sin romper el repo.
- Access events deben tener retencion acotada configurable.
- Filas legacy de `access_events` con `repo = NULL`, `workspace_root = NULL` o metadata parcial no deben romper `recent-accesses`, `admin export` ni `/api/metrics`; la lectura debe derivar `workspace_root` y error typing cuando sea posible.
- `runtime_snapshots` representa el estado observado del run activo; no es historico infinito.

## Related docs

- [TECH-DAEMON-GOBERNANZA.md](../07_tech/TECH-DAEMON-GOBERNANZA.md)
- [CT-CLI-DAEMON-ADMIN.md](../09_contratos/CT-CLI-DAEMON-ADMIN.md)
