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
  - internal/telemetry/access_events.go
  - internal/telemetry/access_diagnostics.go
tests:
  - internal/cli/root_test.go
  - internal/daemon/server_test.go
  - internal/service/app_test.go
  - internal/telemetry/access_events_test.go
  - internal/cli/telemetry_test.go
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
7. Si el backend primario no esta disponible, el core usa un backend degradado y registra warnings.

## 4. Typed Errors

| Codigo | Causa | Trigger | Respuesta esperada |
|---|---|---|---|
| `QRY_ROUTE_WORKSPACE_NOT_FOUND` | workspace invalido | alias/path no resoluble | abortar con `ok=false` |
| `QRY_BACKEND_UNAVAILABLE` | no hay backend ejecutable | sin daemon y sin backend directo utilizable | abortar con warning/error explicito |
| `QRY_PROTOCOL_MISMATCH` | handshake incompatible | CLI y daemon/worker difieren en protocolo | abortar con mensaje accionable |
| `QRY_ROUTING_AMBIGUOUS` | selector insuficiente en `container` | simbolo o target coincide con varios repos | devolver candidatos y `next_hint` |
| `process_spawn_access_denied` | runtime o herramienta externa no arranca por permisos | `CreateProcess`, `Access is denied`, `permission denied` | preservar fallback posible y registrar `backend_runtime` |
| `process_spawn_failed` | runtime o herramienta externa no arranca por error de proceso | `fork/exec`, `failed to start`, imagen invalida | preservar fallback posible y registrar `backend_runtime` |

## 5. Special Cases and Variants

- Daemon caido: no es error fatal si el modo directo puede responder.
- `nav.find`, `nav.search`, `nav.intent`, `nav.symbols`, `nav.outline`, `nav.overview` y `nav.multi-read` no deben bloquearse esperando daemon; su contrato es repo-local directo.
- Workspace `container`: `find/search/intent/overview` operan globalmente; `find/search/intent` pueden acotar con `--repo` sin pasar a routing semantico, mientras `refs/context/deps` pueden requerir `--repo` o `--entrypoint`.
- El `backend` usado siempre se informa; si hay ambiguedad controlada, el backend canonico es `router`.
- Para archivos `.py`/`.pyi`, el routing resuelve a `pyright` si esta disponible; si no, degrada a `catalog`/`text` con warning explicito. El valor `--backend pyright` fuerza el uso de Pyright sin fallback automatico.
- Para `nav context` sobre archivos no semanticos, el routing salta directo a `backend=text` y conserva el mismo envelope.
- Para `nav context` sobre `ts/js`, el core prioriza `tsserver` pero debe conservar `slice_text` y degradar a `catalog` o `text` cuando `tsserver` no exista.
- Para `nav context` sobre C#/TS/Python, el core es slice-first: si Roslyn/tsserver/Pyright fallan al arrancar por permisos o proceso bloqueado, devuelve `ok=true`, conserva `slice_text`, agrega warning tipado `backend_runtime/<code>` y registra `requested_backend`, `result_backend`, `backend_fallback_taken` y `runtime_error_code` en telemetria sanitizada.
- Para `nav search`, si `rg` existe pero falla por permisos o arranque de proceso, el runtime debe degradar a busqueda Go nativa, emitir warning tipado `backend_runtime/<code>` y no exponer argv, payload ni contenido de archivos en `decision_json`.
- Para `nav search`, si la busqueda agota timeout durante el scan despues de materializar resultados parciales seguros, el runtime debe conservar esos resultados, responder `ok=true`, emitir warning humano de timeout, registrar `hint_code=search_timeout`, `coach.trigger=search_timeout` y `failure_stage=none`, y proponer un `next_hint` de narrowing sin repetir ciegamente la misma consulta.
- Para queries symbol-like en `nav search` literal, el envelope puede emitir `coach.trigger=symbol_query_detected` con acciones hacia `nav find --exact` y `nav related`; esta guia complementa, no reemplaza, el resultado textual.

## 6. Data Model Impact

- `QueryEnvelope`
- `AccessEvent`
- `WorkspaceEntrypoint`
