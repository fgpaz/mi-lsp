---
wiki_contract: WIKI-CONTRACT-v1
id: CT-NAV-WIKI-ROOT
kind: contract
status: active
title: CT-NAV-WIKI-ROOT
links:
  supports:
  - RF-WIKI-007
  - RF-WKS-008
---

```yaml
harness_protocol: SDD-HARNESS-v1
id: CT-NAV-WIKI-ROOT
kind: cli-contract
audience: dual
imports: [.docs/wiki/07_technical/TECH-DOCUMENTARY-GOVERNANCE.md]
exports: [CT-NAV-WIKI-ROOT]
agent_must_read: [.docs/wiki/07_technical/contracts/CT-NAV-WIKI-ROOT.md, internal/service/wiki_root.go]
agent_may_edit: [.docs/wiki/07_technical/contracts/CT-NAV-WIKI-ROOT.md]
agent_must_not_edit: [internal/service/wiki_root.go, .docs/wiki/_mi-lsp/read-model.toml, ../wiki-repo/]
verify: ["Leer .docs/wiki/07_technical/contracts/CT-NAV-WIKI-ROOT.md y .docs/wiki/07_technical/TECH-DOCUMENTARY-GOVERNANCE.md; cotejar las afirmaciones con la fuente publicada internal/service/wiki_root.go y sus límites. Registrar únicamente comandos y resultados efectivamente ejecutados; esta especificación no afirma una ejecución."]
stop_if: [Se invente governance_doc, se haga fallback con Setup pending o se muten canon/proyección al resolver raíces.]
evidence: [.docs/wiki/07_technical/contracts/CT-NAV-WIKI-ROOT.md, .docs/wiki/07_technical/TECH-DOCUMENTARY-GOVERNANCE.md, internal/service/wiki_root.go]
```

## Consumidor

CLI, agentes y consumidores locales de las operaciones descritas; no se amplía la autoridad del consumidor.

### Propósito

Definir el contrato visible de `mi-lsp nav wiki-root` (alias `mi-lsp nav wiki root`).

## Operaciones

Se conservan requests, envelopes, campos, límites y semántica detallados a continuación.

### Solicitud
- Operación: `nav.wiki-root`
- Protocolo: `mi-lsp-v1.1` (sin bump)
- Input:
  - `workspace`
  - `format` (`compact|json|text|toon|yaml`)
  - `role` opcional (`producto|ecosistema|gobierno_local`)

### Envelope de respuesta
- `backend = wiki-root`
- `items[]` contiene:
  - `wiki_root`
  - `role`
  - `workspace`
  - `governance_doc`
  - `resolved_from`
  - `id`

- **Marcador estable**: ct-nav-wiki-root-envelope
- **subcommand**: "nav wiki-root"
- **alias**: "nav wiki root"
- **operation**: nav.wiki-root
- **backend**: wiki-root
- **formats**: [compact, json, text, toon, yaml]
- **item_shape**:
  - **wiki_root**: portable relative path with /
  - **role**: producto|ecosistema|gobierno_local or empty on default
  - **workspace**: alias of the queried workspace
  - **governance_doc**: portable relative path to resolved governance source; empty when a declared canon source is unresolved
  - **resolved_from**: canon.<id> | default | registry.link
  - **id**: [[RF-WKS-008|canon]] id or linked alias; empty on default
- **paths**: never absolute
- **Comprobación prevista**:
  - mi-lsp nav wiki-root --workspace <alias> --format toon
  - mi-lsp nav wiki root --workspace <alias> --role producto --format toon
- **Condiciones de rechazo**:
  - envelope_type_changed=true
  - protocol_bumped=true
- **Evidencia referenciada**:
  - internal/service/wiki_root.go
  - internal/model/types.go
  - .docs/wiki/05_requirements/RF-WIKI-007.md

### Reglas

- El comando publica raíces portables para que skills y agentes no hardcodeen `.docs/wiki`.
- `.sandinas/project.yaml` legible sin clave `schema` no es Setup, incluso con `wiki.canon_roots`: se usa la resolución publicada sin Setup, incluido `governance.source_doc` validado. Una clave presente inválida o desconocida mantiene `pending` con `setup_schema_unsupported`; YAML ilegible mantiene `setup_contract_unreadable`. Setup v1/v2 conserva su precedencia y no permite fallback cuando está pendiente.
- Sin `[[RF-WKS-008|canon]]` ni `CanonLinks`, `wiki_root=.docs/wiki` y `resolved_from=default`; no es error. El literal histórico de `governance_doc` es el de `defaultGovernanceDocPath` conservado en RF-WIKI-007, no el nombre nuevo deducido por la migración.
- Con `[[RF-WKS-008|canon]]`, `resolved_from=canon.<id>` y `wiki_root` es el `root` declarado (ejemplo: `../wiki-repo/Ingenieria`). `governance_doc` usa el `source_doc` validado y existente del read-model aplicable, sin imponer un basename. Los paths de un read-model propio del canon se resuelven respecto de ese canon; se conserva la precedencia de la proyección local.
- Si una raíz declarada no tiene una fuente de gobierno resoluble, el item conserva su raíz/rol/ID, devuelve `governance_doc=""` y el envelope incluye un warning. No se afirma que exista un archivo sintetizado. Las raíces inválidas siguen fallando de forma cerrada; no se modifican autoridad, canon ni proyección.
- La raíz y el `source_doc` resueltos alimentan la autoridad de route/pack descrita en [[CT-NAV-WIKI]]; la ambigüedad de propietarios se resuelve allí, no inventando otro gobierno. [[TP-WIKI]] cubre la combinación de canon externo, read-model relativo y ancla no indexada.
- Con `workspace link`, `resolved_from=registry.link`.
- `--role` filtra por rol declarado; un rol sin coincidencia es error.
- Una fuente de gobierno única fuera del default basta; el resolver conserva compatibilidad de nombres y usa el source_doc real sin duplicar el documento.
- `auto_sync` no escribe un root de canon `read-only`.

### Estado

implemented (`nav wiki-root`, `nav wiki root`)

### RF asociado

RF-WIKI-007, RF-WKS-008

## Errores

Se mantienen las razones, warnings, rechazos y fallbacks explícitos de cada operación.

## Compatibilidad

La migración cambia el contrato documental, no el protocolo runtime ni sus versiones. Los esquemas legacy que la herramienta consume continúan como compatibilidad de entrada, no como formato de autoría de esta wiki.
