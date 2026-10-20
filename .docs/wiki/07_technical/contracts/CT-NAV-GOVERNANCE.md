---
wiki_contract: WIKI-CONTRACT-v1
id: CT-NAV-GOVERNANCE
kind: contract
status: active
title: CT-NAV-GOVERNANCE
links:
  supports:
  - RF-WKS-008
  - RF-IDX-003
  - RF-WKS-005
---

## Consumidor

CLI, agentes y consumidores locales de las operaciones descritas; no se amplía la autoridad del consumidor.

### Propósito

Definir el contrato visible de `mi-lsp nav governance`.

## Operaciones

Se conservan requests, envelopes, campos, límites y semántica detallados a continuación.

### Solicitud
- Operación: `nav.governance`
- Input:
  - `workspace`
  - `format`

### Envelope de respuesta
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

### Campos v1 adicionales

Con WIKI-CONTRACT-v1 se exponen wiki_contract, traits y ui_roots. Las superficies por kind se encuentran en la proyección marcada wiki-contract-v1/traits. Los campos profile/effective_base y las cadenas legacy se conservan en el envelope por compatibilidad; no se exige declararlos en governance v1. Drift o salida no escribible de la proyección v1 es advisory y no sustituye el diagnóstico de index_sync o de seguridad del canon.

### Reglas

- El comando siempre esta permitido, incluso cuando el repo esta bloqueado.
- `human_doc` y `projection_doc` siguen las rutas declaradas (`governance.source_doc` y `projection.output`): relativas, in-workspace o, para `source_doc`, dentro de un root `[[RF-WKS-008|canon]]` declarado.
- Puede auto-sincronizar `read-model.toml` en el workspace de código, pero no debe escribir un root de canon `read-only` y no debe ocultar que hace falta reindex si `index_sync=stale`.
- Si la gobernanza es inválida, `nav ask` y `nav pack` deben devolver el mismo estado bloqueado en vez de continuar.

## Errores

Se mantienen las razones, warnings, rechazos y fallbacks explícitos de cada operación.

## Compatibilidad

La migración cambia el contrato documental, no el protocolo runtime ni sus versiones. Los esquemas legacy que la herramienta consume continúan como compatibilidad de entrada, no como formato de autoría de esta wiki.
