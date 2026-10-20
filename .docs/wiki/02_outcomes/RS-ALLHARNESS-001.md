---
wiki_contract: WIKI-CONTRACT-v1
id: RS-ALLHARNESS-001
kind: outcome
status: draft
source_type: operator_decision
---

# RS-ALLHARNESS-001 — mi-lsp para todos los harness

## Resultado observable
Agentes en pi, Claude Code, Codex y PiG navegan y diagnostican con el mismo binario y una skill corta, sin aprender todo el catálogo.

Fuente: pedido explícito de Gabriel `home/ideas/mi-lsp-para-todos-los-harness.md`, 2026-10-20; persona:herramientas-pocas-enganchables-20261020. Tipo de fuente: decisión del operador; no evidencia runtime.

## Beneficiario
Agentes en pi, Claude Code, Codex y PiG.

## Criterios de éxito
- `lsp.diagnostics` y `fs.find` se ejecutan realmente desde mi-mcp execute en ambos hosts objetivo, no solo aparecen en prompts.
- El padre publica tabla medida antes/después con minimax y longcat: tokens, aciertos y errores detectados antes del test.
- CLI publicada para Linux, Windows y ARM64; configuración personal fuera de este repo público.

## Fuera de alcance del RS
Sin hooks nuevos, nuevos modelos, segunda fuente argv/schema, compra de servicios ni retiro de APIs legacy.

[[FL-ALLHARNESS-001]] · [[RF-ALLHARNESS-001]]
