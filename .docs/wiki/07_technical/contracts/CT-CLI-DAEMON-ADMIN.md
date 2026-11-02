---
wiki_contract: WIKI-CONTRACT-v1
id: CT-CLI-DAEMON-ADMIN
kind: contract
status: active
title: CT-CLI-DAEMON-ADMIN
links:
  supports:
  - RF-IDX-001
  - RF-WKS-001
  code:
  - path: internal/service/doctor.go
---

```yaml
harness_protocol: SDD-HARNESS-v1
id: CT-CLI-DAEMON-ADMIN
kind: cli-contract
audience: dual
imports: [.docs/wiki/07_technical/TECH-DAEMON-GOBERNANZA.md, .docs/wiki/07_technical/TECH-LOCAL-DAEMON.md]
exports: [CT-CLI-DAEMON-ADMIN]
agent_must_read: [.docs/wiki/07_technical/contracts/CT-CLI-DAEMON-ADMIN.md, internal/cli/daemon.go, internal/service/doctor.go]
agent_may_edit: [.docs/wiki/07_technical/contracts/CT-CLI-DAEMON-ADMIN.md]
agent_must_not_edit: [internal/cli/, internal/daemon/, ~/.mi-lsp/daemon/state.json]
verify: ["Leer .docs/wiki/07_technical/contracts/CT-CLI-DAEMON-ADMIN.md y .docs/wiki/07_technical/TECH-DAEMON-GOBERNANZA.md; cotejar las afirmaciones con la fuente publicada internal/cli/daemon.go y sus límites. Registrar únicamente comandos y resultados efectivamente ejecutados; esta especificación no afirma una ejecución."]
stop_if: [Una propuesta borre directorios/worktrees/índices o debilite aislamiento, permisos o autenticación local de endpoints mutantes.]
evidence: [.docs/wiki/07_technical/contracts/CT-CLI-DAEMON-ADMIN.md, .docs/wiki/07_technical/TECH-DAEMON-GOBERNANZA.md, internal/cli/daemon.go]
```

## Consumidor

CLI, agentes y consumidores locales de las operaciones descritas; no se amplía la autoridad del consumidor.

### Frontera y responsable
- Boundary: usuario/agente/browser local -> CLI/daemon
- Owner logico: CLI surface + Runtime supervision
- Scope: comandos, flags globales, requests a daemon y endpoints admin locales
- El pool compartido del nodo usa `bots-herramientas/mold/daemon` en la revisión publicada `b0d1d19b04f0d557edec47cefd965a0b21a2d3cd` (`github.com/fgpaz/bots-herramientas/mold/daemon@v0.0.0-20261024021848-b0d1d19b04f0`) y el nombre `mi-lsp-<nodo>`; el listener y los requests legacy permanecen compatibles. El cliente intenta el endpoint común antes del legacy y usa `DialContext` para respetar cancelación/deadline durante la conexión. Solo la ausencia del endpoint antes de enviar permite fallback legacy. Después de conectar, cancelación, error de dominio/admisión, error de escritura o lectura no reenvían ni redispatchan. La cancelación cierra la conexión. `Submit` propaga su contexto al `Handler`; el servidor lo cancela ante EOF/error real de lectura mientras el handler corre, no al primer byte ni al iniciar la petición siguiente. Este contrato se deriva de la fuente publicada; el runtime de este checkout no se declara verificado.

## Operaciones

Se conservan requests, envelopes, campos, límites y semántica detallados a continuación.

Volver a [07_technical/contracts/CT-TECHNICAL-BASELINE.md](CT-TECHNICAL-BASELINE.md).

### Resumen
Define la frontera entre clientes locales y el runtime compartido: CLI publica, control del daemon y superficie admin/gobernanza.

### Familias de contrato
#### CLI publica

Comandos canónicos:

- `workspace add|scan|list|warm|status|remove|doctor|hygiene|prune|which`
- `nav symbols|find|refs|overview|outline|service|search|context|deps|ask|pack|prepare|batch|related|workspace-map|diff-context|affected|trace|intent|suggest`
- `index [path] [--clean] [--docs-only] [--entrypoint <id|repo-relative-path>]`
- `index start|status|cancel` (`start` admite `--entrypoint <id|repo-relative-path>`)
- `info`
- `doctor` (alias unificado para diagnóstico workspace-aware)
- `daemon start|stop|status|restart|open|logs [--tail N]`
- `daemon perf-smoke [--callers N] [--max-working-set-mb N] [--max-private-mb N] [--max-handles N]`
- `worker install|status`
- `admin open|status`
- `version`

- **Marcador estable**: CT-CLI-DAEMON-ADMIN.workspace-prune-v2
- **description**: Workspace prune command (GC for dead aliases)
- **command_signature**: workspace prune --stale [--apply] [--dry-run]
- **Comportamiento**:
  - **default**: dry-run preview
  - **--stale**: required; scans for aliases cuya workspace_root no exista
  - **--apply**: ejecuta la purga; crea backup registry.toml.bak-<timestamp>
  - **--dry-run**: default true; preview sin mutar
  - **result**: purga SOLO aliases cuya raíz no existe o está vacía; NO borra archivos ni worktrees

- **Marcador estable**: CT-CLI-DAEMON-ADMIN.registry-gc-v1
- **description**: Registry GC command (limpieza de aliases con raiz inexistente)
- **command_signature**: registry gc [--apply] [--dry-run]
- **Comportamiento**:
  - **default**: dry-run preview
  - **--dry-run**: default true; lista candidatos alias/path/reason sin mutar
  - **--apply**: ejecuta la purga; crea backup registry.toml.bak-<timestamp>
  - **criteria**: purga SOLO aliases cuya workspace_root está vacía o no existe (os.Stat NotExist); nunca por antiguedad ni por falta de index
  - **note**: coexiste con 'workspace prune --stale' (legacy compatible); mismo report WorkspacePruneReport

Flags globales mínimos:

- `--workspace`
- `--axi`
- `--classic`
- `--full`
- `--format compact|json|text|toon|yaml`
- `--token-budget`
- `--max-items`
- `--max-chars` (global; `0` = sin tope explícito; un valor `> 0` es el tope de caracteres y gana sobre defaults AXI; el recorte conserva `continuation.next` y un marcador de truncación)
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

- **Marcador estable**: CT-CLI-DAEMON-ADMIN.native-plugin-surface
- **description**: Superficie publica de la puerta nativa; la autoridad sigue siendo la CLI
- **authority**: CLI
- **protocol**: mi-lsp-v1.1
- **decision**: TECH-MCP-ADAPTER
- **commands**:
  - **workspace_which**: solo lectura; workspace resuelto y ruta del ejecutable en uso; no muta el registry
  - **nav_suggest**: sugerencia acotada; --event user_prompt [--prompt] o --event post_tool --tool --consecutive; salida mi-lsp nav … o JSON command separado de hint/suggested_command; no reemplaza nav intent
- **binary_override**:
  - **name**: MI_LSP_BIN
  - **must_point_at**: real_executable
  - **windows**: mi-lsp.exe
  - **cmd_shim**: not_valid
- **max_chars**:
  - **flag**: --max-chars
  - **scope**: global
  - **unset**: 0
  - **explicit_positive**: hard_character_cap
  - **truncation_keeps**:
    - continuation.next
    - truncation_marker
- **terminal_fallback**:
  - **fields**:
    - reason_code
    - detail
  - **detail**: separate_canonical_sanitized_never_raw_input
  - **allowlist**:
    - **reason_code**: unsupported_operation
    - **detail**: the requested operation is not supported
    - **reason_code**: unavailable_binary
    - **detail**: the required backend binary is unavailable
    - **reason_code**: invalid_workspace
    - **detail**: the requested workspace is invalid
    - **reason_code**: explicit_incomplete
    - **detail**: the result is explicitly incomplete
  - **source**:
    - CT-NAV-INTENT
    - CT-GRAPH-CLI
- **host_doors**:
  - **path**: integrations/
  - **optional**: true
  - **authority**: CLI

#### Reparación manual puntual de `[project].name`

Este bloque define exclusivamente el procedimiento manual de [[RF-WKS-001]]; no agrega una API ni un comando público. `workspace add --no-index` evita indexación, pero reconstruye/serializa configuración y hace upsert del registro: no es una reparación conservadora de un valor.

- **Marcador estable**: CT-CLI-DAEMON-ADMIN.project-name-manual-v1
- **owner**: Owner de configuración del target confirmado
- **scope**: Solo el valor TOML name dentro de [project] en <raíz-física>/.mi-lsp/project.toml
- **preconditions**:
  - Owner confirma raíz física absoluta, archivo concreto y target correcto; distinguir main de cada worktree
  - Confirmar CLI/revisión en uso y contrato aplicable; no extrapolar una revisión de fuente a otro ejecutable o target
  - Confirmar por lectura del registro que el alias deseado existe y apunta exactamente a esa raíz física, no solo al mismo Git common dir
  - Si el alias falta o apunta a otra raíz, no editar ni reasignarlo
  - El archivo existente es TOML válido y el único desajuste a reparar es el valor name de [project]
  - Owner asegura ausencia de writer concurrente; no intervenir WIP ajeno ni cerrar sesiones para obtener exclusividad
- **procedure**:
  - **1**: Conservar original privadamente, incluidos bytes y permisos; registrar privadamente valor original, valor deseado y raíz confirmada
  - **2**: Releer antes de editar; si difiere del original o no puede asegurarse exclusividad, detenerse sin sobrescribir
  - **3**: Editar únicamente el valor TOML de name bajo [project], con representación/escapes válidos del alias deseado; no reemplazar otras claves name
  - **4**: Preservar todos los bytes fuera de ese valor: comentarios, espaciado, finales de línea, topología y otros settings; mantener permisos
  - **5**: Si la herramienta reserializa el archivo o no garantiza ese alcance puntual, no usarla: declarar gap específico con owner
  - **6**: Relectura normal readonly con raíz absoluta explícita confirma raíz/alias resultantes y conservación del resto; no registrar, indexar ni arrancar daemon para comprobar
- **rollback**:
  - **allowed**: Solo sin cambios concurrentes desde la edición, tras releer el archivo y confirmar el valor recién escrito y el resto esperado
  - **action**: Reponer únicamente el valor original de name en [project], preservando el resto de bytes y permisos actuales
  - **concurrent_change**: No sobrescribir; dejar gap de conciliación con owner para reparación puntual sobre la versión vigente
  - **forbidden**: Nunca restaurar el backup completo encima de una configuración reciente ni tratarlo como rollback transaccional automático
- **forbidden_actions**:
  - Cambiar registry, last_workspace u otros defaults, topología, raíz física, índice o graph
  - Usar init, workspace add/upsert, index/reindex, reset o restart como workaround
  - Inventar rename/config set público, DDL, automatización o garantías de atomicidad
- **acceptance_boundary**:
  - **permitted**: Comprobación posterior por lectura normal readonly de la raíz/alias concretos y del cambio de un único valor
  - **not_implied**: No es qPASS, matriz nueva, probe, job, validator ni evidencia de reparación de catálogo/generaciones/null
  - **failed_precondition**: No ejecutar receta; informar condición incumplida, owner y salida manual específica

#### Workspace mismatch guard

Cuando un comando workspace-aware recibe `--workspace <alias>` y el `caller_cwd` esta dentro de otro workspace registrado:

- `client_name` humano conserva compatibilidad: ejecuta sobre el alias explícito y emite warning con alias/root seleccionado, cwd, workspace/root detectado por cwd, comando recomendado y `--allow-cross-workspace`.
- `client_name` harness/agente (`codex`, `claude-code`, `claude-ai`, `opencode`, `copilot`, `jetbrains`, `cursor`, `neovim`, `emacs`, `vim`) rechaza la operación salvo `--allow-cross-workspace`.
- El error estructurado debe usar `error.kind=workspace`, `error.code=workspace_cross_workspace_refused`, `error.stage=selector_validation` y `error.hint_code=workspace_cross_workspace_refused`.
- El override `--allow-cross-workspace` permite continuar pero no suprime el warning; el warning es evidencia de intención cross-workspace.
- `index --docs-only`
- `index start --mode full|docs|catalog --wait`
- `daemon start|restart|serve --watch-mode off|lazy|eager --max-watched-roots N --max-inflight N --max-workers N`

- **Marcador estable**: CT-CLI-DAEMON-ADMIN.daemon-flags-v2
- **description**: Daemon start flags with new defaults
- **defaults**:
  - **watch_mode**: lazy
  - **max_watched_roots**: 8
  - **max_inflight**: 48 (env MI_LSP_DAEMON_MAX_INFLIGHT)
  - **max_workers**: 8 (env MI_LSP_DAEMON_MAX_WORKERS; entero positivo; flag --max-workers 0 = env u 8)
  - **max_runtime_memory_mb**: 1024 (env MI_LSP_DAEMON_MAX_RUNTIME_MEMORY_MB)
  - **soft_memory_threshold_mb**: 500
  - **wal_checkpoint_interval_sec**: 1800
- **note**: max_inflight raised from 16 to 48; max_workers raised from 3 to 8 (one shared default for start, serve, restart, auto-start and manager fallback) so concurrent C#/Go/Python/TS/Rust/Kotlin projects do not thrash expensive runtimes via LRU

#### Estado público del daemon

- `daemon status`, `daemon restart`, `daemon start` y `admin status` nunca exponen el valor de `admin_token`; muestran `[REDACTADO:token]` cuando hay un token.
- El daemon conserva el token real en memoria/estado persistido para autenticación; la redacción aplica únicamente a la salida CLI.

#### `index` + indexación async

Input:

```text
mi-lsp index [path] [--workspace <alias>] [--clean] [--docs-only] [--entrypoint <id|repo-relative-path>] [--go-offline]
mi-lsp index start [path] [--workspace <alias>] [--mode full|docs|catalog] [--clean] [--wait] [--entrypoint <id|repo-relative-path>] [--go-offline]
mi-lsp index status [job-id] [--workspace <alias>]
mi-lsp index cancel <job-id> [--workspace <alias>] [--force]

mi-lsp init [path] [--name alias] [--no-index]
mi-lsp workspace add [path] [--name alias] [--no-index]
```

Reglas:

- `index [path]` es wrapper de compatibilidad que ejecuta `index start --mode full --wait`; con `--docs-only`, ejecuta `--mode docs --wait`.
- `index start` crea un registro durable en `index_jobs` sin consultar primero jobs activos; sin `--wait` lanza un proceso detached y retorna el `job_id`; con `--wait` bloquea hasta que la indexación complete. Si hay competencia, el runner queda serializado por `.mi-lsp/index.lock` y el resultado se inspecciona con `index status`.
- `--entrypoint` selecciona un ID declarado o una ruta relativa segura del repositorio a la entrada solicitada en modo `full`. Go acepta `go.mod` y dirige la observación al backend `go`; C# acepta proyectos `.csproj` declarados o rutas seguras compatibles y dirige la observación a Roslyn; las soluciones `.sln` explícitas fallan cerrado porque no existe un helper de membresía autoritativo. No aplica a `docs`/`catalog`. Sin el flag se conservan exactamente los defaults actuales y no se infiere un módulo Go. El selector se persiste con el job y se rechazan rutas absolutas, traversal, symlinks, `go.work`, tipos no compatibles o repositorios no coincidentes.
La política offline y su envelope se definen exclusivamente en este bloque; aceptación acotada en [[TP-IDX]] / TC-IDX-029.

- **Marcador estable**: CT-CLI-DAEMON-ADMIN-GO-OFFLINE
- **requirement_id**: RF-IDX-001
- **observable_contract**: RF-IDX-001-GO-OFFLINE
- **public_routes**: index wrapper direct wait; index start direct wait o detached; run-job recarga go_offline durable
- **daemon_route**: CLI index-daemon unsupported/noapplicable actual; dispatch App/server interno compartido no establece paridad ni habilita routing
- **durable_policy**: `--go-offline` es opt-in (false por default) en `index` y `index start`: payload `go_offline` booleano y columna durable `index_jobs.go_offline` (legacy=0). `index run-job` carga esa columna; no necesita reenviar un flag ni compartir el entorno del cliente. La proyección pública del job/status incluye `go_offline` sin valores privados.
- **process_and_metadata**: Toda invocación Go del observador usa GOTOOLCHAIN local, GOPROXY/GOSUMDB off, GOVCS *:off, GOFLAGS con un espacio (cero flags, override no vacío del valor persistido), GOWORK off y `-buildvcs=false -mod=readonly`. Ignora GOFLAGS heredados/configurados y observa el módulo sin `go.work` (evita modificar su sum); el resto del entorno y caches locales se preserva. `-modfile` apunta a copias temporales propias de go.mod/go.sum, porque readonly por sí solo no impide agregar sumas; los archivos del usuario quedan intactos. No modifica configuración global ni instala nada. El importador ambiental se deshabilita en ese modo.
- **completeness**: Full offline recompone el candidato incluso sin cambios de catálogo; fingerprint distingue la política y warning `go_offline=true` registra la observación efectiva. Capabilities existentes conservan estado real: compilación/export local permitidos, metadata ausente devuelve `GPH_GO_OFFLINE_UNAVAILABLE`, metadata parcial puede producir diagnóstico genérico/index_failed, type_check_error o external_target; go_offline_metadata_unavailable no es universal y la precisión offline específica queda explicit_incomplete. Nunca retry online ni facts de compilador por fallback textual.
- **scope_and_mismatch**: Docs/catalog no ejecutan ese observador y no se alteran; el flag no bloquea embeddings, otros backends ni consultas gopls, ni es un sandbox de red para un CC/cgo externo personalizado. La deduplicación compara el `go_offline` persistido del activo con el solicitado: si difieren en cualquiera de los dos sentidos, devuelve `ok=false`, `error.kind=validation`, `error.code=index_job_policy_mismatch`, `error.stage=index.start` y el job público real en items, con job_id y ambas políticas en el mensaje. No altera/cancela el activo, no crea otro runner ni hace retry automático; next_hint apunta a status y el reintento es manual después de terminal. Con política coincidente se conserva la deduplicación previa (sin nuevas restricciones de mode/clean/entrypoint). Contrato funcional y aceptación: [[RF-IDX-001]] / RF-IDX-001-GO-OFFLINE y [[TP-IDX]] / TC-IDX-029.
- **verification_basis**: FINAL_VERIFY1 FAIL histórico; RECHECK1 PASS policy / PARTIAL global; audit1 Changes requested, reparación editorial no equivale a accepted/complete
- **evidence_scope**: Rechazo simétrico, dedupe coincidente, snapshots durables sin mutación y retry terminal comprobados; stdlib bwrap succeeded, namespace distinto, solo lo y sin rutas
- **evidence_limits**: No cero syscall, ejecución normal fuera de namespace, salud global ni capacidades/precisión externas completas; graph query daemon no prueba observador
- **owner**: coord-milsp
- **disclosure_due**: antes de publicación acotada
- **followup_due**: 2026-10-20
- **followup_authority**: Precisión/evidencia futura con objetivo separado, sin renovación de este presupuesto
- **budget**: FINAL_VERIFY1/audit1/RECHECK1 agotado; sin recheck2 ni verificador pendiente
- **readiness_debt**: Sin migración global ni PASS Harness/wiki-source/P6/Graph; deuda preexistente preservada

- `index status` consulta el ultimo job del workspace si no se pasa `job-id`.
- `index status.phase` conserva `indexing` durante el trabajo pesado y solo pasa a `publishing` en el cierre/publicacion final.
- `index status` expone progreso vivo en `current_stage`, `current_path`, `files_total`, `files`, `symbols`, `docs` y `updated_at`; esos campos deben refrescarse durante catalogo/docs/embeddings antes de publicar. En embeddings, `current_stage=embeddings` y `current_path` incluye `N/M chunks embedded`.
- `index cancel` marca cancelacion solicitada; la cancelacion es cooperativa y puede no interrumpir una publicación que ya llego al commit.
- `index cancel --force` puede terminar el PID vivo del job, marcarlo `canceled` y remover el `.mi-lsp/index.lock` si pertenece a ese PID ya muerto; se reserva para jobs colgados.
- `workspace.add`, `init` usan **hibrido smart-sync** (FD1): indexan sincronicamente dentro de `SmartSyncTimeout` (`MI_LSP_INDEX_SYNC_TIMEOUT`, 20s) para preservar el contrato init-then-query; si una primera indexación grande lo excede, degradan a background y devuelven `job_id` con warning. `--background`/`background:true` fuerza async inmediato; `--wait`/`wait:true` fuerza sync completo (`IndexTimeout`, `MI_LSP_INDEX_TIMEOUT` default 30min); con `[embeddings]` activo el wrapper de index puede elevar ese presupuesto por corpus/batches o por `[embeddings].index_timeout_ms`; `--no-index` omite indexacion.
- sin `--docs-only`, indexa catálogo de código y grafo documental, con incremental git-aware cuando corresponde.
- con `--docs-only`, reconstruye `doc_records`, `doc_edges`, `doc_mentions` y `memory_pointer` sin reemplazar `files` ni `symbols`; cada invocación vuelve a descubrir el corpus actual y publica create/change/delete, incluso sin cambios Git detectables.
- toda indexación toma `.mi-lsp/index.lock`; si ya existe, la operación debe fallar con mensaje accionable que incluya el lock owner cuando este disponible.
- locks con PID inexistente se consideran stale y pueden recuperarse automaticamente.
- la publicación full de catálogo + docs + memoria es all-or-nothing dentro de SQLite.
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
- Si existe `continuation`, TOON la serializa antes del resto del envelope y `continuation.next` contiene un comando CLI ejecutable en una sola línea; JSON conserva el objeto `ContinuationTarget`.
- La sanitizacion TOON reemplaza controles no imprimibles, excepto tab/newline/carriage-return, por escapes ASCII visibles (`\u0000`, `\u001f`, etc.).
- Cuando la sanitizacion cambia al menos un string, `warnings` debe agregar una única entrada `toon output sanitized unsafe control characters`.
- `--format compact`/JSON mantiene su comportamiento compatible existente y no debe depender de la sanitizacion TOON.

#### `doctor`: salida única y error ya emitido

`doctor` de salud (distinto de `workspace doctor`) emite el envelope `backend=doctor` mediante el formatter común. Cuando el daemon responde, agrega `daemon_pool` con las estadísticas del pool compartido (`listo`, `vivo`, `ayudantes`, `fila`, `capacidad_fila`, `rechazos_ocupado`, `memoria_bytes`), derivadas de la misma respuesta `system.status`; si el daemon no está disponible, omite ese campo. No sustituye el estado por métricas del pool Roslyn/workspace. `daemon-version-drift` compara ambos strings de versión tras quitar como máximo un prefijo `v` cuando precede un dígito; no relaja comparaciones de otras identidades cuando existan. Si los checks fallan, conserva `ok=false`, checks y resumen, y retorna el sentinel `envelopePrintedError` por valor: coincide con el `errors.As` existente de `IsEnvelopePrintedError`. El handler externo en `cmd/mi-lsp/main.go` no vuelve a emitir `WriteProcessFailure` para ese error; preserva el exit code envuelto, distinto de cero. Los errores anteriores a la emisión y los fallos de impresión no llevan sentinel y siguen por el handler de errores normal.

- **Marcador estable**: cli-doctor-single-emission
- **formats**:
  - text
  - json
  - toon
- **formatter**: common_unchanged
- **failed_checks**:
  - **envelope_count**: 1
  - **exit_code**: report_exit_code_nonzero
  - **second_items_null_document**: forbidden
- **error_matching**: errors.As_with_Unwrap
- **scope**: source_only
- **runtime_status**: explicit_incomplete

La reparación no modifica los checks, el acceso al daemon ni afirma el contrato read-only completo del candidato. El FAIL histórico y LSP-03/04/05/06 NOT_RUN permanecen intactos; presupuesto FINAL_VERIFY/audit/recheck agotado.

#### `version`

Input:

```text
mi-lsp version [--format text|compact|json|toon|yaml]
```

Reglas:

- `version` es una operación local, read-only y sin dependencia de workspace, registry, daemon o workers vivos.
- Sin `--format` explícito, emite salida `text` legible para humanos.
- Con `--format` explícito, respeta el envelope comun y devuelve `backend=version`.
- El item expone `command`, `version`, `module_path`, `go_version`, `goos`, `goarch`, `protocol_version`, `worker_rid`, `tool_root`, `cli_path`, `executable_sha256`, `vcs_revision`, `vcs_time` y `vcs_modified` cuando Go build info los provee.
- `vcs_modified=false` es la prueba operativa de que el binario fue construido desde un árbol limpio; si falta metadata VCS, los campos se omiten o se exponen como desconocidos en `text`.
- El comando no reemplaza `worker status`: `version` prueba provenance del ejecutable; `worker status` diagnostica candidatos de worker y compatibilidad.

#### `admin export --summary`

El summary puede incluir un bloque aditivo `recommendations` para usage-doctor. Cada item debe derivarse de telemetria agregada y sanitizada (`hint_code`, `failure_stage`, `truncation_rate`, latencias, breakdowns y conteos), incluir acción sugerida y razon breve, y nunca copiar query cruda, argv, payloads, paths sensibles ni contenido de archivos.

Sin `--limit` explícito, el summary agrega toda la ventana filtrada mediante acumulacion streaming desde `daemon.db`; no debe cargar todos los eventos crudos en memoria. Si el usuario pasa `--limit`, el summary conserva la semántica de muestra acotada. `--by-backend`, `--percentile`, `--by-route`, `--by-client`, `--by-hint` y `--by-failure-stage` siguen siendo opt-in de visualizacion. `--format json` y `--format compact` deben serializar `ExportSummary` como JSON válido; `--format toon` usa TOON y la salida humana tabular queda reservada para formatos text/csv compatibles.

`--summary --attribution` es un bloque aditivo y opt-in (requiere binario actualizado): expone cobertura de atribucion por cliente y sesion sin copiar eventos crudos, patrones, argv, transcripts ni texto de error. Denominadores y límites: `client_name` vacio o `manual-cli` cuenta como atribucion desconocida; `real_sessions` cuenta IDs de sesion distintos y nunca es un conteo de repeticiones por sesion; los cohortes `work`/`test`/`system`/`unknown` son heuristicas derivadas de marcadores de nombre, presencia de sesion y operación `bench` (que cuenta como `test`, nunca como `work`), no prueba de trabajo real por evento; los workspaces con prefijo `demo` solo se etiquetan como candidatos; `repeated_failures` y `slow_operations` son candidatos agregados, no repeticiones probadas ni tokens desperdiciados; la telemetria mi-lsp no estima costo ni ahorro de tokens (el costo de modelo pertenece a la integracion nativa Pi) y nunca reporta costo cero para costo desconocido. Uso recomendado: bajo demanda durante analisis y en cierre de ciclo.

La telemetria asigna `q_parse_failed`, `q_execute_failed`, `q_workspace_failed` o `q_projection_failed` según `error.stage` de q, sin cambiar los códigos públicos congelados de q-v1. El fallback de `nav.find` agrega `nav_find_index_absent`, `nav_find_index_unreadable` o `nav_find_index_broken` según el estado del catálogo. Estos códigos son agregados sanitizados: nunca se persisten consultas crudas ni diagnósticos SQLite.

#### `daemon logs`

`daemon logs --tail N` muestra el tail del log local del daemon. Puede omitir líneas benignas de cierre normal de sockets/pipes (`use of closed network connection`, pipe cerrado, reset/broken pipe) junto con el bloque de ayuda generado por Cobra, para que el tail diagnóstico no parezca fallo accionable cuando solo refleja shutdown.

Envelope de error estructurado (`ok=false`):

- `ok=false`
- `backend` con el subsistema que intento responder
- `items=[]` salvo diagnóstico seguro y tipado
- `warnings` con códigos accionables cuando existan
- `error.kind` (`daemon`, `workspace`, `worker_bootstrap`, `sdk`, `backend_runtime`, `validation`, `transport`)
- `error.code` estable y grep-friendly
- `error.message` breve, sin stack trace ni payload crudo
- `error.stage` opcional (`selector_validation`, `router`, `backend`, `transport`)
- `error.retryable` opcional
- `error.hint` opcional con remediacion concreta
- `stats` y `truncated=false` cuando el fallo ocurre antes de emitir items

#### `nav service`

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
- si el catálogo es insuficiente, la operación sigue con evidencia textual y warning

#### `nav context`

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

- contrato slice-first: el core devuelve primero el bloque legible alrededor de la línea
- `backend=text` para archivos no semánticos
- `backend=roslyn`, `backend=tsserver` o `backend=catalog` cuando hay enriquecimiento correspondiente
- si el backend semántico falla pero el archivo existe, la operación sigue con `slice_text`, degrada a catalog/text cuando sea posible y agrega warning accionable
- si el warning proviene de bootstrap Roslyn, debe sugerir `mi-lsp worker install`; si proviene de SDK/global.json, la telemetria debe clasificarlo como `sdk/*`; si proviene de permisos/arranque de proceso, debe clasificarlo como `backend_runtime/process_spawn_access_denied` o `backend_runtime/process_spawn_failed`

#### `nav affected`

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
- si no hay paths explícitos ni stdin, puede usar git diff del workspace como input por default
- `--from-git-diff` debe considerar cambios staged, unstaged y untracked cuando `--changed-ref` queda en `HEAD`
- `--include-tests` agrega comandos sugeridos de prueba por familia de path y respeta `--test-command` como override literal del usuario
- `--include-docs` agrega docs canónicos por familias de paths gobernadas, sin editar ni validar esos docs
- debe ignorar sidecars operativos `.mi-lsp/**`, `.docs/raw/**`, `.docs/auditoria/**` y `.git/**`
- toda respuesta heuristica debe incluir warning visible y conservar `confidence` como dato advisory

#### CLI -> daemon

Reglas de routing:

- `nav.find`, `nav.intent`, `nav.symbols`, `nav.outline`, `nav.overview`, `nav.multi-read`, `nav.affected` y `nav.intent` no deben cruzar esta frontera en el hot path.
- `nav.ask`, `nav.search`, `nav.pack` son daemon-elegibles cuando daemon vive (sin auto-start; fallback directo si daemon no responde).
- `nav.refs`, `nav.context`, `nav.deps`, `nav.related`, `nav.service`, `nav.diff-context` y `nav.batch` pueden preferir daemon cuando corresponda.
- `nav.workspace-map` summary-first queda directo por default; `--full` puede seguir siendo una operación pesada pero no debe autostartear daemon en el contrato base.
- `workspace.warm` puede preferir daemon pero no debe auto-iniciarlo.

- **Marcador estable**: CT-CLI-DAEMON-ADMIN.daemon-eligible-v2
- **description**: Operations that prefer daemon when available
- **daemon_eligible_ops**:
  - nav.ask
  - nav.search
  - nav.pack
  - nav.governance
  - nav.route
  - nav.prepare
- **Comportamiento**:
  - **auto_start**: false
  - **fallback**: direct if daemon unavailable
  - **caching**: LRU 256 entries, TTL 10 min per operation+workspace+generation

Canal:

- Windows: named pipe
- Linux: unix socket

Request envelope actual:

- `protocol_version` (requerido, no puede estar vacio; versiones incompatibles rechazadas tempranamente)
- `operation`
- `context`
- `payload`

Metadata mínima en `context`:

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
- si `route=daemon` y la ejecución fue normal, el registro canónico lo escribe el daemon; la CLI no debe duplicar esa misma operación en `access_events`
- `result_count` representa items emitidos en el envelope final, no `Stats.Symbols`

`admin export` debe soportar:

- raw `json|csv|compact|toon`
- ventana `--since`/`--recent`
- filtros `--workspace`, `--backend`, `--operation`, `--session-id`, `--client-name`, `--route`, `--query-format`, `--truncated`, `--pattern-mode`, `--routing-outcome`, `--failure-stage`, `--hint-code`
- `--summary` sobre toda la ventana filtrada salvo `--limit` explícito, incluyendo salida `toon`
- breakdowns opcionales `--by-route`, `--by-client`, `--by-hint`, `--by-failure-stage`

#### Governance admin

Endpoints mínimos:

- `GET /`
- `GET /api/status?window=<recent|7d|30d|90d>`
- `GET /api/workspaces?window=<recent|7d|30d|90d>`
- `GET /api/workspaces/{workspace}?window=<recent|7d|30d|90d>`
- `POST /api/workspaces/{workspace}/warm` (requiere admin token + validación Host/Origin)
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

Deep-link admin canónico:

- `/?workspace=<alias>&panel=<overview|activity|logs|metrics>&window=<recent|7d|30d|90d>&backend=<type>`

Reglas:

- solo `127.0.0.1` (loopback local)
- una UI global
- acciones seguras solamente
- query params, no hash-state
- el resumen agregado debe distinguir cortes por workspace y por operación
- `GET /api/logs?tail=n` lee el tail con memoria acotada y puede devolver warning si el archivo se capeo por bytes.
- Endpoints mutantes (`POST /api/workspaces/{workspace}/warm`) requieren admin token pre-compartido en `state.json` y validación explícita de `Host` y `Origin` headers para CSRF/DNS-rebinding local.
- Frames de protocolo daemon-CLI limitados a MaxFrameSize 256MB para prevenir OOM DoS por header malformado; se valida antes de alocar.

#### Comandos del workspace

##### `workspace remove`

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

#### Comandos del daemon

##### `daemon restart`

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

#### Comandos del worker

##### `worker install`

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

##### `worker status`

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
- si el daemon atiende esta operación, debe devolver exactamente el mismo envelope canónico que el modo directo; `active_workers` queda anidado dentro del item diagnóstico
- en repo de desarrollo, los artefactos locales `bin/workers/<rid>` no deben presentarse como bundle canónico de distribucion

#### Public install/update contract

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

### Referencias de contexto
- [TECH-DAEMON-GOBERNANZA.md](../TECH-DAEMON-GOBERNANZA.md)
- [TECH-SERVICE-EXPLORATION.md](../TECH-SERVICE-EXPLORATION.md)
- [DB-STATE-Y-TELEMETRIA.md](../db/DB-STATE-Y-TELEMETRIA.md)

## Errores

Se mantienen las razones, warnings, rechazos y fallbacks explícitos de cada operación.

### Payload, error y compatibilidad

- `daemon start` debe devolver la instancia existente si ya corre.
- `daemon status` debe exponer `state`, `daemon_process`, `watchers`, `active_runtimes`, `recent_accesses` y `elastic_pool` con métricas observadas del servidor común. `coordinator_node` y `coordinator_count_at_start` son metadatos históricos de arranque; el conteo dinámico se informa en `coordinator_sizing`.
- `coordinator_sizing` incluye `source_state` (`current`/`stale`), límites solicitados, `last_successful_count` y `last_success_at`. Si falla el censo, no inventa un conteo: expone `last_error_code=coordinator_source_unavailable` y mantiene los últimos límites válidos; el conteo se etiqueta explícitamente como el último exitoso.
- `doctor` agrega `daemon_pool` cuando el daemon responde, con los mismos siete campos de `elastic_pool` y el objeto de `coordinator_sizing`; la ausencia del daemon deja el campo omitido, no en cero.
- El sizing se refresca cada minuto con context/cancelación de lifecycle y usa `Server.SetLimits`; la biblioteca impone machine cap. No se replica la fórmula ni el cap localmente.
- El daemon anfitrión observa `Server.Wait()` del pool común: el cierre común ordenado (incluido `op=shutdown`) debe cerrar también el listener legacy y su lifecycle, después del ack. El `Shutdown()` del anfitrión cierra el pool común tras marcar el stop para evitar reentrada. El idle común se habilita; su cierre se propaga al anfitrión. Esto describe el contrato implementado, no una verificación runtime.
- `state` debe incluir metadata del ejecutable del daemon (`executable_path`, `executable_size`, `executable_mtime`, `executable_sha256`) cuando el daemon corre una build actual; si falta o si el hash/tamano/mtime prueban que difiere del CLI que invoca `daemon status`, el CLI debe emitir warning accionable con `mi-lsp daemon restart`. El hash tiene prioridad para evitar falsos positivos cuando `go run` usa paths temporales distintos para el mismo contenido.
- Si una operación daemon-aware excede `max_inflight`, devuelve envelope `ok=false`, item con `error_kind=daemon`, `error_code=backpressure_busy`, y warning `daemon/backpressure_busy`.
- Ese mismo caso debe mapear `error.kind=daemon`, `error.code=backpressure_busy`, `error.stage=backend`, `error.retryable=true` y persistir `failure_stage=backend` en telemetria.
- Si no hay daemon, el CLI debe poder ejecutar directo.
- Si falta un backend opcional, devolver warning accionable, no fallo ambiguo.
- `backend` debe reflejar el backend realmente usado.
- `admin open` y `daemon open` deben abrir la misma `admin_url` con deep-link consistente.
- Las fallas de bootstrap del worker deben sugerir remediacion concreta, al menos `mi-lsp worker install` cuando corresponda.

## Compatibilidad

La migración cambia el contrato documental, no el protocolo runtime ni sus versiones. Los esquemas legacy que la herramienta consume continúan como compatibilidad de entrada, no como formato de autoría de esta wiki.

### Versioning y migración

- Cambios incompatibles en request/response del daemon requieren bump de `protocol_version`.
- La UI admin no debe prometer estabilidad publica externa fuera del host local.
