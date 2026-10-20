---
wiki_contract: WIKI-CONTRACT-v1
id: RF-DAE-002
kind: requirement
status: active
title: RF-DAE-002 - Compartir runtimes, governance UI y telemetria local
links:
  implements:
  - FL-DAE-01
  code:
  - path: internal/daemon/admin.go
    role: behavior
  - path: internal/daemon/export.go
    role: behavior
  - path: internal/daemon/log_tail.go
    role: behavior
  - path: internal/daemon/perf_smoke.go
    role: behavior
  - path: internal/daemon/server.go
    role: behavior
  - path: internal/service/app.go
    role: behavior
  - path: internal/service/autoindex.go
    role: behavior
  - path: internal/telemetry/access_diagnostics.go
    role: behavior
---

## Propósito

RF-DAE-002 - Compartir runtimes, governance UI y telemetria local

### 1. Hoja de ejecución
- **Campo**: ID; **Valor**: RF-DAE-002
- **Campo**: Título; **Valor**: Compartir runtimes, governance UI y telemetria local
- **Campo**: Actores; **Valor**: Desarrollador, Agente, CLI, Daemon, Admin UI
- **Campo**: Prioridad; **Valor**: alta
- **Campo**: Severidad; **Valor**: media
- **Campo**: FL origen; **Valor**: FL-DAE-01

## Comportamiento ordenado

### 2. Precondiciones detalladas
- **Condición**: Daemon global saludable; **Tipo**: técnica; **Estado requerido**: obligatorio
- **Condición**: `daemon.db` escribible; **Tipo**: operativa; **Estado requerido**: obligatorio
- **Condición**: Cliente envia metadata operativa cuando esta disponible; **Tipo**: operativa; **Estado requerido**: recomendado
- **Condición**: `admin_url` accesible por loopback; **Tipo**: operativa; **Estado requerido**: obligatorio para UI

### 4. Secuencia principal
1. El daemon recibe consultas desde uno o varios clientes locales.
2. El lifecycle manager reutiliza o crea runtimes por `(workspace_root, backend_type, entrypoint_id)`.
3. El daemon registra `AccessEvent` y actualiza `RuntimeSnapshot`.
4. La telemetria local preserva la ruta efectiva (`direct`, `daemon`, `direct_fallback`) y el presupuesto pedido por el cliente para diferenciar fallas de selección vs. fallas de truncacion.
5. La Admin UI expone el estado actual, KPIs, accesos recientes y tabs por workspace via loopback, sin perder la agrupacion canónica por `workspace_root`.
6. Si el usuario ejecuta `warm workspace` desde la UI, se invoca `POST /api/workspaces/{workspace}/warm` sin reiniciar el daemon.
7. Si se exceden `max_workers` (default 8) o `idle_timeout`, el daemon aplica eviction LRU. En Linux los workers mueren con el daemon (`Pdeathsig=SIGKILL`), sin dejar language servers huerfanos.
8. `admin export --format toon` y `admin export --summary --format toon` serializan telemetria local en formato agent-readable sin cambiar el origen canónico (`daemon.db`).
9. Cuando una query degrada de backend por falla runtime, la telemetria guarda solo metadata causal sanitizada (`requested_backend`, `result_backend`, `backend_fallback_taken`, `fallback_from`, `fallback_to`, `runtime_error_code`).
10. `admin export --summary --attribution` conserva `manual-cli` y el cliente vacío en `unknown`; la operación `bench` se atribuye a `test`, nunca a `work`. Los fallos q se tipan por etapa (`q_parse_failed`, `q_execute_failed`, `q_workspace_failed`, `q_projection_failed`) y el fallback de `nav.find` distingue índice ausente, ilegible y roto (`nav_find_index_absent`, `nav_find_index_unreadable`, `nav_find_index_broken`). Solo se persisten códigos y etapas sanitizados, nunca consultas crudas.

### 7. Casos especiales y variantes
- Multiples terminales y agentes del mismo usuario comparten el mismo daemon.
- `admin open --workspace <alias>` y `daemon open --workspace <alias>` abren la misma UI global con foco en ese workspace usando query params; el daemon resuelve el `workspace_root` canónico sin perder el input visible.
- `worker status` puede servirse via daemon, pero debe conservar el mismo envelope canónico del core; `active_workers` vive dentro del item diagnóstico y nunca reemplaza `items` por snapshots crudos.
- `daemon logs [--tail N]` y `GET /api/logs?tail=<n>` muestran el tail de `{repoRoot}/.mi-lsp/daemon.log`; warning accionable si no existe aun.
- La telemetria persiste metadata operativa, nunca payloads completos, y distingue foco visible (`workspace`) de identidad canónica (`workspace_root`).
- `decision_json` puede exponer evidencia de fallback runtime/backend, pero no query cruda, argv, payloads, paths sensibles ni contenido de archivos.
- `admin export` raw y summary soportan `--format toon` para revision de logs por harnesses/agentes sin postproceso ad hoc.
- `admin export --summary` puede agregar `recommendations` derivadas de telemetria local y usage-doctor actions; son aditivas, no cambian agregados existentes y nunca incluyen query cruda, argv, payloads ni contenido de archivos.
- La UI solo expone acciones seguras: `refresh`, `warm workspace`, `open logs` y `copy CLI command`.

### 11. Decisiones cerradas
- Supuestos prohibidos:
  - no asumir una UI por workspace
  - no persistir payloads completos en telemetria
  - no usar hash-state como deep-link canónico
- Decisiones cerradas:
  - la UI de gobernanza es única y local
  - los runtimes se comparten por `(workspace_root, backend_type, entrypoint_id)`
  - `workspace` sigue siendo el input visible y `workspace_root` la identidad canónica técnica
  - la UI usa query params y solo acciones seguras
- TODO explicit = 0
- Fuera de alcance:
  - autenticacion, multiusuario y observabilidad remota
- Dependencias externas explícitas:
  - loopback HTTP local y persistencia `daemon.db`

## Superficies

CLI `mi-lsp`, con operaciones y selectores detallados en el comportamiento del requisito.

## Entradas/salidas tipadas

### 3. Entradas
- **Campo**: `workspace`; **Tipo**: alias/path; **Req.**: no; **Origen**: CLI/query; **Validacion**: input visible para foco de UI y warm; el daemon resuelve `workspace_root` canónico; **RN**: RF-DAE-002
- **Campo**: `panel`; **Tipo**: enum; **Req.**: no; **Origen**: query/admin UI; **Validacion**: `overview`, `activity`, `logs`; **RN**: RF-DAE-002
- **Campo**: `client_name`; **Tipo**: string; **Req.**: no; **Origen**: CLI/env; **Validacion**: default `manual-cli`; **RN**: RF-DAE-002
- **Campo**: `session_id`; **Tipo**: string; **Req.**: no; **Origen**: CLI/env; **Validacion**: puede ser omitido; **RN**: RF-DAE-002
- **Campo**: `format`; **Tipo**: enum; **Req.**: no; **Origen**: CLI; **Validacion**: `compact`, `json`, `text`, `toon`, `yaml`; **RN**: RF-DAE-002
- **Campo**: `token_budget`; **Tipo**: entero; **Req.**: no; **Origen**: CLI; **Validacion**: > 0 cuando se explícita; **RN**: RF-DAE-002
- **Campo**: `max_items`; **Tipo**: entero; **Req.**: no; **Origen**: CLI; **Validacion**: > 0 cuando se explícita; **RN**: RF-DAE-002
- **Campo**: `max_chars`; **Tipo**: entero; **Req.**: no; **Origen**: CLI; **Validacion**: >= 0; **RN**: RF-DAE-002
- **Campo**: `compress`; **Tipo**: booleano; **Req.**: no; **Origen**: CLI; **Validacion**: default `false`; **RN**: RF-DAE-002
- **Campo**: `backend_type`; **Tipo**: enum; **Req.**: derivado; **Origen**: daemon; **Validacion**: `roslyn`, `tsserver`, `pyright`, `gopls`, `rust-analyzer`, `catalog`, `text`, `tree-sitter`, `daemon`; **RN**: RF-DAE-002
- **Campo**: `tail`; **Tipo**: entero; **Req.**: no; **Origen**: UI/CLI; **Validacion**: > 0 y acotado; **RN**: RF-DAE-002

### 5. Salidas
- **Campo**: `runtime_key`; **Tipo**: string; **Destino**: telemetria/UI; **Efecto observable**: identifica `(workspace_root, backend_type, entrypoint_id)`
- **Campo**: `memory_bytes`; **Tipo**: número; **Destino**: UI/status; **Efecto observable**: memoria observada por runtime
- **Campo**: `last_used_at`; **Tipo**: timestamp; **Destino**: UI/status; **Efecto observable**: ultimo uso del runtime
- **Campo**: `admin_url`; **Tipo**: string; **Destino**: usuario/skill; **Efecto observable**: acceso a gobernanza loopback
- **Campo**: `metrics`; **Tipo**: objeto; **Destino**: UI/status; **Efecto observable**: resume runtimes activos, accesos, degradaciones y cold starts
- **Campo**: `warnings`; **Tipo**: lista; **Destino**: UI/status/logs; **Efecto observable**: comunica degradaciones sin romper el flujo principal

### 8. Impacto en el modelo de datos
- `RuntimeSnapshot`
- `AccessEvent`
- `DaemonState`

## Errores

### 6. Errores tipados
- **Código**: `DAE_RUNTIME_STATUS_FAILED`; **Causa**: no se puede leer estado del runtime; **Trigger**: snapshot o proceso inconsistente; **Respuesta esperada**: warning operativo y degradacion del status
- **Código**: `DAE_TELEMETRY_WRITE_FAILED`; **Causa**: no se puede persistir evento; **Trigger**: fallo sobre `daemon.db`; **Respuesta esperada**: warning operativo sin romper la query principal
- **Código**: `DAE_ADMIN_UNAVAILABLE`; **Causa**: UI o API no responden; **Trigger**: loopback admin no saludable; **Respuesta esperada**: error explícito para comandos `admin`
- **Código**: `DAE_WARM_FAILED`; **Causa**: no se puede materializar un runtime; **Trigger**: `POST /api/workspaces/{workspace}/warm` falla; **Respuesta esperada**: warning/error visible y log utilizable
- **Código**: `DAE_LOG_TAIL_FAILED`; **Causa**: no se puede leer el log local; **Trigger**: endpoint `/api/logs` o `daemon logs`; **Respuesta esperada**: warning accionable o error explícito

## Criterios de aceptación (numerados)

1. Compartir runtime caliente entre clientes: Dado un daemon saludable y un runtime Roslyn ya activo para un workspace Cuando un segundo cliente local ejecuta una consulta compatible Entonces el daemon reutiliza el mismo runtime Y la respuesta conserva semántica warm
2. Exponer gobernanza local única: Dado un daemon saludable Cuando ejecuto "mi-lsp admin status" Entonces obtengo una "admin_url" loopback valida Y la UI representa accesos y runtimes del daemon actual
3. Enfocar workspace por query params: Dado un daemon saludable con admin_url valida Cuando ejecuto "mi-lsp admin open --workspace gastos" Entonces el browser del sistema abre la admin_url con `?workspace=gastos&panel=overview` Y la UI enfoca ese workspace sin crear otra instancia
4. Calentar workspace desde la UI: Dado un daemon saludable y un workspace registrado Cuando la UI invoca `POST /api/workspaces/{workspace}/warm` Entonces el daemon intenta materializar runtimes para ese workspace Y la UI refleja el resultado sin reiniciar el daemon
5. Ver log tail del daemon: Dado que el daemon ya corrio al menos una vez Cuando consulto `GET /api/logs?tail=20` Entonces obtengo las ultimas líneas del log local o un warning no fatal si aun no existe
6. Exportar telemetria para un harness agente: Dado que existen `access_events` recientes Cuando ejecuto "mi-lsp admin export --format toon" Entonces obtengo una salida agent-readable con metadata operativa sanitizada Y no contiene payloads crudos, argv, queries ni contenido de archivos

### 9. Escenarios de aceptación (Gherkin)
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

Scenario: Exportar telemetria para un harness agente
  Given que existen `access_events` recientes
  When ejecuto "mi-lsp admin export --format toon"
  Then obtengo una salida agent-readable con metadata operativa sanitizada
  And no contiene payloads crudos, argv, queries ni contenido de archivos
```

### 10. Trazabilidad de pruebas
- Positivo: `TP-DAE / TC-DAE-004`
- Positivo: `TP-DAE / TC-DAE-005`
- Negativo: `TP-DAE / TC-DAE-006`
- Positivo: `TP-DAE / TC-DAE-007`
- Positivo: `TP-DAE / TC-DAE-024`
