---
doc_id: TECH-MCP-ADAPTER
title: Puerta MCP nativa sobre la CLI (retirada)
layer: TECH
family: CLI
status: retired
implements: []
tests: []
---

# TECH-MCP-ADAPTER

> **Estado: RETIRADO (decisión del operador, 2026-10-15).** La puerta stdio `mi-lsp mcp` fue eliminada: se borraron `internal/mcp`, el comando `mcp` y las integraciones de puerta MCP de Pi, Cursor, Grok y `integrations/claude-code/.mcp.json`. Hoy `mi-mcp` ejecuta el CLI `mi-lsp` en cada llamada de herramienta, y `sandinas-mcp` pasa por `mi-mcp`. El CLI sigue siendo la única autoridad. Esta ficha conserva el id y el histórico; no describe comportamiento vigente.

```yaml
harness_protocol: SDD-HARNESS-v1
id: "TECH-MCP-ADAPTER"
kind: "support-doc"
audience: "dual"
imports:
  - '[[00_gobierno_documental]]'
  - '[[02_arquitectura]]'
  - '[[TECH-MCP-ADAPTER]]'
exports:
  - 'TECH-MCP-ADAPTER'
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/02_arquitectura.md
  - .docs/wiki/07_tech/TECH-MCP-ADAPTER.md
agent_may_edit:
  - .docs/wiki/07_tech/TECH-MCP-ADAPTER.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
evidence:
  - .docs/wiki/07_tech/TECH-MCP-ADAPTER.md
  - .docs/wiki/02_arquitectura.md
```

Volver a [02_arquitectura.md](../02_arquitectura.md) y a [07_baseline_tecnica.md](../07_baseline_tecnica.md).

## Decisión

Retirada el 2026-10-15. Reemplaza la decisión aceptada el 2026-09-24, que fijaba `mi-lsp mcp` como adaptador stdio fino y sin estado sobre la CLI. El bloque `architecture-mcp-door` de [02_arquitectura.md](../02_arquitectura.md) refleja el mismo estado.

```toon
doc_id: TECH-MCP-ADAPTER
block_id: architecture-mcp-door
status: retired
date: 2026-10-15
previous_status: accepted (2026-09-24)
authority: CLI
replaced_by: mi-mcp invoca el CLI mi-lsp en cada llamada de herramienta
door: mi-lsp mcp eliminada
exceptions: solo el contador PostToolUse de integrations/claude-code (no aplica a una puerta)
```

## Histórico (resumen)

- **Propósito original.** `mi-lsp mcp` exponía por JSON-RPC/NDJSON sobre stdio las operaciones `nav_*` de la CLI, sin índice, caché ni estado propio. La CLI era la autoridad y el protocolo `mi-lsp-v1.1` no cambiaba.
- **Presupuesto.** Objetivo de 20 MB de RSS por sesión. Medición del 2026-09-24: 14340096 bytes (13,676 MiB), con una sesión que respondió `initialize` y `tools/list` sin índice cargado. Evidencia: `.docs/raw/reportes/2026-09-24-native-plugin-surface.md`. Ese presupuesto deja de aplicar con la puerta retirada.
- **Motivo de retiro.** Decisión del operador del 2026-10-15: `mi-mcp` ejecutar el CLI por cada llamada supera a un proceso `mi-lsp mcp` persistente.
- **Reglas que no son de la puerta.** `MI_LSP_BIN`, `workspace which`, `nav suggest`, los `reason_code` de fallo terminal y el alcance de `--max-chars` pertenecen al CLI. Se mantienen en [07_baseline_tecnica.md](../07_baseline_tecnica.md), [CT-NAV-INTENT](../09_contratos/CT-NAV-INTENT.md), [CT-GRAPH-CLI](../09_contratos/CT-GRAPH-CLI.md) y [CT-CLI-AXI-MODE](../09_contratos/CT-CLI-AXI-MODE.md).

## Excepciones

El hook PostToolUse de Claude Code (`integrations/claude-code`) guarda un contador de sesión para avisar una sola vez al tercer Read, Grep o Glob seguido. No es caché de navegación ni índice, y no hay worker residente. Es un archivo de dos campos, `consecutiveRaw` y `advised`, bajo `MI_LSP_CLAUDE_STATE_DIR` o el temporal del sistema, un archivo por sesión. Medición del 2026-09-24: archivo de 35 bytes; RSS del proceso Node del hook de 56000512 bytes, que es el intérprete del hook y no un caché de mi-lsp. El proceso del hook termina con el evento.

## Consecuencias

- Las llamadas MCP pasan por `mi-mcp`, que invoca el CLI `mi-lsp` por llamada. No hay proceso residente de `mi-lsp` para MCP.
- La CLI sigue siendo usable sin `mi-mcp` ni ninguna puerta de host.
- Una nueva puerta, caché, índice o worker en `integrations/` exige una ficha nueva o reabrir esta con decisión del operador y medición previa.
