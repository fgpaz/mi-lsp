---
wiki_contract: WIKI-CONTRACT-v1
id: RF-UPD-001
kind: requirement
status: active
title: Actualizar la CLI portable con estado común y promoción verificable
links:
  implements:
  - FL-UPD-001
  code:
  - path: internal/selfupdate/engine.go
    role: behavior
  - path: internal/selfupdate/state.go
    role: contract
  - path: internal/selfupdate/source.go
    role: behavior
  - path: internal/cli/update.go
    role: behavior
  - path: internal/daemon/update.go
    role: behavior
---

## Propósito

Mantener actualizadas las instalaciones portables de todos los consumidores sin duplicar el updater ni perder trabajo vivo.

## Comportamiento ordenado

Un comando normal y el daemon programan trabajo desconectado; un dueño compatible evita duplicación. Metadata automática se limita globalmente por home a 24 horas. La aplicación es por ejecutable canónico, sin mezclar user-bin y ~/.local/bin. Las operaciones explícitas update y update --check no disparan un segundo autorun.

## Superficies

mi-lsp update aplica; mi-lsp update --check --format json informa estado existente y readback sin red ni escrituras, incluso si la caché venció. Ausencia o ilegibilidad de estado es un error normal; no fabrica una comprobación.

## Entradas/salidas tipadas

update-status/v1 contiene exactamente schema, tool, installed, latest, last_check, last_result, code, previous, channel y auto. Resultados y diez códigos son los de [[TECH-RUNTIME-UPDATE]]. En updated, installed == latest y previous contiene la versión anterior. En otros resultados previous es null. installed siempre se lee del disco, no del proceso viejo.

## Errores

Red sin aviso y reintento en ventana siguiente; checksum incorrecto fail-closed; nunca downgrade ni bajo el piso 0.16.0. No se declara rollback si no se confirmó restauración. Un fallo técnico no representable termina con error sanitizado en vez de fabricar un código o estado exitoso. Cuarentena requiere evidencia específica de desaparición/bloqueo del nuevo; no se atribuye antivirus a cualquier error.

## Criterios de aceptación (numerados)

1. latest oficial y SHA exacto del RID, con límites de red/archivo, piso y comparación semántica, impiden instalar una candidata inválida o downgrade.
2. Lock entre procesos y caché de 24 horas sobreviven reinicios; dos instalaciones del mismo home pueden aplicar la candidata común sin una segunda consulta de metadata.
3. Pin explícito y MI_LSP_AUTO_UPDATE=0 se respetan; user-bin y ~/.local/bin de mis-entornos no constituyen pin por su ruta.
4. Windows conserva y restaura .old tras readback fallido; Unix no deja una ventana sin ejecutable. Limpieza sólo propia y fuera del readback interno.
5. Check nunca escribe ni crea locks/staging ni contacta red, devuelve diez campos canónicos y exit 0 si pudo leer un estado failed/deferred.
6. Autorun no bloquea la operación ni abre ventana; excluye builds dev, hijo, help, update/check, doctor, probe y repair.
7. Daemon actualizado en disco conserva requests, respuestas, HTTP, actividad y contextos vivos; sólo se reejecuta con barrera idle real, preservando opciones/endpoint y limpiando estado viejo antes del nuevo.
8. Pruebas reales Windows y Linux VPS acreditan origen, readback y no interrupción; la compilación macOS no se vende como ejecución nativa. Una fixture bootstrap 0.15.x usa fuente nueva identificada, nunca se confunde con una release antigua sin updater.

## Restricciones ADR

No requiere admin, no logs con rutas de usuario, no modifica índices/registries por actualizar y no instala workers como efecto de consultas. Una primera adopción de updater desde versiones antiguas sin esa función se informa como bootstrap manual.

```yaml
harness_protocol: SDD-HARNESS-v1
id: RF-UPD-001
kind: requirement
audience: llm-first
imports:
- .docs/wiki/04_flows/FL-UPD-001.md
exports:
- RF-UPD-001
- Actualizar la CLI portable con estado común y promoción verificable
agent_must_read:
- .docs/wiki/05_requirements/RF-UPD-001.md
- .docs/wiki/04_flows/FL-UPD-001.md
- .docs/wiki/06_tests/TP-UPD-001.md
agent_may_edit:
- .docs/wiki/05_requirements/RF-UPD-001.md
agent_must_not_edit:
- .docs/wiki/00_governance.md
- AGENTS.md
- internal/selfupdate/engine.go
- internal/selfupdate/state.go
- internal/selfupdate/source.go
- internal/cli/update.go
- internal/daemon/update.go
verify:
- Validar el contrato y sus referencias con `mi-lsp nav wiki validate-harness --workspace
  <alias>`; contrastar los casos de .docs/wiki/06_tests/TP-UPD-001.md sin inferir
  que fueron ejecutados.
stop_if:
- Detenerse si la propuesta requiere alterar criterios, decisiones o comportamiento
  más allá de RF-UPD-001.
evidence:
- .docs/wiki/05_requirements/RF-UPD-001.md
- .docs/wiki/06_tests/TP-UPD-001.md
```
