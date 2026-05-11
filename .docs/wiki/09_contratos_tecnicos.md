# 09. Contratos tecnicos

```yaml
harness_protocol: SDD-HARNESS-v1
id: "09_contratos_tecnicos"
kind: "support-doc"
audience: "dual"
imports:
  - '[[00_gobierno_documental]]'
  - '.docs/wiki/09_contratos_tecnicos.md'
exports:
  - '09_contratos_tecnicos'
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/09_contratos_tecnicos.md
agent_may_edit:
  - .docs/wiki/09_contratos_tecnicos.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
evidence:
  - .docs/wiki/09_contratos_tecnicos.md
```

## Proposito y alcance

Este documento resume la superficie de contratos tecnicos de `mi-lsp`: CLI publica, canal CLI-daemon-admin y protocolo daemon-worker.
Su foco es compatibilidad, ownership y postura comun de errores/versionado.
El detalle por frontera vive en `09_contratos/`.

## Inventario de contratos

| Familia | Boundary | Owner logico | Canal |
|---|---|---|---|
| CLI publica | Usuario/agente -> `mi-lsp` | CLI surface | args/stdout/stderr |
| AXI discovery mode | Usuario/agente -> `mi-lsp` | CLI surface | args/stdout/stderr |
| Control daemon | CLI -> daemon | Runtime supervision | named pipe / unix socket |
| Governance admin | Browser/CLI -> daemon | Runtime supervision | HTTP loopback |
| Worker protocol | Daemon/core -> worker Roslyn | Semantic backends | stdio JSON length-prefixed |
| TS semantic bridge | Daemon/core -> `tsserver` | TS semantic backend | `tsserver` Content-Length protocol |
| Pyright LSP bridge | Daemon/core -> `pyright-langserver` | Python semantic backend | LSP JSON-RPC 2.0 Content-Length protocol |

## Boundaries y ownership

- La CLI publica es la frontera estable para humanos, skills y wrappers.
- AXI es parte de la CLI publica como overlay selectivo por superficie: no todo comando entra en AXI por default.
- El repo publica una skill curada en `skills/mi-lsp/` para herramientas compatibles con skills; esa skill documenta buenas practicas de uso, pero no redefine la semantica del CLI.
- El daemon comparte estado entre clientes pero no redefine la CLI publica.
- La UI/admin es una vista local del daemon; no es API publica remota.
- El protocolo daemon-worker es interno, versionado y con envelope estable.
- Cada contrato debe exponer `warnings`, fallas accionables y degradacion clara cuando aplique.
- `worker status` forma parte de la CLI publica y debe exponer `tool_root`, `tool_root_kind`, `cli_path`, `protocol_version`, origen del worker seleccionado y compatibilidad de candidatos.
- `workspace status` forma parte de la CLI publica y debe exponer `docs_read_model`, `doc_count`, `docs_index_ready`, `governance_profile`, `governance_sync`, `governance_index_sync` y `governance_blocked`; en `--full` puede expandir el digest repo-local de memoria de reentrada.
- En AXI efectivo, `workspace status`, `nav search`, `nav wiki search`, `nav wiki validate-harness`, `nav wiki validate-source`, `nav intent`, `nav route`, `nav wiki route`, `nav pack` y `nav wiki pack` pertenecen a la superficie preview/full por default; `nav ask` solo lo hace para preguntas de orientacion y `nav workspace-map` solo cuando se fuerza AXI.
- `init` pertenece a la CLI publica como shortcut de onboarding; no reemplaza `workspace add`, pero reutiliza su semantica base.
- `nav ask` pertenece a la CLI publica y usa un contrato docs-first explainable, no un blob opaco ni una respuesta puramente textual.
- `nav pack` pertenece a la CLI publica y usa un contrato de reading pack canonico, no una respuesta textual libre.
- `nav wiki` pertenece a la CLI publica como superficie documental explicita para agentes; `search` devuelve candidatos wiki, `route|pack|trace` reutilizan las superficies canonicas existentes, `validate-harness` compila readiness de contratos `SDD-HARNESS-v1` y `validate-source` compila readiness de artefactos `SDD-WIKI-SOURCE-v1`.
- `nav governance` pertenece a la CLI publica y devuelve el estado efectivo de gobernanza del workspace.
- `nav service` pertenece a la CLI publica y usa un contrato evidence-first, no uno de scoring.
- `nav context` pertenece a la CLI publica y su salida visible es slice-first; el backend profundo solo enriquece el mismo item.
- `nav intent` pertenece a la CLI publica y expone `mode=docs|code`: en `docs` usa routing documental owner-aware; en `code` conserva el ranking BM25 sobre `search_text`. En workspaces `container`, `--repo` acota solo `mode=code`.

## Versionado, auth y errores

- El proyecto usa compatibilidad best-effort intra-version; cambios incompatibles deben reflejar `protocol_version`.
- La governance UI es solo local (`127.0.0.1`) y no incorpora auth en esta fase; la ventana temporal visible se negocia via `window=recent|7d|30d|90d`.
- Las respuestas del CLI deben seguir envelope estable y explicitar `backend`, `truncated`, `warnings`, `stats` y `hint` (omitempty, presente cuando `items=[]` o daemon no disponible).
- Las fallas deben usar envelope estructurado `ok=false`: conservar `backend`, `warnings`, `stats`, `truncated=false` cuando aplique, y exponer `error {kind, code, message, stage?, retryable?, hint?}` sin texto libre no tipado como unica evidencia.
- El contexto interno de la request puede incluir `caller_cwd`; si el usuario omite `--workspace`, la resolucion observable sigue `selector explicito > caller_cwd > last_workspace`.
- `nav.find`, `nav.search` y `nav.intent` con `--repo` deben aceptar resolucion smart cuando el selector humano tiene un match unico y, si no, devolver candidatos concretos en `items` + `next_hint`.
- `nav ask`, `nav route` y `nav pack` aceptan `--repo` solo por compatibilidad guiada; el flag se ignora para routing documental y el envelope debe incluir warning/hint hacia `nav wiki`.
- En AXI, las respuestas preview-first pueden anunciar expansion via `next_hint` hacia `--full` sin cambiar el envelope base.
- El envelope estable puede agregar `coach` como bloque opcional, query-level y machine-readable; es aditivo y no reemplaza `warnings`, `hint`, `next_hint` ni `next_queries`.
- El envelope estable puede agregar `continuation` como bloque opcional, tiny y machine-readable; su `next` y `alternate` usan campos estructurados (`op`, `query`, `repo`, `path`, `symbol`, `doc_id`, `full`) y no comandos raw.
- El envelope estable puede agregar `memory_pointer` como bloque opcional, wiki-anchored y de muy bajo costo, para señalar cambios canonicos recientes, mejor reentry y estado `stale`.
- El envelope estable puede agregar `mode` cuando la superficie necesita distinguir variantes estables sin cambiar `backend` (por ejemplo `nav.intent docs|code`).
- Los errores de bootstrap deben ser accionables: por ejemplo `Run: mi-lsp worker install`.
- Los contratos internos no deben transportar ASTs ni blobs completos salvo comando futuro explicito.
- `nav ask` debe devolver una estructura explainable con `summary`, `primary_doc`, `doc_evidence`, `code_evidence`, `why` y `next_queries`.
- `nav search` y `nav ask` pueden agregar `coach.trigger` en casos balanceados (`repo_selector_invalid`, `regex_auto_healed`, `no_matches_refinable`, `preview_trimmed`, `text_fallback`, `low_confidence`, `scope_narrowing_available`, `symbol_query_detected`).
- `nav search`, `nav ask`, `nav pack`, `nav route`, `nav.related`, `nav.service` y `nav workspace-map` pueden agregar `continuation.reason` (`recent_change`, `narrow_scope`, `follow_doc`, `expand_preview`, `low_evidence`, `handoff_reentry`) cuando existe un siguiente paso mejor que repetir la misma consulta.
- `nav pack` debe devolver una estructura estable con `task`, `family`, `mode`, `primary_doc`, `docs`, `why` y `next_queries`.
- `nav wiki search` debe devolver `WikiSearchResult` con `doc_id`, `path`, `title`, `layer`, `family`, `stage`, `score`, `why`, `snippet/content` y `next_queries`; `RS-*` y las rutas `02_resultados*` pertenecen a `layer=RS`, `stage=outcome`.
- `nav wiki validate-harness` debe devolver `HarnessValidationResult` con `harness_protocol`, `harness_readiness`, `harness_verdict`, blockers/warnings, conteos de contratos/links revisados, evidencia requerida/encontrada y docs sin contrato/audience.
- `nav wiki validate-source` debe devolver `WikiSourceValidationResult` con `wiki_source_protocol`, `wiki_source_readiness`, `wiki_source_verdict`, blockers/warnings, conteos de artefactos/bloques/records/tablas revisados, `navigation_readiness` y `navigation_blockers`.
- `nav trace <DOC-ID>` debe clasificar explicitamente `RS|RF|TP`: `RS-*` devuelve `doc_id`, `layer=RS`, `stage=outcome` sin poblar el campo legacy `rf`; `RF-*` conserva evidencia spec-to-code y `TP-*` conserva evidencia de cobertura documental.
- `nav trace <DOC-ID>` tambien puede resolver `block_id` o `record_id` fuente exacto desde `doc_source_blocks`/`doc_source_records` y devolver evidencia `wiki-source`.
- `nav governance` debe devolver una estructura estable con perfil, base efectiva, overlays, sync, blockers y siguientes pasos.
- `nav service` puede usar `backend=catalog`, `backend=text` o `backend=catalog+text`.

## Compatibilidad y migracion

- La presencia o ausencia del daemon no debe cambiar la semantica visible de los comandos.
- `daemon start` acepta `--watch-mode off|lazy|eager`, `--max-watched-roots` y `--max-inflight`; el hidden `daemon serve` recibe los mismos valores.
- `MI_LSP_WATCH_MODE`, `MI_LSP_WATCH_MAX_ROOTS` y `MI_LSP_DAEMON_MAX_INFLIGHT` son overrides runtime equivalentes cuando el flag no se pasa.
- Si `--workspace` se omite y el runtime cae a `last_workspace` o resuelve una ambiguedad de aliases sobre el mismo root, el contrato visible debe incluir warning explicito con el alias elegido.
- `MI_LSP_AXI=1` habilita AXI a nivel sesion; `--classic` prevalece sobre defaults/env y `--axi` fuerza AXI en superficies soportadas.
- `--axi=false` explicito anula el default AXI de la superficie actual; equivalente a `--classic` para esa invocacion.
- `--axi` y `--classic` juntos deben fallar antes de ejecutar la operacion.
- `worker status` debe conservar el mismo payload visible con y sin daemon; el daemon no puede reemplazar `items` por `RuntimeSnapshot`/`WorkerStatus` crudos.
- `nav.find`, `nav.search`, `nav.intent`, `nav.symbols`, `nav.outline`, `nav.overview`, `nav.multi-read` y `nav.workspace-map` summary-first pertenecen a la superficie publica directa: no deben esperar daemon ni cambiar de comportamiento por su health.
- `nav.wiki.search`, `nav.wiki.route`, `nav.wiki.pack`, `nav.wiki.trace`, `nav.wiki.validate-harness` y `nav.wiki.validate-source` pertenecen a la superficie publica directa y no deben esperar daemon.
- `nav.ask` tambien pertenece al hot path directo por default; la presencia del daemon no debe ser requisito para una primera respuesta docs-first util.
- `index` puede degradar a full rebuild aun sin cambios git detectados cuando el runtime observa que `doc_records` no contiene docs canonicos pese a que la wiki existe en disco; el contrato visible no debe quedar en `no changes detected` en ese caso.
- `index --docs-only` es un modo publico de recuperacion: reconstruye el corpus documental y la memoria de reentrada sin reemplazar el catalogo de codigo.
- `index [path]` es wrapper compatible de `index start --mode full --wait`; `--docs-only` equivale a `index start --mode docs --wait`.
- `index start [path] --mode full|docs|catalog` crea un job durable. Por default retorna sin esperar; con `--wait` bloquea hasta completar.
- `index status [job-id]` devuelve el ultimo job del workspace o el job pedido; mientras un job largo corre, `updated_at`, `current_stage`, `current_path`, `files_total` y los contadores deben reflejar progreso vivo.
- `index cancel <job-id> [--force]` solicita cancelacion cooperativa y, con `--force`, puede terminar el PID vivo del job; la variante force tambien debe remover el `index.lock` si pertenece al PID cancelado y ya no esta vivo.
- Los envelopes de jobs usan `backend=index-job`, `mode=<full|docs|catalog>` e `items[0]` con `job_id`, `generation_id`, `status`, `phase`, `current_stage`, `current_path`, `files_total`, `pid`, contadores, timestamps y `error` cuando existe; `phase=indexing` cubre el trabajo pesado y `phase=publishing` queda reservado para el cierre/publicacion final.
- La politica comun de subprocessos no interactivos debe evitar UI extra; en Windows aplica `HideWindow + CREATE_NO_WINDOW`, y los procesos background del daemon agregan `DETACHED_PROCESS`.
- La resolucion de bootstrap del worker usa el ejecutable/distribucion activa o, en desarrollo, el repo `mi-lsp`; nunca el `cwd` arbitrario del workspace consultado.
- La distribucion publica canonica es un bundle por RID que incluye `mi-lsp(.exe)` y `workers/<rid>/`; una build desde source no redefine ese contrato de bootstrap.
- Las queries Roslyn deben resolver candidatos en orden `bundle -> installed -> dev-local` por presencia de archivos; el probe `status` queda reservado para `worker status` y diagnostico explicito.
- Si el primer candidato Roslyn falla por bootstrap al arrancar, el caller puede reintentar una sola vez con el siguiente candidato determinista antes de devolver error accionable.
- Si `tsserver` no existe, el sistema debe degradar a catalog/text con warning explicito.
- Si `tsserver`, `pyright` o `gopls` estan en cooldown por fallas recientes de bootstrap, el contrato visible puede degradar directamente a catalog/text con warning explicito.
- Si `pyright-langserver` o `gopls` no existen, el sistema debe degradar a catalog/text con warning explicito.
- Si `nav context` se ejecuta sobre un archivo no semantico, el sistema debe responder con `backend=text` sin pasar por workers.
- Si `nav context` encuentra una falla de bootstrap Roslyn o de arranque de proceso/worker y el archivo existe, el sistema debe preservar `slice_text`, degradar a catalog/text cuando aplique y agregar warning accionable `backend_runtime/<code>` o el codigo bootstrap correspondiente.
- El protocolo CLI-daemon debe rechazar versiones incompatibles tempranamente.

## Documentos detalle

- [CT-CLI-DAEMON-ADMIN.md](09_contratos/CT-CLI-DAEMON-ADMIN.md)
- [CT-CLI-AXI-MODE.md](09_contratos/CT-CLI-AXI-MODE.md)
- [CT-DAEMON-WORKER.md](09_contratos/CT-DAEMON-WORKER.md)
- [CT-NAV-ASK.md](09_contratos/CT-NAV-ASK.md)
- [CT-NAV-INTENT.md](09_contratos/CT-NAV-INTENT.md)
- [CT-NAV-PACK.md](09_contratos/CT-NAV-PACK.md)
- [CT-NAV-GOVERNANCE.md](09_contratos/CT-NAV-GOVERNANCE.md)
- [CT-NAV-ROUTE.md](09_contratos/CT-NAV-ROUTE.md)
- [CT-NAV-WIKI.md](09_contratos/CT-NAV-WIKI.md)

## Operaciones adicionales

- `init [path] [--name alias] [--no-index]`: detecta, registra e indexa el workspace actual o el path pedido
- `mi-lsp [--classic] [--axi] [--full]`: por default devuelve home content-first; `--classic` restaura help generica
- `workspace.remove`: elimina un workspace registrado de `registry.toml`
- `workspace prune --stale [--dry-run|--apply]`: lista o remueve aliases del registry cuyo root ya no existe; `--dry-run` es el default seguro, `--apply` persiste la limpieza y no borra archivos del workspace.
- `admin export`: exporta telemetria de `access_events` desde `daemon.db`; raw soporta `--format json|csv|compact|toon` y con `--summary` agrega sobre toda la ventana filtrada salvo que `--limit` se haya seteado explicitamente
- `admin export` filtra raw por `--operation`, `--session-id`, `--client-name`, `--route`, `--query-format`, `--truncated`, `--pattern-mode`, `--routing-outcome`, `--failure-stage` y `--hint-code`
- `admin export --summary` agrega breakdowns opcionales `--by-route`, `--by-client`, `--by-hint`, `--by-failure-stage`, ademas de los histogramas/percentiles existentes
- el export raw de `access_events` preserva metadata operativa minima del request (`route`, `format`, `token_budget`, `max_items`, `max_chars`, `compress`) y diagnosticos causales sanitizados (`warning_count`, `pattern_mode`, `routing_outcome`, `failure_stage`, `hint_code`, `truncation_reason`, `decision_json`) para diferenciar uso directo, daemonizado, routing errors, backend runtime failures y truncacion; `decision_json` puede agregar solo metadata derivada como `doc_ranker`, `intent_mode`, `requested_backend`, `result_backend`, `backend_fallback_taken`, `fallback_from`, `fallback_to` y `runtime_error_code`, nunca texto libre, query, argv, payloads ni contenido de archivos; en operaciones daemonizadas normales debe existir una sola fila canonica por request
- `index [path] [--clean] [--docs-only]`: wrapper compatible que espera a completar; indexa codigo + docs, o solo docs cuando `--docs-only` esta presente
- `index start [path] [--mode full|docs|catalog] [--clean] [--wait]`: crea un job de indexacion; sin `--wait` lanza proceso detached y devuelve `job_id`
- `index status [job-id]`: inspecciona el ultimo job o el job indicado
- `index cancel <job-id> [--force]`: solicita cancelacion cooperativa; con `--force` termina el proceso vivo del job cuando existe y limpia el lock matching cuando el PID ya no esta vivo
- `nav route <task>`: resuelve el documento canonico de anclaje y un mini reading pack con minimos tokens; si la tarea trae un `RF-*` embebido en un doc agregado, Tier 1 ancla el contenedor canonico; `--include-code-discovery` agrega discovery de codigo; `--full` expande canonical lane y discovery
- `nav wiki search <query>`: busca en el docgraph gobernado con filtros `--layer RS,RF,FL,TP,CT,TECH,DB`, paginacion `--top/--offset` y contenido opcional `--include-content`; la superficie textual directa `nav search` debe incluir docs gobernados y artefactos repo-locales ocultos cuando el repo use directorios hidden
- `nav wiki route|pack|trace`: aliases documentales para agentes que reutilizan `nav route`, `nav pack` y `nav trace`
- `nav wiki validate-harness`: valida contratos `SDD-HARNESS-v1` en docs gobernados, resuelve referencias contra todo el docgraph gobernado para evitar falsos links rotos en imports globales y emite `PASS|WARN|BLOCKED` sin crear un parser documental paralelo
- `nav ask <question>`: responde usando wiki + evidencia de codigo y fallback generico/textual cuando haga falta; `--all-workspaces` habilita fan-out cross-workspace para el mismo contrato explainable
- `nav pack <task>`: construye un reading pack canonico con preview/full y anchors opcionales `--rf`, `--fl`, `--doc`
- `nav search <pattern>`: si `--regex` lleva un patron invalido, el runtime puede reintentar como literal y devolver warning explicito en vez de error duro; si el patron literal parece identificador de codigo, debe emitir guidance agent-first hacia `nav find --exact`/`nav related` y rankear fuentes antes que docs/tests/backups/generados; si `rg` falla por permisos/arranque, debe caer a Go search con warning `backend_runtime/<code>`
- `workspace status [--full] [--no-auto-sync]`: muestra `workspace_root`, `workspace_source`, estado de gobernanza/index y, en `--full`, el digest repo-local de memoria (`recent_canonical_changes`, `handoff`, `best_reentry`, `stale`); si la memoria esta stale, `auto_sync` esta habilitado y la gobernanza no esta bloqueada, puede ejecutar refresh docs-only y devolver warning visible; `--no-auto-sync` conserva la consulta read-only y reporta `governance_sync=stale` sin escribir `read-model.toml` ni refrescar memoria; si `docs_index_ready=true` pero `index_ready=false`, debe explicitar que el corpus docs-only quedo util pero el catalogo de codigo sigue ausente
- `workspace list --group-by-root`: agrupa aliases por root exacto con `root`, `alias_count`, `aliases`, `canonical_alias`, `selection_reason`, `kind` y warnings; `workspace list` sin flag sigue alias-preserving.
- `workspace doctor`: diagnostico no mutante de aliases con root compartido, worktrees con mismo `git common dir`, paths stale, shadowing de binario y sugerencias de comandos.
- cuando `--workspace` se omite, `nav ask`, `nav pack`, `nav governance`, `workspace status` y las queries directas equivalentes resuelven primero contra `caller_cwd`; si no hay match, pueden usar `last_workspace` con warning visible. Si `--workspace <alias>` esta presente y el `caller_cwd` cae dentro de otro workspace registrado, el alias explicito gana y debe quedar warning visible.
- `nav governance`: diagnostica perfil efectivo, sync, stale index y pasos de reparacion de gobernanza
- `nav service`: resume evidencia observable de un servicio en un unico summary estructurado; si el catalogo del path es Go, devuelve `profile=go-package`, no ejecuta scans .NET de endpoints/event bus y puede detectar rutas Go (`net/http`, chi/gin/fiber-style), Cobra y workers.
- `nav context`: acepta `nav context <file> <line>` y `nav context <file>:<line>`; devuelve `slice_text` y metadatos opcionales de catalogo o backend semantico para la linea pedida; ante fallas runtime de worker/proceso conserva `slice_text`, degrada a catalog/text cuando sea posible y emite warning `backend_runtime/<code>`
- `nav.find|intent|symbols|overview`: lecturas SQL-backed repo-locales; aceptan `--offset` para pedir la pagina siguiente sin cambiar el envelope base. En workspaces `container`, `find/intent` aceptan `--repo` y el offset se aplica despues del filtro de repo.
- `nav.search|outline|multi-read`: lecturas directas repo-locales sin contrato `--offset`; `search` sigue siendo text/rg-backed y puede exponer hints de refinamiento, pero no cursor SQL.
- `worker install`: instala o refresca el worker por RID desde un bundle adjunto o, en desarrollo, desde `worker-dotnet/`
- `worker status`: diagnostica el estado de candidatos `bundle`, `installed` y `dev-local`, e identifica el `cli_path` y `protocol_version` visibles para detectar binarios stale o inesperados en `PATH`
- `nav multi-read`: lee N rangos de archivo en una sola invocacion, reduce round-trips de agentes AI
- `nav search --include-content`: extiende search con contenido inline; modo hibrido (symbol body si indexado, +-N lineas fallback)
- `nav batch`: meta-comando que acepta N operaciones heterogeneas via stdin JSON, ejecucion paralela por defecto
- `nav related`: devuelve vecindario de un simbolo (definicion, callers, implementors, tests); el contenido expandido queda reservado para `--full`
- `nav workspace-map`: mapa de alto nivel del workspace; el modo base es summary-first y `--full` habilita scans profundos de endpoints, eventos y dependencias. Para Go, expone paquetes `cmd/*`, `internal/*` y `pkg/*` como servicios `go-package` derivados del catalogo.
- `nav diff-context [ref] --include-content`: contexto semantico de simbolos cambiados en un git diff, con analisis de impacto
- `nav trace <DOC-ID>`: acepta IDs `RS-*`, `RF-*` y `TP-*`; para `RS-*` resuelve docs outcome gobernados o fallbacks embebidos (`.docs/wiki/02_resultados_soluciones_usuario.md`, `.docs/wiki/02_resultados/*.md`) y devuelve `doc_id`, `layer=RS`, `stage=outcome` sin `rf`; si un `RF-*` no existe como `doc_records.doc_id` primario, puede resolverlo por `doc_mentions(doc_id)` dentro de un documento RF agregado y devolver `RF=<RF-ID>` en el resultado visible; los docs TP del layer `06` que mencionan ese `RF-*` cuentan como evidencia documental de cobertura y deben evitar falsos `missing` despues de `index --docs-only`; si el doc index todavia no publico ese ID, el fallback a disco debe recorrer primero las rutas documentales gobernadas por `00`/`read-model` y solo despues los layouts legacy conocidos (`.docs/wiki/04_RF*.md`, `.docs/wiki/RF/*.md`, `.docs/wiki/RF.md`, equivalentes TP); para `TP-*` puede resolver el caso dentro de docs agregados `06_pruebas/*.md` y usar el titulo del caso embebido; `--all` sigue siendo RF-only hasta que el contrato declare otro universo; los marcadores file-only verifican contra el catalogo de simbolos o contra existencia de archivo en el workspace cuando el lenguaje no esta indexado semanticamente
- `nav ask --all-workspaces` / `nav search --all-workspaces` / `nav find --all-workspaces`: fan-out paralelo cross-workspace; los aliases cuyo root ya no existe se omiten con warning agregado y sugerencia hacia `workspace prune --stale --dry-run`.
- `--no-auto-daemon` global flag: desactiva auto-start de daemon para queries semanticas
- `daemon perf-smoke`: valida presupuesto de daemon y callers paralelos; falla el envelope si supera working set, private bytes o handles configurados
- `--axi` global flag / `MI_LSP_AXI=1`: fuerzan overlay AXI en superficies soportadas
- `--classic` global flag: restaura modo clasico en superficies AXI-default y prevalece sobre el env
- `--full` global flag: expande surfaces AXI efectivas sin cambiar routing ni semantica base
- `workspace add --no-index`: agrega workspace sin indexar
- `--compress` global flag: compresion agresiva de output

## Envelope `nav ask`

```json
{
  "ok": true,
  "backend": "ask",
  "workspace": "alias",
  "items": [
    {
      "summary": "...",
      "primary_doc": {"path": ".docs/wiki/07_baseline_tecnica.md"},
      "doc_evidence": [],
      "code_evidence": [],
      "why": [],
      "next_queries": []
    }
  ],
  "warnings": ["read_model=project"],
  "coach": {
    "trigger": "text_fallback",
    "message": "This answer relied on textual fallback instead of strong indexed evidence.",
    "confidence": "low",
    "actions": [
      {
        "kind": "refine",
        "label": "Inspect supporting code",
        "command": "mi-lsp nav search \"daemon\" --workspace mi-lsp --include-content"
      }
    ]
  },
  "continuation": {
    "reason": "follow_doc",
    "next": {
      "op": "nav.search",
      "query": "daemon",
      "doc_id": "TECH-DAEMON-GOBERNANZA"
    }
  },
  "memory_pointer": {
    "doc_id": "CT-NAV-ASK",
    "why": "Contrato tecnico actualizado recientemente para guidance query-level.",
    "reentry_op": "workspace.status",
    "handoff": "continuation-v1",
    "stale": false
  },
  "stats": {"files": 2},
  "truncated": false
}
```

## Change triggers

Actualizar `09` y/o `CT-*` cuando cambie cualquiera de estos puntos:

- surface publica de comandos o flags globales
- overlay AXI (`--axi`, `--classic`, `MI_LSP_AXI=1`, `--full`) y semantica de preview/full
- handshake/version del daemon
- envelope JSON de salida
- endpoints/admin URL o payloads de gobernanza
- protocolo con Roslyn worker o bridge con `tsserver` o `pyright`
- politica de bootstrap, instalacion o compatibilidad del worker por RID
- contrato explainable de `nav ask` o shortcut publico `init`
