# RF-WIKI-007 - Resolver la raíz de wiki con `nav wiki-root`

```yaml
harness_protocol: SDD-HARNESS-v1
wiki_source_protocol: SDD-WIKI-SOURCE-v1
source_protocol: SDD-WIKI-SOURCE-v1
id: "RF-WIKI-007"
doc_id: "RF-WIKI-007"
kind: "requirement"
audience: "llm-first"
imports:
  - '[[00_gobierno_documental]]'
  - '[[FL-WIKI-01]]'
  - '[[RF-WKS-008]]'
  - '[[CT-NAV-WIKI-ROOT]]'
  - '[[TP-WIKI]]'
exports:
  - 'RF-WIKI-007'
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/03_FL/FL-WIKI-01.md
  - .docs/wiki/04_RF/RF-WIKI-007.md
  - .docs/wiki/09_contratos/CT-NAV-WIKI-ROOT.md
agent_may_edit:
  - .docs/wiki/04_RF/RF-WIKI-007.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --paths .docs/wiki/04_RF/RF-WIKI-007.md --format toon
  - mi-lsp nav wiki validate-source --workspace mi-lsp --ids RF-WIKI-007 --format toon
  - go test ./internal/cli ./internal/service -count=1 -run 'TestNavWikiRoot|TestWikiRoot'
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
  - wiki_root_emits_absolute_paths=true
  - protocol_version_bumped=true
evidence:
  - .docs/wiki/04_RF/RF-WIKI-007.md
  - .docs/wiki/09_contratos/CT-NAV-WIKI-ROOT.md
  - .docs/wiki/06_pruebas/TP-WIKI.md
  - internal/service/wiki_root.go
```

## Contrato de harness

```toon
doc_id: RF-WIKI-007
block_id: RF-WIKI-007.harness
kind: harness-contract
audience: llm-first
source_of_truth: this
harness_protocol: SDD-HARNESS-v1
wiki_source_protocol: SDD-WIKI-SOURCE-v1
id: RF-WIKI-007
imports:
  - '[[00_gobierno_documental]]'
  - '[[FL-WIKI-01]]'
  - '[[RF-WKS-008]]'
  - '[[CT-NAV-WIKI-ROOT]]'
  - '[[TP-WIKI]]'
exports:
  - RF-WIKI-007
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/03_FL/FL-WIKI-01.md
  - .docs/wiki/04_RF/RF-WIKI-007.md
  - .docs/wiki/09_contratos/CT-NAV-WIKI-ROOT.md
agent_may_edit:
  - .docs/wiki/04_RF/RF-WIKI-007.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav wiki-root --workspace <alias> --format toon
  - mi-lsp nav wiki root --workspace <alias> --role producto --format toon
  - mi-lsp nav wiki validate-source --workspace mi-lsp --ids RF-WIKI-007 --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
  - consumers_hardcode_dot_docs_wiki=true
evidence:
  - .docs/wiki/04_RF/RF-WIKI-007.md
  - internal/service/wiki_root.go
  - internal/cli/nav.go
```

## Resultado requerido

```toon
doc_id: RF-WIKI-007
block_id: RF-WIKI-007.behavior
kind: normative-requirement
source_of_truth: this
status: implemented
actor: Usuario|Skill|Agente|CLI
origin: FL-WIKI-01
command: mi-lsp nav wiki-root --workspace <alias> [--role producto|ecosistema|gobierno_local] [--format compact|json|text|toon|yaml]
alias: mi-lsp nav wiki root
operation: nav.wiki-root
backend: wiki-root
protocol: mi-lsp-v1.1
intent: publicar la raíz de wiki portable para que skills y agentes no hardcodeen .docs/wiki
envelope_items:
  - wiki_root
  - role
  - workspace
  - governance_doc
  - resolved_from
  - id
resolved_from:
  - canon.<id>
  - default
  - registry.link
no_canon:
  wiki_root: .docs/wiki
  governance_doc: .docs/wiki/00_gobierno_documental.md
  resolved_from: default
  error: false
declared_canon_governance:
  source: source_doc existente y validado del read-model aplicable
  missing_source: governance_doc vacío y warning explícito; nunca un nombre inventado
  profile_paths: relativos al canon propietario; precedencia local sin cambios
  downstream_contract: "[[CT-NAV-WIKI]] conserva el ancla gobernada en route/pack aunque no esté indexada"
  combined_oracle: TestDeclaredCanonUnindexedAnchorSurvivesRankedSupport
paths: portable relative with /; never absolute
role_flag: filtra [[RF-WKS-008|canon]] o CanonLinks por role; si no hay match, error
precedence:
  - project [[RF-WKS-008|canon]]
  - registry CanonLinks
  - default .docs/wiki
out_of_scope:
  - fan-out --all-workspaces
  - protocol bump
  - envelope type change
verify:
  - go test ./internal/cli -count=1 -run TestNavWikiRootCommandExists
  - go test ./internal/service -count=1 -run 'TestWikiRootResolvesCanonRelativeToWorkspaceRoot|TestWikiRootNoCanonDefaults|TestWikiRootRoleMissingMatchErrors'
stop_if:
  - wiki_root_is_absolute=true
  - missing_canon_is_hard_error=true
evidence:
  - internal/service/wiki_root.go
  - internal/service/wiki_root_test.go
  - internal/cli/nav.go
  - .docs/wiki/06_pruebas/TP-WIKI.md
```

## Aceptación

```toon
doc_id: RF-WIKI-007
block_id: RF-WIKI-007.acceptance
kind: acceptance
source_of_truth: this
positives:
  - TC-WIKI-034
  - TC-WIKI-035
  - TC-WIKI-036
negatives:
  - TC-WIKI-037
invariants:
  - nav wiki-root y nav wiki root son el mismo operation nav.wiki-root
  - sin [[RF-WKS-008|canon]] no hay error; default es .docs/wiki
  - la salida nunca incluye paths absolutos
  - consumers resuelven wiki_root con este comando, no con un path fijo
verify:
  - mi-lsp nav wiki trace RF-WIKI-007 --workspace mi-lsp --format toon
evidence:
  - .docs/wiki/06_pruebas/TP-WIKI.md
```
