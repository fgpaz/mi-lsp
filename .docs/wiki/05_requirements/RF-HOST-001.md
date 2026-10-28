---
wiki_contract: WIKI-CONTRACT-v1
id: RF-HOST-001
kind: requirement
status: draft
title: Ejecución Windows con host sin ventana de consola
---

## Resultado y alcance

Un consumidor autorizado puede ejecutar un ejecutable Windows por medio del productor `mi-lsp-host`, sin ventana de consola propia del host ni de su creación del hijo, y sin alterar los bytes de sus tres streams. Esta capacidad es candidata, no publicada ni admitida; el draft registra cobertura y bindings, no conformidad. Ver [[RS-MILSP-RUNTIME]], [[FL-HOST-001]] y [[TECH-WINDOWS-HOST]].

## Requisitos

1. El productor acepta `execsafe-host/v1`, ejecutable absoluto, argumentos explícitos y control privado existente; no invoca shell ni busca por PATH. Control y resultado son IPC privado del caller, no autenticación ni autorización adicional.
2. El host hereda sólo tres handles stdio hacia el hijo. Control y resultado quedan separados del transporte binario, sin logs de argumentos, entorno, payload ni secretos.
3. El consumidor espera fin del host y cierre de streams; acepta únicamente transporte host cero y dos registros completos, válidos y consistentes. Propaga el DWORD tipado del payload; 125, 130 y 143 no son por sí solos evidencia de infraestructura o cancelación. Host cero no significa éxito del payload.
4. Configurar `KILL_ON_JOB_CLOSE` antes de crear root suspendido y `CREATE_NO_WINDOW`; asignar al job antes de reanudar. Root exit no completa hasta job vacío y cleanup. Cancelación y fallos terminan/drenan el job propio por handle; no matar por PID ni abrir procesos ajenos. No se promete supervivencia de daemons descendientes.
5. Los assets candidatos x64 y arm64 Windows son PE GUI subsystem 2 separados de la CLI CUI, de la misma release oficial y cubiertos por checksum SHA-256 y procedencia de fuente limpia. Falta de asset o procedencia es BLOCKED, sin fallback CUI. El tag 0.16.1 es objetivo aún no publicado.
6. La evidencia de ausencia de flashes requiere observación nativa del árbol propio; flags y subsystem no la prueban. No abrir ventana visible de calibración. No se asegura que un payload malicioso no abra su propia GUI ni que privilegios permitan aislamiento total.

## Trazabilidad

Bindings de implementación previstos: `cmd/mi-lsp-host/main_windows.go`, `internal/winhost/host_windows.go`, `.goreleaser.yaml`, `cmd/mi-lsp-host/winres.json`. Ver [[TP-HOST-001]] para oráculos pendientes. No cambia UI del daemon ni agrega web UI, ACL, UAC o hooks de red.
