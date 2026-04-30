# CT-NAV-GOVERNANCE

```yaml
harness_protocol: SDD-HARNESS-v1
id: "CT-NAV-GOVERNANCE"
kind: "support-doc"
audience: "llm-first"
imports:
  - '[[00_gobierno_documental]]'
  - '[[CT-NAV-GOVERNANCE]]'
exports:
  - 'CT-NAV-GOVERNANCE'
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/09_contratos/CT-NAV-GOVERNANCE.md
agent_may_edit:
  - .docs/wiki/09_contratos/CT-NAV-GOVERNANCE.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
evidence:
  - .docs/wiki/09_contratos/CT-NAV-GOVERNANCE.md
```

## Proposito

Definir el contrato visible de `mi-lsp nav governance`.

## Request

- Operacion: `nav.governance`
- Input:
  - `workspace`
  - `format`

## Response envelope

- `backend = governance`
- `items[0]` contiene:
  - `human_doc`
  - `projection_doc`
  - `profile`
  - `extends`
  - `effective_base`
  - `effective_overlays`
  - `context_chain`
  - `closure_chain`
  - `audit_chain`
  - `blocking_rules`
  - `numbering_recommended`
  - `sync`
  - `index_sync`
  - `blocked`
  - `issues`
  - `warnings`
  - `allowed_actions`
  - `next_steps`
  - `summary`

## Reglas

- El comando siempre esta permitido, incluso cuando el repo esta bloqueado.
- Puede auto-sincronizar `read-model.toml`, pero no debe ocultar que hace falta reindex si `index_sync=stale`.
- Si la gobernanza es invalida, `nav ask` y `nav pack` deben devolver el mismo estado bloqueado en vez de continuar.
