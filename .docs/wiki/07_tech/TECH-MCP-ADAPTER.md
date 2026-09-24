---
doc_id: TECH-MCP-ADAPTER
title: Puerta MCP nativa sobre la CLI
layer: TECH
family: CLI
status: accepted
implements:
  - internal/cli/mcp.go
  - integrations/
tests:
  - internal/cli/mcp_test.go
---

# TECH-MCP-ADAPTER

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

Aceptada el 2026-09-24. Esta ficha es la decisión de arquitectura de `mi-lsp mcp`. El bloque `architecture-mcp-door` de [02_arquitectura.md](../02_arquitectura.md) la resume.

```toon
doc_id: TECH-MCP-ADAPTER
block_id: architecture-mcp-door
status: accepted
date: 2026-09-24
authority: CLI
budget_rss: 20MB
exceptions: none
```

## 1. Propósito

`mi-lsp mcp` solo expone capacidades que la CLI ya tiene, como herramientas MCP bien descritas para que un agente las consuma. Es un adaptador fino y sin estado sobre los mismos caminos de código. La CLI sigue siendo la autoridad. Si la puerta MCP falla, el agente ejecuta `mi-lsp` en una terminal y obtiene el mismo resultado.

La puerta no inventa una segunda semántica, un segundo índice ni un protocolo paralelo de navegación. El protocolo sigue siendo `mi-lsp-v1.1`. Cada herramienta describe un comando existente (`nav intent`, `nav route`, `nav pack`, `nav wiki`, `nav search`, `nav find`, `nav refs`, `nav related`, `nav flow-slice`, `nav change-pack`, `nav affected`, `nav multi-read`, `nav overview`) y delega en ese comando. `workspace which` y `nav suggest` también son comandos de la CLI; la puerta no les da otro significado.

## 2. Memoria y eficiencia

No se agrega lógica que infle el uso de memoria. Quedan prohibidos, en el proceso de la puerta y en las integraciones del repositorio:

- cachés de respuestas, de workspaces o de herramientas;
- índices propios;
- workers en segundo plano y pools residentes;
- estado de sesión propio.

El trabajo pesado permanece en el daemon compartido. Cuando el daemon no está activo, la ejecución directa es la que ya usa la CLI, no una copia de esa maquinaria dentro de la puerta.

Una excepción solo vale si mejora la usabilidad de forma clara y queda escrita en la sección «Excepciones» con su costo medido. Sin esa medición, la excepción no está aceptada.

El proceso `mi-lsp mcp` no retiene resultados ni abre el índice. Un proceso hijo que nace para una llamada y termina con ella es transporte hacia la CLI, no un worker residente, y no se reutiliza como runtime privado.

## 3. Presupuesto

El objetivo es 20 MB de RSS o menos por sesión de `mi-lsp mcp`. La medición se registra en esta ficha. Un valor por encima del objetivo impide dar por cerrada la puerta hasta corregirlo o hasta aceptar una excepción con el costo medido.

| Superficie | Objetivo RSS | RSS medido | Fecha | Evidencia |
|---|---|---|---|---|
| `mi-lsp mcp` | <= 20 MB por sesión | 14340096 bytes (13,676 MiB) | 2026-09-24 | `.docs/raw/reportes/2026-09-24-native-plugin-surface.md` |

La celda de RSS medido se completa con el working set de una sesión que ya respondió `initialize` y `tools/list`, sin cargar un índice dentro del proceso MCP.

## Presupuesto global `--max-chars`

`--max-chars` es un flag global de la CLI, no un presupuesto privado de la puerta. El default `0` significa que no hay tope explícito: si `--token-budget` es mayor que cero, el truncador puede derivar un tope de `token_budget * 4`. Un valor explícito mayor que cero es el tope de caracteres y gana sobre los defaults AXI, igual que `--format`, `--max-items` y `--token-budget`.

Al recortar, la respuesta deja `truncated=true`, un marcador de truncación y `continuation.next` cuando esa continuación existía. El recorte no es un `reason_code` de fallback ni autorización para salir de la CLI.

## Fallos terminales

Un fallo terminal externo publica exactamente un `error.reason_code` y, en un campo distinto, `error.detail`. El detalle es canónico, acotado y sanitizado: no es el input crudo, no se concatena al código y no se sustituye por un mensaje libre. La lista ya está cerrada en [[CT-NAV-INTENT]] y [[CT-GRAPH-CLI]]. Esta ficha no abre otra familia.

| `reason_code` | `detail` |
|---|---|
| `unsupported_operation` | `the requested operation is not supported` |
| `unavailable_binary` | `the required backend binary is unavailable` |
| `invalid_workspace` | `the requested workspace is invalid` |
| `explicit_incomplete` | `the result is explicitly incomplete` |

Timeout, silencio, `DONE` o `PASS` sin diagnóstico fresco no son un quinto código. Una degradación interna etiquetada por el runtime sigue siendo omisión, no fallback externo.

## Binario, `workspace which` y `nav suggest`

`MI_LSP_BIN`, si está definido, es el override y debe apuntar al ejecutable real: `mi-lsp.exe` en Windows y `mi-lsp` en el resto. Un shim `.cmd` o `.bat` no es el producto ni un override válido. Si el valor falta como archivo, no es el ejecutable real o apunta a un shim, el fallo visible es `unavailable_binary` con el `detail` de esa fila. No se reemplaza en silencio por otro candidato de `PATH`.

`workspace which` es de solo lectura. Publica el workspace resuelto con la misma precedencia que el resto de la CLI (`--workspace` explícito, luego el root registrado que contiene el cwd, luego `last_workspace`) y la ruta del ejecutable en uso. No muta el registry. El diagnóstico amplio sigue siendo `workspace doctor`.

`nav suggest` devuelve una sugerencia acotada de un comando `nav` que la CLI ya expone. No reemplaza a `nav intent`, no abre un router externo y no es fallback. Las puertas que lo invocan usan argv, no un shell:

```text
mi-lsp nav suggest --event user_prompt [--prompt TEXT]
mi-lsp nav suggest --event post_tool --tool NAME --consecutive N
```

La salida útil es una sola línea `mi-lsp nav …`, o JSON con `command` separado de `hint` o `suggested_command`. Cualquier otro texto se ignora. El comando y el motivo no se concatenan en un string de shell.

## Integraciones del repositorio

Las integraciones bajo [`integrations/`](../../../integrations/) (Claude Code, Pi, Grok, Codex y Cursor) obedecen esta misma decisión. Cada puerta de host es opcional: configura o invoca `mi-lsp mcp` y, cuando hace falta un equivalente de una lectura cruda, `mi-lsp nav suggest`. Si ninguna puerta está instalada, la CLI sigue siendo la autoridad y el camino usable. Fallan abiertas. No incorporan caché, índice, worker ni estado propio.

Un contador de bucle, un prefetch o un archivo de sesión son estado propio. No forman parte de esta decisión mientras no aparezcan en «Excepciones» con el RSS medido. No se porta a este repo un router externo, un cliente de modelo ni un runtime de hooks ajeno: eso no es la puerta.

## Excepciones

Ninguna.

## Consecuencias

- La CLI se mantiene usable aunque MCP no arranque.
- Las descripciones de las herramientas dicen cuándo convienen frente a una lectura cruda, sin duplicar la implementación de `nav`.
- Cualquier futura caché, índice, worker o estado en la puerta o en `integrations/` actualiza primero esta ficha con la medición.
