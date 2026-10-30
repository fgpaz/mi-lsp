---
wiki_contract: WIKI-CONTRACT-v1
id: RF-QRY-023
kind: requirement
status: active
title: RF-QRY-023 — Componer consultas q-v1 por CLI, daemon y proveedor mi-mcp con recetas versionadas
links:
  implements:
  - FL-QRY-01
  code:
  - path: internal/service/app.go
    role: behavior
---

## Propósito

RF-QRY-023 — Componer consultas q-v1 por CLI, daemon y proveedor mi-mcp con recetas versionadas

## Comportamiento ordenado

### Resultado requerido

- **Marcador estable**: RF-QRY-023.q-execution
- **Comprobación prevista**:
  - go test ./internal/query ./internal/service ./internal/grepx
  - mi-lsp nav wiki validate-source --workspace mi-lsp --ids RF-QRY-023 --format toon
- **Evidencia referenciada**:
  - .docs/wiki/05_requirements/RF-QRY-023.md
  - .docs/wiki/07_technical/contracts/CT-Q-V1.md
  - internal/query/parser_test.go
  - internal/service/primitives_q_test.go

Ofrecer una sola gramática de pipelines semánticas `q-v1` mediante CLI y daemon. La herramienta MCP Go `milsp` fue retirada el 2026-10-15 junto con la puerta MCP; el proveedor mi-mcp consume el mismo contrato por su manifiesto. La frontera del proveedor, envelope, errores, paginación y bytes es normativa en [[CT-Q-V1]]. El sandbox y `execute` pertenecen a mi-mcp; mi-lsp es proveedor y expone `milsp.q(...)` por su manifiesto de proveedor.

### Gramática y ejecución

`mi-lsp q "<pipeline>"` acepta entre una y ocho etapas separadas por `|`, argumentos citados, opciones, fuentes, transformaciones, salidas y meta `describe`. El mismo pipeline se ejecuta por daemon o fallback directo, bajo cancelación y timeout end-to-end. El resultado declara `contract_version:"q-v1"`, etapas, conteos, tiempos, revisión de snapshot y presupuesto; `max_bytes` mide el envelope JSON UTF-8 completo.

El cursor es opaco y autenticado, ligado a consulta canónica, workspace, generación del índice y posición; dura 10 minutos. Consulta/scope incorrectos se rechazan; los códigos para inválido, vencido o snapshot cambiado son los fijados por CT-Q-V1. El tamaño total nunca supera `max_bytes`; se recortan ítems completos y un único texto sobredimensionado lleva `truncated_item:true` o se omite con `max_bytes_item_exceeded`.

### Superficie MCP (retirada 2026-10-15)

[RETIRADO] `tools/list` antepone `milsp` al catálogo existente de trece `nav_*`. La descripción incluye gramática y ejemplos; el schema publica `q`, `contract_version`, `workspace`, `budget`, `max_bytes`, `timeout_ms`, `session_id`, `page`, `fresh` y `dedupe`. La tool devuelve el envelope completo del contrato. Los trece `nav_*` mantienen compatibilidad mientras los consumidores migran; no se edita mi-mcp desde esta entrega.

### Recetas

Las recetas iniciales son pipelines versionadas `@who-calls`, `@trace`, `@find-def`, `@explain`, `@impact` y `@explain-change`. También hay recetas de compatibilidad `@nav-intent`, `@nav-route`, `@nav-pack`, `@nav-wiki`, `@nav-search`, `@nav-find`, `@nav-refs`, `@nav-related`, `@nav-flow-slice`, `@nav-change-pack`, `@nav-affected`, `@nav-multi-read` y `@nav-overview`. El expansionado conserva `contract_version`; una receta desconocida o argumentos faltantes fallan con `stage_failed` tipado.

Los ejemplos de uso se limitan a las tres recetas canónicas de [[CT-Q-V1]]. Los errores de parse identifican carácter, expectativa y ejemplo correcto. La resolución de workspace es de solo lectura: propone el alias que coincide con cwd/ruta registrada, indica `workspace add ... --no-index` si falta registro e `index --workspace <alias>` si falta índice; nunca registra ni reindexa en silencio.

### Descubrimiento del proveedor mi-mcp

`mi-lsp provider-manifest --format json` publica por stdout el manifiesto `mi-mcp-provider/v2` del proveedor `milsp`, incluyendo binary, descripción, tags, contratos y schemas de entrada/salida. Declara las operaciones de lectura `q` (`q-v1`), `intent` (CLI `nav intent`) y `multi-read` (CLI `nav multi-read`) consumidas por los proveedores mi-mcp, con versión propia derivada de `mi-lsp --version` y mínima `0.10.0`. El comando no inicia daemon ni listener, registra workspaces, escribe archivos ni usa red. El mapping CLI fija `--no-auto-register` y describe explícitamente subcomandos, argumentos y flags. El socket Unix opcional `$XDG_RUNTIME_DIR/mi-lsp.sock` usa modo `0600`, JSONL y límite de 1048576 bytes; sirve las mismas operaciones y se mantiene el spawn CLI como alternativa ante falla de conexión, nunca tras una respuesta válida. También se publican `version`, `schema`, `--help` y `--version` de lectura.

El manifiesto versionado en `integrations/mi-mcp/provider-manifest.json` refleja la salida del comando salvo `provider_version`. La verificación compara el contrato emitido, operaciones requeridas y manifest versionado.

## Superficies

CLI `mi-lsp`, con operaciones y selectores detallados en el comportamiento del requisito.

## Entradas/salidas tipadas

Las entradas, salidas, enums y límites forman parte del contrato de comportamiento ordenado de este documento; los campos conservan sus nombres de protocolo.

## Errores

Se aplican las condiciones de rechazo y los warnings descritos en el comportamiento ordenado y sus variantes. No se añade ningún fallback silencioso.

## Criterios de aceptación (numerados)

1. Gramática quoted, opciones, pipelines de máximo ocho etapas, errores de sintaxis con posición de carácter, expectativa y ejemplo canónico, y `describe` hasta 600 tokens.
2. Las tres recetas canónicas de [[CT-Q-V1]] pasan por el parser en pruebas Go; q no crea workspaces ni reindexa al resolver o consultar.
3. CLI agrega exactamente el subcomando `q`; daemon atiende la operación, registra límites y usa el mismo executor que fallback directo. `serve --socket` expone los mismos handlers CLI por JSONL Unix opcional, con ruta absoluta bajo `XDG_RUNTIME_DIR`, permisos `0600` y cierre limpio; una ruta socket existente se conserva y bloquea un segundo listener; el daemon interno existente permanece separado.
4. [RETIRADO] `milsp` es la primera herramienta MCP, su schema valida opciones q-v1 y las trece herramientas previas siguen listadas.
5. Cursor completo: firma inválida, expiración >10 minutos y cambio de generation se distinguen; orden estable `(path,line,id)`.
6. Bytes medidos sobre el envelope serializado; ítems completos; clipping UTF-8 y marcador/razón tipados.
7. Recetas iniciales y los trece alias se resuelven de forma determinista; cada pipeline fuente está versionada en `internal/query/recipes/*.q`.

### Aceptación

- Gramática quoted, opciones, pipelines de máximo ocho etapas, errores de sintaxis con posición de carácter, expectativa y ejemplo canónico, y `describe` hasta 600 tokens.
- Las tres recetas canónicas de [[CT-Q-V1]] pasan por el parser en pruebas Go; q no crea workspaces ni reindexa al resolver o consultar.
- CLI agrega exactamente el subcomando `q`; daemon atiende la operación, registra límites y usa el mismo executor que fallback directo. `serve --socket` expone los mismos handlers CLI por JSONL Unix opcional, con ruta absoluta bajo `XDG_RUNTIME_DIR`, permisos `0600` y cierre limpio; una ruta socket existente se conserva y bloquea un segundo listener; el daemon interno existente permanece separado.
- [RETIRADO] `milsp` es la primera herramienta MCP, su schema valida opciones q-v1 y las trece herramientas previas siguen listadas.
- Cursor completo: firma inválida, expiración >10 minutos y cambio de generation se distinguen; orden estable `(path,line,id)`.
- Bytes medidos sobre el envelope serializado; ítems completos; clipping UTF-8 y marcador/razón tipados.
- Recetas iniciales y los trece alias se resuelven de forma determinista; cada pipeline fuente está versionada en `internal/query/recipes/*.q`.
- Casos: TP-QRY-197 a TP-QRY-200 y TC-QRY-204.

```yaml
harness_protocol: SDD-HARNESS-v1
id: RF-QRY-023
kind: requirement
audience: llm-first
imports:
- .docs/wiki/04_flows/FL-QRY-01.md
exports:
- RF-QRY-023
- RF-QRY-023 — Componer consultas q-v1 por CLI, daemon y proveedor mi-mcp con recetas
  versionadas
agent_must_read:
- .docs/wiki/05_requirements/RF-QRY-023.md
- .docs/wiki/04_flows/FL-QRY-01.md
- .docs/wiki/06_tests/TP-QRY.md
- .docs/wiki/06_tests/TP-QRY-PREPARATION.md
agent_may_edit:
- .docs/wiki/05_requirements/RF-QRY-023.md
agent_must_not_edit:
- .docs/wiki/00_governance.md
- AGENTS.md
- internal/service/app.go
verify:
- 'Para los casos de este requisito en .docs/wiki/06_tests/TP-QRY.md, ejecutar el
  comando publicado: go test ./internal/model ./internal/service ./internal/cli ./internal/indexer
  (la evidencia de ejecución debe registrarse por separado).'
stop_if:
- Detenerse si la propuesta requiere alterar criterios, decisiones o comportamiento
  más allá de RF-QRY-023.
evidence:
- .docs/wiki/05_requirements/RF-QRY-023.md
- .docs/wiki/06_tests/TP-QRY.md
- .docs/wiki/06_tests/TP-QRY-PREPARATION.md
```
