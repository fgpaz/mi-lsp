# RF-WIKI-006 - Publicar un mapa compacto de hubs de una wiki de conocimiento

```yaml
harness_protocol: SDD-HARNESS-v1
wiki_source_protocol: SDD-WIKI-SOURCE-v1
source_protocol: SDD-WIKI-SOURCE-v1
id: "RF-WIKI-006"
doc_id: "RF-WIKI-006"
kind: "requirement"
audience: "llm-first"
imports:
  - '[[00_gobierno_documental]]'
  - '[[FL-WIKI-01]]'
  - '[[CT-NAV-WIKI]]'
  - '[[TP-WIKI]]'
exports:
  - 'RF-WIKI-006'
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/03_FL/FL-WIKI-01.md
  - .docs/wiki/04_RF/RF-WIKI-006.md
  - .docs/wiki/09_contratos/CT-NAV-WIKI.md
agent_may_edit:
  - .docs/wiki/04_RF/RF-WIKI-006.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --paths .docs/wiki/04_RF/RF-WIKI-006.md --format toon
  - mi-lsp nav wiki validate-source --workspace mi-lsp --ids RF-WIKI-006 --format toon
  - go test ./internal/cli ./internal/service -count=1 -run 'TestNavWikiMap|TestClassifyWikiMapHub|TestGroupWikiMapDocsOrder'
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
  - wiki_map_returns_full_bodies=true
  - tedi_invents_javascript_graph=true
evidence:
  - .docs/wiki/04_RF/RF-WIKI-006.md
  - .docs/wiki/09_contratos/CT-NAV-WIKI.md
  - .docs/wiki/06_pruebas/TP-WIKI.md
  - internal/service/wiki_map.go
```

## Contrato de harness

```toon
doc_id: RF-WIKI-006
block_id: RF-WIKI-006.harness
kind: harness-contract
audience: llm-first
source_of_truth: this
harness_protocol: SDD-HARNESS-v1
wiki_source_protocol: SDD-WIKI-SOURCE-v1
id: RF-WIKI-006
imports:
  - '[[00_gobierno_documental]]'
  - '[[FL-WIKI-01]]'
  - '[[CT-NAV-WIKI]]'
  - '[[TP-WIKI]]'
exports:
  - RF-WIKI-006
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/03_FL/FL-WIKI-01.md
  - .docs/wiki/04_RF/RF-WIKI-006.md
  - .docs/wiki/09_contratos/CT-NAV-WIKI.md
agent_may_edit:
  - .docs/wiki/04_RF/RF-WIKI-006.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav wiki map --workspace <alias> --format toon
  - mi-lsp nav wiki validate-source --workspace mi-lsp --ids RF-WIKI-006 --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
  - wiki_map_returns_full_bodies=true
evidence:
  - .docs/wiki/04_RF/RF-WIKI-006.md
  - internal/service/wiki_map.go
  - internal/cli/nav.go
```

## Resultado requerido

```toon
doc_id: RF-WIKI-006
block_id: RF-WIKI-006.behavior
kind: normative-requirement
source_of_truth: this
status: implemented
actor: Usuario|Skill|Agente|CLI
origin: FL-WIKI-01
command: mi-lsp nav wiki map --workspace <alias> [--token-budget N] [--format compact|json|text|toon|yaml]
backend: wiki.map
intent: mapa compacto de hubs para personalización diaria y Momento; nunca cuerpos completos
locks:
  - D-TEDI-017
hubs:
  - id: persona
    title: Persona
    paths: wiki/00-09 markdown de primer nivel
  - id: proyectos
    title: Proyectos
    paths: wiki/10-19
  - id: sistema
    title: Sistema
    paths: wiki/20-30 de primer nivel o 30-dashboard
  - id: materia
    title: Materia
    paths: bibliotecas/**
excluded:
  - wiki/31-workers
  - wiki/32-contratos
  - yaml
  - old|archive|deprecated|historico|legacy
  - .docs/wiki SDD del producto
sources:
  preferred: doc_records indexados cuyo path clasifica a un hub
  fallback: walk de wiki/ y bibliotecas/ si el índice no tiene hubs
envelope:
  items: hubs[{id,title,docs[{path,title}]}]
  stats: {files, ms, tokens_estimate}
  warnings: walk si el índice documental está vacío
  hint: no se encontraron hubs de wiki en wiki/ o bibliotecas/
budgets:
  max_items: Context.MaxItems recorta docs conservando orden de hubs
  token_budget: Context.TokenBudget recorta docs hasta caber
out_of_scope:
  - fan-out --all-workspaces
  - grafo de vecinos (lo publica el Graph Kernel, no este comando)
  - memoria propia de Tedi v1
  - Gastos y Notas vivos
verify:
  - go test ./internal/service -count=1 -run 'TestClassifyWikiMapHub|TestGroupWikiMapDocsOrder'
  - go test ./internal/cli -count=1 -run 'TestNavWikiMapCommandExists'
stop_if:
  - map_emits_full_markdown_bodies=true
  - map_classifies_sdd_dot_docs_wiki_as_hub=true
evidence:
  - internal/service/wiki_map.go
  - internal/service/wiki_map_test.go
  - internal/cli/nav.go
  - .docs/wiki/06_pruebas/TP-WIKI.md
```

## Aceptación

```toon
doc_id: RF-WIKI-006
block_id: RF-WIKI-006.acceptance
kind: acceptance
source_of_truth: this
positives:
  - TC-WIKI-029
  - TC-WIKI-030
  - TC-WIKI-031
  - TC-WIKI-032
negatives:
  - TC-WIKI-033
invariants:
  - el mapa es catálogo, no lectura de ficha
  - el orden de hubs es persona, proyectos, sistema, materia
  - Tedi no inventa un grafo en JavaScript a partir de este envelope
verify:
  - mi-lsp nav wiki trace RF-WIKI-006 --workspace mi-lsp --format toon
evidence:
  - .docs/wiki/06_pruebas/TP-WIKI.md
```
