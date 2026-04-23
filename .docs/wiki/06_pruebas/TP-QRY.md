# TP-QRY

## Cobertura objetivo

- RF-QRY-001
- RF-QRY-002
- RF-QRY-003
- RF-QRY-004
- RF-QRY-005
- RF-QRY-006
- RF-QRY-007
- RF-QRY-008
- RF-QRY-009
- RF-QRY-010
- RF-QRY-011
- RF-QRY-012
- RF-QRY-013
- RF-QRY-014
- RF-QRY-015
- RF-QRY-016

## Casos

| Caso | Tipo | RF | Descripcion |
|---|---|---|---|
| TC-QRY-001 | positivo | RF-QRY-001 | emite envelope estable con campos obligatorios |
| TC-QRY-002 | positivo | RF-QRY-001 | trunca de forma determinista con `next_hint` |
| TC-QRY-003 | negativo | RF-QRY-001 | rechaza presupuestos invalidos |
| TC-QRY-004 | positivo | RF-QRY-002 | usa daemon saludable cuando esta disponible para queries semanticas o compuestas |
| TC-QRY-005 | positivo | RF-QRY-002 | hace fallback directo si el daemon no responde para una query daemon-aware |
| TC-QRY-006 | negativo | RF-QRY-002 | falla cuando no existe backend ejecutable |
| TC-QRY-007 | positivo | RF-QRY-002 | enruta `nav refs` sobre `.py` a pyright si esta disponible |
| TC-QRY-008 | positivo | RF-QRY-002 | degrada a catalog/text con warning si pyright no esta instalado |
| TC-QRY-009 | positivo | RF-QRY-003 | resume endpoints, consumers, publishers, entidades e infraestructura de un servicio con evidencia estructurada |
| TC-QRY-010 | positivo | RF-QRY-003 | oculta placeholders de arquetipo por default y los incluye con `--include-archetype` |
| TC-QRY-011 | negativo | RF-QRY-003 | devuelve warning accionable si no hay catalogo util o no se encuentra evidencia suficiente bajo el path |
| TC-QRY-012 | positivo | RF-QRY-002 | `nav context` sobre `ts/tsx` devuelve `slice_text` y warning si `tsserver` no esta disponible |
| TC-QRY-013 | positivo | RF-QRY-002 | `nav search` sin matches devuelve `ok=true` e insinua `--regex` cuando el patron parece regex |
| TC-QRY-013A | positivo | RF-QRY-001 | `nav.search` agrega `coach.trigger=no_matches_refinable` cuando la query no matchea pero tiene rerun accionable |
| TC-QRY-014 | positivo | RF-QRY-004 | lee multiples rangos en una sola invocacion con truncacion por presupuesto |
| TC-QRY-015 | positivo | RF-QRY-004 | incluye numeros de linea en contenido leido |
| TC-QRY-016 | negativo | RF-QRY-004 | rechaza path traversal (`../../../etc/passwd`) |
| TC-QRY-017 | positivo | RF-QRY-005 | ejecuta batch con operaciones paralelas y retorna todos los resultados |
| TC-QRY-018 | positivo | RF-QRY-005 | continua si una operacion batch falla, devuelve resultados parciales |
| TC-QRY-019 | negativo | RF-QRY-005 | rechaza stdin > 10MB |
| TC-QRY-020 | positivo | RF-QRY-006 | devuelve vecindario semantico con definicion, callers, implementors, tests |
| TC-QRY-021 | positivo | RF-QRY-006 | degrada a sintactico con warning si backend semantico no disponible |
| TC-QRY-022 | negativo | RF-QRY-006 | rechaza simbolo no encontrado con sugerencia de busqueda |
| TC-QRY-023 | positivo | RF-QRY-007 | genera mapa de workspace con servicios, endpoints, eventos, dependencias |
| TC-QRY-024 | positivo | RF-QRY-007 | devuelve mapa parcial con warning si catalogo incompleto |
| TC-QRY-025 | negativo | RF-QRY-007 | rechaza workspace invalido |
| TC-QRY-026 | positivo | RF-QRY-008 | devuelve archivos cambiados y simbolos afectados en diff |
| TC-QRY-027 | positivo | RF-QRY-008 | incluye contenido modificado con --include-content |
| TC-QRY-028 | negativo | RF-QRY-008 | warning si no hay cambios o git no disponible |
| TC-QRY-029 | positivo | RF-QRY-009 | busca simbolos en todos los workspaces con --all-workspaces |
| TC-QRY-030 | positivo | RF-QRY-009 | degrade si algunos workspaces fallan, devuelve resultados parciales |
| TC-QRY-031 | negativo | RF-QRY-009 | rechaza cross-workspace sin --all-workspaces flag |
| TC-QRY-032 | positivo | RF-QRY-010 | `nav ask` prioriza el documento canonico correcto y devuelve evidencia de codigo |
| TC-QRY-033 | positivo | RF-QRY-010 | `nav ask` usa `.docs/wiki/_mi-lsp/read-model.toml` cuando existe |
| TC-QRY-034 | negativo | RF-QRY-010 | `nav ask` degrada a fallback generico o textual cuando falta corpus fuerte |
| TC-QRY-034A | positivo | RF-QRY-010 | `nav ask` fallback textual emite `coach.trigger=text_fallback` con `confidence=low` |
| TC-QRY-035 | positivo | RF-QRY-002 | `nav find` responde por catalogo aunque el daemon este caido o detenido |
| TC-QRY-036 | positivo | RF-QRY-002 | `nav search`, `nav.symbols`, `nav.outline`, `nav.overview` y `nav.multi-read` no auto-inician daemon y mantienen salida estable |
| TC-QRY-037 | positivo | RF-QRY-002 | `nav find` y `nav search` aceptan `--repo` en workspaces `container` y acotan resultados sin depender del daemon |
| TC-QRY-038 | negativo | RF-QRY-002 | `nav find/search/intent --repo` desconocido devuelve `backend=router`, candidatos y `next_hint` |
| TC-QRY-039 | positivo | RF-QRY-010 | `nav ask` emite `next_queries` con `--repo` cuando la evidencia apunta a un repo unico del workspace `container` |
| TC-QRY-040 | positivo | RF-QRY-011 | `nav intent --repo` acota candidatos al repo seleccionado y conserva output compacto |
| TC-QRY-041 | negativo | RF-QRY-011 | `nav intent` rechaza pregunta vacia con error explicito |
| TC-QRY-042 | positivo | RF-QRY-001 | `nav search` usa TOON por default en superficie AXI-default y agrega guidance de expansion con `--full` |
| TC-QRY-043 | positivo | RF-QRY-010 | `nav ask` en pregunta de orientacion condensa evidencia inicial y evita `--axi` redundante en `next_queries` |
| TC-QRY-043A | positivo | RF-QRY-010 | `nav ask` en AXI preview con evidencia condensada puede emitir `coach.trigger=preview_trimmed` con una sola accion |
| TC-QRY-044 | positivo | RF-QRY-011 | `nav intent` mantiene ranking base pero expone `next_hint` para `--full` por default |
| TC-QRY-045 | positivo | RF-QRY-011 | `nav intent --classic` restaura la salida clasica y mantiene envelope estable |
| TC-QRY-046 | positivo | RF-QRY-010 | `nav ask` con pregunta de implementacion queda clasico por default salvo `--axi` |
| TC-QRY-047 | positivo | RF-QRY-007 | `nav workspace-map` sigue clasico por default y solo anuncia preview/full cuando se fuerza `--axi` |
| TC-QRY-047A | positivo | RF-QRY-002, RF-QRY-007 | `nav workspace-map` no auto-inicia ni enruta por daemon en el modo summary-first por default |
| TC-QRY-048 | positivo | RF-QRY-012 | `nav pack` construye un reading pack funcional en orden canonico desde tarea libre |
| TC-QRY-049 | positivo | RF-QRY-012 | `nav pack --full` expande slices legibles del mismo pack sin cambiar el backend |
| TC-QRY-050 | negativo | RF-QRY-012 | `nav pack` devuelve warning accionable cuando la wiki canonica existe pero el indice documental esta vacio o stale |
| TC-QRY-051 | positivo | RF-QRY-013 | `nav governance` devuelve perfil efectivo, overlays, sync y siguientes pasos |
| TC-QRY-052 | negativo | RF-QRY-013 | `nav ask` y `nav pack` bloquean y devuelven estado de gobernanza cuando `00` falta o el indice esta stale |
| TC-QRY-053 | negativo | RF-QRY-014 | `TestNavRouteRequiresTask`: `nav route` sin argumento de tarea devuelve error explicito (`QRY_ROUTE_TASK_REQUIRED`) |
| TC-QRY-054 | positivo | RF-QRY-014 | `TestNavRouteReturnsCanonicalDocFromGovernance`: `nav route <task>` resuelve `anchor_doc` desde governance/read-model (Tier 1) cuando el indice no esta disponible |
| TC-QRY-055 | positivo | RF-QRY-014 | `TestNavRoutePreviewModeByDefault`: sin flags, el modo es `preview` y `discovery` puede estar ausente |
| TC-QRY-056 | positivo | RF-QRY-014 | `TestNavRouteFullModeActivatesWithFlag`: `--full` expande canonical lane y activa discovery |
| TC-QRY-057 | positivo | RF-QRY-015 | `TestNavRouteUsesTaskFallbackFromQuestion`: el route core extrae familia desde la pregunta cuando no hay anchor explicito |
| TC-QRY-058 | positivo | RF-QRY-012 | `TestNavPackNextQueriesArePopulated`: `nav pack` popula `next_queries` con al menos un elemento que empieza con `mi-lsp` |
| TC-QRY-059 | positivo | RF-QRY-012 | `TestNavPackExplicitRFAnchorWinsOverRouteCore`: anchor `--rf` explicito en payload sobreescribe el anchor del route core y determina `primary_doc` |
| TC-QRY-060 | positivo | RF-QRY-014 | `TestNavRouteAnchorDocHasAnchorStage`: `AnchorDoc.Stage == "anchor"` en Tier 1 y Tier 2 (Wave 3b stage signal) |
| TC-QRY-061 | positivo | RF-QRY-014 | `TestNavRoutePreviewPackHasPreviewStage`: cada doc del `PreviewPack` lleva campo `Stage` no vacio |
| TC-QRY-062 | positivo | RF-QRY-014 | `TestNavRouteDiscoveryDocsHaveDiscoveryStage`: cuando `discovery.docs` existe, cada doc tiene `Stage == "discovery"` |
| TC-QRY-063 | positivo | RF-QRY-014, RF-QRY-015 | queries naturales sobre capabilities nuevas (`continuation`, `memory_pointer`) priorizan docs owner-aware del slice y no `README` cuando existe match canonico positivo |
| TC-QRY-063A | positivo | RF-QRY-010, RF-QRY-011, RF-QRY-014, RF-QRY-015 | superficies docs-first (`nav ask`, `nav route`, `nav pack`, `nav intent`) orientadas a auditoria/wiki-to-code parity priorizan `.docs/wiki/*` y no dejan que `.docs/raw/*` gane el documento primario cuando existe match canonico positivo |
| TC-QRY-064 | positivo | RF-QRY-011 | `nav intent` clasifica `mode=docs` para consultas capability-like y devuelve items documentales con `doc_path/doc_id/title/family/layer/score/evidence/next_queries` |
| TC-QRY-065 | positivo | RF-QRY-011 | `nav intent` clasifica `mode=code` para consultas symbol-like y conserva ranking BM25 de catalogo |
| TC-QRY-066 | positivo | RF-QRY-014, RF-QRY-015 | `MI_LSP_DOC_RANKING=legacy` deja un override diagnostico reversible y no persiste hints/queries crudas en telemetria |
| TC-QRY-067 | positivo | RF-QRY-010, RF-QRY-012 | `continuation`, `memory_pointer` y `memory_pointer.stale` siguen visibles en las superficies docs-first despues del reranking owner-aware |
| TC-QRY-068 | positivo | RF-QRY-014 | `TestNavRouteExplicitEmbeddedRFUsesContainingRFDocWhenDocsIndexEmpty`: `nav route RF-*` ancla el documento RF agregado aunque el indice documental este vacio |
| TC-QRY-069 | positivo | RF-QRY-013 | `TestNavTraceFindsRFEmbeddedInAggregateDoc`: `nav trace RF-*` resuelve IDs mencionados dentro de documentos agregados via `doc_mentions` |
| TC-QRY-070 | positivo | RF-QRY-014 | `TestNavRoutePreservesExplicitEmbeddedRFWhenDocsIndexExists`: Tier 2 no reemplaza el RF explicito por el indice general `04_RF.md` |
| TC-QRY-071 | positivo | RF-QRY-013 | `TestNavTracePrefersAggregateRFDocOverRFIndexDoc`: `nav trace` prioriza el doc bajo `04_RF/` sobre el indice general cuando ambos mencionan el RF |
| TC-QRY-071A | positivo | RF-QRY-013 | `nav trace RF-*` hace fallback a `.docs/wiki/04_RF*.md` cuando el RF existe en disco pero todavia no figura en `doc_records`/`doc_mentions` |
| TC-QRY-071B | positivo | RF-QRY-013 | `nav trace RF-*` hace fallback al layout legacy `.docs/wiki/RF/*.md` cuando el RF existe en disco pero el indice documental aun no lo publico |
| TC-QRY-071C | positivo | RF-QRY-013 | `nav trace RF-*` hace fallback al indice root legacy `.docs/wiki/RF.md` cuando el RF existe en disco pero el indice documental aun no lo publico |
| TC-QRY-072 | negativo | RF-QRY-014 | `TestNavRouteDoesNotAttachMissingExplicitRFToGovernanceFallback`: un `RF-*` inexistente no se pega como `doc_id` al fallback de gobernanza |
| TC-QRY-073 | positivo | RF-QRY-016 | `TestNavWikiSearchReturnsLayerFilteredDocs`: `nav wiki search` devuelve candidatos filtrados por capa con `next_queries` hacia pack/trace/multi-read |
| TC-QRY-074 | negativo | RF-QRY-016 | `TestNavWikiSearchDocIndexEmptyReturnsDiagnostic`: docgraph vacio devuelve diagnostico accionable de `index --docs-only` |
| TC-QRY-075 | negativo | RF-QRY-016 | `TestNavWikiSearchBlocksWhenGovernanceBlocked`: governance bloqueada corta `nav wiki search` |
| TC-QRY-076 | positivo | RF-QRY-016 | `TestNavAskRoutePackRepoCompatWarnings`: `nav ask|route|pack --repo docs` no falla y orienta a `nav wiki` |
