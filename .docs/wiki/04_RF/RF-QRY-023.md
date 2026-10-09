---
doc_id: RF-QRY-023
title: Componer consultas q-v1 por CLI, daemon y MCP Go con recetas versionadas
layer: RF
status: implemented
source_schema: SDD-WIKI-SOURCE-v1
wiki_source_protocol: SDD-WIKI-SOURCE-v1
source_kind: canonical-requirement
normative_format: toon
harness_protocol: SDD-HARNESS-v1
id: RF-QRY-023
kind: requirement
audience: llm-first
imports:
  - '[[00_gobierno_documental]]'
  - '[[FL-QRY-01]]'
  - '[[CT-Q-V1]]'
  - '[[RF-QRY-021]]'
  - '[[TP-QRY]]'
exports:
  - RF-QRY-023
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/03_FL/FL-QRY-01.md
  - .docs/wiki/09_contratos/CT-Q-V1.md
  - .docs/wiki/04_RF/RF-QRY-023.md
agent_may_edit:
  - .docs/wiki/04_RF/RF-QRY-023.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - go test ./internal/query ./internal/cli ./internal/mcp ./internal/daemon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-source --workspace mi-lsp --ids RF-QRY-023 --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
evidence:
  - .docs/wiki/09_contratos/CT-Q-V1.md
  - .docs/wiki/04_RF/RF-QRY-023.md
  - .docs/wiki/06_pruebas/TP-QRY.md
---

# RF-QRY-023 — Componer consultas q-v1 por CLI, daemon y MCP Go con recetas versionadas

## Resultado requerido

```toon
doc_id: RF-QRY-023
block_id: RF-QRY-023.q-execution
kind: normative
source_of_truth: this
verify:
  - go test ./internal/query ./internal/service ./internal/grepx
  - mi-lsp nav wiki validate-source --workspace mi-lsp --ids RF-QRY-023 --format toon
evidence:
  - .docs/wiki/04_RF/RF-QRY-023.md
  - .docs/wiki/09_contratos/CT-Q-V1.md
  - internal/query/parser_test.go
  - internal/service/primitives_q_test.go
```

Ofrecer una sola gramática de pipelines semánticas `q-v1` mediante CLI, daemon y herramienta MCP Go `milsp`. La frontera del proveedor, envelope, errores, paginación y bytes es normativa en [[CT-Q-V1]]. El sandbox y `execute` pertenecen a mi-mcp; mi-lsp es proveedor y expone `milsp.q(...)` por su manifiesto de proveedor.

## Gramática y ejecución

`mi-lsp q "<pipeline>"` acepta entre una y ocho etapas separadas por `|`, argumentos citados, opciones, fuentes, transformaciones, salidas y meta `describe`. El mismo pipeline se ejecuta por daemon o fallback directo, bajo cancelación y timeout end-to-end. El resultado declara `contract_version:"q-v1"`, etapas, conteos, tiempos, revisión de snapshot y presupuesto; `max_bytes` mide el envelope JSON UTF-8 completo.

El cursor es opaco y autenticado, ligado a consulta canónica, workspace, generación del índice y posición; dura 10 minutos. Consulta/scope incorrectos se rechazan; los códigos para inválido, vencido o snapshot cambiado son los fijados por CT-Q-V1. El tamaño total nunca supera `max_bytes`; se recortan ítems completos y un único texto sobredimensionado lleva `truncated_item:true` o se omite con `max_bytes_item_exceeded`.

## Superficie MCP

`tools/list` antepone `milsp` al catálogo existente de trece `nav_*`. La descripción incluye gramática y ejemplos; el schema publica `q`, `contract_version`, `workspace`, `budget`, `max_bytes`, `timeout_ms`, `session_id`, `page`, `fresh` y `dedupe`. La tool devuelve el envelope completo del contrato. Los trece `nav_*` mantienen compatibilidad mientras los consumidores migran; no se edita mi-mcp desde esta entrega.

## Recetas

Las recetas iniciales son pipelines versionadas `@who-calls`, `@trace`, `@find-def`, `@explain`, `@impact` y `@explain-change`. También hay recetas de compatibilidad `@nav-intent`, `@nav-route`, `@nav-pack`, `@nav-wiki`, `@nav-search`, `@nav-find`, `@nav-refs`, `@nav-related`, `@nav-flow-slice`, `@nav-change-pack`, `@nav-affected`, `@nav-multi-read` y `@nav-overview`. El expansionado conserva `contract_version`; una receta desconocida o argumentos faltantes fallan con `stage_failed` tipado.

Los ejemplos de uso se limitan a las tres recetas canónicas de [[CT-Q-V1]]. Los errores de parse identifican carácter, expectativa y ejemplo correcto. La resolución de workspace es de solo lectura: propone el alias que coincide con cwd/ruta registrada, indica `workspace add ... --no-index` si falta registro e `index --workspace <alias>` si falta índice; nunca registra ni reindexa en silencio.

## Aceptación

- Gramática quoted, opciones, pipelines de máximo ocho etapas, errores de sintaxis con posición de carácter, expectativa y ejemplo canónico, y `describe` hasta 600 tokens.
- Las tres recetas canónicas de [[CT-Q-V1]] pasan por el parser en pruebas Go; q no crea workspaces ni reindexa al resolver o consultar.
- CLI agrega exactamente el subcomando `q`; daemon atiende la operación, registra límites y usa el mismo executor que fallback directo.
- `milsp` es la primera herramienta MCP, su schema valida opciones q-v1 y las trece herramientas previas siguen listadas.
- Cursor completo: firma inválida, expiración >10 minutos y cambio de generation se distinguen; orden estable `(path,line,id)`.
- Bytes medidos sobre el envelope serializado; ítems completos; clipping UTF-8 y marcador/razón tipados.
- Recetas iniciales y los trece alias se resuelven de forma determinista; cada pipeline fuente está versionada en `internal/query/recipes/*.q`.
- Casos: TP-QRY-197 a TP-QRY-200 y TC-QRY-204.

## Descubrimiento del proveedor mi-mcp

`mi-lsp provider-manifest --format json` publica por stdout el manifiesto `mi-mcp-provider/v1` del proveedor `milsp`, con versión propia derivada de la misma versión que `mi-lsp --version` (sin prefijo `v`) y versión mínima `0.10.0`. El formato por defecto es JSON y no inicia el daemon, registra workspaces, escribe archivos ni usa red. La única operación publicada es `q` con contrato `q-v1` y efecto `read`; la invocación fija `--no-auto-register`. El esquema de entrada describe el pipeline y opciones reales de `mi-lsp q`; las decisiones completas y los límites están en [[CT-Q-V1]].

El manifiesto versionado en `integrations/mi-mcp/provider-manifest.json` es el mismo contrato, salvo `provider_version`, cuyo valor al publicar esta integración es `0.10.1`. El test del comando verifica los campos normativos, comprueba las propiedades frente a flags Cobra existentes y compara la salida con ese archivo.
