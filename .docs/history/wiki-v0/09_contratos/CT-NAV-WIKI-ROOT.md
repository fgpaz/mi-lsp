# CT-NAV-WIKI-ROOT

```yaml
harness_protocol: SDD-HARNESS-v1
id: "CT-NAV-WIKI-ROOT"
kind: "support-doc"
audience: "llm-first"
imports:
  - '[[00_gobierno_documental]]'
  - '[[RF-WIKI-007]]'
  - '[[RF-WKS-008]]'
  - '[[CT-NAV-GOVERNANCE]]'
exports:
  - 'CT-NAV-WIKI-ROOT'
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/09_contratos/CT-NAV-WIKI-ROOT.md
agent_may_edit:
  - .docs/wiki/09_contratos/CT-NAV-WIKI-ROOT.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav wiki-root --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
evidence:
  - .docs/wiki/09_contratos/CT-NAV-WIKI-ROOT.md
```

## Propósito

Definir el contrato visible de `mi-lsp nav wiki-root` (alias `mi-lsp nav wiki root`).

## Request

- Operación: `nav.wiki-root`
- Protocolo: `mi-lsp-v1.1` (sin bump)
- Input:
  - `workspace`
  - `format` (`compact|json|text|toon|yaml`)
  - `role` opcional (`producto|ecosistema|gobierno_local`)

## Response envelope

- `backend = wiki-root`
- `items[]` contiene:
  - `wiki_root`
  - `role`
  - `workspace`
  - `governance_doc`
  - `resolved_from`
  - `id`

```toon
doc_id: CT-NAV-WIKI-ROOT
block_id: ct-nav-wiki-root-envelope
kind: cli-contract
source_of_truth: this
subcommand: "nav wiki-root"
alias: "nav wiki root"
operation: nav.wiki-root
backend: wiki-root
formats: [compact, json, text, toon, yaml]
item_shape:
  wiki_root: portable relative path with /
  role: producto|ecosistema|gobierno_local or empty on default
  workspace: alias of the queried workspace
  governance_doc: portable relative path to resolved governance source; empty when a declared canon source is unresolved
  resolved_from: canon.<id> | default | registry.link
  id: [[RF-WKS-008|canon]] id or linked alias; empty on default
paths: never absolute
verify:
  - mi-lsp nav wiki-root --workspace <alias> --format toon
  - mi-lsp nav wiki root --workspace <alias> --role producto --format toon
stop_if:
  - envelope_type_changed=true
  - protocol_bumped=true
evidence:
  - internal/service/wiki_root.go
  - internal/model/types.go
  - .docs/wiki/04_RF/RF-WIKI-007.md
```

## Reglas

- El comando publica raíces portables para que skills y agentes no hardcodeen `.docs/wiki`.
- Sin `[[RF-WKS-008|canon]]` ni `CanonLinks`, `wiki_root=.docs/wiki`, `governance_doc=.docs/wiki/00_gobierno_documental.md` y `resolved_from=default`; no es error.
- Con `[[RF-WKS-008|canon]]`, `resolved_from=canon.<id>` y `wiki_root` es el `root` declarado (ejemplo: `../wiki-repo/Ingenieria`). `governance_doc` usa el `source_doc` validado y existente del read-model aplicable, sin imponer un basename. Los paths de un read-model propio del canon se resuelven respecto de ese canon; se conserva la precedencia de la proyección local.
- Si una raíz declarada no tiene una fuente de gobierno resoluble, el item conserva su raíz/rol/ID, devuelve `governance_doc=""` y el envelope incluye un warning. No se afirma que exista un archivo sintetizado. Las raíces inválidas siguen fallando de forma cerrada; no se modifican autoridad, canon ni proyección.
- La raíz y el `source_doc` resueltos alimentan la autoridad de route/pack descrita en [[CT-NAV-WIKI]]; la ambigüedad de propietarios se resuelve allí, no inventando otro gobierno. [[TP-WIKI]] cubre la combinación de canon externo, read-model relativo y ancla no indexada.
- Con `workspace link`, `resolved_from=registry.link`.
- `--role` filtra por rol declarado; un rol sin coincidencia es error.
- Un único `00_gobierno_documental.md` fuera de `.docs/wiki` basta; no hace falta duplicar el documento humano.
- `auto_sync` no escribe un root de canon `read-only`.

## Estado

implemented (`nav wiki-root`, `nav wiki root`)

## RF asociado

RF-WIKI-007, RF-WKS-008
