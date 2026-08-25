# RF-WKS-008 - Declarar raíces de wiki externas con `[[canon]]`

```yaml
harness_protocol: SDD-HARNESS-v1
wiki_source_protocol: SDD-WIKI-SOURCE-v1
source_protocol: SDD-WIKI-SOURCE-v1
id: "RF-WKS-008"
doc_id: "RF-WKS-008"
kind: "requirement"
audience: "llm-first"
imports:
  - '[[00_gobierno_documental]]'
  - '[[FL-BOOT-01]]'
  - '[[RF-WIKI-007]]'
  - '[[CT-NAV-WIKI-ROOT]]'
  - '[[CT-GOVERNANCE-AE-KERNEL-V2]]'
  - '[[TP-WKS]]'
exports:
  - 'RF-WKS-008'
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/03_FL/FL-BOOT-01.md
  - .docs/wiki/04_RF/RF-WKS-008.md
  - .docs/wiki/09_contratos/CT-NAV-WIKI-ROOT.md
agent_may_edit:
  - .docs/wiki/04_RF/RF-WKS-008.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --paths .docs/wiki/04_RF/RF-WKS-008.md --format toon
  - mi-lsp nav wiki validate-source --workspace mi-lsp --ids RF-WKS-008 --format toon
  - go test ./internal/workspace ./internal/docgraph ./internal/cli -count=1 -run 'TestResolveCanons|TestCanonLinks|TestWorkspaceLink'
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
  - canon_root_resolved_from_cwd=true
  - canon_root_resolved_from_mi_lsp_dir=true
  - protocol_version_bumped=true
evidence:
  - .docs/wiki/04_RF/RF-WKS-008.md
  - .docs/wiki/09_contratos/CT-NAV-WIKI-ROOT.md
  - .docs/wiki/06_pruebas/TP-WKS.md
  - internal/workspace/canon.go
```

## Contrato de harness

```toon
doc_id: RF-WKS-008
block_id: RF-WKS-008.harness
kind: harness-contract
audience: llm-first
source_of_truth: this
harness_protocol: SDD-HARNESS-v1
wiki_source_protocol: SDD-WIKI-SOURCE-v1
id: RF-WKS-008
imports:
  - '[[00_gobierno_documental]]'
  - '[[FL-BOOT-01]]'
  - '[[RF-WIKI-007]]'
  - '[[CT-NAV-WIKI-ROOT]]'
  - '[[TP-WKS]]'
exports:
  - RF-WKS-008
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/03_FL/FL-BOOT-01.md
  - .docs/wiki/04_RF/RF-WKS-008.md
  - .docs/wiki/09_contratos/CT-NAV-WIKI-ROOT.md
agent_may_edit:
  - .docs/wiki/04_RF/RF-WKS-008.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav wiki-root --workspace <alias> --format toon
  - mi-lsp nav wiki validate-source --workspace mi-lsp --ids RF-WKS-008 --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
  - absolute_canon_root_accepted=true
evidence:
  - .docs/wiki/04_RF/RF-WKS-008.md
  - internal/workspace/canon.go
  - internal/model/types.go
```

## Resultado requerido

```toon
doc_id: RF-WKS-008
block_id: RF-WKS-008.behavior
kind: normative-requirement
source_of_truth: this
status: implemented
actor: Usuario|Skill|Agente|CLI
origin: FL-BOOT-01
intent: declarar raíces de wiki externas portables sin paths absolutos
command: mi-lsp workspace link <alias> --role producto|ecosistema|gobierno_local
project_file: .mi-lsp/project.toml
workspace_root: directorio que contiene .mi-lsp/project.toml
canon_table:
  syntax: '[[canon]]'
  fields: {id, root, role, mode}
  resolve_relative_to: workspace_root
  never_resolve_from: [.mi-lsp/, cwd]
  example_root: ../wiki-repo/Ingenieria
role: producto | ecosistema | gobierno_local
mode: omitted | read-only
id: unique case-insensitive
canon_policy:
  table: '[canon_policy]'
  field: escape_max
  omitted: 1
  zero: no parent escape
rejected_roots:
  - absolute POSIX /
  - home ~/
  - UNC \\server\share
  - Windows drive C:
symlink_policy: fail-closed if resolved path contains symlink or junction
no_canon:
  wiki_root: .docs/wiki
  governance_doc: .docs/wiki/00_gobierno_documental.md
  resolved_from: default
  error: false
governance:
  source_doc: relative in-workspace OR inside declared [[canon]] root
  unique_00_gobierno: one 00_gobierno_documental.md outside .docs/wiki is enough
  projection_output: stays in the code workspace; never a read-only foreign canon root
index_docs_only:
  walks: declared [[canon]] roots as ../relative markdown
  invalid_root: skip with warning; do not write that root
registry:
  command: mi-lsp workspace link <alias> --role
  stores: CanonLinks
out_of_scope:
  - writable foreign canon
  - protocol bump
  - envelope type change
verify:
  - go test ./internal/workspace -count=1 -run 'TestResolveCanons|TestCanonLinks|TestValidateCanon'
  - go test ./internal/docgraph -count=1 -run 'TestCollectCanon|GovernanceSourceDoc|Projection'
stop_if:
  - canon_root_absolute_accepted=true
  - projection_writes_foreign_canon=true
evidence:
  - internal/workspace/canon.go
  - internal/workspace/canon_link.go
  - internal/docgraph/docgraph.go
  - internal/cli/workspace.go
```

## Aceptación

```toon
doc_id: RF-WKS-008
block_id: RF-WKS-008.acceptance
kind: acceptance
source_of_truth: this
positives:
  - TC-WKS-038
  - TC-WKS-039
  - TC-WKS-040
negatives:
  - TC-WKS-041
  - TC-WKS-042
invariants:
  - root se resuelve desde la raíz del workspace, no desde cwd ni .mi-lsp/
  - ids de [[canon]] son únicos
  - mode omitido equivale a read-only
  - index --docs-only no escribe un root de canon inválido
verify:
  - mi-lsp nav wiki trace RF-WKS-008 --workspace mi-lsp --format toon
evidence:
  - .docs/wiki/06_pruebas/TP-WKS.md
```
