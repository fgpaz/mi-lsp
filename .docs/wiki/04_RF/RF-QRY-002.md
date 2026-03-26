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
3. Para lecturas baratas de catalogo/texto (`nav.find`, `nav.search`, `nav.symbols`, `nav.outline`, `nav.overview`, `nav.multi-read`), la CLI ejecuta directo sin depender del daemon.
4. Para queries semanticas o compuestas, la CLI intenta enviar la request al daemon global si esta disponible.
5. Si el daemon responde, enruta al backend adecuado y devuelve el envelope.
6. Si el daemon no responde o no aplica, la CLI ejecuta fallback directo.
7. Si el backend primario no esta disponible, el core usa un backend degradado y registra warnings.

## 4. Typed Errors

| Codigo | Causa | Trigger | Respuesta esperada |
|---|---|---|---|
| `QRY_ROUTE_WORKSPACE_NOT_FOUND` | workspace invalido | alias/path no resoluble | abortar con `ok=false` |
| `QRY_BACKEND_UNAVAILABLE` | no hay backend ejecutable | sin daemon y sin backend directo utilizable | abortar con warning/error explicito |
| `QRY_PROTOCOL_MISMATCH` | handshake incompatible | CLI y daemon/worker difieren en protocolo | abortar con mensaje accionable |
| `QRY_ROUTING_AMBIGUOUS` | selector insuficiente en `container` | simbolo o target coincide con varios repos | devolver candidatos y `next_hint` |

## 5. Special Cases and Variants

- Daemon caido: no es error fatal si el modo directo puede responder.
- `nav.find`, `nav.search`, `nav.symbols`, `nav.outline`, `nav.overview` y `nav.multi-read` no deben bloquearse esperando daemon; su contrato es repo-local directo.
- Workspace `container`: `find/search/overview` operan globalmente; `refs/context/deps` pueden requerir `--repo` o `--entrypoint`.
- El `backend` usado siempre se informa; si hay ambiguedad controlada, el backend canonico es `router`.
- Para archivos `.py`/`.pyi`, el routing resuelve a `pyright` si esta disponible; si no, degrada a `catalog`/`text` con warning explicito. El valor `--backend pyright` fuerza el uso de Pyright sin fallback automatico.
- Para `nav context` sobre archivos no semanticos, el routing salta directo a `backend=text` y conserva el mismo envelope.
- Para `nav context` sobre `ts/js`, el core prioriza `tsserver` pero debe conservar `slice_text` y degradar a `catalog` o `text` cuando `tsserver` no exista.

## 6. Data Model Impact

- `QueryEnvelope`
- `AccessEvent`
- `WorkspaceEntrypoint`
