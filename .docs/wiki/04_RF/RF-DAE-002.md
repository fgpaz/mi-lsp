---
id: RF-DAE-002
title: Compartir runtimes, governance UI y telemetria local
implements:
  - internal/daemon/server.go
  - internal/daemon/admin.go
  - internal/daemon/perf_smoke.go
  - internal/daemon/log_tail.go
tests:
  - internal/daemon/server_test.go
  - internal/daemon/admin_test.go
---

# RF-DAE-002 - Compartir runtimes, governance UI y telemetria local

## 1. Execution Sheet

| Campo | Valor |
|---|---|
| ID | RF-DAE-002 |
| Titulo | Compartir runtimes, governance UI y telemetria local |
| Actores | Desarrollador, Agente, CLI, Daemon, Admin UI |
| Prioridad | alta |
| Severidad | media |
| FL origen | FL-DAE-01 |

## 2. Detailed Preconditions

| Condicion | Tipo | Estado requerido |
|---|---|---|
| Daemon global saludable | tecnica | obligatorio |
| `daemon.db` escribible | operativa | obligatorio |
| Cliente envia metadata operativa cuando esta disponible | operativa | recomendado |
| `admin_url` accesible por loopback | operativa | obligatorio para UI |

## 3. Inputs

| Campo | Tipo | Req. | Origen | Validacion | RN |
|---|---|---|---|---|---|
| `workspace` | alias/path | no | CLI/query | input visible para foco de UI y warm; el daemon resuelve `workspace_root` canonico | RF-DAE-002 |
| `panel` | enum | no | query/admin UI | `overview`, `activity`, `logs` | RF-DAE-002 |
| `client_name` | string | no | CLI/env | default `manual-cli` | RF-DAE-002 |
| `session_id` | string | no | CLI/env | puede ser omitido | RF-DAE-002 |
| `format` | enum | no | CLI | `compact`, `json`, `text`, `toon`, `yaml` | RF-DAE-002 |
| `token_budget` | entero | no | CLI | > 0 cuando se explicita | RF-DAE-002 |
| `max_items` | entero | no | CLI | > 0 cuando se explicita | RF-DAE-002 |
| `max_chars` | entero | no | CLI | >= 0 | RF-DAE-002 |
| `compress` | booleano | no | CLI | default `false` | RF-DAE-002 |
| `backend_type` | enum | derivado | daemon | `roslyn`, `tsserver`, `text`, `tree-sitter`, `daemon` | RF-DAE-002 |
| `tail` | entero | no | UI/CLI | > 0 y acotado | RF-DAE-002 |

## 4. Process Steps (Happy Path)

1. El daemon recibe consultas desde uno o varios clientes locales.
2. El lifecycle manager reutiliza o crea runtimes por `(workspace_root, backend_type, entrypoint_id)`.
3. El daemon registra `AccessEvent` y actualiza `RuntimeSnapshot`.
4. La telemetria local preserva la ruta efectiva (`direct`, `daemon`, `direct_fallback`) y el presupuesto pedido por el cliente para diferenciar fallas de seleccion vs. fallas de truncacion.
5. La Admin UI expone el estado actual, KPIs, accesos recientes y tabs por workspace via loopback, sin perder la agrupacion canonica por `workspace_root`.
6. Si el usuario ejecuta `warm workspace` desde la UI, se invoca `POST /api/workspaces/{workspace}/warm` sin reiniciar el daemon.
7. Si se exceden `max_workers` o `idle_timeout`, el daemon aplica eviction LRU.

## 5. Outputs

| Campo | Tipo | Destino | Efecto observable |
|---|---|---|---|
| `runtime_key` | string | telemetria/UI | identifica `(workspace_root, backend_type, entrypoint_id)` |
| `memory_bytes` | numero | UI/status | memoria observada por runtime |
| `last_used_at` | timestamp | UI/status | ultimo uso del runtime |
| `admin_url` | string | usuario/skill | acceso a gobernanza loopback |
| `metrics` | objeto | UI/status | resume runtimes activos, accesos, degradaciones y cold starts |
| `warnings` | lista | UI/status/logs | comunica degradaciones sin romper el flujo principal |

## 6. Typed Errors

| Codigo | Causa | Trigger | Respuesta esperada |
|---|---|---|---|
| `DAE_RUNTIME_STATUS_FAILED` | no se puede leer estado del runtime | snapshot o proceso inconsistente | warning operativo y degradacion del status |
| `DAE_TELEMETRY_WRITE_FAILED` | no se puede persistir evento | fallo sobre `daemon.db` | warning operativo sin romper la query principal |
| `DAE_ADMIN_UNAVAILABLE` | UI o API no responden | loopback admin no saludable | error explicito para comandos `admin` |
| `DAE_WARM_FAILED` | no se puede materializar un runtime | `POST /api/workspaces/{workspace}/warm` falla | warning/error visible y log utilizable |
| `DAE_LOG_TAIL_FAILED` | no se puede leer el log local | endpoint `/api/logs` o `daemon logs` | warning accionable o error explicito |

## 7. Special Cases and Variants

- Multiples terminales y agentes del mismo usuario comparten el mismo daemon.
- `admin open --workspace <alias>` y `daemon open --workspace <alias>` abren la misma UI global con foco en ese workspace usando query params; el daemon resuelve el `workspace_root` canonico sin perder el input visible.
- `worker status` puede servirse via daemon, pero debe conservar el mismo envelope canonico del core; `active_workers` vive dentro del item diagnostico y nunca reemplaza `items` por snapshots crudos.
- `daemon logs [--tail N]` y `GET /api/logs?tail=<n>` muestran el tail de `{repoRoot}/.mi-lsp/daemon.log`; warning accionable si no existe aun.
- La telemetria persiste metadata operativa, nunca payloads completos, y distingue foco visible (`workspace`) de identidad canonica (`workspace_root`).
- La UI solo expone acciones seguras: `refresh`, `warm workspace`, `open logs` y `copy CLI command`.

## 8. Data Model Impact

- `RuntimeSnapshot`
- `AccessEvent`
- `DaemonState`

## 9. Expanded Acceptance Criteria (Gherkin)

```gherkin
Scenario: Compartir runtime caliente entre clientes
  Given un daemon saludable y un runtime Roslyn ya activo para un workspace
  When un segundo cliente local ejecuta una consulta compatible
  Then el daemon reutiliza el mismo runtime
  And la respuesta conserva semantica warm

Scenario: Exponer gobernanza local unica
  Given un daemon saludable
  When ejecuto "mi-lsp admin status"
  Then obtengo una "admin_url" loopback valida
  And la UI representa accesos y runtimes del daemon actual

Scenario: Enfocar workspace por query params
  Given un daemon saludable con admin_url valida
  When ejecuto "mi-lsp admin open --workspace gastos"
  Then el browser del sistema abre la admin_url con `?workspace=gastos&panel=overview`
  And la UI enfoca ese workspace sin crear otra instancia

Scenario: Calentar workspace desde la UI
  Given un daemon saludable y un workspace registrado
  When la UI invoca `POST /api/workspaces/{workspace}/warm`
  Then el daemon intenta materializar runtimes para ese workspace
  And la UI refleja el resultado sin reiniciar el daemon

Scenario: Ver log tail del daemon
  Given que el daemon ya corrio al menos una vez
  When consulto `GET /api/logs?tail=20`
  Then obtengo las ultimas lineas del log local o un warning no fatal si aun no existe
```

## 10. Test Traceability

- Positivo: `TP-DAE / TC-DAE-004`
- Positivo: `TP-DAE / TC-DAE-005`
- Negativo: `TP-DAE / TC-DAE-006`
- Positivo: `TP-DAE / TC-DAE-007`

## 11. No Ambiguities Left

- Supuestos prohibidos:
  - no asumir una UI por workspace
  - no persistir payloads completos en telemetria
  - no usar hash-state como deep-link canonico
- Decisiones cerradas:
  - la UI de gobernanza es unica y local
  - los runtimes se comparten por `(workspace_root, backend_type, entrypoint_id)`
  - `workspace` sigue siendo el input visible y `workspace_root` la identidad canonica tecnica
  - la UI usa query params y solo acciones seguras
- TODO explicit = 0
- Fuera de alcance:
  - autenticacion, multiusuario y observabilidad remota
- Dependencias externas explicitas:
  - loopback HTTP local y persistencia `daemon.db`
