---
id: RF-QRY-002
title: Resolver routing con fallback de daemon y backend
implements:
  - internal/cli/root.go
  - internal/daemon/server.go
  - internal/service/context.go
  - internal/service/search.go
  - internal/service/coach.go
  - internal/service/semantic_errors.go
  - internal/service/semantic.go
  - internal/service/semantic_refs.go
  - internal/service/autoindex.go
  - internal/daemon/lifecycle.go
  - internal/worker/gopls.go
  - internal/telemetry/access_events.go
  - internal/telemetry/access_diagnostics.go
tests:
  - internal/cli/root_test.go
  - internal/daemon/server_test.go
  - internal/service/app_test.go
  - internal/telemetry/access_events_test.go
  - internal/cli/telemetry_test.go
  - internal/service/autoindex_test.go
  - internal/service/semantic_refs_test.go
  - internal/worker/gopls_test.go
  - internal/cli/root_maxitems_test.go
---

```yaml
harness_protocol: SDD-HARNESS-v1
id: "RF-QRY-002"
kind: "support-doc"
audience: "llm-first"
imports:
  - '[[00_gobierno_documental]]'
  - '[[RF-QRY-002]]'
exports:
  - 'RF-QRY-002'
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/04_RF/RF-QRY-002.md
agent_may_edit:
  - .docs/wiki/04_RF/RF-QRY-002.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
evidence:
  - .docs/wiki/04_RF/RF-QRY-002.md
```

# RF-QRY-002 - Resolver routing con fallback de daemon y backend

## 1. Execution Sheet

| Campo | Valor |
|---|---|
| ID | RF-QRY-002 |
| Titulo | Resolver routing con fallback de daemon y backend |
| Actores | Usuario, Skill, Agente, CLI, Daemon/Core |
| Prioridad | alta |
| Severidad | alta |
| FL origen | FL-QRY-01 |

## 2. Detailed Preconditions

| Condicion | Tipo | Estado requerido |
|---|---|---|
| Workspace resoluble | funcional | obligatorio |
| Operacion `nav` soportada | funcional | obligatorio |
| Backend candidato identificable | tecnica | obligatorio |

## 3. Process Steps (Happy Path)

1. La CLI resuelve workspace y comando.
2. La CLI decide en forma centralizada si la operacion debe usar daemon o ejecutarse directo.
3. Para lecturas baratas de catalogo/texto (`nav.find`, `nav.search`, `nav.intent`, `nav.symbols`, `nav.outline`, `nav.overview`, `nav.multi-read`), la CLI ejecuta directo sin depender del daemon.
4. Para queries semanticas o compuestas, la CLI intenta enviar la request al daemon global si esta disponible.
5. Si el daemon responde, enruta al backend adecuado y devuelve el envelope.
6. Si el daemon no responde o no aplica, la CLI ejecuta fallback directo y emite `hint: "daemon_unavailable; served from local text index"` en el envelope.
7. Si el backend primario no esta disponible, el core usa un backend degradado, registra warnings y marca el envelope con `degraded=true`, `reason` tipificado y `fallback_used` (conjunto cerrado en [[RF-QRY-001]], contrato `primitives-v1`).

## 4. Typed Errors

| Codigo | Causa | Trigger | Respuesta esperada |
|---|---|---|---|
| `QRY_ROUTE_WORKSPACE_NOT_FOUND` | workspace invalido | alias/path no resoluble | abortar con `ok=false` |
| `QRY_BACKEND_UNAVAILABLE` | no hay backend ejecutable | sin daemon y sin backend directo utilizable | abortar con warning/error explicito |
| `QRY_PROTOCOL_MISMATCH` | handshake incompatible | CLI y daemon/worker difieren en protocolo | abortar con mensaje accionable |
| `QRY_ROUTING_AMBIGUOUS` | selector insuficiente en `container` | simbolo o target coincide con varios repos | devolver candidatos y `next_hint` |
| `process_spawn_access_denied` | runtime o herramienta externa no arranca por permisos | `CreateProcess`, `Access is denied`, `permission denied` | preservar fallback posible y registrar `backend_runtime` |
| `process_spawn_failed` | runtime o herramienta externa no arranca por error de proceso | `fork/exec`, `failed to start`, imagen invalida | preservar fallback posible y registrar `backend_runtime` |
| `lsp_unavailable` / `lsp_error` | el backend semantico de `nav.refs` falta o falla | binario ausente (`executable file not found`, `node is required`) o error del backend | no es error terminal: `ok=true`, fallback de texto por limites de palabra, `degraded=true`, `reason=lsp_unavailable` o `lsp_error`, `fallback_used=text` |

## 5. Special Cases and Variants

- Daemon caido: no es error fatal si el modo directo puede responder.
- `nav.find`, `nav.search`, `nav.intent`, `nav.symbols`, `nav.outline`, `nav.overview` y `nav.multi-read` no deben bloquearse esperando daemon; su contrato es repo-local directo.
- Workspace `container`: `find/search/intent/overview` operan globalmente; `find/search/intent` pueden acotar con `--repo` sin pasar a routing semantico, mientras `refs/context/deps` pueden requerir `--repo` o `--entrypoint`.
- El `backend` usado siempre se informa; si hay ambiguedad controlada, el backend canonico es `router`.
- Para archivos `.py`/`.pyi`, el routing resuelve a `pyright` si esta disponible; si no, degrada a `catalog`/`text` con warning explicito. El valor `--backend pyright` fuerza el uso de Pyright sin fallback automatico.
- Para `nav context` sobre archivos no semanticos, el routing salta directo a `backend=text` y conserva el mismo envelope.
- Para `nav context` sobre `ts/js`, el core prioriza `tsserver` pero debe conservar `slice_text` y degradar a `catalog` o `text` cuando `tsserver` no exista.
- Para archivos `.go`, el extractor AST nativo conserva catalogo navegable sin `gopls`; `nav context` puede enriquecer la respuesta con `gopls` y degrada a `catalog` o `text` con warning cuando no esta disponible.
- Para `nav context` sobre C#/TS/Python/Go, el core es slice-first: si Roslyn/tsserver/Pyright/gopls fallan al arrancar por permisos o proceso bloqueado, devuelve `ok=true`, conserva `slice_text`, agrega warning tipado `backend_runtime/<code>` y registra `requested_backend`, `result_backend`, `backend_fallback_taken` y `runtime_error_code` en telemetria sanitizada.
- Para `nav search`, si `rg` existe pero falla por permisos o arranque de proceso, el runtime debe degradar a busqueda Go nativa, emitir warning tipado `backend_runtime/<code>` y no exponer argv, payload ni contenido de archivos en `decision_json`.
- `nav.search` puede procesar archivos en paralelo; la selección acotada conserva los primeros resultados por orden lexicográfico de ruta y línea, con salida determinista sin ordenar todo el conjunto de coincidencias.
- `nav.find` puede refrescar los archivos candidatos antes de consultar: hasta 32 archivos, 4 MiB por archivo y 16 MiB agregados, con presupuesto de 250 ms. Si publica una actualización del catálogo, repite la consulta; si el refresh falla o vence, sirve el snapshot publicado y añade una advertencia accionable. Este refresh no ejecuta Roslyn ni deja escrituras en segundo plano.
- `nav.find` decide si puede declarar cero coincidencias usando una generación publicada (`active_catalog_generation_id`) o metadata completa de `ReplaceCatalog` (`indexed_at` + `total_files`, ambos escritos atómicamente, incluso `total_files=0`). Un esquema sin esos campos o metadata parcial no basta. La ausencia de filas no demuestra completitud; un root transitorio puede responder si su catálogo se publicó con una de esas señales.
- `nav.find` sin catálogo publicado, con base ilegible o con esquema roto responde desde texto (`ok=true`, `backend=text`, `degraded=true`, `reason=index_not_ready|index_schema_broken`) y lanza un único reindex completo en segundo plano; ver [[RF-IDX-001]]. Nunca responde un vacío sin marcarlo como degradado.
- `nav.refs` elige el backend por el lenguaje del símbolo y no por el primer lenguaje del workspace: definición en el catálogo, si no el primer hit de texto, si no los lenguajes registrados. Roslyn se usa solo para C#. Cuando no hay `--file`, inyecta el ancla de la definición para `gopls` y `tsserver`, que resuelven por posición. `--file` explícito se enruta por la extensión del archivo; una extensión sin backend semántico cae a `backend=text` con `reason=language_unsupported`.
- `nav.refs` nunca devuelve un falso vacío: cualquier error del backend (`reason=lsp_unavailable` si falta el binario o runtime, `lsp_error` en el resto) o un resultado semántico vacío vuelve a consultar por texto con límites de palabra. Si el texto encuentra coincidencias tras un vacío semántico, la razón es `semantic_empty_text_hits`; si tampoco encuentra, el vacío es válido con `reason=no_matches`. Cada item lleva `origin` (`semantic` o `text`) y, cuando el catálogo conoce el símbolo contenedor, `caller {name, kind, line}`.
- `nav.refs --context N` (0 a 5, también `context` en la herramienta MCP `nav_refs`) agrega N líneas de contexto antes y después de cada referencia; un valor fuera de rango se rechaza antes de ejecutar.
- Localización de `gopls` (`internal/worker/gopls.go`), en este orden: `MI_LSP_GOPLS_PATH`, `PATH`, `GOBIN`, `GOPATH/bin`, `~/go/bin` y `bin/` o `.bin/` del workspace. Si ninguno existe, `nav.refs` degrada con `reason=lsp_unavailable`.
- El warm del daemon es por lenguaje del workspace e incluye `gopls` para Go; ya no calienta Roslyn por defecto, de modo que un repo Go, TypeScript o Python no paga el worker .NET (`backendsForWorkspace`).
- `nav.search` sigue siendo una búsqueda textual directa y puede devolver coincidencias de archivos aunque el workspace no esté registrado o no tenga índice semántico. No depende de la evidencia de completitud que requiere `nav.find`.
- Con `--allow-cross-workspace`, un path existente pero todavía no registrado sólo recibe `workspace_resolution_failed` (`Kind=workspace`, `Stage=workspace_resolution`, `ReasonCode=invalid_workspace`) y el comando dinámico `workspace add <path>` cuando tampoco tiene catálogo publicado listo. Un catálogo transitorio publicado con pointer activo o par atómico `indexed_at` + `total_files` se puede consultar antes del registro. Sin ese flag, la negativa de mismatch conserva su error terminal `explicit_incomplete` y su comportamiento fail-closed.
- JSON explícito y MCP mantienen el mismo diagnóstico y los mismos datos estructurados que la llamada directa. La vista compacta de MCP expone un resumen útil y el hint. Las advertencias stale existentes se mantienen cuando se responde con un snapshot publicado.
- `nav.route` y `nav.intent` pueden refrescar hasta cinco paths candidatos Markdown con presupuesto de 500 ms. Tras una publicación confirmada recargan el catálogo y vuelven a puntuar una única vez; si no se confirma por error o deadline, conservan los resultados en memoria y añaden una advertencia. El refresh no reconstruye el grafo ni deja escrituras de fondo.
- El refresh lee y extrae contenido bajo bloqueo de escritura cancelable; una solicitud que esperaba el lock usa el contenido observado al obtenerlo. Un dialecto de hash desconocido no se interpreta como stale ni elimina una fila del índice legacy.
- Para un path consultado cuya fila de catálogo existe pero el archivo ya no existe, el refresh publica un tombstone sólo para esa fila/path; también trata así un `ENOENT` ocurrido durante la lectura. `nav.find` repite la query tras publicar y no devuelve los símbolos stale. La publicación invalida el estado graph-native; no sintetiza evidencia de grafo.
- Para `nav search`, si la busqueda agota timeout durante el scan despues de materializar resultados parciales seguros, el runtime debe conservar esos resultados, responder `ok=true`, emitir warning humano de timeout, registrar `hint_code=search_timeout`, `coach.trigger=search_timeout` y `failure_stage=none`, y proponer un `next_hint` de narrowing sin repetir ciegamente la misma consulta.
- Para `nav search --include-content`, si un match viene del indice pero el archivo ya no existe en disco, el runtime debe conservar el item y emitir stale-index warning accionable hacia reindexado, no fallar toda la busqueda.
- Para queries symbol-like en `nav search` literal, el envelope puede emitir `coach.trigger=symbol_query_detected` con acciones hacia `nav find --exact` y `nav related`; esta guia complementa, no reemplaza, el resultado textual.

## 6. Data Model Impact

- `QueryEnvelope`
- `AccessEvent`
- `WorkspaceEntrypoint`
