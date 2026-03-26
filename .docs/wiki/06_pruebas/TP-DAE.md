# TP-DAE

## Cobertura objetivo

- RF-DAE-001
- RF-DAE-002
- RF-DAE-003
- RF-DAE-004

## Casos

| Caso | Tipo | RF | Descripcion |
|---|---|---|---|
| TC-DAE-001 | positivo | RF-DAE-001 | inicia daemon global y persiste `state.json` consistente |
| TC-DAE-002 | positivo | RF-DAE-001 | `daemon start` reutiliza instancia saludable existente |
| TC-DAE-003 | negativo | RF-DAE-001 | reporta fallo de boot o stop sin dejar estado inconsistente |
| TC-DAE-004 | positivo | RF-DAE-002 | comparte runtime entre clientes locales del mismo usuario |
| TC-DAE-005 | positivo | RF-DAE-002 | expone `admin_url` y status enriquecido de runtimes/accesos |
| TC-DAE-006 | negativo | RF-DAE-002 | degrada con warning si falla telemetria, warm o status admin |
| TC-DAE-007 | positivo | RF-DAE-002 | deep-link por query params enfoca workspace/panel correcto |
| TC-DAE-008 | positivo | RF-DAE-002 | `POST /api/workspaces/{workspace}/warm` materializa runtimes sin reiniciar daemon |
| TC-DAE-009 | positivo | RF-DAE-002 | `GET /api/logs?tail=n` devuelve tail del log local o warning no fatal |
| TC-DAE-010 | positivo | RF-DAE-003 | semantic query auto-inicia daemon en background |
| TC-DAE-011 | positivo | RF-DAE-003 | --no-auto-daemon previene auto-start |
| TC-DAE-012 | negativo | RF-DAE-003 | auto-start timeout → fallback a direct mode |
| TC-DAE-013 | positivo | RF-DAE-004 | file watcher detecta cambio .cs y re-indexa |
| TC-DAE-014 | positivo | RF-DAE-004 | file watcher respeta debounce 500ms |
| TC-DAE-015 | positivo | RF-DAE-004 | file watcher ignora archivos en node_modules |
| TC-DAE-016 | positivo | RF-DAE-002 | `worker status` servido por daemon conserva el contrato canonico del core y expone `active_workers` dentro del item diagnostico |
