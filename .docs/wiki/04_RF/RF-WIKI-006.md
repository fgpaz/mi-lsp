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
command: mi-lsp nav wiki map --workspace <alias> [--max-items N] [--token-budget N] [--format compact|json|text|toon|yaml]
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
configuration:
  source: .docs/wiki/_mi-lsp/read-model.toml
  defaults: {enabled: true, roots: [wiki/, bibliotecas/], hubs: D-TEDI-017}
  roots: "wiki/ y bibliotecas/ permanecen; roots configuradas se agregan de forma ordenada y deduplicada"
  custom_hubs: "[[wiki_map.hub]] en orden declarado; primer patrón coincidente gana y no mezcla hubs default"
  disabled: "enabled=false desactiva mapa y alta automática de raíces de conocimiento"
sources:
  preferred: query SQLite acotada a path,title y raíces configuradas
  fallback: walk bounded de raíces configuradas si el índice falla o no contiene hubs
  safety: ignore matcher, cancelación, symlink/reparse skip y warnings sanitizados
envelope:
  items: hubs[{id,title,total_docs,docs[{path,title}]}]
  stats: {files: total_returned, total_docs, total_returned, truncation_reason, ms, tokens_estimate}
  truncated: true cuando total_returned < total_docs
  next_hint: aumentar --max-items o --token-budget según el límite efectivo
budgets:
  max_items: asignación round-robin equitativa que conserva orden interno y de hubs
  token_budget: búsqueda binaria del mayor conteo equitativo que cabe
out_of_scope:
  - fan-out --all-workspaces
  - grafo de vecinos (lo publica el Graph Kernel, no este comando)
  - memoria propia de Tedi v1
  - Gastos y Notas vivos
verify:
  - go test ./internal/service -count=1 -run 'TestClassifyWikiMapHubDefaultsRemainLocked|TestGroupWikiMapDocsUsesDefaultOrCustomDeclarationOrder|TestTrimWikiMapHubs|TestWalkWikiMapDocs'
  - go test ./internal/cli -count=1 -run 'TestNavWikiMapUsesDirectExecution'
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
  - TC-WIKI-034
  - TC-WIKI-036
  - TC-WIKI-037
  - TC-WIKI-039
  - TC-WIKI-040
negatives:
  - TC-WIKI-033
  - TC-WIKI-035
  - TC-WIKI-038
invariants:
  - el mapa es catálogo, no lectura de ficha
  - sin configuración, el orden de hubs es persona, proyectos, sistema, materia
  - con hubs custom, solo se emiten los declarados y en ese orden
  - ningún hub no vacío desaparece mientras el budget alcance una ronda
  - truncation, totales y siguiente acción nunca se ocultan
  - nav.wiki.map siempre ejecuta directo
  - Tedi no inventa un grafo en JavaScript a partir de este envelope
verify:
  - mi-lsp nav wiki trace RF-WIKI-006 --workspace mi-lsp --format toon
evidence:
  - .docs/wiki/06_pruebas/TP-WIKI.md
```
