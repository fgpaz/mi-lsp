# 09. Contratos tecnicos

## Proposito y alcance

Este documento resume la superficie de contratos tecnicos de `mi-lsp`: CLI publica, canal CLI-daemon-admin y protocolo daemon-worker.
Su foco es compatibilidad, ownership y postura comun de errores/versionado.
El detalle por frontera vive en `09_contratos/`.

## Inventario de contratos

| Familia | Boundary | Owner logico | Canal |
|---|---|---|---|
| CLI publica | Usuario/agente -> `mi-lsp` | CLI surface | args/stdout/stderr |
| Control daemon | CLI -> daemon | Runtime supervision | named pipe / unix socket |
| Governance admin | Browser/CLI -> daemon | Runtime supervision | HTTP loopback |
| Worker protocol | Daemon/core -> worker Roslyn | Semantic backends | stdio JSON length-prefixed |
| TS semantic bridge | Daemon/core -> `tsserver` | TS semantic backend | `tsserver` Content-Length protocol |
| Pyright LSP bridge | Daemon/core -> `pyright-langserver` | Python semantic backend | LSP JSON-RPC 2.0 Content-Length protocol |

## Boundaries y ownership

- La CLI publica es la frontera estable para humanos, skills y wrappers.
- El repo publica una skill curada en `skills/mi-lsp/` para herramientas compatibles con skills; esa skill documenta buenas practicas de uso, pero no redefine la semantica del CLI.
- El daemon comparte estado entre clientes pero no redefine la CLI publica.
- La UI/admin es una vista local del daemon; no es API publica remota.
- El protocolo daemon-worker es interno, versionado y con envelope estable.
- Cada contrato debe exponer `warnings`, fallas accionables y degradacion clara cuando aplique.
- `worker status` forma parte de la CLI publica y debe exponer `tool_root`, `tool_root_kind`, `cli_path`, `protocol_version`, origen del worker seleccionado y compatibilidad de candidatos.
- `workspace status` forma parte de la CLI publica y debe exponer `docs_read_model` (`builtin-default` o path del proyecto).
- `init` pertenece a la CLI publica como shortcut de onboarding; no reemplaza `workspace add`, pero reutiliza su semantica base.
- `nav ask` pertenece a la CLI publica y usa un contrato docs-first explainable, no un blob opaco ni una respuesta puramente textual.
- `nav service` pertenece a la CLI publica y usa un contrato evidence-first, no uno de scoring.
- `nav context` pertenece a la CLI publica y su salida visible es slice-first; el backend profundo solo enriquece el mismo item.
- `nav intent` pertenece a la CLI publica y usa ranking BM25 sobre `search_text`; en workspaces `container` puede acotar por `--repo` sin cambiar a un backend semantico.

## Versionado, auth y errores

- El proyecto usa compatibilidad best-effort intra-version; cambios incompatibles deben reflejar `protocol_version`.
- La governance UI es solo local (`127.0.0.1`) y no incorpora auth en esta fase; la ventana temporal visible se negocia via `window=recent|7d|30d|90d`.
- Las respuestas del CLI deben seguir envelope estable y explicitar `backend`, `truncated`, `warnings`, `stats` y `hint` (omitempty, presente cuando `items=[]` o daemon no disponible).
- Los errores de bootstrap deben ser accionables: por ejemplo `Run: mi-lsp worker install`.
- Los contratos internos no deben transportar ASTs ni blobs completos salvo comando futuro explicito.
- `nav ask` debe devolver una estructura explainable con `summary`, `primary_doc`, `doc_evidence`, `code_evidence`, `why` y `next_queries`.
- `nav service` puede usar `backend=catalog`, `backend=text` o `backend=catalog+text`.

## Compatibilidad y migracion

- La presencia o ausencia del daemon no debe cambiar la semantica visible de los comandos.
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
- [CT-DAEMON-WORKER.md](09_contratos/CT-DAEMON-WORKER.md)
- [CT-NAV-ASK.md](09_contratos/CT-NAV-ASK.md)

## Operaciones adicionales

- `init [path] [--name alias] [--no-index]`: detecta, registra e indexa el workspace actual o el path pedido
- `workspace.remove`: elimina un workspace registrado de `registry.toml`
- `admin export`: exporta telemetria de `access_events` desde `daemon.db`
- `nav ask <question>`: responde usando wiki + evidencia de codigo y fallback generico/textual cuando haga falta
- `nav service`: resume evidencia observable de un servicio en un unico summary estructurado
- `nav context`: devuelve `slice_text` y metadatos opcionales de catalogo o backend semantico para la linea pedida
- `nav.find|search|intent|symbols|outline|overview|multi-read`: lecturas directas repo-locales; conservan envelope estable sin dependencia funcional del daemon. En workspaces `container`, `find/search/intent` aceptan `--repo` para narrowing directo.
- `worker install`: instala o refresca el worker por RID desde un bundle adjunto o, en desarrollo, desde `worker-dotnet/`
- `worker status`: diagnostica el estado de candidatos `bundle`, `installed` y `dev-local`, e identifica el `cli_path` y `protocol_version` visibles para detectar binarios stale o inesperados en `PATH`
- `nav multi-read`: lee N rangos de archivo en una sola invocacion, reduce round-trips de agentes AI
- `nav search --include-content`: extiende search con contenido inline; modo hibrido (symbol body si indexado, +-N lineas fallback)
- `nav batch`: meta-comando que acepta N operaciones heterogeneas via stdin JSON, ejecucion paralela por defecto
- `nav related`: devuelve vecindario de un simbolo (definicion, callers, implementors, tests) con contenido incluido
- `nav workspace-map`: mapa de alto nivel del workspace con repos, servicios, endpoints, consumers, publishers y dependencias
- `nav diff-context [ref] --include-content`: contexto semantico de simbolos cambiados en un git diff, con analisis de impacto
- `nav search --all-workspaces` / `nav find --all-workspaces`: busqueda paralela cross-workspace
- `--no-auto-daemon` global flag: desactiva auto-start de daemon para queries semanticas
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
- handshake/version del daemon
- envelope JSON de salida
- endpoints/admin URL o payloads de gobernanza
- protocolo con Roslyn worker o bridge con `tsserver` o `pyright`
- politica de bootstrap, instalacion o compatibilidad del worker por RID
- contrato explainable de `nav ask` o shortcut publico `init`
