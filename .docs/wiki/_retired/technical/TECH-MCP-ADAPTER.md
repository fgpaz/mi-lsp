---
wiki_contract: WIKI-CONTRACT-v1
id: TECH-MCP-ADAPTER
kind: technical
status: retired
title: TECH-MCP-ADAPTER
---

## Resumen

TECH-MCP-ADAPTER

## Mapa de componentes

Los componentes, paths exactos, responsabilidades y límites se mantienen en el detalle técnico siguiente.

## Detalle conservado

### TECH-MCP-ADAPTER

> **Estado: RETIRADO (decisión del operador, 2026-10-15).** La puerta stdio `mi-lsp mcp` fue eliminada: se borraron `internal/mcp`, el comando `mcp` y las integraciones de puerta MCP de Pi, Cursor, Grok y `integrations/claude-code/.mcp.json`. Hoy `mi-mcp` ejecuta el CLI `mi-lsp` en cada llamada de herramienta, y `sandinas-mcp` pasa por `mi-mcp`. El CLI sigue siendo la única autoridad. Esta ficha conserva el id y el histórico; no describe comportamiento vigente.



Volver a [03_architecture.md](../../03_architecture.md) y a [07_technical.md](../../07_technical.md).

### Decisión

Retirada el 2026-10-15. Reemplaza la decisión aceptada el 2026-09-24, que fijaba `mi-lsp mcp` como adaptador stdio fino y sin estado sobre la CLI. El bloque `architecture-mcp-door` de [03_architecture.md](../../03_architecture.md) refleja el mismo estado.

- **Documento propietario**: TECH-MCP-ADAPTER
- **Marcador estable**: architecture-mcp-door
- **Estado del comportamiento**: retired
- **date**: 2026-10-15
- **previous_status**: accepted (2026-09-24)
- **authority**: CLI
- **replaced_by**: mi-mcp invoca el CLI mi-lsp en cada llamada de herramienta
- **door**: mi-lsp mcp eliminada
- **exceptions**: solo el contador PostToolUse de integrations/claude-code (no aplica a una puerta)

### Histórico (resumen)

- **Propósito original.** `mi-lsp mcp` exponía por JSON-RPC/NDJSON sobre stdio las operaciones `nav_*` de la CLI, sin índice, caché ni estado propio. La CLI era la autoridad y el protocolo `mi-lsp-v1.1` no cambiaba.
- **Presupuesto.** Objetivo de 20 MB de RSS por sesión. Medición del 2026-09-24: 14340096 bytes (13,676 MiB), con una sesión que respondió `initialize` y `tools/list` sin índice cargado. Evidencia: `.docs/raw/reportes/2026-09-24-native-plugin-surface.md`. Ese presupuesto deja de aplicar con la puerta retirada.
- **Motivo de retiro.** Decisión del operador del 2026-10-15: `mi-mcp` ejecutar el CLI por cada llamada supera a un proceso `mi-lsp mcp` persistente.
- **Reglas que no son de la puerta.** `MI_LSP_BIN`, `workspace which`, `nav suggest`, los `reason_code` de fallo terminal y el alcance de `--max-chars` pertenecen al CLI. Se mantienen en [07_technical.md](../../07_technical.md), [CT-NAV-INTENT](../../07_technical/contracts/CT-NAV-INTENT.md), [CT-GRAPH-CLI](../../07_technical/contracts/CT-GRAPH-CLI.md) y [CT-CLI-AXI-MODE](../../07_technical/contracts/CT-CLI-AXI-MODE.md).

### Excepciones

El hook PostToolUse de Claude Code (`integrations/claude-code`) guarda un contador de sesión para avisar una sola vez al tercer Read, Grep o Glob seguido. No es caché de navegación ni índice, y no hay worker residente. Es un archivo de dos campos, `consecutiveRaw` y `advised`, bajo `MI_LSP_CLAUDE_STATE_DIR` o el temporal del sistema, un archivo por sesión. Medición del 2026-09-24: archivo de 35 bytes; RSS del proceso Node del hook de 56000512 bytes, que es el intérprete del hook y no un caché de mi-lsp. El proceso del hook termina con el evento.

### Consecuencias

- Las llamadas MCP pasan por `mi-mcp`, que invoca el CLI `mi-lsp` por llamada. No hay proceso residente de `mi-lsp` para MCP.
- La CLI sigue siendo usable sin `mi-mcp` ni ninguna puerta de host.
- Una nueva puerta, caché, índice o worker en `integrations/` exige una ficha nueva o reabrir esta con decisión del operador y medición previa.
