---
wiki_contract: WIKI-CONTRACT-v1
id: FL-HOST-001
kind: flow
status: draft
title: Ejecución y cancelación por host Windows
links:
  fulfills: [RS-MILSP-RUNTIME]
---

## Objetivo

Ejecutar y cancelar por host Windows preservando los streams y separando el transporte del resultado de ejecución.

## Alcance

El flujo comprende selección del host, control privado, ejecución del árbol, drenaje y validación del resultado; la admisión del consumidor se decide por separado.

## Actores

Consumidor o caller, host GUI, job privado y root con sus descendientes.

## Pre/postcondiciones

El consumidor espera el cierre del host y de los streams y valida el resultado; transporte ausente, incompleto o no cero falla cerrado.

## Precondiciones

El consumidor selecciona un host de la release oficial y arquitectura correctas con tag público, SHA, fuente y readback verificados; sin esa evidencia permanece BLOCKED. La admisión del consumidor se decide por separado. Crea un archivo de control regular, vacío, privado y estable, en directorio propio; el sibling `.result` no existe. El caller mantiene exclusividad y seguridad del directorio sin cambios ACL por el host.

## Secuencia

```mermaid
sequenceDiagram
    participant C as Consumidor
    participant H as mi-lsp-host GUI
    participant J as Job privado
    participant P as Root y descendientes
    C->>H: argv absoluto, control privado y 3 streams
    H->>H: crea result exclusivo
    H->>J: crear y configurar KILL_ON_JOB_CLOSE
    H->>P: CreateProcess suspendido y CREATE_NO_WINDOW
    H->>J: asignar root por handle
    H->>P: reanudar tras asignación
    C->>P: stdin sin framing
    P-->>C: stdout y stderr sin framing
    alt INT o TERM, timeout o error
        C->>H: token en control (cuando corresponde)
        H->>J: terminar y drenar por handle
    else salida normal
        P-->>H: root exit DWORD
        H->>J: esperar activeProcesses=0
    end
    H->>H: cerrar handles y escribir header + JSON, cerrar result
    H-->>C: status de transporte
    C->>C: esperar cierre host y streams; validar registros y propagar resultado tipado
```

Un prefijo de cancelación incompleto queda pendiente. Si job ya está vacío, la observación de vaciado linealiza la finalización; no se transforma root exit en cancelación por un token tardío. Timeout y errores son infraestructura y nunca éxito implícito. Si el host termina sin resultado íntegro o con status no cero, el consumidor falla cerrado. Véase el framing y los límites de [[TECH-WINDOWS-HOST]], requisitos [[RF-HOST-001]] y oráculos [[TP-HOST-001]].

## Restricciones ADR

No se identifica una restricción ADR específica en este flujo; la admisión del consumidor y su evidencia se consideran por separado.

```yaml
harness_protocol: SDD-HARNESS-v1
id: FL-HOST-001
kind: flow
audience: llm-first
imports:
  - .docs/wiki/00_governance.md
  - .docs/wiki/07_technical/TECH-WINDOWS-HOST.md
  - .docs/wiki/05_requirements/RF-HOST-001.md
  - .docs/wiki/06_tests/TP-HOST-001.md
exports:
  - FL-HOST-001
agent_must_read:
  - .docs/wiki/04_flows/FL-HOST-001.md
agent_may_edit:
  - .docs/wiki/04_flows/FL-HOST-001.md
agent_must_not_edit:
  - .docs/wiki/07_technical/TECH-WINDOWS-HOST.md
  - .docs/wiki/05_requirements/RF-HOST-001.md
  - .docs/wiki/06_tests/TP-HOST-001.md
verify:
  - .docs/wiki/06_tests/TP-HOST-001.md#alcance-y-evidencia
stop_if:
  - harness_verdict=BLOCKED
  - host_oracle_or_acceptance_would_change
  - incomplete_result_is_treated_as_success
evidence:
  - .docs/wiki/04_flows/FL-HOST-001.md
  - .docs/wiki/06_tests/TP-HOST-001.md
  - .docs/wiki/05_requirements/RF-HOST-001.md
```
