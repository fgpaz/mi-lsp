---
doc_id: CT-CLI-DAEMON-ADMIN
title: CLI daemon admin y export de telemetria
layer: CT
family: CLI-DAEMON-ADMIN
status: implemented
implements:
  - internal/cli/admin.go
  - internal/cli/daemon.go
  - internal/daemon/admin.go
  - internal/daemon/export.go
  - internal/daemon/log_tail.go
  - internal/daemon/state_store.go
tests:
  - internal/daemon/export_test.go
  - internal/daemon/log_tail_test.go
---

# CT-CLI-DAEMON-ADMIN

```yaml
harness_protocol: SDD-HARNESS-v1
id: "CT-CLI-DAEMON-ADMIN"
kind: "support-doc"
audience: "llm-first"
imports:
  - '[[00_gobierno_documental]]'
  - '[[CT-CLI-DAEMON-ADMIN]]'
exports:
  - 'CT-CLI-DAEMON-ADMIN'
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/09_contratos/CT-CLI-DAEMON-ADMIN.md
agent_may_edit:
  - .docs/wiki/09_contratos/CT-CLI-DAEMON-ADMIN.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
evidence:
  - .docs/wiki/09_contratos/CT-CLI-DAEMON-ADMIN.md
```

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

- `workspace add|scan|list|warm|status|remove|doctor|hygiene|prune`
- `nav symbols|find|refs|overview|outline|service|search|context|deps|ask|pack|batch|related|workspace-map|diff-context|affected|trace|intent`
- `index [path] [--clean] [--docs-only]`
- `index start|status|cancel`
- `info`
- `doctor` (alias unificado para diagnostico workspace-aware)
- `daemon start|stop|status|restart|open|logs [--tail N]`
- `daemon perf-smoke [--callers N] [--max-working-set-mb N] [--max-private-mb N] [--max-handles N]`
- `worker install|status`
- `admin open|status`
- `version`

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
- `--profile agent` (auto para harness cuando se detecta `client_name=harness-*`)
- `--allow-cross-workspace`

Flags especificos:

- `nav find|search|intent --repo`
- `nav pack --rf|--fl|--doc`
- `nav search --regex`
- `nav affected --from-git-diff --changed-ref <ref> --stdin --include-tests --include-docs --quiet --test-command <cmd>`
- `nav service --include-archetype`

### Workspace mismatch guard

Cuando un comando workspace-aware recibe `--workspace <alias>` y el `caller_cwd` esta dentro de otro workspace registrado:

- `client_name` humano conserva compatibilidad: ejecuta sobre el alias explicito y emite warning con alias/root seleccionado, cwd, workspace/root detectado por cwd, comando recomendado y `--allow-cross-workspace`.
- `client_name` harness/agente (`codex`, `claude-code`, `claude-ai`, `opencode`, `copilot`, `jetbrains`, `cursor`, `neovim`, `emacs`, `vim`) rechaza la operacion salvo `--allow-cross-workspace`.
- El error estructurado debe usar `error.kind=workspace`, `error.code=workspace_cross_workspace_refused`, `error.stage=selector_validation` y `error.hint_code=workspace_cross_workspace_refused`.
- El override `--allow-cross-workspace` permite continuar pero no suprime el warning; el warning es evidencia de intencion cross-workspace.
- `index --docs-only`
- `index start --mode full|docs|catalog --wait`
- `daemon start|restart|serve --watch-mode off|lazy|eager --max-watched-roots N --max-inflight N`

### `index` + indexacion async

Input:

```text
mi-lsp index [path] [--workspace <alias>] [--clean] [--docs-only]
mi-lsp index start [path] [--workspace <alias>] [--mode full|docs|catalog] [--clean] [--wait]
mi-lsp index status [job-id] [--workspace <alias>]
mi-lsp index cancel <job-id> [--workspace <alias>] [--force]

mi-lsp init [path] [--name alias] [--no-index]
mi-lsp workspace add [path] [--name alias] [--no-index]
```

Reglas:

- `index [path]` es wrapper de compatibilidad que ejecuta `index start --mode full --wait`; con `--docs-only`, ejecuta `--mode docs --wait`.
- `index start` crea un registro durable en `index_jobs`; sin `--wait` lanza un proceso detached y retorna el `job_id`; con `--wait` bloquea hasta que la indexacion complete.
- `index status` consulta el ultimo job del workspace si no se pasa `job-id`.
- `index status.phase` conserva `indexing` durante el trabajo pesado y solo pasa a `publishing` en el cierre/publicacion final.
- `index status` expone progreso vivo en `current_stage`, `current_path`, `files_total`, `files`, `symbols`, `docs` y `updated_at`; esos campos deben refrescarse durante catalogo/docs antes de publicar.
- `index cancel` marca cancelacion solicitada; la cancelacion es cooperativa y puede no interrumpir una publicacion que ya llego al commit.
- `index cancel --force` puede terminar el PID vivo del job, marcarlo `canceled` y remover el `.mi-lsp/index.lock` si pertenece a ese PID ya muerto; se reserva para jobs colgados.
- `workspace.add`, `init` usan **hibrido smart-sync** (FD1): indexan sincronicamente dentro de `SmartSyncTimeout` (`MI_LSP_INDEX_SYNC_TIMEOUT`, 20s) para preservar el contrato init-then-query; si una primera indexacion grande lo excede, degradan a background y devuelven `job_id` con warning. `--background`/`background:true` fuerza async inmediato; `--wait`/`wait:true` fuerza sync completo (`IndexTimeout`, `MI_LSP_INDEX_TIMEOUT` 5min); `--no-index` omite indexacion.
- sin `--docs-only`, indexa catalogo de codigo y grafo documental, con incremental git-aware cuando corresponde.
- con `--docs-only`, reconstruye `doc_records`, `doc_edges`, `doc_mentions` y `memory_pointer` sin reemplazar `files` ni `symbols`.
- toda indexacion toma `.mi-lsp/index.lock`; si ya existe, la operacion debe fallar con mensaje accionable que incluya el lock owner cuando este disponible.
- locks con PID inexistente se consideran stale y pueden recuperarse automaticamente.
- la publicacion full de catalogo + docs + memoria es all-or-nothing dentro de SQLite.
- el indexador debe respetar cancelacion de contexto durante el walk y el parseo documental.
- la lista interna de ignores excluye dependencias/caches generadas como `.venv`, `venv`, `__pycache__`, `.pytest_cache`, `.turbo`, `.next` y `node_modules`.

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
      "current_stage": "done",
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
- `coach`
- `continuation`
- `memory_pointer`

Reglas de formato:

- `--format toon` debe recibir el envelope como mapa JSON-compatible y sanitizado recursivamente justo antes de `toon.Marshal`.
- La sanitizacion TOON reemplaza controles no imprimibles, excepto tab/newline/carriage-return, por escapes ASCII visibles (`\u0000`, `\u001f`, etc.).
- Cuando la sanitizacion cambia al menos un string, `warnings` debe agregar una unica entrada `toon output sanitized unsafe control characters`.
- `--format compact`/JSON mantiene su comportamiento compatible existente y no debe depender de la sanitizacion TOON.

### `version`

Input:

```text
mi-lsp version [--format text|compact|json|toon|yaml]
```

Reglas:

- `version` es una operacion local, read-only y sin dependencia de workspace, registry, daemon o workers vivos.
- Sin `--format` explicito, emite salida `text` legible para humanos.
- Con `--format` explicito, respeta el envelope comun y devuelve `backend=version`.
- El item expone `command`, `version`, `module_path`, `go_version`, `goos`, `goarch`, `protocol_version`, `worker_rid`, `tool_root`, `cli_path`, `executable_sha256`, `vcs_revision`, `vcs_time` y `vcs_modified` cuando Go build info los provee.
- `vcs_modified=false` es la prueba operativa de que el binario fue construido desde un arbol limpio; si falta metadata VCS, los campos se omiten o se exponen como desconocidos en `text`.
- El comando no reemplaza `worker status`: `version` prueba provenance del ejecutable; `worker status` diagnostica candidatos de worker y compatibilidad.

### `admin export --summary`

El summary puede incluir un bloque aditivo `recommendations` para usage-doctor. Cada item debe derivarse de telemetria agregada y sanitizada (`hint_code`, `failure_stage`, `truncation_rate`, latencias, breakdowns y conteos), incluir accion sugerida y razon breve, y nunca copiar query cruda, argv, payloads, paths sensibles ni contenido de archivos.

Sin `--limit` explicito, el summary agrega toda la ventana filtrada mediante acumulacion streaming desde `daemon.db`; no debe cargar todos los eventos crudos en memoria. Si el usuario pasa `--limit`, el summary conserva la semantica de muestra acotada. `--by-backend`, `--percentile`, `--by-route`, `--by-client`, `--by-hint` y `--by-failure-stage` siguen siendo opt-in de visualizacion.

### `daemon logs`

`daemon logs --tail N` muestra el tail del log local del daemon. Puede omitir lineas benignas de cierre normal de sockets/pipes (`use of closed network connection`, pipe cerrado, reset/broken pipe) junto con el bloque de ayuda generado por Cobra, para que el tail diagnostico no parezca fallo accionable cuando solo refleja shutdown.

Envelope de error estructurado (`ok=false`):

- `ok=false`
- `backend` con el subsistema que intento responder
- `items=[]` salvo diagnostico seguro y tipado
- `warnings` con codigos accionables cuando existan
- `error.kind` (`daemon`, `workspace`, `worker_bootstrap`, `sdk`, `backend_runtime`, `validation`, `transport`)
- `error.code` estable y grep-friendly
- `error.message` breve, sin stack trace ni payload crudo
- `error.stage` opcional (`selector_validation`, `router`, `backend`, `transport`)
- `error.retryable` opcional
- `error.hint` opcional con remediacion concreta
- `stats` y `truncated=false` cuando el fallo ocurre antes de emitir items

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
mi-lsp nav context <file>:<line> --workspace <alias> [--backend <hint>] [--format compact|json|text|toon|yaml]
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
- si el backend semantico falla pero el archivo existe, la operacion sigue con `slice_text`, degrada a catalog/text cuando sea posible y agrega warning accionable
- si el warning proviene de bootstrap Roslyn, debe sugerir `mi-lsp worker install`; si proviene de SDK/global.json, la telemetria debe clasificarlo como `sdk/*`; si proviene de permisos/arranque de proceso, debe clasificarlo como `backend_runtime/process_spawn_access_denied` o `backend_runtime/process_spawn_failed`

### `nav affected`

Input:

```text
mi-lsp nav affected [paths...] --workspace <alias> [--from-git-diff] [--changed-ref <ref>] [--stdin] [--include-tests] [--include-docs] [--quiet] [--test-command <cmd>] [--format compact|json|text|toon|yaml]
```

Output item:

- `kind` (`code`, `test`, `doc`)
- `path`
- `reason`
- `confidence` numerico entre 0 y 1
- `suggested_command` cuando aplica
- `evidence` con `source`, `input_path`, `change_type`, `symbol` y/o `trigger_path`

Reglas:

- contrato conservador: los items son candidatos accionables, no prueba de impacto completo
- si no hay paths explicitos ni stdin, puede usar git diff del workspace como input por default
- `--from-git-diff` debe considerar cambios staged, unstaged y untracked cuando `--changed-ref` queda en `HEAD`
- `--include-tests` agrega comandos sugeridos de prueba por familia de path y respeta `--test-command` como override literal del usuario
- `--include-docs` agrega docs canonicos por familias de paths gobernadas, sin editar ni validar esos docs
- debe ignorar sidecars operativos `.mi-lsp/**`, `.docs/raw/**`, `.docs/auditoria/**` y `.git/**`
- toda respuesta heuristica debe incluir warning visible y conservar `confidence` como dato advisory

### CLI -> daemon

Reglas de routing:

- `nav.find`, `nav.search`, `nav.intent`, `nav.symbols`, `nav.outline`, `nav.overview`, `nav.multi-read`, `nav.affected` y `nav.pack` no deben cruzar esta frontera en el hot path.
- `nav.refs`, `nav.context`, `nav.deps`, `nav.related`, `nav.service`, `nav.diff-context` y `nav.batch` pueden preferir daemon cuando corresponda.
- `nav.workspace-map` summary-first queda directo por default; `--full` puede seguir siendo una operacion pesada pero no debe autostartear daemon en el contrato base.
- `workspace.warm` puede preferir daemon pero no debe auto-iniciarlo.

Canal:

- Windows: named pipe
- Linux: unix socket

Request envelope actual:

- `protocol_version` (requerido, no puede estar vacio; versiones incompatibles rechazadas tempranamente)
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
- `requested_backend`, `result_backend`, `backend_fallback_taken`, `fallback_from`, `fallback_to`, `runtime_error_code` dentro de `decision_json` cuando hay fallback backend/runtime
- `planner_path`, `planner_outcome`, `safe_degrade_reason` y `guardrail_trigger` dentro de `decision_json` cuando el planner cambia ruta, baja de daemon/semantica a texto, fuerza preview/full, o activa guardrails de search
- `decision_json` sanitizado; nunca `pattern` crudo ni argv
- si `route=daemon` y la ejecucion fue normal, el registro canonico lo escribe el daemon; la CLI no debe duplicar esa misma operacion en `access_events`
- `result_count` representa items emitidos en el envelope final, no `Stats.Symbols`

`admin export` debe soportar:

- raw `json|csv|compact|toon`
- ventana `--since`/`--recent`
- filtros `--workspace`, `--backend`, `--operation`, `--session-id`, `--client-name`, `--route`, `--query-format`, `--truncated`, `--pattern-mode`, `--routing-outcome`, `--failure-stage`, `--hint-code`
- `--summary` sobre toda la ventana filtrada salvo `--limit` explicito, incluyendo salida `toon`
- breakdowns opcionales `--by-route`, `--by-client`, `--by-hint`, `--by-failure-stage`

### Governance admin

Endpoints minimos:

- `GET /`
- `GET /api/status?window=<recent|7d|30d|90d>`
- `GET /api/workspaces?window=<recent|7d|30d|90d>`
- `GET /api/workspaces/{workspace}?window=<recent|7d|30d|90d>`
- `POST /api/workspaces/{workspace}/warm` (requiere admin token + validacion Host/Origin)
- `GET /api/accesses?window=<recent|7d|30d|90d>`
- `GET /api/logs?tail=<n>`
- `GET /api/metrics?window=<recent|7d|30d|90d>`

Payload clave en `GET /api/status`:

- `state`
- `metrics`
- `active_runtimes`
- `daemon_process`
- `watchers`
- `recent_accesses` (default 5 items, en v0.5.0+)
- `workspaces`
- `generated_at`
- `window`
- `window_label`

Deep-link admin canonico:

- `/?workspace=<alias>&panel=<overview|activity|logs|metrics>&window=<recent|7d|30d|90d>&backend=<type>`

Reglas:

- solo `127.0.0.1` (loopback local)
- una UI global
- acciones seguras solamente
- query params, no hash-state
- el resumen agregado debe distinguir cortes por workspace y por operacion
- `GET /api/logs?tail=n` lee el tail con memoria acotada y puede devolver warning si el archivo se capeo por bytes.
- Endpoints mutantes (`POST /api/workspaces/{workspace}/warm`) requieren admin token pre-compartido en `state.json` y validacion explicita de `Host` y `Origin` headers para CSRF/DNS-rebinding local.
- Frames de protocolo daemon-CLI limitados a MaxFrameSize 256MB para prevenir OOM DoS por header malformado; se valida antes de alocar.

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

### Public install/update contract

Public installers are shell wrappers around the release contract, not new runtime semantics:

- `scripts/install/install.ps1` and `scripts/install/install.sh` install or update only the CLI.
- `scripts/install/install-agent.ps1` and `scripts/install/install-agent.sh` run the CLI installer and then install the repo skill through `npx skills add fgpaz/mi-lsp --skill mi-lsp -g -a codex -a claude-code -y`.
- Supported public archive RIDs are exactly `win-x64`, `win-arm64`, `linux-x64`, `linux-arm64`, `darwin-x64`, and `darwin-arm64`; Darwin archives map to worker RIDs `osx-x64` and `osx-arm64`. Unsupported OS/arch combinations must fail with an actionable message.
- Installers consume GitHub `releases/latest`, derive archive names from GoReleaser (`mi-lsp_<version>_<rid>.zip|tar.gz`), download `mi-lsp_<version>_checksums.txt`, and verify SHA256 before extracting.
- The extracted install must preserve `mi-lsp(.exe)` plus `workers/<rid>/`; if a user moves only the binary, `mi-lsp worker install` is the repair path.
- Existing daemons should be stopped before replacing a target binary; after replacement, `daemon restart` is recommended when daemon-backed state is in use.
- Successful install verification must run the installed path through `version --format toon` and `worker status --format compact|toon`.
- PATH shadowing remains diagnosed through `where.exe mi-lsp` on Windows, `command -v mi-lsp` on Linux, `version.cli_path`, and `worker status.cli_path`.
- When a release changes telemetry, search routing, safe-degrade planner behavior, or provenance fields, release evidence must also include `admin export --summary --by-route --by-client --by-hint --by-failure-stage`, plus `version --format toon` from the installed path used by the agent.

## Payload, error y compatibilidad

- `daemon start` debe devolver la instancia existente si ya corre.
- `daemon status` debe exponer `state`, `daemon_process`, `watchers`, `active_runtimes` y `recent_accesses`.
- `state` debe incluir metadata del ejecutable del daemon (`executable_path`, `executable_size`, `executable_mtime`, `executable_sha256`) cuando el daemon corre una build actual; si falta o si el hash/tamano/mtime prueban que difiere del CLI que invoca `daemon status`, el CLI debe emitir warning accionable con `mi-lsp daemon restart`. El hash tiene prioridad para evitar falsos positivos cuando `go run` usa paths temporales distintos para el mismo contenido.
- Si una operacion daemon-aware excede `max_inflight`, devuelve envelope `ok=false`, item con `error_kind=daemon`, `error_code=backpressure_busy`, y warning `daemon/backpressure_busy`.
- Ese mismo caso debe mapear `error.kind=daemon`, `error.code=backpressure_busy`, `error.stage=backend`, `error.retryable=true` y persistir `failure_stage=backend` en telemetria.
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
