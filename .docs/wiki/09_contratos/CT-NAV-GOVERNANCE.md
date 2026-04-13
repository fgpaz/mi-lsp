# CT-NAV-GOVERNANCE

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
