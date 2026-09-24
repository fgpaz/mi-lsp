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

## Latencia de referencia (no comparable con un baseline previo)

En la misma sesión de evidencia de esta puerta se corrió `nav overview --workspace mi-lsp --format toon` contra un daemon ya caliente (pid vivo, no reiniciado), tres calentamientos y veinte llamadas cronometradas, todas exit 0. Metodo nearest-rank `ceil(p*n)` sobre esas veinte muestras. p50 232.484 ms; p95 373.564 ms; minimo 199.928 ms; maximo 385.836 ms. Evidencia: `.docs/raw/reportes/2026-09-24-native-plugin-surface.md`.

Este numero no es una linea base nueva del backend `catalog`/`daemon` de `nav overview`, ni de la puerta MCP: no hubo `tools/call` en la sesion MCP, y la llamada cronometrada fue la CLI directa contra el daemon, no `mi-lsp mcp`. Tampoco es comparable de forma estricta contra una corrida anterior con el mismo comando que reporto p50 4677.136 ms / p95 6102.285 ms: el binario cliente cambio, el daemon es el mismo proceso sin reiniciar, y esa corrida anterior no archivo el tamano del payload. Se deja el numero como evidencia puntual del working set de la sesion, no como SLA ni como regla de aceptacion.

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
mi-lsp nav suggest --format json --tool Read|Grep|Glob --args <json>
```

La salida útil es el envelope JSON. El comando está en `items[0].argv`, separado de `reason`. Un prompt de usuario no es una herramienta Read/Grep/Glob: la llamada sin `--tool` responde vacía y el hook puede mostrar una sola línea estática `mi-lsp nav intent "<goal>"`. Cualquier otro texto se ignora. El comando y el motivo no se concatenan en un string de shell.

## Integraciones del repositorio

Las integraciones bajo [`integrations/`](../../../integrations/) (Claude Code, Pi, Grok, Codex y Cursor) obedecen esta misma decisión. Cada puerta de host es opcional: configura o invoca `mi-lsp mcp` y, cuando hace falta un equivalente de una lectura cruda, `mi-lsp nav suggest`. Si ninguna puerta está instalada, la CLI sigue siendo la autoridad y el camino usable. Fallan abiertas. No incorporan caché, índice, worker ni estado propio.

Un prefetch, una caché de respuestas o un runtime de hooks ajeno no forman parte de esta decisión. No se porta a este repo un router externo ni un cliente de modelo. La única excepción aceptada está abajo.

## Excepciones

El hook PostToolUse de Claude Code guarda un contador de sesión. Hace falta para avisar una sola vez al tercer Read, Grep o Glob seguido, y para no repetir el aviso hasta que una llamada mi-lsp reinicie la racha. Sin ese recuerdo el aviso no distingue la tercera lectura de la primera. No es caché de navegación, no es índice y no hay worker residente.

El archivo es un JSON de dos campos, `consecutiveRaw` y `advised`, bajo `MI_LSP_CLAUDE_STATE_DIR` o el temporal del sistema, un archivo por sesión. El costo medido queda en la tabla. El proceso del hook termina con el evento.

| Excepción | Qué se midió | Costo | Fecha |
|---|---|---|---|
| Contador PostToolUse | archivo tras tres eventos Read en un directorio temporal, y RSS del proceso Node que ejecutó ese hook | archivo 35 bytes (`consecutiveRaw` y `advised`). RSS del proceso Node: 56000512 bytes. El contador no abre índice ni worker; el RSS es el intérprete del hook, no un caché de mi-lsp | 2026-09-24 |

## Consecuencias

- La CLI se mantiene usable aunque MCP no arranque.
- Las descripciones de las herramientas dicen cuándo convienen frente a una lectura cruda, sin duplicar la implementación de `nav`.
- Cualquier futura caché, índice, worker o estado en la puerta o en `integrations/` actualiza primero esta ficha con la medición.
