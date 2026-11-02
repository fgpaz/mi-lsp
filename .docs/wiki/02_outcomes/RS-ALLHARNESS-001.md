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

```yaml
harness_protocol: SDD-HARNESS-v1
id: RS-ALLHARNESS-001
kind: outcome
audience: dual
imports:
  - .docs/wiki/04_flows/FL-ALLHARNESS-001.md
  - .docs/wiki/05_requirements/RF-ALLHARNESS-001.md
exports:
  - RS-ALLHARNESS-001
agent_must_read:
  - .docs/wiki/02_outcomes/RS-ALLHARNESS-001.md
agent_may_edit:
  - .docs/wiki/02_outcomes/RS-ALLHARNESS-001.md
agent_must_not_edit:
  - .docs/wiki/05_requirements/RF-ALLHARNESS-001.md
  - .docs/wiki/06_tests/TP-ALLHARNESS-001.md
verify:
  - "Lectura/cotejo: comprobar cada criterio del outcome contra RF-ALLHARNESS-001 y FL-ALLHARNESS-001, y cotejar sus casos/disposiciones con TP-ALLHARNESS-001; conservar estados pendientes y no inferir ejecución desde el plan."
stop_if:
  - harness_verdict=BLOCKED
  - linked_source_missing
  - outcome_acceptance_or_status_would_change
evidence:
  - .docs/wiki/05_requirements/RF-ALLHARNESS-001.md
  - .docs/wiki/04_flows/FL-ALLHARNESS-001.md
  - .docs/wiki/06_tests/TP-ALLHARNESS-001.md
```
