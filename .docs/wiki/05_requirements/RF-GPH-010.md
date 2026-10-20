---
wiki_contract: WIKI-CONTRACT-v1
id: RF-GPH-010
kind: requirement
status: draft
title: RF-GPH-010 - Ejecutar extensiones aisladas mediante MILX-v1 sin MCP
links:
  implements:
  - FL-GPH-03
  code:
  - path: internal/service/app.go
    role: behavior
---

## Propósito

RF-GPH-010 - Ejecutar extensiones aisladas mediante MILX-v1 sin MCP

## Comportamiento ordenado

### 1. Resultado requerido

Permitir algoritmos, visualizacion, backends e imports opcionales fuera del proceso core mediante `MILX-v1`. El host entrega una vista/pack read-only, valida capabilities, limita recursos y acepta solo resultados serializados derivativos. MILX no es MCP, no expone server HTTP y no otorga red implicita.

### 2. Transporte y framing

- Transporte v1: stdio de proceso hijo local. Named pipe/socket local puede agregarse en otra version con el mismo security model.
- Cada frame es `uint32` big-endian de longitud seguido por exactamente ese número de bytes JSON UTF-8 canonico.
- Max frame, max output total y max in-flight se fijan antes de spawn; oversize se rechaza sin parse parcial.
- Cada mensaje incluye `schema: milx-envelope/v1`, `request_id`, `operation`, protocol version y payload. Responses ecoan request ID y status.
- Operaciones core: `describe`, `prepare`, `execute`, `cancel`, `health`, `shutdown`.

No existe endpoint MCP, tool discovery MCP, SSE, HTTP ni red como fallback.

### 3. Manifest y capabilities

`MILXManifest` declara extension ID/version, executable digest, protocol range, operations, input/output schemas, capabilities, deterministic flag y resource hints. Capabilities v1 son allowlisted, por ejemplo:

- `graph.read.nodes`, `graph.read.edges`, `graph.read.evidence`;
- `documents.read.pack`;
- `analysis.emit`, `visual.emit`, `import.emit-advisory`.

No existen `graph.write`, `wiki.write`, `network`, `process.spawn` o `secrets.read` en v1. Capability desconocida o no autorizada bloquea antes de spawn/prepare.

### 4. Lifecycle

1. Core valida manifest, digest, protocol y capabilities.
2. Host crea sandbox/job object y budgets de wall time, CPU, RSS, frames y output.
3. `describe` confirma identidad/capabilities reales.
4. `prepare` recibe metadata de generation y un pack bounded o handle read-only controlado; nunca credenciales ni DB writable.
5. `execute` produce `MILXResult` derivativo con provenance, generation, parameters digest y omissions.
6. `cancel` es cooperativo hasta grace deadline; luego host termina solo el process tree de la extension.
7. `shutdown` y cleanup se ejecutan aun ante timeout/crash/malformed output.

### 5. Resultados y autoridad

Resultados se persisten opcionalmente como `GraphAnalysis` cacheado por `(generation, extension id/version/digest, operation, parameters digest)`. No crean/modifican nodes/edges core. Imports Graphify viven en `external:graphify`, se validan como advisory y nunca reemplazan NodeKey, evidence nativa o autoridad wiki.

### 6. Seguridad e invariantes

- Snapshot/pack read-only y mínimo privilegio.
- Entorno allowlisted sin secretos; cwd temporal; paths fuera de sandbox denegados.
- Network y MCP ausentes por contrato y verificados por scan/runtime tests.
- Timeout, crash o output inválido degradan solo la extension; active generation/core quedan disponibles.
- Logs/payloads arbitrarios no se incorporan a evidencia durable; solo summary/reason codes sanitizados.
- Direct mode puede invocar el host; daemon no es requisito.

## Superficies

CLI `mi-lsp`, con operaciones y selectores detallados en el comportamiento del requisito.

## Entradas/salidas tipadas

Las entradas, salidas, enums y límites forman parte del contrato de comportamiento ordenado de este documento; los campos conservan sus nombres de protocolo.

## Errores

### 7. Errores tipados

- **Código**: `GPH_MILX_PROTOCOL_UNSUPPORTED`; **Causa**: rango incompatible; **Respuesta**: reject antes de spawn
- **Código**: `GPH_MILX_MANIFEST_INVALID`; **Causa**: schema/digest/ID inválido; **Respuesta**: reject
- **Código**: `GPH_MILX_CAPABILITY_DENIED`; **Causa**: capability desconocida/prohibida; **Respuesta**: reject
- **Código**: `GPH_MILX_TIMEOUT`; **Causa**: wall/CPU deadline; **Respuesta**: cancel/kill tree + warning
- **Código**: `GPH_MILX_OUTPUT_INVALID`; **Causa**: frame/schema/size inválido; **Respuesta**: descartar resultado
- **Código**: `GPH_MILX_PROCESS_FAILED`; **Causa**: crash/nonzero; **Respuesta**: aislar y conservar core
- **Código**: `GPH_MILX_CLEANUP_FAILED`; **Causa**: recursos/proceso persisten; **Respuesta**: FAIL de seguridad
- **Código**: `GPH_MILX_NETWORK_FORBIDDEN`; **Causa**: intento de red/MCP; **Respuesta**: terminate + FAIL

## Criterios de aceptación (numerados)

1. Golden framing/handshake, versiones, schema y deterministic digest.
2. Denial de cada capability prohibida, malformed/oversize, timeout, crash, cancel y cleanup.
3. No-write del graph/wiki, no-network/no-MCP y process-tree containment por RID.
4. Resultado identical en 30 repeticiones cuando manifest declara deterministic.

### 8. Aceptacion y trazabilidad

- Golden framing/handshake, versiones, schema y deterministic digest.
- Denial de cada capability prohibida, malformed/oversize, timeout, crash, cancel y cleanup.
- No-write del graph/wiki, no-network/no-MCP y process-tree containment por RID.
- Resultado identical en 30 repeticiones cuando manifest declara deterministic.
- `TP-GPH / TP-GPH-006 / TC-GPH-043..048`.
