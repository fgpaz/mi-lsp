---
id: RF-QRY-024
doc_id: RF-QRY-024
kind: requirement
audience: llm-first
harness_protocol: SDD-HARNESS-v1
wiki_source_protocol: SDD-WIKI-SOURCE-v1
wiki_graph_protocol: SDD-WIKI-GRAPH-v1
imports:
  - '[[00_gobierno_documental]]'
  - '[[RF-QRY-001]]'
  - '[[TP-QRY]]'
exports:
  - 'RF-QRY-024'
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/04_RF/RF-QRY-001.md
  - .docs/wiki/04_RF/RF-QRY-024.md
  - .docs/wiki/06_pruebas/TP-QRY.md
agent_may_edit:
  - .docs/wiki/04_RF/RF-QRY-024.md
  - .docs/wiki/06_pruebas/TP-QRY.md
  - .docs/wiki/06_matriz_pruebas_RF.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
evidence:
  - .docs/wiki/04_RF/RF-QRY-024.md
  - .docs/wiki/06_pruebas/TP-QRY.md
---

# Búsqueda rg passthrough con anotación opcional por línea

```toon
block_id: rf-qry-024-grep-v1
kind: normative
source_of_truth: this
status: implemented
contract: grep-v1
purpose: mantener la semántica de ripgrep y añadir contexto de símbolos sin cambiar resultados de búsqueda
inputs:
  command: mi-lsp grep [rg-args...]
  passthrough: ejecutar el binario rg real desde MI_LSP_RG o PATH con los argumentos restantes
  flags_milsp: [--rg-compat, --annotate]
behavior:
  - Mantener selección, orden, globs, tipos y exit codes 0/1/2 de rg.
  - Por defecto anotar únicamente líneas de match de archivos de código indexados cuando stdout va directamente al agente.
  - Añadir a la línea un sufijo tabulado ⟦clase símbolo⟧; clase es def, ref, com o str y símbolo es el qualified_name contenedor cuando exista.
  - Si no hay catálogo, el archivo no es código soportado o ningún match pertenece a código indexado, entregar la salida original de rg.
  - Si no hay índice, emitir una sola advertencia index_not_ready por stderr y no registrar automáticamente el workspace.
  - Ante timeout, catálogo inválido o fallo recuperado del anotador, conservar íntegra la salida de rg y su exit code.
  - --rg-compat entrega salida rg pura; --annotate fuerza anotación incluso si stdout está redirigido.
  - Los modos de salida incompatibles con anotación (-l, -c, --json, --files, -o, --vimgrep) se mantienen como rg puro salvo --annotate explícito.
  - La telemetría registra operación grep y bytes de salida, nunca patrón, texto de match ni contenido de archivo.
outputs:
  stdout: coincidencias de rg, potencialmente anotadas por línea
  stderr: errores originales de rg y advertencia local única cuando falta el índice
  exit_status: código original de rg
implementation_bindings:
  - internal/grepx/
  - internal/cli/grep.go
  - internal/daemon/state_store.go
verification:
  - TP-QRY/TC-QRY-192
  - TP-QRY/TC-QRY-193
  - TP-QRY/TC-QRY-194
stop_gates:
  - No alterar salida o código de salida en --rg-compat.
  - Nunca hacer que una falla de anotación convierta un éxito/fallo de rg en otro resultado.
  - No persistir patrones ni contenido de resultados en telemetría.
durable_evidence:
  - .docs/wiki/06_pruebas/TP-QRY.md
```
