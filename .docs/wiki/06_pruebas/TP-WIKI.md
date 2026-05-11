# TP-WIKI

```yaml
harness_protocol: SDD-HARNESS-v1
wiki_source_protocol: SDD-WIKI-SOURCE-v1
id: "TP-WIKI"
doc_id: "TP-WIKI"
kind: "test-plan"
audience: "llm-first"
imports:
  - '[[RF-WIKI-001]]'
  - '[[RF-WIKI-002]]'
  - '[[RF-WIKI-003]]'
  - '[[RF-WIKI-004]]'
  - '[[RF-WIKI-005]]'
exports:
  - 'TP-WIKI'
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/04_RF/RF-WIKI-001.md
  - .docs/wiki/04_RF/RF-WIKI-002.md
  - .docs/wiki/04_RF/RF-WIKI-003.md
  - .docs/wiki/04_RF/RF-WIKI-004.md
  - .docs/wiki/04_RF/RF-WIKI-005.md
  - .docs/wiki/06_pruebas/TP-WIKI.md
agent_may_edit:
  - .docs/wiki/06_pruebas/TP-WIKI.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-source --workspace mi-lsp --paths .docs/wiki/06_pruebas/TP-WIKI.md --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
evidence:
  - .docs/wiki/06_pruebas/TP-WIKI.md
  - .docs/wiki/06_matriz_pruebas_RF.md
```

## Cobertura objetivo

- RF-WIKI-001 (search --all-workspaces)
- RF-WIKI-002 (inventory --all-workspaces)
- RF-WIKI-003 (route --all-workspaces)
- RF-WIKI-004 (trace --all-workspaces)
- RF-WIKI-005 (pack --all-workspaces)

## Casos

```toon
block_id: tp-wiki-rf-001-cases
kind: test-cases
rf: RF-WIKI-001
title: "Test cases para RF-WIKI-001 (search --all-workspaces)"
source_of_truth: this
verify: "mi-lsp nav wiki validate-source --workspace mi-lsp --format toon"
evidence: ".docs/wiki/06_pruebas/TP-WIKI.md"
cases:
  - id: TC-WIKI-001
    type: positivo
    given: "registry con N workspaces docs_ready=true; flag --all-workspaces ausente"
    when: "mi-lsp nav wiki search 'governance' --workspace mi-lsp --format toon"
    then: "envelope shape idéntico al actual single-workspace; stats.workspaces_queried no presente o = 1; campo 'workspace' ausente en items"

  - id: TC-WIKI-002
    type: positivo
    given: "registry con >= 3 workspaces docs_ready=true"
    when: "mi-lsp nav wiki search 'governance' --all-workspaces --format toon"
    then: "items[N>0]; cada item incluye 'workspace<>'''; stats.workspaces_queried >= 3; ok=true"

  - id: TC-WIKI-003
    type: positivo
    given: "registry con K workspaces donde al menos uno tiene governance_blocked=true"
    when: "mi-lsp nav wiki search 'docs' --all-workspaces --format toon"
    then: "envelope ok=true; workspace bloqueado aparece en stats.workspaces_failed[] con motivo; otros workspaces retornan items normalmente"

  - id: TC-WIKI-004
    type: positivo
    given: "búsqueda --all-workspaces con >10 hits globales"
    when: "mi-lsp nav wiki search 'wiki' --all-workspaces --top-global 10 --format toon"
    then: "envelope retorna máximo 10 items totales; truncated=true; cada item anotado con workspace"
```

```toon
block_id: tp-wiki-rf-002-cases
kind: test-cases
rf: RF-WIKI-002
title: "Test cases para RF-WIKI-002 (inventory --all-workspaces)"
source_of_truth: this
verify: "mi-lsp nav wiki validate-source --workspace mi-lsp --format toon"
evidence: ".docs/wiki/06_pruebas/TP-WIKI.md"
cases:
  - id: TC-WIKI-005
    type: positivo
    given: "registry con >= 2 workspaces registrados"
    when: "mi-lsp nav wiki inventory --all-workspaces --format toon"
    then: "items[] sin --with-layer-counts; cada item contiene: alias, root, wiki_root, governance_blocked, docs_ready, doc_count, last_indexed_at; campo 'layers' ausente"

  - id: TC-WIKI-006
    type: positivo
    given: "registry con >= 2 workspaces documentados"
    when: "mi-lsp nav wiki inventory --all-workspaces --with-layer-counts --format toon"
    then: "items[] incluye nuevo campo 'layers' con conteos por capa (layer: count); e.g., layers=[{layer: '03_FL', count: 5}, {layer: '04_RF', count: 12}]"

  - id: TC-WIKI-007
    type: positivo
    given: "al menos dos workspaces con docs_ready=true; uno con fallo esperado de indexing"
    when: "mi-lsp nav wiki inventory --all-workspaces --format toon"
    then: "stats.workspaces_queried >= 2; stats.workspaces_failed[] presente (puede estar vacío si todos responden); ok=true siempre"

  - id: TC-WIKI-008
    type: positivo
    given: "registry con workspaces donde docs_ready=true y docs_ready=false"
    when: "mi-lsp nav wiki inventory --all-workspaces --format toon"
    then: "items[] contiene todos los workspaces registrados independientemente de docs_ready; flag docs_ready anotado por workspace"
```

```toon
block_id: tp-wiki-rf-003-cases
kind: test-cases
rf: RF-WIKI-003
title: "Test cases para RF-WIKI-003 (route --all-workspaces)"
source_of_truth: this
verify: "mi-lsp nav wiki validate-source --workspace mi-lsp --format toon"
evidence: ".docs/wiki/06_pruebas/TP-WIKI.md"
cases:
  - id: TC-WIKI-009
    type: positivo
    given: "query 'RF-WIKI-001' existe en documentos de >= 2 workspaces diferentes"
    when: "mi-lsp nav wiki route 'RF-WIKI-001' --all-workspaces --format toon"
    then: "items[N>0] cada uno con workspace<>''; respuesta devuelve candidatos desde ambos wikis; ok=true"

  - id: TC-WIKI-010
    type: positivo
    given: "query pattern no coincide en ningun workspace"
    when: "mi-lsp nav wiki route 'nonexistent-task-xyz' --all-workspaces --format toon"
    then: "items=[]; hint presente con sugerencias de búsqueda (e.g., 'navega con nav wiki search'); ok=true"

  - id: TC-WIKI-011
    type: positivo
    given: "múltiples workspaces con candidatos para la misma query"
    when: "mi-lsp nav wiki route 'governance' --all-workspaces --top 5 --tier preview --format toon"
    then: "flags --top y --tier se aplican a cada workspace independientemente; items totales respetan --top global; tier de cada item = preview"

  - id: TC-WIKI-012
    type: positivo
    given: "mismo task slug aparece en dos wikis diferentes con contenido distinto"
    when: "mi-lsp nav wiki route 'task-slug' --all-workspaces --format toon"
    then: "cada candidato anotado con workspace; no fusionamos ni ambiguamos; ok=true; usuario puede navegar ambas"
```

```toon
block_id: tp-wiki-rf-004-cases
kind: test-cases
rf: RF-WIKI-004
title: "Test cases para RF-WIKI-004 (trace --all-workspaces)"
source_of_truth: this
verify: "mi-lsp nav wiki validate-source --workspace mi-lsp --format toon"
evidence: ".docs/wiki/06_pruebas/TP-WIKI.md"
cases:
  - id: TC-WIKI-013
    type: positivo
    given: "RF-WIKI-001 mencionado en >= 2 workspaces diferentes"
    when: "mi-lsp nav wiki trace 'RF-WIKI-001' --all-workspaces --format toon"
    then: "items[] agrupados por workspace; no se fusionan trazas cross-wiki; cada item incluye workspace de origen; ok=true"

  - id: TC-WIKI-014
    type: positivo
    given: "registry con múltiples workspaces con documentación RF/TP"
    when: "mi-lsp nav wiki trace --all --all-workspaces --format toon"
    then: "devuelve TODAS las trazas (RF, TP, etc.) de todos los workspaces; items[] contiene evidencia completa anotada por workspace"

  - id: TC-WIKI-015
    type: positivo
    given: "múltiples workspaces con trazas de RF-WIKI-001"
    when: "mi-lsp nav wiki trace 'RF-WIKI-001' --summary --all-workspaces --format toon"
    then: "modo summary se respeta per-workspace; cada item contiene resumen (no detalles completos); workspace presente"

  - id: TC-WIKI-016
    type: positivo
    given: "workspace sin TP definidos aún registrado en registry"
    when: "mi-lsp nav wiki trace --all-workspaces --format toon"
    then: "workspace sin TPs aparece en stats.workspaces_queried; items[] puede estar vacío para ese workspace pero entra en conteo global; ok=true"
```

```toon
block_id: tp-wiki-rf-005-cases
kind: test-cases
rf: RF-WIKI-005
title: "Test cases para RF-WIKI-005 (pack --all-workspaces)"
source_of_truth: this
verify: "mi-lsp nav wiki validate-source --workspace mi-lsp --format toon"
evidence: ".docs/wiki/06_pruebas/TP-WIKI.md"
cases:
  - id: TC-WIKI-017
    type: positivo
    given: "registry con >= 2 workspaces documentados"
    when: "mi-lsp nav wiki pack --all-workspaces --format toon"
    then: "items[] contiene N mini-packs (uno por workspace), no un super-pack fusionado; cada item es un pack completo anotado con workspace; ok=true"

  - id: TC-WIKI-018
    type: positivo
    given: "RF-WIKI-001 mencionado en >= 2 workspaces; ausente en otro"
    when: "mi-lsp nav wiki pack --all-workspaces --rf 'RF-WIKI-001' --format toon"
    then: "filtro --rf aplica per-workspace; workspaces sin ese RF devuelven items=[] o se omiten con hint 'RF-WIKI-001 no encontrado en este workspace'; ok=true"

  - id: TC-WIKI-019
    type: positivo
    given: "múltiples workspaces con documentos de diferentes capas"
    when: "mi-lsp nav wiki pack --all-workspaces --doc '04_RF' --format toon"
    then: "flag --doc filtra per-workspace; cada mini-pack contiene solo documentos RF del workspace respectivo"

  - id: TC-WIKI-020
    type: positivo
    given: "registry con workspaces A y B con doc_count distintos"
    when: "mi-lsp nav wiki pack --all-workspaces --format toon"
    then: "cada mini-pack anotado con 'workspace' y 'doc_count_local'; A.doc_count <> B.doc_count reflejado en items; stats.workspaces_queried >= 2"
```

## Regla de mantenimiento

- Ningun RF-WIKI-* se considera completamente especificado si no tiene al menos 4 test cases positivos trazados en TP-WIKI.
- Cada TC-WIKI-* debe ser navegable desde `nav wiki trace TC-WIKI-XXX`.
