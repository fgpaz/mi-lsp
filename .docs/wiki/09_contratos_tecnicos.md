# 09. Contratos tecnicos

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
- `workspace status` forma parte de la CLI publica y debe exponer `docs_read_model`, `governance_profile`, `governance_sync`, `governance_index_sync` y `governance_blocked`.
- En AXI efectivo, `workspace status`, `nav search`, `nav intent` y `nav pack` pertenecen a la superficie preview/full por default; `nav ask` solo lo hace para preguntas de orientacion y `nav workspace-map` solo cuando se fuerza AXI.
- `init` pertenece a la CLI publica como shortcut de onboarding; no reemplaza `workspace add`, pero reutiliza su semantica base.
- `nav ask` pertenece a la CLI publica y usa un contrato docs-first explainable, no un blob opaco ni una respuesta puramente textual.
- `nav pack` pertenece a la CLI publica y usa un contrato de reading pack canonico, no una respuesta textual libre.
- `nav governance` pertenece a la CLI publica y devuelve el estado efectivo de gobernanza del workspace.
- `nav service` pertenece a la CLI publica y usa un contrato evidence-first, no uno de scoring.
- `nav context` pertenece a la CLI publica y su salida visible es slice-first; el backend profundo solo enriquece el mismo item.
- `nav intent` pertenece a la CLI publica y usa ranking BM25 sobre `search_text`; en workspaces `container` puede acotar por `--repo` sin cambiar a un backend semantico.

## Versionado, auth y errores

- El proyecto usa compatibilidad best-effort intra-version; cambios incompatibles deben reflejar `protocol_version`.
- La governance UI es solo local (`127.0.0.1`) y no incorpora auth en esta fase; la ventana temporal visible se negocia via `window=recent|7d|30d|90d`.
- Las respuestas del CLI deben seguir envelope estable y explicitar `backend`, `truncated`, `warnings`, `stats` y `hint` (omitempty, presente cuando `items=[]` o daemon no disponible).
- En AXI, las respuestas preview-first pueden anunciar expansion via `next_hint` hacia `--full` sin cambiar el envelope base.
- Los errores de bootstrap deben ser accionables: por ejemplo `Run: mi-lsp worker install`.
- Los contratos internos no deben transportar ASTs ni blobs completos salvo comando futuro explicito.
- `nav ask` debe devolver una estructura explainable con `summary`, `primary_doc`, `doc_evidence`, `code_evidence`, `why` y `next_queries`.
- `nav pack` debe devolver una estructura estable con `task`, `family`, `mode`, `primary_doc`, `docs`, `why` y `next_queries`.
- `nav governance` debe devolver una estructura estable con perfil, base efectiva, overlays, sync, blockers y siguientes pasos.
- `nav service` puede usar `backend=catalog`, `backend=text` o `backend=catalog+text`.

## Compatibilidad y migracion

- La presencia o ausencia del daemon no debe cambiar la semantica visible de los comandos.
- `MI_LSP_AXI=1` habilita AXI a nivel sesion; `--classic` prevalece sobre defaults/env y `--axi` fuerza AXI en superficies soportadas.
- `--axi=false` explicito anula el default AXI de la superficie actual; equivalente a `--classic` para esa invocacion.
- `--axi` y `--classic` juntos deben fallar antes de ejecutar la operacion.
- `worker status` debe conservar el mismo payload visible con y sin daemon; el daemon no puede reemplazar `items` por `RuntimeSnapshot`/`WorkerStatus` crudos.
- `nav.find`, `nav.search`, `nav.intent`, `nav.symbols`, `nav.outline`, `nav.overview` y `nav.multi-read` pertenecen a la superficie publica directa: no deben esperar daemon ni cambiar de comportamiento por su health.
- La politica comun de subprocessos no interactivos debe evitar UI extra; en Windows aplica `HideWindow + CREATE_NO_WINDOW`, y los procesos background del daemon agregan `DETACHED_PROCESS`.
- La resolucion de bootstrap del worker usa el ejecutable/distribucion activa o, en desarrollo, el repo `mi-lsp`; nunca el `cwd` arbitrario del workspace consultado.
- La distribucion publica canonica es un bundle por RID que incluye `mi-lsp(.exe)` y `workers/<rid>/`; una build desde source no redefine ese contrato de bootstrap.
- Las queries Roslyn deben resolver candidatos en orden `bundle -> installed -> dev-local` por presencia de archivos; el probe `status` queda reservado para `worker status` y diagnostico explicito.
- Si el primer candidato Roslyn falla por bootstrap al arrancar, el caller puede reintentar una sola vez con el siguiente candidato determinista antes de devolver error accionable.
- Si `tsserver` no existe, el sistema debe degradar a catalog/text con warning explicito.
- Si `pyright-langserver` no existe, el sistema debe degradar a catalog/text con warning explicito.
- Si `nav context` se ejecuta sobre un archivo no semantico, el sistema debe responder con `backend=text` sin pasar por workers.
- Si `nav context` encuentra una falla de bootstrap Roslyn y el archivo existe, el sistema debe preservar `slice_text` y agregar warning accionable.
- El protocolo CLI-daemon debe rechazar versiones incompatibles tempranamente.

## Documentos detalle

- [CT-CLI-DAEMON-ADMIN.md](09_contratos/CT-CLI-DAEMON-ADMIN.md)
- [CT-CLI-AXI-MODE.md](09_contratos/CT-CLI-AXI-MODE.md)
- [CT-DAEMON-WORKER.md](09_contratos/CT-DAEMON-WORKER.md)
- [CT-NAV-ASK.md](09_contratos/CT-NAV-ASK.md)
- [CT-NAV-PACK.md](09_contratos/CT-NAV-PACK.md)
- [CT-NAV-GOVERNANCE.md](09_contratos/CT-NAV-GOVERNANCE.md)
- [CT-NAV-ROUTE.md](09_contratos/CT-NAV-ROUTE.md)

## Operaciones adicionales

- `init [path] [--name alias] [--no-index]`: detecta, registra e indexa el workspace actual o el path pedido
- `mi-lsp [--classic] [--axi] [--full]`: por default devuelve home content-first; `--classic` restaura help generica
- `workspace.remove`: elimina un workspace registrado de `registry.toml`
- `admin export`: exporta telemetria de `access_events` desde `daemon.db`; con `--summary` agrega sobre toda la ventana filtrada salvo que `--limit` se haya seteado explicitamente
- `admin export` filtra raw por `--operation`, `--session-id`, `--client-name`, `--route`, `--query-format`, `--truncated`, `--pattern-mode`, `--routing-outcome`, `--failure-stage` y `--hint-code`
- `admin export --summary` agrega breakdowns opcionales `--by-route`, `--by-client`, `--by-hint`, `--by-failure-stage`, ademas de los histogramas/percentiles existentes
- el export raw de `access_events` preserva metadata operativa minima del request (`route`, `format`, `token_budget`, `max_items`, `max_chars`, `compress`) y diagnosticos causales sanitizados (`warning_count`, `pattern_mode`, `routing_outcome`, `failure_stage`, `hint_code`, `truncation_reason`, `decision_json`) para diferenciar uso directo, daemonizado, routing errors y truncacion; en operaciones daemonizadas normales debe existir una sola fila canonica por request
- `nav route <task>`: resuelve el documento canonico de anclaje y un mini reading pack con minimos tokens; `--include-code-discovery` agrega discovery de codigo; `--full` expande canonical lane y discovery
- `nav ask <question>`: responde usando wiki + evidencia de codigo y fallback generico/textual cuando haga falta; `--all-workspaces` habilita fan-out cross-workspace para el mismo contrato explainable
- `nav pack <task>`: construye un reading pack canonico con preview/full y anchors opcionales `--rf`, `--fl`, `--doc`
- `nav governance`: diagnostica perfil efectivo, sync, stale index y pasos de reparacion de gobernanza
- `nav service`: resume evidencia observable de un servicio en un unico summary estructurado
- `nav context`: devuelve `slice_text` y metadatos opcionales de catalogo o backend semantico para la linea pedida
- `nav.find|intent|symbols|overview`: lecturas SQL-backed repo-locales; aceptan `--offset` para pedir la pagina siguiente sin cambiar el envelope base. En workspaces `container`, `find/intent` aceptan `--repo` y el offset se aplica despues del filtro de repo.
- `nav.search|outline|multi-read`: lecturas directas repo-locales sin contrato `--offset`; `search` sigue siendo text/rg-backed y puede exponer hints de refinamiento, pero no cursor SQL.
- `worker install`: instala o refresca el worker por RID desde un bundle adjunto o, en desarrollo, desde `worker-dotnet/`
- `worker status`: diagnostica el estado de candidatos `bundle`, `installed` y `dev-local`, e identifica el `cli_path` y `protocol_version` visibles para detectar binarios stale o inesperados en `PATH`
- `nav multi-read`: lee N rangos de archivo en una sola invocacion, reduce round-trips de agentes AI
- `nav search --include-content`: extiende search con contenido inline; modo hibrido (symbol body si indexado, +-N lineas fallback)
- `nav batch`: meta-comando que acepta N operaciones heterogeneas via stdin JSON, ejecucion paralela por defecto
- `nav related`: devuelve vecindario de un simbolo (definicion, callers, implementors, tests) con contenido incluido
- `nav workspace-map`: mapa de alto nivel del workspace con repos, servicios, endpoints, consumers, publishers y dependencias
- `nav diff-context [ref] --include-content`: contexto semantico de simbolos cambiados en un git diff, con analisis de impacto
- `nav ask --all-workspaces` / `nav search --all-workspaces` / `nav find --all-workspaces`: fan-out paralelo cross-workspace
- `--no-auto-daemon` global flag: desactiva auto-start de daemon para queries semanticas
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
