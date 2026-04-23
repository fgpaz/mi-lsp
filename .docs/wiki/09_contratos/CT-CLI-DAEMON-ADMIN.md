# CT-CLI-DAEMON-ADMIN

Volver a [09_contratos_tecnicos.md](../09_contratos_tecnicos.md).

## Summary

Define la frontera entre clientes locales y el runtime compartido: CLI publica, control del daemon y superficie admin/gobernanza.

## Boundary and owner

- Boundary: usuario/agente/browser local -> CLI/daemon
- Owner logico: CLI surface + Runtime supervision
- Scope: comandos, flags globales, requests a daemon y endpoints admin locales

## Contract family inventory

### CLI publica

Comandos canonicos:

- `workspace add|scan|list|warm|status|remove`
- `nav symbols|find|refs|overview|outline|service|search|context|deps|ask|pack|batch|related|workspace-map|diff-context|trace|intent`
- `index [path] [--clean] [--docs-only]`
- `index start|status|cancel`
- `info`
- `daemon start|stop|status|restart|open|logs [--tail N]`
- `daemon perf-smoke [--callers N] [--max-working-set-mb N] [--max-private-mb N] [--max-handles N]`
- `worker install|status`
- `admin open|status`

Flags globales minimos:

- `--workspace`
- `--axi`
- `--classic`
- `--full`
- `--format compact|json|text|toon|yaml`
- `--token-budget`
- `--max-items`
- `--max-chars`
- `--client-name`
- `--session-id`
- `--backend`
- `--verbose`

Flags especificos:

- `nav find|search|intent --repo`
- `nav pack --rf|--fl|--doc`
- `nav search --regex`
- `nav service --include-archetype`
- `index --docs-only`
- `index start --mode full|docs|catalog --wait`
- `daemon start|restart|serve --watch-mode off|lazy|eager --max-watched-roots N --max-inflight N`

### `index`

Input:

```text
mi-lsp index [path] [--workspace <alias>] [--clean] [--docs-only]
mi-lsp index start [path] [--workspace <alias>] [--mode full|docs|catalog] [--clean] [--wait]
mi-lsp index status [job-id] [--workspace <alias>]
mi-lsp index cancel <job-id> [--workspace <alias>]
```

Reglas:

- `index [path]` es wrapper de compatibilidad que ejecuta `index start --mode full --wait`; con `--docs-only`, ejecuta `--mode docs --wait`
- `index start` crea un registro durable en `index_jobs`; sin `--wait` lanza un proceso detached y retorna el `job_id`
- `index status` consulta el ultimo job del workspace si no se pasa `job-id`
- `index cancel` marca cancelacion solicitada; la cancelacion es cooperativa y puede no interrumpir una publicacion que ya llego al commit
- sin `--docs-only`, indexa catalogo de codigo y grafo documental, con incremental git-aware cuando corresponde
- con `--docs-only`, reconstruye `doc_records`, `doc_edges`, `doc_mentions` y `memory_pointer` sin reemplazar `files` ni `symbols`
- toda indexacion toma `.mi-lsp/index.lock`; si ya existe, la operacion debe fallar con mensaje accionable que incluya el lock owner cuando este disponible
- locks con PID inexistente se consideran stale y pueden recuperarse automaticamente
- la publicacion full de catalogo + docs + memoria es all-or-nothing dentro de SQLite
- el indexador debe respetar cancelacion de contexto durante el walk y el parseo documental
- la lista interna de ignores excluye dependencias/caches generadas como `.venv`, `venv`, `__pycache__`, `.pytest_cache`, `.turbo`, `.next` y `node_modules`

Respuesta `index start --wait` exitosa:

```json
{
  "ok": true,
  "workspace": "<alias>",
  "backend": "index-job",
  "mode": "docs",
  "items": [
    {
      "job_id": "idxjob-...",
      "generation_id": "idxgen-...",
      "status": "succeeded",
      "phase": "done",
      "files": 42,
      "symbols": 0,
      "docs": 42
    }
  ],
  "warnings": ["docs_only=true"],
  "stats": {"files": 42}
}
```

Envelope comun:

- `ok`
- `workspace`
- `backend`
- `items`
- `truncated`
- `stats`
- `warnings`
- `hint` (omitempty — diagnóstico cuando `items=[]` o daemon no disponible)
- `next_hint`

### `nav service`

Input:

```text
mi-lsp nav service <path> --workspace <alias> [--include-archetype] [--format compact|json|text|toon|yaml]
```

Output item (`items[0]`):

- `service`
- `path`
- `profile`
- `sources`
- `symbols`
- `http_endpoints`
- `event_consumers`
- `event_publishers`
- `entities`
- `infrastructure`
- `archetype_matches`
- `next_queries`

Reglas:

- contrato evidence-first; no expone score fuerte de completitud
- puede devolver `backend=catalog`, `backend=text` o `backend=catalog+text`
- si el catalogo es insuficiente, la operacion sigue con evidencia textual y warning

### `nav context`

Input:

```text
mi-lsp nav context <file> <line> --workspace <alias> [--backend <hint>] [--format compact|json|text|toon|yaml]
```

Output item (`items[0]`):

- `file`
- `line`
- `focus_line`
- `slice_start_line`
- `slice_end_line`
- `slice_text`
- `name` / `kind` / `signature` / `qualified_name` / `scope` cuando hay enriquecimiento

Reglas:

- contrato slice-first: el core devuelve primero el bloque legible alrededor de la linea
- `backend=text` para archivos no semanticos
- `backend=roslyn`, `backend=tsserver` o `backend=catalog` cuando hay enriquecimiento correspondiente
- si el backend semantico falla pero el archivo existe, la operacion sigue con `slice_text` y warning accionable
- si el warning proviene de bootstrap Roslyn, debe sugerir `mi-lsp worker install`; si proviene de SDK/global.json, la telemetria debe clasificarlo como `sdk/*`

### CLI -> daemon

Reglas de routing:

- `nav.find`, `nav.search`, `nav.intent`, `nav.symbols`, `nav.outline`, `nav.overview`, `nav.multi-read` y `nav.pack` no deben cruzar esta frontera en el hot path.
- `nav.refs`, `nav.context`, `nav.deps`, `nav.related`, `nav.service`, `nav.diff-context` y `nav.batch` pueden preferir daemon cuando corresponda.
- `nav.workspace-map` summary-first queda directo por default; `--full` puede seguir siendo una operacion pesada pero no debe autostartear daemon en el contrato base.
- `workspace.warm` puede preferir daemon pero no debe auto-iniciarlo.

Canal:

- Windows: named pipe
- Linux: unix socket

Request envelope actual:

- `protocol_version`
- `operation`
- `context`
- `payload`

Metadata minima en `context`:

- `workspace`
- `axi`
- `full`
- `format`
- `token_budget`
- `max_items`
- `max_chars`
- `client_name`
- `session_id`
- `backend_hint`
- `verbose`
- `compress`

La CLI resuelve `--classic` en el borde y no lo transporta al daemon; el daemon solo recibe el resultado efectivo via `axi` y `full`.

Metadata opcional cuando aplica paginacion CLI:

- `offset`

Metadata operativa que debe quedar observable en `access_events` y `admin export`:

- `route` (`direct`, `daemon`, `direct_fallback`)
- `format`
- `token_budget`
- `max_items`
- `max_chars`
- `compress`
- `warning_count`
- `pattern_mode`
- `routing_outcome`
- `failure_stage`
- `hint_code`
- `truncation_reason`
- `decision_json` sanitizado; nunca `pattern` crudo ni argv
- si `route=daemon` y la ejecucion fue normal, el registro canonico lo escribe el daemon; la CLI no debe duplicar esa misma operacion en `access_events`
- `result_count` representa items emitidos en el envelope final, no `Stats.Symbols`

`admin export` debe soportar:

- raw `json|csv|compact`
- ventana `--since`/`--recent`
- filtros `--workspace`, `--backend`, `--operation`, `--session-id`, `--client-name`, `--route`, `--query-format`, `--truncated`, `--pattern-mode`, `--routing-outcome`, `--failure-stage`, `--hint-code`
- `--summary` sobre toda la ventana filtrada salvo `--limit` explicito
- breakdowns opcionales `--by-route`, `--by-client`, `--by-hint`, `--by-failure-stage`

### Governance admin

Endpoints minimos:

- `GET /`
- `GET /api/status?window=<recent|7d|30d|90d>`
- `GET /api/workspaces?window=<recent|7d|30d|90d>`
- `GET /api/workspaces/{workspace}?window=<recent|7d|30d|90d>`
- `POST /api/workspaces/{workspace}/warm`
- `GET /api/accesses?window=<recent|7d|30d|90d>`
- `GET /api/logs?tail=<n>`
- `GET /api/metrics?window=<recent|7d|30d|90d>`

Payload clave en `GET /api/status`:

- `state`
- `metrics`
- `active_runtimes`
- `daemon_process`
- `watchers`
- `recent_accesses`
- `workspaces`
- `generated_at`
- `window`
- `window_label`

Deep-link admin canonico:

- `/?workspace=<alias>&panel=<overview|activity|logs|metrics>&window=<recent|7d|30d|90d>&backend=<type>`

Reglas:

- solo `127.0.0.1`
- una UI global
- acciones seguras solamente
- query params, no hash-state
- el resumen agregado debe distinguir cortes por workspace y por operacion
- `GET /api/logs?tail=n` lee el tail con memoria acotada y puede devolver warning si el archivo se capeo por bytes.

### Comandos del workspace

#### `workspace remove`

Elimina un workspace registrado y limpia su estado:

- Elimina entrada en `registry.toml`
- Detiene runtimes asociados en el daemon si existe
- Limpia entrada en `~/.mi-lsp/daemon/state.json`
- El repo-local `.mi-lsp/` puede quedar intacto; se considera estado "olvidado"

Respuesta exitosa:
```json
{
  "ok": true,
  "workspace": "<alias>",
  "backend": "router",
  "warnings": [],
  "stats": { "removed_at": "ISO8601" }
}
```

Errores comunes:
- `WORKSPACE_NOT_FOUND`: el workspace no estaba registrado
- `DAEMON_ERROR`: no se pudo contactar al daemon para limpieza

### Comandos del daemon

#### `daemon restart`

Reinicia el daemon de forma segura:

1. Detiene el daemon existente si corre
2. Espera a que terminen runtimes activos (timeout configurable)
3. Limpia state y temp files
4. Inicia nueva instancia

Respuesta exitosa:
```json
{
  "ok": true,
  "backend": "router",
  "daemon": {
    "pid": 1234,
    "endpoint": "<pipe_or_socket>",
    "admin_url": "http://127.0.0.1:<port>"
  },
  "warnings": [],
  "stats": { "restart_duration_ms": 123 }
}
```

Errores comunes:
- `DAEMON_NOT_RUNNING`: no hay daemon para reiniciar (empieza uno nuevo, no error)
- `TIMEOUT_WAITING_FOR_SHUTDOWN`: runtimes no cerraron a tiempo

### Comandos del worker

#### `worker install`

Input:

```text
mi-lsp worker install [--rid <rid>] [--format compact|json|text|toon|yaml]
```

Reglas:

- si la distribucion del ejecutable trae un worker bundled para el `rid`, debe copiarlo a `~/.mi-lsp/workers/<rid>/`
- si la CLI corre dentro del repo `mi-lsp` y no hay bundle adjunto, puede publicar el worker desde `worker-dotnet/`
- no debe depender del `cwd` del repo usuario donde se invoca el comando

Respuesta exitosa (`items[0]`):

- `path`
- `rid`

#### `worker status`

Input:

```text
mi-lsp worker status [--format compact|json|text|toon|yaml]
```

Respuesta exitosa (`items[0]`):

- `dotnet`
- `rid`
- `tool_root`
- `tool_root_kind`
- `cli_path`
- `protocol_version`
- `install_hint`
- `active_workers`
- `selected`
- `selected_source`
- `selected_path`
- `selected_compatible`
- `selected_error`
- `bundled`
- `bundled_error`
- `bundled_compatible`
- `installed`
- `installed_error`
- `installed_compatible`
- `dev_local`
- `dev_local_error`

Reglas:

- debe distinguir candidatos `bundle`, `installed` y `dev-local`
- debe exponer el candidato realmente elegido para el runtime actual
- si el daemon atiende esta operacion, debe devolver exactamente el mismo envelope canonico que el modo directo; `active_workers` queda anidado dentro del item diagnostico
- en repo de desarrollo, los artefactos locales `bin/workers/<rid>` no deben presentarse como bundle canonico de distribucion

## Payload, error y compatibilidad

- `daemon start` debe devolver la instancia existente si ya corre.
- `daemon status` debe exponer `state`, `daemon_process`, `watchers`, `active_runtimes` y `recent_accesses`.
- Si una operacion daemon-aware excede `max_inflight`, devuelve envelope `ok=false`, item con `error_kind=daemon`, `error_code=backpressure_busy`, y warning `daemon/backpressure_busy`.
- Si no hay daemon, el CLI debe poder ejecutar directo.
- Si falta un backend opcional, devolver warning accionable, no fallo ambiguo.
- `backend` debe reflejar el backend realmente usado.
- `admin open` y `daemon open` deben abrir la misma `admin_url` con deep-link consistente.
- Las fallas de bootstrap del worker deben sugerir remediacion concreta, al menos `mi-lsp worker install` cuando corresponda.

## Versioning y migracion

- Cambios incompatibles en request/response del daemon requieren bump de `protocol_version`.
- La UI admin no debe prometer estabilidad publica externa fuera del host local.

## Related docs

- [TECH-DAEMON-GOBERNANZA.md](../07_tech/TECH-DAEMON-GOBERNANZA.md)
- [TECH-SERVICE-EXPLORATION.md](../07_tech/TECH-SERVICE-EXPLORATION.md)
- [DB-STATE-Y-TELEMETRIA.md](../08_db/DB-STATE-Y-TELEMETRIA.md)
