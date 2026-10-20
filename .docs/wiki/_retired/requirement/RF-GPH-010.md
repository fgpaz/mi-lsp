---
id: RF-GPH-010
title: Ejecutar extensiones aisladas mediante MILX-v1 sin MCP
status: planned
flows:
  - FL-GPH-03
tests:
  - .docs/wiki/06_pruebas/TP-GPH.md
---

```yaml
harness_protocol: SDD-HARNESS-v1
id: "RF-GPH-010"
kind: "support-doc"
audience: "llm-first"
imports:
  - '[[00_gobierno_documental]]'
  - '[[FL-GPH-03]]'
  - '[[RF-GPH-002]]'
  - '[[RF-GPH-005]]'
  - '[[RF-GPH-009]]'
exports:
  - 'RF-GPH-010'
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/03_FL/FL-GPH-03.md
  - .docs/wiki/04_RF/RF-GPH-002.md
  - .docs/wiki/04_RF/RF-GPH-005.md
  - .docs/wiki/04_RF/RF-GPH-009.md
  - .docs/wiki/04_RF/RF-GPH-010.md
agent_may_edit:
  - .docs/wiki/04_RF/RF-GPH-010.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
evidence:
  - .docs/wiki/04_RF/RF-GPH-010.md
```

# RF-GPH-010 - Ejecutar extensiones aisladas mediante MILX-v1 sin MCP

## 1. Resultado requerido

Permitir algoritmos, visualizacion, backends e imports opcionales fuera del proceso core mediante `MILX-v1`. El host entrega una vista/pack read-only, valida capabilities, limita recursos y acepta solo resultados serializados derivativos. MILX no es MCP, no expone server HTTP y no otorga red implicita.

## 2. Transporte y framing

- Transporte v1: stdio de proceso hijo local. Named pipe/socket local puede agregarse en otra version con el mismo security model.
- Cada frame es `uint32` big-endian de longitud seguido por exactamente ese numero de bytes JSON UTF-8 canonico.
- Max frame, max output total y max in-flight se fijan antes de spawn; oversize se rechaza sin parse parcial.
- Cada mensaje incluye `schema: milx-envelope/v1`, `request_id`, `operation`, protocol version y payload. Responses ecoan request ID y status.
- Operaciones core: `describe`, `prepare`, `execute`, `cancel`, `health`, `shutdown`.

No existe endpoint MCP, tool discovery MCP, SSE, HTTP ni red como fallback.

## 3. Manifest y capabilities

`MILXManifest` declara extension ID/version, executable digest, protocol range, operations, input/output schemas, capabilities, deterministic flag y resource hints. Capabilities v1 son allowlisted, por ejemplo:

- `graph.read.nodes`, `graph.read.edges`, `graph.read.evidence`;
- `documents.read.pack`;
- `analysis.emit`, `visual.emit`, `import.emit-advisory`.

No existen `graph.write`, `wiki.write`, `network`, `process.spawn` o `secrets.read` en v1. Capability desconocida o no autorizada bloquea antes de spawn/prepare.

## 4. Lifecycle

1. Core valida manifest, digest, protocol y capabilities.
2. Host crea sandbox/job object y budgets de wall time, CPU, RSS, frames y output.
3. `describe` confirma identidad/capabilities reales.
4. `prepare` recibe metadata de generation y un pack bounded o handle read-only controlado; nunca credenciales ni DB writable.
5. `execute` produce `MILXResult` derivativo con provenance, generation, parameters digest y omissions.
6. `cancel` es cooperativo hasta grace deadline; luego host termina solo el process tree de la extension.
7. `shutdown` y cleanup se ejecutan aun ante timeout/crash/malformed output.

## 5. Resultados y autoridad

Resultados se persisten opcionalmente como `GraphAnalysis` cacheado por `(generation, extension id/version/digest, operation, parameters digest)`. No crean/modifican nodes/edges core. Imports Graphify viven en `external:graphify`, se validan como advisory y nunca reemplazan NodeKey, evidence nativa o autoridad wiki.

## 6. Seguridad e invariantes

- Snapshot/pack read-only y minimo privilegio.
- Entorno allowlisted sin secretos; cwd temporal; paths fuera de sandbox denegados.
- Network y MCP ausentes por contrato y verificados por scan/runtime tests.
- Timeout, crash o output invalido degradan solo la extension; active generation/core quedan disponibles.
- Logs/payloads arbitrarios no se incorporan a evidencia durable; solo summary/reason codes sanitizados.
- Direct mode puede invocar el host; daemon no es requisito.

## 7. Errores tipados

| Codigo | Causa | Respuesta |
|---|---|---|
| `GPH_MILX_PROTOCOL_UNSUPPORTED` | rango incompatible | reject antes de spawn |
| `GPH_MILX_MANIFEST_INVALID` | schema/digest/ID invalido | reject |
| `GPH_MILX_CAPABILITY_DENIED` | capability desconocida/prohibida | reject |
| `GPH_MILX_TIMEOUT` | wall/CPU deadline | cancel/kill tree + warning |
| `GPH_MILX_OUTPUT_INVALID` | frame/schema/size invalido | descartar resultado |
| `GPH_MILX_PROCESS_FAILED` | crash/nonzero | aislar y conservar core |
| `GPH_MILX_CLEANUP_FAILED` | recursos/proceso persisten | FAIL de seguridad |
| `GPH_MILX_NETWORK_FORBIDDEN` | intento de red/MCP | terminate + FAIL |

## 8. Aceptacion y trazabilidad

- Golden framing/handshake, versiones, schema y deterministic digest.
- Denial de cada capability prohibida, malformed/oversize, timeout, crash, cancel y cleanup.
- No-write del graph/wiki, no-network/no-MCP y process-tree containment por RID.
- Resultado identical en 30 repeticiones cuando manifest declara deterministic.
- `TP-GPH / TP-GPH-006 / TC-GPH-043..048`.
