---
wiki_contract: WIKI-CONTRACT-v1
id: RF-HOST-001
kind: requirement
status: draft
title: Ejecución Windows con host sin ventana de consola
links:
  implements: [FL-HOST-001]
---

## Propósito

Un consumidor autorizado puede ejecutar un ejecutable Windows mediante el productor `mi-lsp-host`, preservando los streams; este draft no acredita conformidad ni admisión de consumidores.

## Resultado y alcance

Un consumidor autorizado puede ejecutar un ejecutable Windows por medio del productor `mi-lsp-host`, sin ventana de consola propia del host ni de su creación del hijo, y sin alterar los bytes de sus tres streams. El draft registra contrato y bindings, no demuestra conformidad ni admisión de ningún consumidor. Ver [[RS-MILSP-RUNTIME]], [[FL-HOST-001]] y [[TECH-WINDOWS-HOST]].

## Comportamiento ordenado

El productor recibe el ejecutable y el control privado, prepara el job y ejecuta el árbol; el consumidor valida el transporte y el resultado tipado. Las condiciones detalladas se conservan en los requisitos siguientes.

## Requisitos

1. El productor acepta `execsafe-host/v1`, ejecutable absoluto, argumentos explícitos y control privado existente; no invoca shell ni busca por PATH. Control y resultado son IPC privado del caller, no autenticación ni autorización adicional.
2. El host hereda sólo tres handles stdio hacia el hijo. Control y resultado quedan separados del transporte binario, sin logs de argumentos, entorno, payload ni secretos.
3. El consumidor espera fin del host y cierre de streams; acepta únicamente transporte host cero y dos registros completos, válidos y consistentes. Propaga el DWORD tipado del payload; 125, 130 y 143 no son por sí solos evidencia de infraestructura o cancelación. Host cero no significa éxito del payload.
4. Configurar `KILL_ON_JOB_CLOSE` antes de crear root suspendido y `CREATE_NO_WINDOW`; asignar al job antes de reanudar. Root exit no completa hasta job vacío y cleanup. Cancelación y fallos terminan/drenan el job propio por handle; no matar por PID ni abrir procesos ajenos. No se promete supervivencia de daemons descendientes.
5. Los assets x64 y arm64 Windows son PE GUI subsystem 2 separados de la CLI CUI, de la misma release oficial y cubiertos por checksum SHA-256 y procedencia de fuente limpia. La admisión requiere tag público, source, checksum y readback verificables; falta de asset o evidencia es BLOCKED, sin fallback CUI.
6. La evidencia de ausencia de flashes requiere observación nativa del árbol propio; flags y subsystem no la prueban. No abrir ventana visible de calibración. No se asegura que un payload malicioso no abra su propia GUI ni que privilegios permitan aislamiento total.

## Superficies

Productor `mi-lsp-host` y consumidor autorizado Windows; stdin, stdout y stderr conservan sus bytes.

## Entradas/salidas tipadas

Entrada: `execsafe-host/v1`, ejecutable absoluto, argumentos explícitos y control privado. Salida: transporte host y dos registros completos y consistentes con resultado DWORD tipado; ver [[TECH-WINDOWS-HOST]].

## Errores

No se acepta transporte host no cero ni registros incompletos o inconsistentes. La falta de asset o evidencia bloquea la admisión, sin fallback CUI; host cero no implica éxito del payload.

## Criterios de aceptación (numerados)

Estos criterios son una transcripción de los requisitos anteriores para este documento draft; no acreditan aprobación ni ejecución.

1. El productor acepta `execsafe-host/v1`, ejecutable absoluto, argumentos explícitos y control privado existente; no invoca shell ni busca por PATH. Control y resultado son IPC privado del caller, no autenticación ni autorización adicional.
2. El host hereda sólo tres handles stdio hacia el hijo. Control y resultado quedan separados del transporte binario, sin logs de argumentos, entorno, payload ni secretos.
3. El consumidor espera fin del host y cierre de streams; acepta únicamente transporte host cero y dos registros completos, válidos y consistentes. Propaga el DWORD tipado del payload; 125, 130 y 143 no son por sí solos evidencia de infraestructura o cancelación. Host cero no significa éxito del payload.
4. Configurar `KILL_ON_JOB_CLOSE` antes de crear root suspendido y `CREATE_NO_WINDOW`; asignar al job antes de reanudar. Root exit no completa hasta job vacío y cleanup. Cancelación y fallos terminan/drenan el job propio por handle; no matar por PID ni abrir procesos ajenos. No se promete supervivencia de daemons descendientes.
5. Los assets x64 y arm64 Windows son PE GUI subsystem 2 separados de la CLI CUI, de la misma release oficial y cubiertos por checksum SHA-256 y procedencia de fuente limpia. La admisión requiere tag público, source, checksum y readback verificables; falta de asset o evidencia es BLOCKED, sin fallback CUI.
6. La evidencia de ausencia de flashes requiere observación nativa del árbol propio; flags y subsystem no la prueban. No abrir ventana visible de calibración. No se asegura que un payload malicioso no abra su propia GUI ni que privilegios permitan aislamiento total.

## Trazabilidad

Bindings de implementación: `cmd/mi-lsp-host/main_windows.go`, `internal/winhost/host_windows.go`, `.goreleaser.yaml`, `cmd/mi-lsp-host/winres.json`. Ver [[TP-HOST-001]] para oráculos y evidencia. No cambia UI del daemon ni agrega web UI, ACL, UAC o hooks de red.

```yaml
harness_protocol: SDD-HARNESS-v1
id: RF-HOST-001
kind: requirement
audience: llm-first
imports:
- .docs/wiki/04_flows/FL-HOST-001.md
exports:
- RF-HOST-001
- Ejecución Windows con host sin ventana de consola
agent_must_read:
- .docs/wiki/05_requirements/RF-HOST-001.md
- .docs/wiki/04_flows/FL-HOST-001.md
- .docs/wiki/06_tests/TP-HOST-001.md
agent_may_edit:
- .docs/wiki/05_requirements/RF-HOST-001.md
agent_must_not_edit:
- .docs/wiki/00_governance.md
- AGENTS.md
verify:
- 'Para los casos de este requisito en .docs/wiki/06_tests/TP-HOST-001.md, ejecutar
  el comando publicado: go test ./internal/winhost (la evidencia de ejecución debe
  registrarse por separado).'
stop_if:
- Detenerse si la propuesta requiere alterar criterios, decisiones o comportamiento
  más allá de RF-HOST-001.
evidence:
- .docs/wiki/05_requirements/RF-HOST-001.md
- .docs/wiki/06_tests/TP-HOST-001.md
```
