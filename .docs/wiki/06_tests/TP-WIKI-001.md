---
wiki_contract: WIKI-CONTRACT-v1
id: TP-WIKI-001
kind: test
status: active
title: "Verificar el recorrido wiki+código con CLI real"
links:
  verifies: [RF-WGRAPH-001]
---

## Alcance
Piloto aislado con Git, módulo Go, símbolo real y wiki v1; indexado exclusivamente por la CLI nativa. No se escriben tablas SQL. Las lecturas de store para contar aristas son mode=ro. La evidencia persistente pertenece al paquete de cierre, no al canon.

## Casos
- Criterio 1 — disposición: manual; evidencia: declaraciones del fixture, vecinos por relación, inversos y conteos SQL de solo lectura con generación.
- Criterio 2 — disposición: manual; evidencia: JSON de neighbors RF, path RS→símbolo real, explain de edge y trace/ wiki trace con wiki-graph/v1 y la misma generación.
- Criterio 3 — disposición: manual; evidencia: diff real, affected/change-pack, index incremental nativo, generación antes/después y vecinos del link cambiado.
- Criterio 4 — disposición: manual; evidencia: JSON con Mermaid, hashes de wiki antes/después, cursor de página 1 consumido en página 2 y manifiesto de proveedor.

```yaml
harness_protocol: SDD-HARNESS-v1
id: TP-WIKI-001
kind: test-plan
audience: llm-first
imports:
  - .docs/wiki/00_governance.md
  - .docs/wiki/05_requirements/RF-WIKI-001.md
  - .docs/wiki/02_outcomes/RS-WIKI-001.md
exports:
  - TP-WIKI-001
agent_must_read:
  - .docs/wiki/06_tests/TP-WIKI-001.md
agent_may_edit:
  - .docs/wiki/06_tests/TP-WIKI-001.md
agent_must_not_edit:
  - .docs/wiki/05_requirements/
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - .docs/wiki/06_tests/TP-WIKI-001.md#Casos
stop_if:
  - harness_verdict=BLOCKED
  - manual_evidence_pending_is_reported_as_pass
  - test_oracle_or_acceptance_would_change
evidence:
  - .docs/wiki/06_tests/TP-WIKI-001.md
  - .docs/wiki/05_requirements/RF-WIKI-001.md
```
