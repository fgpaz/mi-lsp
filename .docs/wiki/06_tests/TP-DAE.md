---
wiki_contract: WIKI-CONTRACT-v1
id: TP-DAE
kind: test
status: active
title: TP-DAE
links:
  verifies:
  - RF-DAE-001
  - RF-DAE-002
  - RF-DAE-003
  - RF-DAE-004
  - RF-DAE-005
  code:
  - path: internal/daemon/newserver_owner_mitigation_test.go
    role: test
  - path: internal/daemon/instance_owner_lifetime_test.go
    role: test
---

## Alcance

RF-DAE-001, RF-DAE-002, RF-DAE-003, RF-DAE-004, RF-DAE-005. Se conserva el inventario de escenarios, límites, disposiciones y evidencia histórica, sin reejecución.

## Casos

### Mitigación de dueño huérfano — campaña nueva, no cierre global de CPU/WAL
- **TC-DAE-031**, RF-DAE-003 criterio4: `TestNewServerWithOptionsRejectsUnreachableLiveLegacyOwner`. Perfil y estado propios temporales, PID propio vivo y endpoint no alcanzable; exige error de dueño antes de crear SQLite y preservación del estado. RED observado contra source6ac45b7: el constructor devolvió servidor. GREEN observado en campaña parent sobre source3cdd8e29f656137e25d9dd15da117ae1ea96ee65; esta evidencia no prueba otra sesión S4U real.
- **TC-DAE-032**, RF-DAE-003 criterio4: validar rechazo pre-fork en `SpawnBackgroundWithOptions`, sin confundir error IPC con muerte. Acceso denegado debe conservar liveness indeterminada. Verificación nativa observada sobre candidata Windows ARM64 sourcea6773efdd6cd8862954d1edce03875ce65509bd7: `daemon serve` y `daemon start` rechazan en 104/75 ms el PID propio vivo, sin crear DB ni alterar state. No se abren procesos ajenos ni se cambian ACL. Acceso denegado queda cubierto por las pruebas existentes de `processExists` del mismo pase dirigido; no se acredita una sesión S4U real.
- **TC-DAE-033**, RF-DAE-003 criterio5: dos constructores del mismo directorio con endpoints diferentes, reserva retenida hasta cierre de DB y liberada en error; limpieza idempotente sin remover reserva ajena. GREEN observado en `TestInstanceLeaseDifferentEndpointsSameDatabase` y `TestInstanceLeaseReleasedOnConstructorFailure`, source3cdd8e29f656137e25d9dd15da117ae1ea96ee65. Incluye rechazo acotado, cierre repetido y reutilización del almacenamiento propio tras cierre/fallo.
- La campaña Go conserva cinco fallas baseline y cero nuevas aceptadas. El primer launcher contaminó los tests con dos flags de updater deshabilitado; se preservó ese fallo y se reejecutaron únicamente los dos paquetes afectados, ambos verdes con los flags retirados. No se declara suite globalmente verde.
- La campaña no acredita eliminación de duplicados legacy, raíz del CPU, expiración de servidor, límite de WAL ni adopción por mi-mcp-http. Esos resultados conservan gates propios del bug completo.

### Cobertura objetivo

- RF-DAE-001
- RF-DAE-002
- RF-DAE-003
- RF-DAE-004

### Escenarios
- **Caso**: TC-DAE-001; **Tipo**: positivo; **RF**: RF-DAE-001; **Descripción**: inicia daemon global y persiste `state.json` consistente
  - RF-DAE-001 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: inicia daemon global y persiste `state.json` consistente. No acredita una ejecución de esta migración.
- **Caso**: TC-DAE-002; **Tipo**: positivo; **RF**: RF-DAE-001; **Descripción**: `daemon start` reutiliza instancia saludable existente
  - RF-DAE-001 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: `daemon start` reutiliza instancia saludable existente. No acredita una ejecución de esta migración.
- **Caso**: TC-DAE-003; **Tipo**: negativo; **RF**: RF-DAE-001; **Descripción**: reporta fallo de boot o stop sin dejar estado inconsistente
  - RF-DAE-001 — Criterio 3. Disposición: manual_evidence_pending. Evidencia esperada: reporta fallo de boot o stop sin dejar estado inconsistente. No acredita una ejecución de esta migración.
- **Caso**: TC-DAE-004; **Tipo**: positivo; **RF**: RF-DAE-002; **Descripción**: comparte runtime entre clientes locales del mismo usuario
  - RF-DAE-002 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: comparte runtime entre clientes locales del mismo usuario. No acredita una ejecución de esta migración.
- **Caso**: TC-DAE-005; **Tipo**: positivo; **RF**: RF-DAE-002; **Descripción**: expone `admin_url` y status enriquecido de runtimes/accesos
  - RF-DAE-002 — Criterio 2. Disposición: manual_evidence_pending. Evidencia esperada: expone `admin_url` y status enriquecido de runtimes/accesos. No acredita una ejecución de esta migración.
- **Caso**: TC-DAE-006; **Tipo**: negativo; **RF**: RF-DAE-002; **Descripción**: degrada con warning si falla telemetria, warm o status admin
  - RF-DAE-002 — Criterio 2. Disposición: manual_evidence_pending. Evidencia esperada: degrada con warning si falla telemetria, warm o status admin. No acredita una ejecución de esta migración.
- **Caso**: TC-DAE-007; **Tipo**: positivo; **RF**: RF-DAE-002; **Descripción**: deep-link por query params enfoca workspace/panel correcto
  - RF-DAE-002 — Criterio 3. Disposición: manual_evidence_pending. Evidencia esperada: deep-link por query params enfoca workspace/panel correcto. No acredita una ejecución de esta migración.
- **Caso**: TC-DAE-008; **Tipo**: positivo; **RF**: RF-DAE-002; **Descripción**: `POST /api/workspaces/{workspace}/warm` materializa runtimes sin reiniciar daemon
  - RF-DAE-002 — Criterio 4. Disposición: manual_evidence_pending. Evidencia esperada: `POST /api/workspaces/{workspace}/warm` materializa runtimes sin reiniciar daemon. No acredita una ejecución de esta migración.
- **Caso**: TC-DAE-009; **Tipo**: positivo; **RF**: RF-DAE-002; **Descripción**: `GET /api/logs?tail=n` devuelve tail del log local o warning no fatal
  - RF-DAE-002 — Criterio 5. Disposición: manual_evidence_pending. Evidencia esperada: `GET /api/logs?tail=n` devuelve tail del log local o warning no fatal. No acredita una ejecución de esta migración.
- **Caso**: TC-DAE-010; **Tipo**: positivo; **RF**: RF-DAE-003; **Descripción**: semantic query auto-inicia daemon en background
  - RF-DAE-003 — Criterio 3. Disposición: manual_evidence_pending. Evidencia esperada: semantic query auto-inicia daemon en background. No acredita una ejecución de esta migración.
- **Caso**: TC-DAE-011; **Tipo**: positivo; **RF**: RF-DAE-003; **Descripción**: --no-auto-daemon previene auto-start
  - RF-DAE-003 — Criterio 3. Disposición: manual_evidence_pending. Evidencia esperada: --no-auto-daemon previene auto-start. No acredita una ejecución de esta migración.
- **Caso**: TC-DAE-012; **Tipo**: negativo; **RF**: RF-DAE-003; **Descripción**: auto-start timeout → fallback a direct mode
  - RF-DAE-003 — Criterio 2. Disposición: manual_evidence_pending. Evidencia esperada: auto-start timeout → fallback a direct mode. No acredita una ejecución de esta migración.
- **Caso**: TC-DAE-013; **Tipo**: positivo; **RF**: RF-DAE-004; **Descripción**: file watcher detecta cambio .cs y re-indexa
  - RF-DAE-004 — Criterio 2. Disposición: manual_evidence_pending. Evidencia esperada: file watcher detecta cambio .cs y re-indexa. No acredita una ejecución de esta migración.
- **Caso**: TC-DAE-014; **Tipo**: positivo; **RF**: RF-DAE-004; **Descripción**: file watcher respeta debounce 500ms
  - RF-DAE-004 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: file watcher respeta debounce 500ms. No acredita una ejecución de esta migración.
- **Caso**: TC-DAE-015; **Tipo**: positivo; **RF**: RF-DAE-004; **Descripción**: file watcher ignora archivos en node_modules
  - RF-DAE-004 — Criterio 2. Disposición: manual_evidence_pending. Evidencia esperada: file watcher ignora archivos en node_modules. No acredita una ejecución de esta migración.
- **Caso**: TC-DAE-016; **Tipo**: positivo; **RF**: RF-DAE-002; **Descripción**: `worker status` servido por daemon conserva el contrato canónico del core y expone `active_workers` dentro del item diagnóstico
  - RF-DAE-002 — Criterio 2. Disposición: manual_evidence_pending. Evidencia esperada: `worker status` servido por daemon conserva el contrato canónico del core y expone `active_workers` dentro del item diagnóstico. No acredita una ejecución de esta migración.
- **Caso**: TC-DAE-017; **Tipo**: positivo; **RF**: RF-DAE-004; **Descripción**: watcher lazy deduplica aliases del mismo root y respeta `max_watched_roots` con eviction LRU
  - RF-DAE-004 — Criterio 4. Disposición: manual_evidence_pending. Evidencia esperada: watcher lazy deduplica aliases del mismo root y respeta `max_watched_roots` con eviction LRU. No acredita una ejecución de esta migración.
- **Caso**: TC-DAE-018; **Tipo**: positivo; **RF**: RF-DAE-002, RF-DAE-004; **Descripción**: `daemon status` y `/api/status` exponen `daemon_process` y `watchers`
  - RF-DAE-002 — Criterio 2; RF-DAE-004 — Criterio 5. Disposición: manual_evidence_pending. Evidencia esperada: `daemon status` y `/api/status` exponen `daemon_process` y `watchers`. No acredita una ejecución de esta migración.
- **Caso**: TC-DAE-018A; **Tipo**: positivo; **RF**: RF-DAE-002; **Descripción**: `TestDaemonStaleWarningWhenMetadataMissing` / `TestDaemonStaleWarningWhenExecutableHashDiffers` / `TestDaemonStaleWarningIgnoresDifferentPathForSameExecutableHash` / `TestDaemonStaleWarningWhenExecutableMTimeDiffers`: `daemon status` advierte cuando el daemon no reporta metadata de ejecutable o parece correr una build anterior al CLI, priorizando hash para no marcar stale por paths temporales equivalentes
  - RF-DAE-002 — Criterio 2. Disposición: manual_evidence_pending. Evidencia esperada: `TestDaemonStaleWarningWhenMetadataMissing` / `TestDaemonStaleWarningWhenExecutableHashDiffers` / `TestDaemonStaleWarningIgnoresDifferentPathForSameExecutableHash` / `TestDaemonStaleWarningWhenExecutableMTimeDiffers`: `daemon status` advierte cuando el daemon no reporta metadata de ejecutable o parece correr una build anterior al CLI, priorizando hash para no marcar stale por paths temporales equivalentes. No acredita una ejecución de esta migración.
- **Caso**: TC-DAE-019; **Tipo**: negativo; **RF**: RF-DAE-002; **Descripción**: saturacion de requests daemon-aware devuelve `daemon/backpressure_busy` tipado sin bloquear `system.status`
  - RF-DAE-002 — Criterio 2. Disposición: manual_evidence_pending. Evidencia esperada: saturacion de requests daemon-aware devuelve `daemon/backpressure_busy` tipado sin bloquear `system.status`. No acredita una ejecución de esta migración.
- **Caso**: TC-DAE-020; **Tipo**: positivo; **RF**: RF-DAE-002; **Descripción**: `daemon perf-smoke --callers 16` valida una instancia alcanzable y presupuestos de memoria/handles
  - RF-DAE-002 — Criterio 3. Disposición: manual_evidence_pending. Evidencia esperada: `daemon perf-smoke --callers 16` valida una instancia alcanzable y presupuestos de memoria/handles. No acredita una ejecución de esta migración.
- **Caso**: TC-DAE-021; **Tipo**: positivo; **RF**: RF-DAE-002; **Descripción**: aliases distintos con mismo root/backend/entrypoint comparten `runtime_key` canónico
  - RF-DAE-002 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: aliases distintos con mismo root/backend/entrypoint comparten `runtime_key` canónico. No acredita una ejecución de esta migración.
- **Caso**: TC-DAE-022; **Tipo**: positivo; **RF**: RF-DAE-002; **Descripción**: entrypoints distintos del mismo root generan runtimes separados
  - RF-DAE-002 — Criterio 4. Disposición: manual_evidence_pending. Evidencia esperada: entrypoints distintos del mismo root generan runtimes separados. No acredita una ejecución de esta migración.
- **Caso**: TC-DAE-023; **Tipo**: positivo; **RF**: RF-DAE-002; **Descripción**: release smoke separa skipped de failed y reporta `unique_root_count`, `duplicate_root_count`, `aliases_per_root`
  - RF-DAE-002 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: release smoke separa skipped de failed y reporta `unique_root_count`, `duplicate_root_count`, `aliases_per_root`. No acredita una ejecución de esta migración.
- **Caso**: TC-DAE-024; **Tipo**: negativo; **RF**: RF-DAE-002; **Descripción**: `daemon perf-smoke` devuelve `ok=false` tipado si working set, private bytes o handles superan el umbral configurado
  - RF-DAE-002 — Criterio 5. Disposición: manual_evidence_pending. Evidencia esperada: `daemon perf-smoke` devuelve `ok=false` tipado si working set, private bytes o handles superan el umbral configurado. No acredita una ejecución de esta migración.
- **Caso**: TC-DAE-025; **Tipo**: positivo; **RF**: RF-DAE-002; **Descripción**: `admin export --format toon` y `admin export --summary --format toon` serializan telemetria agent-readable sin payloads crudos
  - RF-DAE-002 — Criterio 6. Disposición: manual_evidence_pending. Evidencia esperada: `admin export --format toon` y `admin export --summary --format toon` serializan telemetria agent-readable sin payloads crudos. No acredita una ejecución de esta migración.
- **Caso**: TC-DAE-026; **Tipo**: positivo; **RF**: RF-DAE-002; **Descripción**: `admin export --summary` agrega `recommendations` y usage-doctor actions derivadas de telemetria sanitizada sin cambiar raw events ni exponer payloads
  - RF-DAE-002 — Criterio 6. Disposición: manual_evidence_pending. Evidencia esperada: `admin export --summary` agrega `recommendations` y usage-doctor actions derivadas de telemetria sanitizada sin cambiar raw events ni exponer payloads. No acredita una ejecución de esta migración.
- **Caso**: TC-DAE-027; **Tipo**: positivo; **RF**: RF-DAE-005; **Descripción**: `mi-lsp stats --by-client --days 7 --format json` agrupa llamadas e informa éxito, p50/p90 de latencia y bytes, y fallback sin contenido de consultas
  - RF-DAE-005 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: `mi-lsp stats --by-client --days 7 --format json` agrupa llamadas e informa éxito, p50/p90 de latencia y bytes, y fallback sin contenido de consultas. No acredita una ejecución de esta migración.
  - RF-DAE-005 — Criterio 5. Disposición: manual_evidence_pending. Evidencia esperada: la salida agregada no contiene contenido de consultas ni archivos. No acredita una ejecución de esta migración.
- **Caso**: TC-DAE-028; **Tipo**: negativo; **RF**: RF-DAE-005; **Descripción**: `--days 0` se rechaza; fallo de almacenamiento/consulta no se representa falsamente como actividad cero
  - RF-DAE-005 — Criterio 3. Disposición: manual_evidence_pending. Evidencia esperada: `--days 0` se rechaza antes de consultar `daemon.db`. No acredita una ejecución de esta migración.
  - RF-DAE-005 — Criterio 4. Disposición: manual_evidence_pending. Evidencia esperada: un fallo de almacenamiento o consulta se informa, no se representa como actividad cero. No acredita una ejecución de esta migración.
- **Caso**: TC-DAE-029; **Tipo**: positivo; **RF**: RF-DAE-002; **Descripción**: `TestClientCohortTreatsBenchAsTestAndManualCLIAsUnknown` y `TestSummaryAttributionDoesNotCountBenchOrManualCLIAsWork`: `bench` queda en `test`, `manual-cli`/cliente vacío en `unknown`, y ninguno suma a `work`
  - RF-DAE-002 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: `TestClientCohortTreatsBenchAsTestAndManualCLIAsUnknown` y `TestSummaryAttributionDoesNotCountBenchOrManualCLIAsWork`: `bench` queda en `test`, `manual-cli`/cliente vacío en `unknown`, y ninguno suma a `work`. No acredita una ejecución de esta migración.
- **Caso**: TC-DAE-030; **Tipo**: positivo; **RF**: RF-DAE-002; **Descripción**: `TestEnrichAccessEventAssignsTypedQFailureCodes`, `TestEnrichAccessEventTypesNavFindCatalogFallback` y `TestCatalogTelemetryErrorCodeDistinguishesCatalogStates`: q se tipa por etapa y `nav.find` distingue índice ausente/ilegible/roto sin persistir consulta cruda
  - RF-DAE-002 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: `TestEnrichAccessEventAssignsTypedQFailureCodes`, `TestEnrichAccessEventTypesNavFindCatalogFallback` y `TestCatalogTelemetryErrorCodeDistinguishesCatalogStates`: q se tipa por etapa y `nav.find` distingue índice ausente/ilegible/roto sin persistir consulta cruda. No acredita una ejecución de esta migración.

- **Marcador estable**: tp-dae-stats-cases
- **Estado del comportamiento**: planned_not_executed
- **campaign_status**: NOT_RUN
- **Casos**:
  - **by_client_aggregates**:
    - **id**: TC-DAE-027
  - RF-DAE-005 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
  - RF-DAE-005 — Criterio 5. Disposición: manual_evidence_pending. Evidencia esperada: la salida agregada no contiene contenido de consultas ni archivos. No acredita una ejecución de esta migración.
    - **Estado del comportamiento**: planned
    - **Oráculo**:
      - calls_and_ok_pct
      - p50_p90_latency
      - p50_p90_bytes
      - fallback_pct
      - no_query_content
  - **invalid_days_and_storage_failure**:
    - **id**: TC-DAE-028
  - RF-DAE-005 — Criterio 3. Disposición: manual_evidence_pending. Evidencia esperada: `--days 0` se rechaza antes de consultar `daemon.db`. No acredita una ejecución de esta migración.
  - RF-DAE-005 — Criterio 4. Disposición: manual_evidence_pending. Evidencia esperada: un fallo de almacenamiento o consulta se informa, no se representa como actividad cero. No acredita una ejecución de esta migración.
    - **Estado del comportamiento**: planned
    - **Oráculo**:
      - nonpositive_days_rejected
      - storage_error_not_reported_as_empty_usage
- **Comprobación prevista**:
  - authorized FINAL_VERIFY runs bounded stats aggregation tests
- **Condiciones de rechazo**:
  - stats_exposes_query_or_file_content
  - storage_failure_is_presented_as_zero_activity
- **Evidencia referenciada**:
  - internal/cli/stats.go
  - internal/daemon/state_store.go
  - .docs/wiki/06_tests/TP-DAE.md

```yaml
harness_protocol: SDD-HARNESS-v1
id: TP-DAE
kind: test-plan
audience: llm-first
imports:
  - .docs/wiki/00_governance.md
  - .docs/wiki/05_requirements/RF-DAE-001.md
exports:
  - TP-DAE
agent_must_read:
  - .docs/wiki/06_tests/TP-DAE.md
agent_may_edit:
  - .docs/wiki/06_tests/TP-DAE.md
agent_must_not_edit:
  - .docs/wiki/05_requirements/RF-DAE-001.md
  - internal/daemon/state_store.go
verify:
  - .docs/wiki/06_tests/TP-DAE.md#Casos
stop_if:
  - harness_verdict=BLOCKED
  - test_oracle_or_acceptance_would_change
  - evidence_not_presented_in_the_source
evidence:
  - .docs/wiki/06_tests/TP-DAE.md
  - .docs/wiki/05_requirements/RF-DAE-001.md
  - .docs/wiki/05_requirements/RF-DAE-002.md
  - .docs/wiki/05_requirements/RF-DAE-003.md
  - .docs/wiki/05_requirements/RF-DAE-004.md
  - .docs/wiki/05_requirements/RF-DAE-005.md
```
