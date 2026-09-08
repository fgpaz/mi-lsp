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
  - '[[RF-WIKI-006]]'
  - '[[RF-WIKI-007]]'
exports:
  - 'TP-WIKI'
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/04_RF/RF-WIKI-001.md
  - .docs/wiki/04_RF/RF-WIKI-002.md
  - .docs/wiki/04_RF/RF-WIKI-003.md
  - .docs/wiki/04_RF/RF-WIKI-004.md
  - .docs/wiki/04_RF/RF-WIKI-005.md
  - .docs/wiki/04_RF/RF-WIKI-006.md
  - .docs/wiki/04_RF/RF-WIKI-007.md
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
- RF-WIKI-006 (map compacto de hubs)
- RF-WIKI-007 (nav wiki-root)

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

  - id: TC-WIKI-044
    type: positivo
    given: "DocRecord con doc_id exacto y otro documento que solo referencia ese ID en su contenido"
    when: "se ejecuta nav wiki search con el ID explícito"
    then: "el documento dueño aparece antes que la referencia; la referencia no recibe autoridad canónica por mención"

  - id: TC-WIKI-045
    type: positivo
    given: "coincidencias exactas de doc_id/block_id/record_id junto con candidatos owner-aware y paths repetidos"
    when: "se ejecuta nav wiki search con --top 1 o con --offset N"
    then: "la secuencia se deduplica por path, conserva la resolución source exacta y aplica offset/top una sola vez sin overflow"

  - id: TC-WIKI-046
    type: positivo
    given: "candidatos en varias capas y una búsqueda con --layer"
    when: "se ejecuta nav wiki search con filtro de capa, offset y límite"
    then: "el filtro se aplica antes de paginar; en workspace único, total_matches cuenta todos los candidatos elegibles y shown_matches los emitidos por el servicio antes de límites posteriores del envelope; el fanout no expone esos campos por workspace"

  - id: TC-WIKI-047
    type: positivo
    given: "un DocRecord dueño con doc_id RF-X-1, una referencia textual completa a RF-X-1, una declaración source exacta y un documento RF-X-10"
    when: "se ejecuta nav wiki search 'RF-X-1'"
    then: "se conservan dueño, referencia completa y source; puntuación de frase, [[RF-X-1]] y RF-X-1.md son referencias válidas, mientras RF-X-10, RF-X-1-EXTRA, RF-X-1_EXTRA, RF-X-1.extra, RF-X-1.md.EXTRA y RF X 1 quedan fuera; la secuencia mantiene score/orden de los candidatos admitidos"

  - id: TC-WIKI-048
    type: negativo
    given: "App.Execute sobre SQLite no vacío con candidato AE-MAINTENANCE sin tokens de la query y otro candidato con evidencia doc_id/search_text"
    when: "se ejecuta nav wiki search con una query natural y después con 'totally-absent-term'"
    then: "solo se admite evidencia FTS o léxica real, incluida una coincidencia legítima en doc_id; el no-hit devuelve items=[] y el hint diagnóstico existente"

  - id: TC-WIKI-049
    type: positivo
    given: "App.Execute sobre SQLite temporal con perfil knowledge/wiki y dos Markdown sin doc_id, mismo heading y paths distintos"
    when: "se ejecuta nav wiki search por tokens del heading"
    then: "ambos documentos se conservan por path/provenance sin forzar un ID de software, y line/evidence apunta al Markdown canónico de cada resultado"

  - id: TC-WIKI-050
    type: positivo
    given: "App.Execute sobre SQLite con block_id dotted CT-SOURCE.contract, record_id no software SOURCE-ALPHA y candidato que solo contiene 'contract'"
    when: "se ejecuta nav wiki search con cada identificador source exacto"
    then: "cada declaración source se conserva aunque no tenga texto coincidente; el candidato con solo 'contract' no entra y lookup_status conserva block_id/record_id exactos"

  - id: TC-WIKI-051
    type: regresion
    given: "SQLite no vacío y query route Tier1 no coincidente 'totally absent route task'"
    when: "se ejecuta nav wiki route"
    then: "route mantiene envelope/backend route y orientación canónica; la admisión search-only no altera fallback ni alias de route"

  - id: TC-WIKI-061
    type: positivo
    given: "SQLite temporal con un candidato parcial elevado por owner_hint y otro candidato con cobertura completa de una query natural de varios términos"
    when: "se ejecuta nav wiki search con la query natural"
    then: "la cobertura completa y la evidencia heading/body ordenan primero al candidato completo; el candidato parcial con routing hint legítimo permanece visible cuando conserva evidencia léxica"

  - id: TC-WIKI-062
    type: positivo
    given: "Markdown canónico con frontmatter, heading y pasaje corporal que contienen los términos consultados"
    when: "se ejecuta nav wiki search con esa query natural"
    then: "evidence y start_line/end_line apuntan a líneas reales con heading/pasaje útil, no a doc_id/imports como única evidencia, y el rango incluye como máximo un vecino contiguo por lado"

  - id: TC-WIKI-063
    type: positivo
    given: "Markdown y DocRecord con cache y daemon en un pasaje corporal después de frontmatter y metadatos Harness"
    when: "se consulta nav wiki search con cache daemon y caché daemon"
    then: "ambas queries naturales tienen cobertura y evidencia corporal equivalentes; el rango apunta al pasaje real y no a metadatos. La igualdad de identificadores permanece literal: ID-ñ coincide consigo mismo, no con ID-n"

  - id: TC-WIKI-064
    type: regresion
    given: "candidatos naturales admitidos y una query literal de DocID/source exacta"
    when: "se ejecutan ambas búsquedas, incluyendo --all-workspaces"
    then: "la búsqueda natural usa el score local para la paginación y el merge global, mientras DocID/source conserva precedencia, lookup_status, counts, layer y offset previos"

  - id: TC-WIKI-065
    type: positivo
    given: "la misma consulta natural ejecutada varias veces sobre el mismo índice y sobre workspaces con resultados equivalentes"
    when: "se comparan scores, paths, workspaces y rangos"
    then: "la salida es determinista; los empates se ordenan por path y después doc_id, y el merge global usa exactamente el score emitido"

  - id: TC-WIKI-066
    type: negativo
    given: "DocRecord vigente cuyo Markdown fue eliminado o quedó stale después del indexado"
    when: "se ejecuta nav wiki search"
    then: "el item y su snippet indexado permanecen, pero line/evidence/rango se omiten sin fabricar una cita ni declarar frescura actual"

  - id: TC-WIKI-067
    type: positivo
    given: "Markdown con RF-X-1.extra, RF-X-1.md, RF-X-1-EXTRA y RF-X-10"
    when: "se indexa y se busca RF-X-1"
    then: "solo la referencia completa RF-X-1 y su enlace .md son válidos; las continuaciones y el prefijo distinto permanecen separados"

  - id: TC-WIKI-068
    type: negativo
    given: "índice documental heredado sin versión vigente del extractor y mención persistida"
    when: "se busca el DocID"
    then: "la mención solo se admite tras confirmar el Markdown canónico, su hash y el límite de lectura; evidencia ausente o stale se omite con advertencia"

  - id: TC-WIKI-069
    type: positivo
    given: "snapshot actual con bytes sin cambios"
    when: "se ejecuta indexación documental explícita"
    then: "se conserva el skip-reparse por content_hash y la versión vigente se publica atómicamente con el snapshot"

  - id: TC-WIKI-070
    type: negativo
    given: "publicación documental marcada para cancelación o con error antes del commit"
    when: "finaliza la publicación"
    then: "las filas y la versión del snapshot se revierten juntas; no se adelanta la confianza del índice"
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

## TP-WIKI-T5 - Routing dirigido y validación Wiki Source

```toon
block_id: tp-wiki-t5-group-e
kind: test-cases
source_of_truth: this
evidence: .docs/wiki/06_pruebas/TP-WIKI.md
verify:
  - go run ./cmd/mi-lsp nav wiki --help
  - go run ./cmd/mi-lsp nav wiki validate-source --workspace <alias> --paths <path> --format toon
  - go run ./cmd/mi-lsp nav wiki validate-source --workspace <alias> --ids <doc-id> --format toon
stop_if:
  - governance_blocked=true
  - wiki_source_verdict=BLOCKED
cases:
  - id: TC-WIKI-021
    type: positivo
    given: "consulta documental sobre graph impact y contracts"
    when: "mi-lsp nav wiki route '<task>' --workspace <alias> --format toon"
    then: "route selecciona owner documental y devuelve primary doc, stages relacionados, warnings y next_queries"
  - id: TC-WIKI-022
    type: positivo
    given: "consulta que requiere contexto de gobernanza, scope, arquitectura y detalle tecnico"
    when: "mi-lsp nav wiki pack '<task>' --workspace <alias> --full --format toon"
    then: "pack conserva la autoridad de la wiki y explicita stages, documentos y warnings de graph context si no esta disponible"
  - id: TC-WIKI-023
    type: positivo
    given: "paths existentes que incluyen artefactos SDD-WIKI-SOURCE-v1"
    when: "validate-source --paths <path-1>,<path-2>"
    then: "solo se seleccionan artefactos source-declarados y el resultado expone readiness, verdict, bloques y navigation_readiness"
  - id: TC-WIKI-024
    type: positivo
    given: "IDs existentes de documentos source y duales"
    when: "validate-source --ids <doc-id-1>,<doc-id-2>"
    then: "el filtro acepta IDs; los documentos sin declaracion SDD-WIKI-SOURCE-v1 no se convierten implicitamente en source artifacts"
  - id: TC-WIKI-025
    type: negativo
    given: "path o doc-id inexistente"
    when: "validate-source --paths .docs/wiki/does-not-exist.md o --ids DOES-NOT-EXIST"
    then: "ok=true pero wiki_source_verdict=BLOCKED, navigation_readiness=blocked y navigation_blockers incluye scope=no_match"
  - id: TC-WIKI-026
    type: negativo
    given: "artifact source sin doc_id, block_id, fence toon o evidencia durable"
    when: "validate-source revisa el documento"
    then: "verdict=BLOCKED y el blocker identifica el campo o evidencia faltante"
  - id: TC-WIKI-027
    type: positivo
    given: "preview documental con cobertura parcial"
    when: "route/search/pack devuelven resultados truncados"
    then: "la respuesta incluye next_queries o next_hint y no oculta la omision"
  - id: TC-WIKI-028
    type: negativo
    given: "governance_blocked=true o index documental no listo"
    when: "se invoca una superficie wiki dirigida"
    then: "la operacion se detiene o degrada con diagnostico explicito; no usa fallback silencioso a contenido no gobernado"
```

```toon
block_id: tp-wiki-knowledge-navigation-v1
kind: test-cases
source_of_truth: this
evidence: .docs/wiki/09_contratos/CT-NAV-WIKI.md
verify:
  - mi-lsp nav wiki search "motivo de la decisión" --workspace <alias> --top 5 --format toon
  - mi-lsp nav wiki map --workspace <alias> --max-items 12 --format toon
  - mi-lsp nav wiki search "sustitución" --workspace <alias> --include-content --top 5 --format toon
  - mi-lsp nav multi-read .docs/wiki/04_RF/RF-WIKI-006.md:1-120 --workspace <alias> --format toon
cases:
  - id: TC-WIKI-041
    type: positivo
    given: "wiki Markdown canónica con referencias textuales sobre decisiones, fuentes, sustituciones y aprendizajes"
    when: "se ejecuta el flujo search -> map -> search -> multi-read sin proveedor semántico"
    then: "search devuelve coincidencias textuales bounded, map devuelve hubs sin cuerpos y multi-read devuelve solo el rango solicitado; no se reporta semantic recall"
  - id: TC-WIKI-042
    type: negativo
    given: "el catálogo documental está stale"
    when: "se ejecuta nav graph stats o una consulta de vecinos"
    then: "stats falla cerrado con GPH_QUERY_GRAPH_INVALID; neighbors puede degradar explícitamente a mode=query_only sin claims graph; next_hint orienta a index --docs-only o index completo, sin rebuild destructivo ni bloqueo de wiki search/map"
  - id: TC-WIKI-043
    type: negativo
    given: "el alias solicitado no está registrado o su root ya no existe"
    when: "se ejecuta una superficie wiki dirigida"
    then: "devuelve diagnóstico explícito de workspace ausente; no finge federación ni reindexa otro repositorio"
```

```toon
block_id: tp-wiki-rf-006-cases
kind: test-cases
rf: RF-WIKI-006
title: "Casos para RF-WIKI-006 (nav wiki map)"
source_of_truth: this
verify: "go test ./internal/service ./internal/cli ./internal/docgraph ./internal/indexer -count=1 -run 'TestClassifyWikiMapHubDefaultsRemainLocked|TestGroupWikiMapDocsUsesDefaultOrCustomDeclarationOrder|TestTrimWikiMapHubs|TestWalkWikiMapDocs|TestNavWikiMapUsesDirectExecution|TestKnowledgeWikiRoots|TestDocumentGraph'"
evidence: ".docs/wiki/06_pruebas/TP-WIKI.md"
cases:
  - id: TC-WIKI-029
    type: positivo
    given: "wiki/00-identidad.md, wiki/10-proyecto.md y bibliotecas/memorias/ficha.md"
    when: "mi-lsp nav wiki map --workspace <alias> --format toon"
    then: "backend=wiki.map; hubs en orden persona, proyectos, materia; cada doc tiene path y title; sin cuerpos markdown"
  - id: TC-WIKI-030
    type: positivo
    given: "índice documental vacío pero las raíces configuradas existen en disco"
    when: "nav wiki map"
    then: "walk bounded; warning de fallback; ignore/symlink/reparse respetados; hubs no vacíos si hay Markdown clasificable"
  - id: TC-WIKI-031
    type: positivo
    given: "wiki/30-dashboard.md y wiki/20-proyectos-activos.md de primer nivel"
    when: "classifyWikiMapHub"
    then: "ambos clasifican a sistema; 24-aprendizaje anidado no entra"
  - id: TC-WIKI-032
    type: positivo
    given: "token-budget menor que el catálogo completo"
    when: "nav wiki map --token-budget N"
    then: "búsqueda binaria devuelve el mayor conteo equitativo que cabe; truncated=true; totales, reason y next_hint son explícitos"
  - id: TC-WIKI-033
    type: negativo
    given: "wiki/31-workers, wiki/32-contratos, yaml o .docs/wiki/00_gobierno_documental.md"
    when: "classifyWikiMapHub"
    then: "hub vacío; no aparecen en el mapa"
  - id: TC-WIKI-034
    type: positivo
    given: "read-model.toml con roots y hubs declarados en wiki_map.hub válidos"
    when: "se carga el perfil y se ejecuta nav wiki map"
    then: "wiki/ y bibliotecas/ permanecen, las roots extra se agregan; solo los hubs custom aparecen en orden declarado y gana el primer patrón coincidente"
  - id: TC-WIKI-035
    type: negativo
    given: "roots o patterns absolutos, con traversal o glob malformado"
    when: "se carga read-model.toml"
    then: "se omiten con warnings estables que no exponen el valor rechazado"
  - id: TC-WIKI-036
    type: positivo
    given: "hubs no vacíos de tamaños desbalanceados y max_items suficiente para una ronda"
    when: "nav wiki map --max-items N"
    then: "todos los hubs reciben representación; se conserva orden de docs y hubs; total_returned=N"
  - id: TC-WIKI-037
    type: positivo
    given: "CLI nueva o un daemon anterior sin nav.wiki.map"
    when: "mi-lsp nav wiki map"
    then: "operation=nav.wiki.map se ejecuta directo con preferDaemon=false"
  - id: TC-WIKI-038
    type: negativo
    given: "wiki_map.enabled=false"
    when: "se indexa o consulta el mapa"
    then: "no se agregan automáticamente wiki/ ni bibliotecas/ y el mapa devuelve hint de deshabilitado"
  - id: TC-WIKI-039
    type: positivo
    given: "wikilinks, embeds y Markdown links con anchors/alias dentro y fuera de fences"
    when: "docgraph extrae referencias"
    then: "solo referencias fuera del fence generan edges; anchors/alias son menciones y self-anchor no genera self-edge"
  - id: TC-WIKI-040
    type: negativo
    given: "basename o doc_id con múltiples documentos candidatos"
    when: "docgraph y Graph Kernel resuelven el enlace"
    then: "no eligen silenciosamente; ambiguous_doc_target contiene candidatos sorted y bounded"
```

```toon
block_id: tp-wiki-rf-007-cases
kind: test-cases
rf: RF-WIKI-007
title: "Casos para RF-WIKI-007 (nav wiki-root)"
source_of_truth: this
verify: "go test ./internal/cli ./internal/service -count=1 -run 'TestNavWikiRootCommandExists|TestWikiRootResolvesCanonRelativeToWorkspaceRoot|TestWikiRootNoCanonDefaults|TestWikiRootRoleMissingMatchErrors'"
evidence: ".docs/wiki/06_pruebas/TP-WIKI.md"
cases:
  - id: TC-WIKI-034
    type: positivo
    given: "declaración canon con id=wiki, root=../wiki-repo/Ingenieria y role=producto"
    when: "mi-lsp nav wiki-root --workspace <alias> --format toon"
    then: "backend=wiki-root; wiki_root=../wiki-repo/Ingenieria; resolved_from=canon.wiki; governance_doc=../wiki-repo/Ingenieria/00_gobierno_documental.md; paths portables"
  - id: TC-WIKI-035
    type: positivo
    given: "project.toml sin declaración canon ni CanonLinks"
    when: "nav wiki-root"
    then: "wiki_root=.docs/wiki; governance_doc=.docs/wiki/00_gobierno_documental.md; resolved_from=default; ok=true"
  - id: TC-WIKI-036
    type: positivo
    given: "comando alias nav wiki root con --role producto"
    when: "mi-lsp nav wiki root --workspace <alias> --role producto --format toon"
    then: "operation=nav.wiki-root; filtra por role; envelope type sin cambio"
  - id: TC-WIKI-037
    type: negativo
    given: "--role ecosistema sin declaración canon ni link de ese role"
    when: "nav wiki-root --role ecosistema"
    then: "error fail-closed; no inventa default silencioso para ese role"
```

## Regla de mantenimiento

- Ningun RF-WIKI-* se considera completamente especificado si no tiene al menos 4 test cases positivos trazados en TP-WIKI.
- Cada TC-WIKI-* debe ser navegable desde `nav wiki trace TC-WIKI-XXX`.

## TP-WIKI-T10 - Cierre ejecutado del puente documental

```toon
doc_id: TP-WIKI
block_id: TP-WIKI.t10-live-wiki-code-bridge
kind: executed-acceptance-map
source_of_truth: this
status: implemented_and_executed
verification_basis:
  code_commit: e1835ee
  go_test_all: PASS
  go_test_packages: 28
  git_diff_check: PASS
  binary_sha256: 2f7d94cd1eec05b0055184cc05452725831e5b65404c4a095b7bc01717c36a1a
bridge_campaign:
  schema: wiki-code-bridge-runner/v1
  status: PASS
  case_inventory_count: 37
  stable_digest_runs: 30
  stable_digest: 0f32b7e85bb0bcbd727424321e0a272c27e6a94aca6c11c78bd12541bd9255a2
surfaces:
  trace: [nav.trace, nav.wiki.trace]
  pack: [nav.pack, nav.wiki.pack]
  nav.wiki.trace:
    status: PASS
    result: path_bearing_direct_tests_supporting_and_omissions_are_additive
  nav.wiki.pack:
    status: PASS
    result: existing_pack_primary_doc_and_bridge_context_preserved
bridge_not_attached_in_this_slice: [nav.wiki.search, nav.wiki.route]
identity_policy: governed_document_identity_remains_primary
records:
  path_bearing: true
  fields: [doc_id, path, block_id, target_path, target_symbol, relation, role, binding_ref]
  no_fabricated_record_id: true
policy:
  wiki_authority: canonical
  catalog_graph_sqlite: derived
  raw_and_audit: excluded_from_primary_results
  planned: nonnavigable_by_default
  retired: excluded_by_default
  historical: explicit_old_id_or_path_redirect
acceptance:
  edit_add_remove_delete_overlay: PASS
  stale_graph_and_unknown_state_omissions: PASS
  modern_js_extensions: PASS
  no_query_writes: PASS
  direct_daemon_parity: PASS
  deterministic_30_runs: PASS
  latency_and_cost: PASS
cost:
  bindings_examined: 3
  bytes_read: 3047
  catalog_queries: 5
  files_checked: 3
  files_hashed: 4
  files_parsed: 1
  metadata_checked: 1
  semantic_backend_calls: 0
latency_ms:
  samples: 30
  warm_direct_binding_lookup_p95: {value: 80, target: 100, status: PASS}
  warm_mixed_neighbors_p95: {value: 69, target: 1000, status: PASS}
  dirty_single_file_overlay_p95: {value: 75, target: 250, status: PASS}
  cold_direct_lookup: 85.657
  cold_reverse_lookup: 79.018
  warm_reverse_lookup_p95: 83
validator_boundary:
  governance: PASS_in_sync_valid
  full_workspace_validate_harness: PASS_124_contracts_858_links_0_blockers
  full_workspace_validate_source: PASS_31_artifacts_71_records_0_blockers
  canonical_drift_repaired: true
  verification_scope: full_workspace
  next_action: none
verification_semantics:
  sanitized_runner_metrics_are_evidence: true
  raw_prompt_plan_and_host_paths_are_not_authority: true
  graph_v1: consumed_read_only_subset_only
verify:
  - "FINAL_VERIFY code basis e1835ee: go test ./... PASS; git diff --check PASS"
  - "sanitized FINAL_VERIFY result: wiki-code-bridge-runner/v1 PASS; inventory=37"
evidence:
  - internal/service/app.go
  - internal/service/trace.go
  - internal/service/pack.go
  - internal/service/wiki_code_vertical_test.go
  - internal/service/wiki_code_bindings_test.go
  - internal/indexer/wiki_code_incremental_test.go
  - .docs/wiki/06_pruebas/TP-WIKI.md
```

## TP-WIKI-GRAPH-001 — grafo documental explícito

- **Dado** un Markdown con listas `related`, `depends`, `supports`, `contradicts` y `supersedes`, **cuando** se indexa, **entonces** se conservan las cinco relaciones como aristas tipadas y no como `doc_mentions`.
- **Dado** un destino por `doc_id` o ruta relativa, **cuando** se consulta `nav.neighbors` por ID o ruta, **entonces** se resuelve el documento, se exponen relación, destino y status, y una colisión de ID queda ambigua sin auto-selección.
- **Dado** un ciclo, un destino roto o una generación stale, **cuando** se navega con profundidad, budget y cursor, **entonces** la expansión queda acotada, el unresolved es visible y permanece disponible el fallback textual.
- **Dado** un documento genérico bajo `wiki/` o `bibliotecas/` sin ID SDD, **cuando** se indexa, **entonces** la identidad de ruta basta para navegarlo.
- **Contrato de escritura:** incluir `doc_id`, `summary`, `status` y líneas de relación explícitas; actualizar antes de duplicar un ID; `supersedes` conserva el historial.

Fixtures: `testdata/documentary-graph/software/.docs/wiki/` y `testdata/documentary-graph/generic/{wiki,bibliotecas}/`.

```toon
block_id: tp-wiki-document-identity-cases
kind: test-cases
rf: RF-WIKI-001
source_of_truth: this
identity_evidence_precedence: [yaml_frontmatter_doc_id, leading_harness_doc_id, leading_harness_id, source_protocol_doc_id, legacy_id_leading_title, exact_filename_stem]
references_are_not_ownership: [imports, wikilinks, body_ids, source_block_id, source_record_id]
cases:
  - id: TC-WIKI-052
    type: regresion
    given: "importador legacy con id propio y una referencia [[TECH-DAEMON-GOBERNANZA]]"
    when: "se extrae el DocRecord"
    then: "DocID es el id propio; la importación permanece como mención/arista y no como propietario canónico"
  - id: TC-WIKI-053
    type: negativo
    given: "Markdown sin declaración propia, H1 descriptivo, links y ejemplos de id en el cuerpo"
    when: "se extrae el DocRecord"
    then: "DocID queda vacío y la identidad por path/título se conserva"
  - id: TC-WIKI-054
    type: positivo
    given: "frontmatter doc_id explícito, id fallback, ID Unicode/no estándar y referencias de cuerpo"
    when: "se extrae el DocRecord"
    then: "doc_id declarado prevalece; id Unicode se conserva sin normalización ni regex de familias"
  - id: TC-WIKI-055
    type: negativo
    given: "dos declaraciones de identidad contradictorias"
    when: "se extrae el DocRecord"
    then: "DocID queda vacío; no se elige silenciosamente una declaración"
  - id: TC-WIKI-056
    type: positivo
    given: "documento SDD con owner CT-OWNER, block_id distinto y record_id distinto"
    when: "se indexa"
    then: "DocRecord conserva CT-OWNER y source block/record conservan sus IDs y menciones separadas"
  - id: TC-WIKI-057
    type: regresion
    given: "DocRecord previo con owner incorrecto pero mismo content_hash"
    when: "se ejecuta index docs-only y el reemplazo atómico existente"
    then: "la extracción vigente sustituye Docs, source tables y FTS sin snapshot parcial; no hay reindex automático desde navegación"
  - id: TC-WIKI-058
    type: negativo
    given: "frontmatter o fenced Harness YAML sin cierre"
    when: "se extrae el documento"
    then: "el recorrido termina de forma acotada y no inventa DocID desde el cuerpo"
  - id: TC-WIKI-059
    type: negativo
    given: "declaraciones doc_id duplicadas con valores distintos, frente a duplicados idénticos"
    when: "se extrae el documento"
    then: "el conflicto queda sin propietario; el duplicado idéntico permanece estable"
  - id: TC-WIKI-060
    type: regresion
    given: "SQLite publicado desde extracción docgraph fresca con owner e importer reales"
    when: "App.Execute ejecuta nav.wiki.search por el owner"
    then: "owner exacto aparece una vez y el importer conserva su DocID propio como referencia, sin duplicar autoridad canónica"
refresh_command: "mi-lsp index --workspace <alias> --docs-only"
verification_note: "La verificación usa una fixture SQLite completa, extracción docgraph fresca y confirma owner exacto más referencia importer."
```

```toon
doc_id: TP-WIKI
block_id: tp-wiki-rf-001-implementation-oracles
kind: implementation-oracle-map
source_of_truth: this
rf: RF-WIKI-001
implementation_paths:
  relevance_and_line_evidence: [internal/service/wiki_search.go, internal/store/queries_docs.go]
  identity_and_references: [internal/docidentity/identity.go, internal/docgraph/docgraph.go, internal/wikisource/parser.go]
  publication_and_freshness: [internal/store/index_publish.go, internal/store/meta.go, internal/store/doc_snapshot.go, internal/store/queries_incremental.go]
  indexer_wiring: [internal/indexer/indexer.go]
test_oracles:
  search: internal/service/wiki_search_test.go
  identity: [internal/docidentity/identity_test.go, internal/docgraph/identity_test.go, internal/docgraph/reference_identity_test.go]
  parser: internal/wikisource/parser_test.go
  snapshot: [internal/indexer/indexer_test.go, internal/store/doc_identity_snapshot_test.go]
  declared_cases: [TC-WIKI-044..070]
trace_rules:
  owner_identity: "doc_id declarado precede imports, wikilinks, body IDs, block_id y record_id"
  source_identity: "block_id y record_id permanecen separados del propietario DocRecord"
  natural_query: "solo evidencia FTS/léxica admite; routing hints solo ordenan"
  snapshot_trust: "marker vigente habilita menciones; marker ausente/antiguo exige hash y lectura canónica acotada"
  graph_boundary: "atomicidad documental no promete atomicidad de activación graph"
validation_basis: attested_prior_review_without_reexecution
canonical_evidence: source_and_test_paths
not_canonical: [temporary_logs, installed_index, live_refresh, embeddings]
```
