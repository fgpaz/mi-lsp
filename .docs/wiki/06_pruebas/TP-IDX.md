# TP-IDX

```yaml
harness_protocol: SDD-HARNESS-v1
id: "TP-IDX"
kind: "support-doc"
audience: "llm-first"
imports:
  - '[[00_gobierno_documental]]'
  - '[[TP-IDX]]'
exports:
  - 'TP-IDX'
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/06_pruebas/TP-IDX.md
agent_may_edit:
  - .docs/wiki/06_pruebas/TP-IDX.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
evidence:
  - .docs/wiki/06_pruebas/TP-IDX.md
```

## Cobertura objetivo

- RF-IDX-001
- RF-IDX-002
- RF-IDX-003

## Casos

| Caso | Tipo | RF | Descripcion |
|---|---|---|---|
| TC-IDX-001 | positivo | RF-IDX-001 | crea `index.db` repo-local y reporta stats |
| TC-IDX-002 | positivo | RF-IDX-001 | reindexa con `--clean` reconstruyendo datos derivados |
| TC-IDX-003 | negativo | RF-IDX-001 | falla con workspace no resoluble o error de escritura SQLite |
| TC-IDX-004 | positivo | RF-IDX-001 | indexa clases, funciones y metodos Python con extractor lexical acotado |
| TC-IDX-005 | positivo | RF-IDX-001 | persiste `doc_records`, `doc_edges` y `doc_mentions` a partir de la wiki |
| TC-IDX-006 | positivo | RF-IDX-001 | usa el fallback documental generico (`README*`, `docs/`, `.docs/`) cuando no existe `.docs/wiki` |
| TC-IDX-007 | positivo | RF-IDX-002 | index detecta archivos cambiados via git y solo re-indexa codigo |
| TC-IDX-008 | positivo | RF-IDX-002 | index sin git disponible hace full re-index con warning |
| TC-IDX-009 | positivo | RF-IDX-002 | index con cambios en docs o `read-model.toml` fuerza full re-index |
| TC-IDX-010 | positivo | RF-IDX-003 | `00_gobierno_documental.md` proyecta automaticamente `.docs/wiki/_mi-lsp/read-model.toml` |
| TC-IDX-011 | negativo | RF-IDX-003 | un bloque YAML de gobernanza invalido bloquea la proyeccion y obliga reparacion |
| TC-IDX-012 | positivo | RF-IDX-001 | el indexador documental honra re-includes negados de `.gitignore`/`.milspignore` y no excluye `.docs/wiki/**` cuando fue re-habilitada |
| TC-IDX-013 | positivo | RF-IDX-002 | si `doc_records` quedo solo con docs `generic` aunque la wiki canonica existe, `index` degrada a full re-index y reconstruye el corpus documental |
| TC-IDX-014 | positivo | RF-IDX-001 | `TestIndexRunDocsOnlyRebuildsDocsWithoutReplacingCatalog`: `index --docs-only` reconstruye docs/memory sin borrar simbolos ya catalogados |
| TC-IDX-015 | negativo | RF-IDX-001 | `TestWithWorkspaceIndexLockRejectsConcurrentIndexRun`: dos indexaciones simultaneas del mismo workspace quedan bloqueadas por `.mi-lsp/index.lock` |
| TC-IDX-016 | positivo | RF-IDX-001 | `TestDefaultIgnoreMatcherSkipsGeneratedDependencyCaches`: el indexador ignora `.venv`, `venv`, `__pycache__`, `.pytest_cache`, `.turbo`, `.next` y `node_modules` |
| TC-IDX-017 | positivo | RF-IDX-001 | `TestReplaceWorkspaceIndexPublishesGenerationMetadata`: publica punteros `active_*_generation_id` solo tras commit exitoso |
| TC-IDX-018 | positivo | RF-IDX-001 | `TestWithWorkspaceIndexLockRemovesStaleLock`: un lock con PID inexistente se recupera antes de ejecutar |
| TC-IDX-019 | positivo | RF-IDX-001 | `TestIndexStartWaitCreatesSucceededJobAndGeneration`: `index.start --wait` crea job, publica generacion y deja status `succeeded` |
| TC-IDX-020 | positivo | RF-IDX-001 | `TestMarkIndexJobProgressUpdatesCountersStageAndTimestamp`: progreso vivo refresca counters, stage/path y `updated_at` |
| TC-IDX-021 | negativo | RF-IDX-001 | `TestRunIndexJobHonorsCooperativeCancelRequest`: cancelacion cooperativa marca el job como `canceled` |
| TC-IDX-022 | positivo | RF-IDX-001 | `TestCancelIndexJobForceRemovesMatchingDeadPIDLock`: `cancel --force` mata PID vivo y limpia lock matching |
| TC-IDX-023 | positivo | RF-IDX-001 | `TestExtractPython_EmotionalRubricFixtureDoesNotHang`: fixture Python que antes colgaba se indexa lexicalmente |
| TC-IDX-024 | positivo | RF-IDX-001 | `TestExtractGoSymbols`: el extractor Go cataloga funciones, metodos y tipos para self-dogfood del repo `mi-lsp` |
| TC-IDX-025 | positivo | RF-IDX-001 | `TestWalkWorkspaceIgnoresNestedMiLspState` + `TestHardIgnoreMatcherKeepsMiLspIgnoredAfterNegatedReinclude`: `.mi-lsp/**` se trata como hard ignore operacional incluso anidado y aunque haya re-include negado |
| TC-IDX-026 | positivo | RF-IDX-001 | `TestIncrementalIndexRepairsStaleGraphWithDocumentationGraph`: sin batch semántico completo, el reindex incremental publica un grafo documental fresco contra el catálogo activo |
| TC-IDX-027 | negativo | RF-IDX-001 | `TestIncrementalIndexReturnsTypedErrorWhenStaleGraphCannotBeRebuilt`: sin identidad publicable, incremental y full reindex devuelven `graph_repair_incomplete` en vez de éxito vacío |
| TC-IDX-028 | positivo | RF-IDX-002 | aceptación manual por CLI pública; definición normativa en el bloque `TC-IDX-028` |

### TC-IDX-028

```toon
block_id: TC-IDX-028
kind: normative
source_of_truth: normative
requirement_id: RF-IDX-002
case_id: TC-IDX-028
case_type: positive
verification_mode: manual_evidence
status: manual_evidence_pending
verification_basis: FINAL_VERIFY1 parcial preservada; único recheck1 del parent después de IMPLEMENTATION_COMPLETE
observable_contract: RF-IDX-002-DOCS-OBSERVABLE
fixture_isolation: fixture temporal propio del parent, fuera de Android y de este worktree; no creado durante BUILD
fixture:
  contenido: archivo Go con símbolo reconocible y N documentos markdown elegibles (N>0), sin errores de lectura ni cambios entre las corridas documentales
  preparacion: registrar únicamente el fixture propio mediante CLI pública y publicar su catálogo con index start --mode catalog; no sembrar filas mediante SQL
  referencia: capturar generations.catalog de nav git-frontier y el símbolo visible por nav find antes de la primera indexación documental
expected:
  primera_indexacion_docs: sin snapshot documental previo, parsed=N skipped=0
  incremental_docs_without_clean: mismo contenido y snapshot previo publicado; docs_skip_reparse parsed=0 skipped=N
  docs_mode_clean_job: mismo contenido con --clean; clean=true y docs_skip_reparse parsed=N skipped=0, sin excepción por hash previo
  wrapper_docs_only_clean: mismo contenido con --docs-only --clean; mismo oráculo parsed=N skipped=0
  catalog_generation_preserved: generations.catalog de nav git-frontier permanece igual después de cada publicación documental y el mismo símbolo de código sigue visible por nav find
  docs_generation_published: generations.docs de nav git-frontier coincide con generation_id del job succeeded después de cada corrida documental, incluido el wrapper
  missing_pointer: null o generation_reasons no acreditan la generación esperada; conservar explicit_incomplete
  internal_memory_contract: active_memory_generation_id se activa atómicamente con docs según el publisher; no es observable en estas superficies y permanece sin evidencia pública
  memory_evidence_limit: memory o memory_pointer de workspace status no sustituyen un ID de generación; no declarar aceptación completa de TC-IDX-028 ni PASS por resolver solo contadores y docs
  forbidden_oracles: sin SQL ni APIs internas, sin fixture Android, sin tests ficticios ni automatización propuesta
manual_plan:
  - paso: usar el fixture propio preservado por FINAL_VERIFY1 si sigue disponible; el parent decide preparación aislada si falta; no tocar evidencia previa
  - paso: ejecutar mi-lsp index start --mode catalog --wait --no-daemon --workspace <fixture> --format json; guardar nav git-frontier y nav find del símbolo como referencia de catálogo y código
  - paso: ejecutar mi-lsp index start --mode docs --wait --no-daemon --workspace <fixture> --format json; guardar parsed=N skipped=0 y publicación exitosa
  - paso: repetir el comando documental anterior sin modificar contenido; guardar parsed=0 skipped=N
  - paso: ejecutar mi-lsp index start --mode docs --clean --wait --no-daemon --workspace <fixture> --format json; guardar parsed=N skipped=0
  - paso: ejecutar mi-lsp index --docs-only --clean --no-daemon --workspace <fixture> --format json; guardar parsed=N skipped=0; el wrapper espera implícitamente y no admite --wait
  - paso: tras cada corrida documental, guardar mi-lsp nav git-frontier --no-daemon --workspace <fixture> --format json y nav find del mismo símbolo; comparar generations.docs con el job y generations.catalog con la referencia inicial
  - evidencia_pendiente: guardar warnings de cada comando que espera, index status y workspace status --full como contexto; no consultas SQLite ni APIs internas
  - evidencia_no_observable: generación interna de memoria; registrar limitación separada sin inferirla de succeeded, snapshot_built_at o memory_pointer
execution_note: FINAL_VERIFY1 parcial ya consumida; BUILD solo ajusta autoría code y wiki, sin corridas nuevas ni reescritura del packet; recheck1 reservado al verifier del parent
manual_evidence_owner: parent_final_verify
manual_evidence_due: FINAL_VERIFY del objetivo docs-clean-reparse-20261013, antes del cierre
```
