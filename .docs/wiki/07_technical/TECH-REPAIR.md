---
wiki_contract: WIKI-CONTRACT-v1
id: TECH-REPAIR
kind: technical
status: draft
title: Reparación local v1
links:
  code:
    - path: internal/repair/engine.go
      role: behavior
    - path: internal/repair/actions/actions.go
      role: behavior
---

## Resumen

Plan y aplicación acotada de reparación local; una reparación no garantiza salud semántica sin comprobación posterior.

## Mapa de componentes

Motor en `internal/repair/`, acciones en `internal/repair/actions/` y recibos en runtimeinfo.

# Reparación local v1

```yaml
harness_protocol: SDD-HARNESS-v1
id: TECH-REPAIR
kind: technical-contract
audience: dual
imports: [.docs/wiki/07_technical/TECH-REPAIR-ACTIONS.md]
exports: [TECH-REPAIR]
agent_must_read: [.docs/wiki/07_technical/TECH-REPAIR.md, internal/repair/]
agent_may_edit: [.docs/wiki/07_technical/TECH-REPAIR.md]
agent_must_not_edit: [internal/repair/, ~/.mi-lsp/repair/, PATH, antivirus-policy]
verify: ["Leer .docs/wiki/07_technical/TECH-REPAIR.md y .docs/wiki/07_technical/TECH-REPAIR-ACTIONS.md; cotejar las afirmaciones con la fuente publicada internal/repair/actions/inspect.go y sus límites. Registrar únicamente comandos y resultados efectivamente ejecutados; esta especificación no afirma una ejecución."]
stop_if: [Se proponga descarga, elevación, cambio de políticas de seguridad o reinstalación automática tras un componente registrado ausente.]
evidence: [.docs/wiki/07_technical/TECH-REPAIR.md, .docs/wiki/07_technical/TECH-REPAIR-ACTIONS.md, internal/repair/actions/inspect.go]
```

`mi-lsp repair --format json [--workspace <alias|ruta>] [--reindex] [--worker] [--worker-mode auto|framework-dependent|self-contained] [--apply]` diagnostica por defecto sin escribir. `--apply` intenta como máximo una vez cada acción independiente. No descarga ni sustituye el runtime, no pide elevación, no modifica PATH ni políticas de seguridad y no agrega exclusiones antivirus. No existe `doctor --fix` en v1.

Sin workspace explícito no se registra ni reindexa ninguno. `--reindex` solicita reconstrucción; `--worker` solicita instalación o actualización aunque ya exista. El modo predeterminado es `auto`. La configuración inválida no se pisa; la ausente puede regenerarse con valores públicos. Una reparación no garantiza salud semántica sin comprobación posterior.

## JSON y compatibilidad

La salida completa no usa el truncador del envelope. El objeto contiene `schema: mi-lsp-repair/v1`, `ok`, `mode` (`plan` o `apply`), `runtime`, `actions`, `reasonCodes` y `changed`. Los dos últimos campos de lista son siempre arrays, incluso vacíos. `runtime` incluye `executablePath`, `managedRoot`, `managedExecutablePath`, `receiptPath`. Cada acción tiene `id`, `status`, `reasonCode`, `detail`, `retryable`, `nextAction`, `lastAttemptAt` y `retryAfter`; tiempos UTC RFC3339 o null. IDs: runtime, config, workspace, index, worker. Estados: pass, skip, planned, repaired, blocked, failed. `detail` es texto humano, no una API; clientes ignoran campos adicionales. `ok` es falso ante blocked o failed; `changed` señala cambios efectivos. Salida 0 si ok, 1 si no; los errores sintácticos de Cobra pueden no producir JSON y su código depende de la configuración del ejecutable. No inferir éxito sin schema válido.

Códigos v1: OK, NOT_REQUESTED, WORKSPACE_REQUIRED, WORKSPACE_UNREGISTERED, INDEX_MISSING, REINDEX_REQUESTED, WORKER_MISSING, WORKER_INSTALL_FAILED, CONFIG_MISSING, CONFIG_INVALID, REPAIR_FAILED, REPAIR_COOLDOWN, REPAIR_STATE_INVALID, RUNTIME_REMOVED, WORKER_REMOVED, RUNTIME_UNMANAGED. Las dependencias opcionales no pedidas se omiten. Los errores técnicos reales siguen visibles.

`version --format json` publica ubicación y capacidad `repair-v1` de forma aditiva. `doctor --format json` conserva su envelope y añade `runtime-location`, con `runtime` y `reasonCode`. Clientes detectan capacidad o exigen schema exacto; runtimes anteriores requieren fallback propio. La versión mínima será la próxima que integre estos cambios, todavía no asignada.

## Ubicación, recibos y protección

La ubicación real proviene del ejecutable, resolviendo symlinks cuando sea posible. La ubicación gestionada recomendada en Windows es `LOCALAPPDATA/mi-lsp-runtime` (con fallback local de usuario); en Unix, `XDG_DATA_HOME/mi-lsp-runtime` o `~/.local/share/mi-lsp-runtime`. No implica que todos los instaladores la usen.

Los recibos `~/.mi-lsp/repair/installations.json`, esquema `mi-lsp-installations/v1`, registran componente, ruta absoluta y fecha solamente tras una instalación exitosa. Sin recibo previo, una ausencia no prueba remoción. Una ruta registrada que falta produce RUNTIME_REMOVED o WORKER_REMOVED y no se reinstala automáticamente: revisar seguridad y restaurar mediante el procedimiento autorizado. Esto es compatible con cuarentena o borrado, pero no prueba su causa. Otro ejecutable puede detectar un runtime registrado ausente; un runtime totalmente eliminado no puede autodiagnosticarse.

`~/.mi-lsp/repair/state.json` usa `mi-lsp-repair-state/v1` por acción y destino absoluto. Se persiste el intento antes del efecto, con bloqueo de escritor no bloqueante y reemplazo atómico. Una falla o interrupción impone cooldown de 300 segundos; durante ese período se informa REPAIR_COOLDOWN y `retryAfter`. El plan no modifica el estado. Un estado inválido bloquea sin sobrescribir; no hay flag force. La remoción registrada siempre exige revisión humana, incluso después del cooldown. No se incluyen secretos ni logs crudos; las rutas locales no se publican automáticamente en telemetría.
