# TP-QRY

```yaml
harness_protocol: SDD-HARNESS-v1
id: "TP-QRY"
kind: "support-doc"
audience: "llm-first"
imports:
  - '[[00_gobierno_documental]]'
  - '[[TP-QRY]]'
exports:
  - 'TP-QRY'
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/06_pruebas/TP-QRY.md
agent_may_edit:
  - .docs/wiki/06_pruebas/TP-QRY.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
evidence:
  - .docs/wiki/06_pruebas/TP-QRY.md
```

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
- RF-QRY-017

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
| TC-QRY-008A | positivo | RF-QRY-002 | `TestNavContext_GoFileUsesGoplsBackend`: `nav context` sobre `.go` pide backend `gopls` y conserva `slice_text` enriquecido |
| TC-QRY-008B | positivo | RF-QRY-002 | `TestNavContext_GoplsUnavailableFallsBackToSlice`: si `gopls` falta, `nav context` degrada a catalog/text con warning accionable |
| TC-QRY-009 | positivo | RF-QRY-003 | resume endpoints, consumers, publishers, entidades e infraestructura de un servicio con evidencia estructurada |
| TC-QRY-009A | positivo | RF-QRY-003 | `TestNavServiceGoPackageUsesCatalogProfile`: `nav service` perfila paquetes Go como `go-package` y evita falsos endpoints .NET desde fixtures o strings |
| TC-QRY-009B | positivo | RF-QRY-003 | `TestNavServiceGoPackageDetectsGoHTTPAndCLIInfra`: `nav service` detecta endpoints Go reales, `http.ListenAndServe` y Cobra sin contar rutas dentro de raw strings |
| TC-QRY-010 | positivo | RF-QRY-003 | oculta placeholders de arquetipo por default y los incluye con `--include-archetype` |
| TC-QRY-011 | negativo | RF-QRY-003 | devuelve warning accionable si no hay catalogo util o no se encuentra evidencia suficiente bajo el path |
| TC-QRY-012 | positivo | RF-QRY-002 | `nav context` sobre `ts/tsx` devuelve `slice_text` y warning si `tsserver` no esta disponible |
| TC-QRY-013 | positivo | RF-QRY-002 | `nav search` sin matches devuelve `ok=true` e insinua `--regex` cuando el patron parece regex |
| TC-QRY-013B | positivo | RF-QRY-002 | `nav search` incluye docs gobernados y artefactos repo-locales ocultos aun cuando el repo use directorios hidden, para que un `index --docs-only` deje visibles IDs `RF-*` y `TP-*` en la superficie textual directa |
| TC-QRY-013C | positivo | RF-QRY-002, RF-QRY-016 | `nav search` literal symbol-like emite `coach.trigger=symbol_query_detected` con acciones `nav find --exact` y `nav related` |
| TC-QRY-013D | positivo | RF-QRY-002, RF-QRY-016 | `nav search` symbol-like ordena declaraciones/implementaciones fuente antes que docs, tests, backups y generados |
| TC-QRY-013E | positivo | RF-QRY-002 | `nav context` sobre C# conserva `slice_text` y degrada a catalog/text con warning `backend_runtime/process_spawn_access_denied` si Roslyn no arranca |
| TC-QRY-013F | positivo | RF-QRY-002 | `nav search` cae a Go search con warning tipado si `rg` falla por permisos o arranque de proceso |
| TC-QRY-013H | positivo | RF-QRY-002 | `nav search` con `rg --hidden` preserva `.docs` pero excluye caches/dependencias generadas (`.git`, `.next`, `.turbo`, `node_modules`, `bin/obj`, venvs, worktrees temporales) para evitar latencia espuria |
| TC-QRY-013G | positivo | RF-QRY-002 | `TestParseContextTargetAcceptsFileLineShorthand`: `nav context` acepta `file.go:123` y devuelve guidance corregida si la linea es invalida |
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
| TC-QRY-047B | positivo | RF-QRY-007 | `TestWorkspaceMapGoPackageServices`: `nav workspace-map` expone paquetes Go `cmd/*`, `internal/*` y `pkg/*` como servicios `go-package` desde el catalogo |
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
| TC-QRY-071D | positivo | RF-QRY-013 | `nav trace RF-*` usa docs TP del layer `06` como evidencia documental de cobertura y no devuelve `missing` cuando existe un caso de prueba canonico que referencia el RF |
| TC-QRY-071E | positivo | RF-QRY-013 | `nav trace TP-*` resuelve el titulo embebido del caso en `06_pruebas/*.md` y devuelve al menos `partial` cuando el caso canonico existe en el docgraph |
| TC-QRY-071F | positivo | RF-QRY-013 | `nav trace RF-*` puede hacer fallback a disco usando las rutas documentales definidas por `read-model` aunque el corpus no viva bajo `.docs/wiki/*` |
| TC-QRY-072 | negativo | RF-QRY-014 | `TestNavRouteDoesNotAttachMissingExplicitRFToGovernanceFallback`: un `RF-*` inexistente no se pega como `doc_id` al fallback de gobernanza |
| TC-QRY-073 | positivo | RF-QRY-016 | `TestNavWikiSearchReturnsLayerFilteredDocs`: `nav wiki search` devuelve candidatos filtrados por capa con `next_queries` hacia pack/trace/multi-read |
| TC-QRY-074 | negativo | RF-QRY-016 | `TestNavWikiSearchDocIndexEmptyReturnsDiagnostic`: docgraph vacio devuelve diagnostico accionable de `index --docs-only` |
| TC-QRY-075 | negativo | RF-QRY-016 | `TestNavWikiSearchBlocksWhenGovernanceBlocked`: governance bloqueada corta `nav wiki search` |
| TC-QRY-076 | positivo | RF-QRY-016 | `TestNavAskRoutePackRepoCompatWarnings`: `nav ask|route|pack --repo docs` no falla y orienta a `nav wiki` |
| TC-QRY-077 | positivo | RF-QRY-016 | `TestIndexWorkspaceDocsExtractsWikiSourceBlocksAndRecords`: docgraph extrae `doc_source_blocks`, `doc_source_records` y menciones compatibles desde fences `toon` declarados con `SDD-WIKI-SOURCE-v1` |
| TC-QRY-078 | positivo | RF-QRY-016 | `TestReplaceDocsWithSources_RoundTrip`: SQLite persiste y resuelve source blocks/source records junto con `doc_records` |
| TC-QRY-078A | positivo | RF-QRY-016 | `TestWithSQLiteReadRetryRetriesLockedErrors`: lecturas documentales reintentan locks SQLite breves y no reintentan errores permanentes |
| TC-QRY-079 | positivo | RF-QRY-016 | `TestValidateSourceValidArtifact`: `nav wiki validate-source` devuelve `PASS` para artefacto fuente valido |
| TC-QRY-080 | negativo | RF-QRY-016 | `TestValidateSourceMissingBlockIDBlocks`: `validate-source` bloquea fences `toon` normativos sin `block_id` |
| TC-QRY-081 | negativo | RF-QRY-016 | `TestValidateSourceNormativeTableWithoutExceptionBlocks`: `validate-source` bloquea tablas Markdown normativas sin excepcion |
| TC-QRY-082 | positivo | RF-QRY-016 | `TestNavWikiSearchFindsExactSourceRecordID`: `nav wiki search` resuelve `record_id` fuente exacto antes del ranking textual y expone `lookup_status` con `record_id`, `block_id`, totales y `match_kind` |
| TC-QRY-083 | positivo | RF-QRY-016 | `TestNavTraceFindsSourceBlockID`: `nav trace` devuelve evidencia `wiki-source` para source IDs exactos y conserva `lookup_status.match_kind=canonical_indexed_id` |
| TC-QRY-084 | positivo | RF-QRY-016 | `TestDefaultProfileIndexesOutcomeDocsAsRS`: el perfil embebido reconoce `.docs/wiki/02_resultados_soluciones_usuario.md` y `.docs/wiki/02_resultados/*.md` como `layer=RS`, `stage=outcome` |
| TC-QRY-085 | positivo | RF-QRY-016 | `TestGovernanceProjectionDerivesFunctionalStageOrder`: `reading_pack.functional_stage_order` deriva `outcome` desde `governance.hierarchy[*].pack_stage` entre `scope` y `architecture` |
| TC-QRY-086 | negativo | RF-QRY-016 | `TestValidateSourceMissingDocIDBlocks`: `validate-source` bloquea artefactos fuente declarados sin `doc_id` |
| TC-QRY-087 | negativo | RF-QRY-016 | `TestValidateSourceMissingKindAndSourceOfTruthBlocks`: `validate-source` bloquea bloques fuente sin `kind` o `source_of_truth` |
| TC-QRY-088 | negativo | RF-QRY-016 | `TestValidateSourceRecordWithoutIDBlocks`: `validate-source` bloquea records referenciables sin `id` |
| TC-QRY-089 | negativo | RF-QRY-016 | `TestValidateSourceBrokenImportAndExportBlocks`: `validate-source` bloquea imports rotos y exports no indexados |
| TC-QRY-090 | positivo | RF-QRY-016 | `TestRenderStructuredFormats_PreserveWikiSourceFields`: los formatos `compact`, `toon` y `yaml` preservan campos agregados de `WikiSourceValidationResult` |
| TC-QRY-091 | positivo | RF-QRY-016 | `TestValidateSourceTableExceptionWithToonSourcePasses`: una excepcion explicita de tabla con fuente `toon` equivalente no bloquea |
| TC-QRY-092 | negativo | RF-QRY-016 | `TestValidateHarnessMissingContractBlocks`: docs gobernados sin contrato `SDD-HARNESS-v1` devuelven `BLOCKED` y `harness_docs_missing_contract` |
| TC-QRY-093 | negativo | RF-QRY-016 | `TestValidateHarnessBrokenObsidianImportBlocks`: imports o links Obsidian links Obsidian de ejemplo rotos bloquean readiness |
| TC-QRY-094 | negativo | RF-QRY-016 | `TestValidateHarnessEditAllowDenyConflictBlocks`: conflictos `agent_may_edit` vs `agent_must_not_edit` bloquean readiness |
| TC-QRY-095 | positivo | RF-QRY-016 | `TestValidateHarnessHumanAndDualContractsMaySkipStrictRuntimeGates`: contratos `human` y `dual` pueden omitir gates estrictos como warning no bloqueante |
| TC-QRY-096 | positivo | RF-QRY-016 | `TestValidateSourceUnmigratedDocsIgnored`: documentos no migrados a `SDD-WIKI-SOURCE-v1` no bloquean `validate-source` |
| TC-QRY-097 | positivo | RF-QRY-016 | `TestValidateHarnessValidLLMFirstContract`: `nav wiki validate-harness` devuelve `PASS` para contrato `llm-first` completo con evidencia existente |
| TC-QRY-098 | negativo | RF-QRY-016 | `TestValidateHarnessUnknownAudienceBlocksAndToonExposesFields`: audience desconocida bloquea y los campos Harness son visibles en TOON |
| TC-QRY-099 | positivo | RF-QRY-016 | `TestNavWikiSearchAcceptsRSLayer`: `nav wiki search --layer RS` devuelve docs outcome sin warnings de capa desconocida |
| TC-QRY-100 | positivo | RF-QRY-014, RF-QRY-016 | `TestNavRouteExplicitRSUsesOutcomeDoc`: `nav route RS-*` ancla el documento RS/outcome gobernado antes de usar heuristicas legacy |
| TC-QRY-101 | positivo | RF-QRY-012, RF-QRY-016 | `TestNavPackIncludesOutcomeStage`: `nav pack` incluye la etapa `outcome` en el pack funcional cuando el read-model la declara |
| TC-QRY-102 | positivo | RF-QRY-013, RF-QRY-016 | `TestNavTraceRSTraceUsesDocIDLayerStageWithoutRF`: `nav trace RS-*` devuelve `doc_id`, `layer=RS`, `stage=outcome` y no rellena el campo legacy `rf` |
| TC-QRY-103 | positivo | RF-QRY-016 | `scripts/release/regression-smoke.ps1`: recorre aliases registrados con `workspace status --no-auto-sync`, `nav wiki search`, `nav wiki pack`, `nav wiki trace` cuando hay ID trazable, y registra diagnostico de workspace/root/db_path por operacion sin mutar repos smokeados |
| TC-QRY-104 | positivo | RF-QRY-016 | `nav wiki search RF-QRY-016 --include-content` expone evidencia de linea (`line_start`/`line_end` o rango equivalente) consistente con el markdown canonico |
| TC-QRY-105 | positivo | RF-QRY-012, RF-QRY-016 | `nav wiki pack --rf RF-QRY-016` conserva el anchor como `primary_doc` aunque existan README/docs genericos con mayor recencia |
| TC-QRY-106 | positivo | RF-QRY-001 | `TestRenderTOONEscapesUnsafeControlCharacters`: TOON escapa controles no imprimibles como `\u0000`, no emite NUL crudo y agrega warning unico de sanitizacion |
| TC-QRY-107 | positivo | RF-QRY-010 | `TestBuildAskCodeEvidenceSkipsOperationalAndBinaryPaths` + `TestSearchPatternFallbackIgnoresNestedMiLspState`: `nav ask`/fallback textual descartan `.mi-lsp/**`, `.db`, `.sqlite` y otros sidecars antes de emitir `code_evidence` |
| TC-QRY-108 | positivo | RF-QRY-001, RF-QRY-002 | `nav search` que agota timeout, incluido el presupuesto interactivo configurado de busqueda textual, devuelve `ok=true` con resultados parciales seguros, warning `search_timeout`, `next_hint` de narrowing y `coach.trigger=search_timeout` |
| TC-QRY-109 | positivo | RF-QRY-001 | `mi-lsp version --format compact|json|toon|yaml` conserva envelope estable con `backend=version`, `items[0]` estructurado y sin dependencia de workspace/daemon |
| TC-QRY-110 | positivo | RF-QRY-017 | `nav affected` con paths explicitos o stdin emite items estables `kind`, `path`, `reason`, `confidence`, `suggested_command` y `evidence` sin depender del daemon |
| TC-QRY-111 | positivo | RF-QRY-017 | `nav affected --from-git-diff` descubre cambios staged, unstaged y untracked desde el workspace git y preserva `change_type` en evidencia |
| TC-QRY-112 | positivo | RF-QRY-017 | `nav affected --include-tests` sugiere comandos de prueba conservadores por familia de path y respeta `--test-command` como override explicito |
| TC-QRY-113 | positivo | RF-QRY-017 | `nav affected --include-docs` mapea cambios en CLI, store, service, daemon, worker y wiki hacia docs canonicos de RF, TP, CT, DB o TECH |
| TC-QRY-114 | negativo | RF-QRY-017 | `nav affected --from-git-diff --quiet` sin cambios devuelve `ok=true`, `items=[]` y warning de no afectados sin hint ruidoso |
| TC-QRY-115 | negativo | RF-QRY-017 | `nav affected` siempre declara warning de heuristica/confidence y no afirma impacto completo hasta existir grafo persistido |
| TC-QRY-116 | positivo | RF-QRY-017 | `nav affected` ignora sidecars operativos `.mi-lsp/**`, `.docs/raw/**`, `.docs/auditoria/**` y `.git/**` al seleccionar impacto |
