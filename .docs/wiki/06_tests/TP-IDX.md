---
wiki_contract: WIKI-CONTRACT-v1
id: TP-IDX
kind: test
status: active
title: TP-IDX
links:
  verifies:
  - RF-IDX-001
  - RF-IDX-002
  - RF-IDX-003
---

## Alcance

RF-IDX-001, RF-IDX-002, RF-IDX-003. Se conserva el inventario de escenarios, límites, disposiciones y evidencia histórica, sin reejecución.

## Casos

### Cobertura objetivo

- RF-IDX-001
- RF-IDX-002
- RF-IDX-003

### Escenarios
- **Caso**: TC-IDX-001; **Tipo**: positivo; **RF**: RF-IDX-001; **Descripción**: crea `index.db` repo-local y reporta stats
  - RF-IDX-001 — Criterio 3. Disposición: manual_evidence_pending. Evidencia esperada: crea `index.db` repo-local y reporta stats. No acredita una ejecución de esta migración.
- **Caso**: TC-IDX-002; **Tipo**: positivo; **RF**: RF-IDX-001; **Descripción**: reindexa con `--clean` reconstruyendo datos derivados
  - RF-IDX-001 — Criterio 4. Disposición: manual_evidence_pending. Evidencia esperada: reindexa con `--clean` reconstruyendo datos derivados. No acredita una ejecución de esta migración.
- **Caso**: TC-IDX-003; **Tipo**: negativo; **RF**: RF-IDX-001; **Descripción**: falla con workspace no resoluble o error de escritura SQLite
  - RF-IDX-001 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: falla con workspace no resoluble o error de escritura SQLite. No acredita una ejecución de esta migración.
- **Caso**: TC-IDX-004; **Tipo**: positivo; **RF**: RF-IDX-001; **Descripción**: indexa clases, funciones y metodos Python con extractor lexical acotado
  - RF-IDX-001 — Criterio 6. Disposición: manual_evidence_pending. Evidencia esperada: indexa clases, funciones y metodos Python con extractor lexical acotado. No acredita una ejecución de esta migración.
- **Caso**: TC-IDX-005; **Tipo**: positivo; **RF**: RF-IDX-001; **Descripción**: persiste `doc_records`, `doc_edges` y `doc_mentions` a partir de la wiki
  - RF-IDX-001 — Criterio 7. Disposición: manual_evidence_pending. Evidencia esperada: persiste `doc_records`, `doc_edges` y `doc_mentions` a partir de la wiki. No acredita una ejecución de esta migración.
- **Caso**: TC-IDX-006; **Tipo**: positivo; **RF**: RF-IDX-001; **Descripción**: usa el fallback documental generico (`README*`, `docs/`, `.docs/`) cuando no existe `.docs/wiki`
  - RF-IDX-001 — Criterio 7. Disposición: manual_evidence_pending. Evidencia esperada: usa el fallback documental generico (`README*`, `docs/`, `.docs/`) cuando no existe `.docs/wiki`. No acredita una ejecución de esta migración.
- **Caso**: TC-IDX-007; **Tipo**: positivo; **RF**: RF-IDX-002; **Descripción**: index detecta archivos cambiados via git y solo re-indexa código
  - RF-IDX-002 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: index detecta archivos cambiados via git y solo re-indexa código. No acredita una ejecución de esta migración.
- **Caso**: TC-IDX-008; **Tipo**: positivo; **RF**: RF-IDX-002; **Descripción**: index sin git disponible hace full re-index con warning
  - RF-IDX-002 — Criterio 2. Disposición: manual_evidence_pending. Evidencia esperada: index sin git disponible hace full re-index con warning. No acredita una ejecución de esta migración.
- **Caso**: TC-IDX-009; **Tipo**: positivo; **RF**: RF-IDX-002; **Descripción**: index con cambios en docs o `read-model.toml` fuerza full re-index
  - RF-IDX-002 — Criterio 3. Disposición: manual_evidence_pending. Evidencia esperada: index con cambios en docs o `read-model.toml` fuerza full re-index. No acredita una ejecución de esta migración.
- **Caso**: TC-IDX-010; **Tipo**: positivo; **RF**: RF-IDX-003; **Descripción**: `00_governance.md` proyecta automáticamente `.docs/wiki/_mi-lsp/read-model.toml`
  - RF-IDX-003 — Criterio 3. Disposición: manual_evidence_pending. Evidencia esperada: `00_governance.md` proyecta automáticamente `.docs/wiki/_mi-lsp/read-model.toml`. No acredita una ejecución de esta migración.
- **Caso**: TC-IDX-011; **Tipo**: negativo; **RF**: RF-IDX-003; **Descripción**: un bloque YAML de gobernanza inválido bloquea la proyección y obliga reparación
  - RF-IDX-003 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: un bloque YAML de gobernanza inválido bloquea la proyección y obliga reparación. No acredita una ejecución de esta migración.
- **Caso**: TC-IDX-012; **Tipo**: positivo; **RF**: RF-IDX-001; **Descripción**: el indexador documental honra re-includes negados de `.gitignore`/`.milspignore` y no excluye `.docs/wiki/**` cuando fue re-habilitada
  - RF-IDX-001 — Criterio 7. Disposición: manual_evidence_pending. Evidencia esperada: el indexador documental honra re-includes negados de `.gitignore`/`.milspignore` y no excluye `.docs/wiki/**` cuando fue re-habilitada. No acredita una ejecución de esta migración.
- **Caso**: TC-IDX-013; **Tipo**: positivo; **RF**: RF-IDX-002; **Descripción**: si `doc_records` quedo solo con docs `generic` aunque la wiki canónica existe, `index` degrada a full re-index y reconstruye el corpus documental
  - RF-IDX-002 — Criterio 3. Disposición: manual_evidence_pending. Evidencia esperada: si `doc_records` quedo solo con docs `generic` aunque la wiki canónica existe, `index` degrada a full re-index y reconstruye el corpus documental. No acredita una ejecución de esta migración.
- **Caso**: TC-IDX-014; **Tipo**: positivo; **RF**: RF-IDX-001; **Descripción**: `TestIndexRunDocsOnlyRebuildsDocsWithoutReplacingCatalog`: `index --docs-only` reconstruye docs/memory sin borrar símbolos ya catalogados
  - RF-IDX-001 — Criterio 4. Disposición: manual_evidence_pending. Evidencia esperada: `TestIndexRunDocsOnlyRebuildsDocsWithoutReplacingCatalog`: `index --docs-only` reconstruye docs/memory sin borrar símbolos ya catalogados. No acredita una ejecución de esta migración.
- **Caso**: TC-IDX-015; **Tipo**: negativo; **RF**: RF-IDX-001; **Descripción**: `TestWithWorkspaceIndexLockRejectsConcurrentIndexRun`: dos indexaciones simultaneas del mismo workspace quedan bloqueadas por `.mi-lsp/index.lock`
  - RF-IDX-001 — Criterio 3. Disposición: manual_evidence_pending. Evidencia esperada: `TestWithWorkspaceIndexLockRejectsConcurrentIndexRun`: dos indexaciones simultaneas del mismo workspace quedan bloqueadas por `.mi-lsp/index.lock`. No acredita una ejecución de esta migración.
- **Caso**: TC-IDX-016; **Tipo**: positivo; **RF**: RF-IDX-001; **Descripción**: `TestDefaultIgnoreMatcherSkipsGeneratedDependencyCaches`: el indexador ignora `.venv`, `venv`, `__pycache__`, `.pytest_cache`, `.turbo`, `.next` y `node_modules`
  - RF-IDX-001 — Criterio 7. Disposición: manual_evidence_pending. Evidencia esperada: `TestDefaultIgnoreMatcherSkipsGeneratedDependencyCaches`: el indexador ignora `.venv`, `venv`, `__pycache__`, `.pytest_cache`, `.turbo`, `.next` y `node_modules`. No acredita una ejecución de esta migración.
- **Caso**: TC-IDX-017; **Tipo**: positivo; **RF**: RF-IDX-001; **Descripción**: `TestReplaceWorkspaceIndexPublishesGenerationMetadata`: publica punteros `active_*_generation_id` solo tras commit exitoso
  - RF-IDX-001 — Criterio 8. Disposición: manual_evidence_pending. Evidencia esperada: `TestReplaceWorkspaceIndexPublishesGenerationMetadata`: publica punteros `active_*_generation_id` solo tras commit exitoso. No acredita una ejecución de esta migración.
- **Caso**: TC-IDX-018; **Tipo**: positivo; **RF**: RF-IDX-001; **Descripción**: `TestWithWorkspaceIndexLockRemovesStaleLock`: un lock con PID inexistente se recupera antes de ejecutar
  - RF-IDX-001 — Criterio 4. Disposición: manual_evidence_pending. Evidencia esperada: `TestWithWorkspaceIndexLockRemovesStaleLock`: un lock con PID inexistente se recupera antes de ejecutar. No acredita una ejecución de esta migración.
- **Caso**: TC-IDX-019; **Tipo**: positivo; **RF**: RF-IDX-001; **Descripción**: `TestIndexStartWaitCreatesSucceededJobAndGeneration`: `index.start --wait` crea job, publica generación y deja status `succeeded`
  - RF-IDX-001 — Criterio 3. Disposición: manual_evidence_pending. Evidencia esperada: `TestIndexStartWaitCreatesSucceededJobAndGeneration`: `index.start --wait` crea job, publica generación y deja status `succeeded`. No acredita una ejecución de esta migración.
- **Caso**: TC-IDX-020; **Tipo**: positivo; **RF**: RF-IDX-001; **Descripción**: `TestMarkIndexJobProgressUpdatesCountersStageAndTimestamp`: progreso vivo refresca counters, stage/path y `updated_at`
  - RF-IDX-001 — Criterio 6. Disposición: manual_evidence_pending. Evidencia esperada: `TestMarkIndexJobProgressUpdatesCountersStageAndTimestamp`: progreso vivo refresca counters, stage/path y `updated_at`. No acredita una ejecución de esta migración.
- **Caso**: TC-IDX-021; **Tipo**: negativo; **RF**: RF-IDX-001; **Descripción**: `TestRunIndexJobHonorsCooperativeCancelRequest`: cancelacion cooperativa marca el job como `canceled`
  - RF-IDX-001 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: `TestRunIndexJobHonorsCooperativeCancelRequest`: cancelacion cooperativa marca el job como `canceled`. No acredita una ejecución de esta migración.
- **Caso**: TC-IDX-022; **Tipo**: positivo; **RF**: RF-IDX-001; **Descripción**: `TestCancelIndexJobForceRemovesMatchingDeadPIDLock`: `cancel --force` mata PID vivo y limpia lock matching
  - RF-IDX-001 — Criterio 6. Disposición: manual_evidence_pending. Evidencia esperada: `TestCancelIndexJobForceRemovesMatchingDeadPIDLock`: `cancel --force` mata PID vivo y limpia lock matching. No acredita una ejecución de esta migración.
- **Caso**: TC-IDX-023; **Tipo**: positivo; **RF**: RF-IDX-001; **Descripción**: `TestExtractPython_EmotionalRubricFixtureDoesNotHang`: fixture Python que antes colgaba se indexa lexicalmente
  - RF-IDX-001 — Criterio 4. Disposición: manual_evidence_pending. Evidencia esperada: `TestExtractPython_EmotionalRubricFixtureDoesNotHang`: fixture Python que antes colgaba se indexa lexicalmente. No acredita una ejecución de esta migración.
- **Caso**: TC-IDX-024; **Tipo**: positivo; **RF**: RF-IDX-001; **Descripción**: `TestExtractGoSymbols`: el extractor Go cataloga funciones, metodos y tipos para self-dogfood del repo `mi-lsp`
  - RF-IDX-001 — Criterio 9. Disposición: manual_evidence_pending. Evidencia esperada: `TestExtractGoSymbols`: el extractor Go cataloga funciones, metodos y tipos para self-dogfood del repo `mi-lsp`. No acredita una ejecución de esta migración.
- **Caso**: TC-IDX-025; **Tipo**: positivo; **RF**: RF-IDX-001; **Descripción**: `TestWalkWorkspaceIgnoresNestedMiLspState` + `TestHardIgnoreMatcherKeepsMiLspIgnoredAfterNegatedReinclude`: `.mi-lsp/**` se trata como hard ignore operacional incluso anidado y aunque haya re-include negado
  - RF-IDX-001 — Criterio 2. Disposición: manual_evidence_pending. Evidencia esperada: `TestWalkWorkspaceIgnoresNestedMiLspState` + `TestHardIgnoreMatcherKeepsMiLspIgnoredAfterNegatedReinclude`: `.mi-lsp/**` se trata como hard ignore operacional incluso anidado y aunque haya re-include negado. No acredita una ejecución de esta migración.
- **Caso**: TC-IDX-026; **Tipo**: positivo; **RF**: RF-IDX-001; **Descripción**: `TestIncrementalIndexRepairsStaleGraphWithDocumentationGraph`: sin batch semántico completo, el reindex incremental publica un grafo documental fresco contra el catálogo activo
  - RF-IDX-001 — Criterio 8. Disposición: manual_evidence_pending. Evidencia esperada: `TestIncrementalIndexRepairsStaleGraphWithDocumentationGraph`: sin batch semántico completo, el reindex incremental publica un grafo documental fresco contra el catálogo activo. No acredita una ejecución de esta migración.
- **Caso**: TC-IDX-027; **Tipo**: negativo; **RF**: RF-IDX-001; **Descripción**: `TestIncrementalIndexReturnsTypedErrorWhenStaleGraphCannotBeRebuilt`: sin identidad publicable, incremental y full reindex devuelven `graph_repair_incomplete` en vez de éxito vacío
  - RF-IDX-001 — Criterio 8. Disposición: manual_evidence_pending. Evidencia esperada: `TestIncrementalIndexReturnsTypedErrorWhenStaleGraphCannotBeRebuilt`: sin identidad publicable, incremental y full reindex devuelven `graph_repair_incomplete` en vez de éxito vacío. No acredita una ejecución de esta migración.
- **Caso**: TC-IDX-029; **Tipo**: manual; **RF**: RF-IDX-001; **Descripción**: Go offline por CLI direct/wait/detached; daemon CLI no aplicable. FINAL_VERIFY1 FAIL histórico; RECHECK1 PASS policy / PARTIAL global, límites en bloque TC-IDX-029
  - RF-IDX-001 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: Go offline por CLI direct/wait/detached; daemon CLI no aplicable. FINAL_VERIFY1 FAIL histórico; RECHECK1 PASS policy / PARTIAL global, límites en bloque TC-IDX-029. No acredita una ejecución de esta migración.
- **Caso**: TC-IDX-028; **Tipo**: positivo; **RF**: RF-IDX-002; **Descripción**: aceptación manual por CLI pública; definición normativa en el bloque `TC-IDX-028`
  - RF-IDX-002 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: aceptación manual por CLI pública; definición normativa en el bloque `TC-IDX-028`. No acredita una ejecución de esta migración.

#### TC-IDX-029 — Go offline por job
  - RF-IDX-001 — Criterio 6. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.

- **Marcador estable**: TC-IDX-029
- **requirement_id**: RF-IDX-001
- **observable_contract**: RF-IDX-001-GO-OFFLINE
- **case_id**: TC-IDX-029
- **verification_mode**: manual_evidence
- **Estado del comportamiento**: explicit_incomplete
- **manual_evidence_owner**: coord-milsp
- **manual_evidence_due**: disclosure antes de publicación acotada
- **followup_due**: 2026-10-20
- **followup_authority**: precisión/evidencia futura con objetivo separado, sin renovación del presupuesto
- **fixture_isolation**: fixture temporal propio del verifier, fuera de worktrees B/C/K; no creado en BUILD
- **fixture**:
  - **stdlib**: módulo Go válido con uso de stdlib y repository_identity publicable
  - **cached**: dependencia y exports disponibles en cache local propia del fixture
  - **missing**: dependencia no disponible en cache local propia
  - **toolchain_missing**: go.mod que requiere versión superior al toolchain local disponible
  - **adversarial_env**: GOTOOLCHAIN auto, proxy/sumdb online y GOFLAGS ambiental y persistido con -mod=mod y -toolexec; GOENV propio del fixture, sin mutar config global ni ejecutarlos fuera de la corrida offline
  - **metadata**: go.mod, go.sum y go.work/go.work.sum presentes o ausentes; hashes/existencia antes y después
- **expected**:
  - **stdlib_cached**: observación compiler-backed con capabilities reales; no afirmar completitud solo por succeeded
  - **missing_dependency**: GPH_GO_OFFLINE_UNAVAILABLE si no hay metadata utilizable; metadata parcial puede dar diagnóstico genérico/index_failed, type_check_error o external_target; motivo offline específico no universal, precisión explicit_incomplete; sin retry online ni facts completos
  - **missing_local_toolchain**: error tipado o parcialidad honesta; ninguna descarga ni bootstrap
  - **network_scope**: stdlib succeeded en bwrap, namespace distinto, solo lo y sin rutas; egress-deny del caso aislado, no cero intentos de syscall ni comportamiento sin namespace ni salud global
  - **user_files_intact**: hashes y existencia de go.mod/go.sum/go.work/go.work.sum sin cambios
  - **local_compile**: export y cache de compilación local permitidos; no módulos/toolchains nuevos descargados
  - **importer**: solo exports observados y tipos locales; faltante no activa importer.Default ambiental
  - **mode**: payload y status go_offline=true; warning de observación y fingerprint distinguen offline
  - **policy_mismatch**: pedido offline con job activo online y caso simétrico devuelven ok=false y error.code=index_job_policy_mismatch; job_id y políticas reales visibles, sin modificar/cancelar activo ni crear runner/generación/ownership adicional
  - **matching_policy**: política coincidente conserva deduplicación previa y job_id; defaults restantes intactos
  - **public_routes**: wrapper direct wait, start direct wait y detached con recarga durable
  - **daemon_route**: CLI index-daemon unsupported/noapplicable actual; dispatch interno App/server compartido no acredita paridad; graph query daemon no prueba observador
  - **defaults**: sin flag conserva comportamiento previo; docs/catalog no invocan Go; embeddings y noGo fuera de la garantía
- **historical_final_verify_plan**:
  - preparar fixtures y caches propios únicamente en FINAL_VERIFY; no instalar toolchains para resolver negativos
  - capturar hashes/existencia de metadata y lista de módulos/toolchains de cache antes de la corrida
  - ejecutar mi-lsp index <root-absoluto> --go-offline --no-daemon --format json bajo monitor de salida de red
  - repetir con mi-lsp index start <root-absoluto> --go-offline --wait --no-daemon --format json
  - repetir start sin --wait; consultar index status del job hasta terminal y conservar go_offline del job
  - expectativa histórica daemon no aplicable a CLI index actual; no ejecutada ni pendiente; consulta graph daemon no acredita runner
  - contrastar las variantes stdlib/cache/missing/toolchain_missing con iguales inputs e identidad en fixtures separados
  - contrastar metadata y cache antes/después y capacidades/omisiones; faltante de monitor o ruta daemon queda explicit_incomplete
  - comprobar docs-only y catalog sin ejecución Go; comparar defaults sin flag solo en fixture de verificación controlado
- **historical_recheck1_plan_consumed**:
  - después del freeze IMPLEMENTATION_COMPLETE, único recheck1 del parent en fixture propio docs-only con job activo controlado; probar ambos sentidos de mismatch y dedupe coincidente
  - observar rechazo CLI no exitoso, job_id y políticas; contrastar activo y cantidad de jobs/generaciones/fence antes y después sin cancelarlo ni relanzarlo por rechazo
  - reintento manual solo después de terminal; sin repetir wave offline ni inferir red/daemon no observados
- **verification_basis**: FINAL_VERIFY1 FAIL histórico intacto; RECHECK1 PASS policy / PARTIAL global; audit1 Changes requested, canon reparado sin aceptación global
- **consumed_evidence**: rechazo simétrico, dedupe coincidente, snapshots durables sin mutación y retry manual terminal comprobados; stdlib aislada succeeded
- **uncovered**: precisión offline específica missing, capacidades/precisión externas completas y red normal/cero syscall; daemon CLI no aplicable
- **budget**: FINAL_VERIFY1/audit1/RECHECK1 agotado; sin recheck2 ni verificador pendiente
- **readiness_debt**: no PASS de Harness/wiki-source/P6/Graph afirmado; deuda preexistente fuera de esta normalización
- **execution_note**: planes históricos conservados como contexto de evidencia consumida, no instrucciones de nueva ejecución; reparación solo editorial, sin corridas ni aceptación completa

#### TC-IDX-028
  - RF-IDX-002 — Criterio 3. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.

- **Marcador estable**: TC-IDX-028
- **requirement_id**: RF-IDX-002
- **case_id**: TC-IDX-028
- **case_type**: positive
- **verification_mode**: manual_evidence
- **Estado del comportamiento**: manual_evidence_pending
- **verification_basis**: FINAL_VERIFY1 parcial preservada; único recheck1 del parent después de IMPLEMENTATION_COMPLETE
- **observable_contract**: RF-IDX-002-DOCS-OBSERVABLE
- **fixture_isolation**: fixture temporal propio del parent, fuera de Android y de este worktree; no creado durante BUILD
- **fixture**:
  - **contenido**: archivo Go con símbolo reconocible y N documentos markdown elegibles (N>0), sin errores de lectura ni cambios entre las corridas documentales
  - **preparacion**: registrar únicamente el fixture propio mediante CLI pública y publicar su catálogo con index start --mode catalog; no sembrar filas mediante SQL
  - **referencia**: capturar generations.catalog de nav git-frontier y el símbolo visible por nav find antes de la primera indexación documental
- **expected**:
  - **primera_indexacion_docs**: sin snapshot documental previo, parsed=N skipped=0
  - **incremental_docs_without_clean**: mismo contenido y snapshot previo publicado; docs_skip_reparse parsed=0 skipped=N
  - **docs_mode_clean_job**: mismo contenido con --clean; clean=true y docs_skip_reparse parsed=N skipped=0, sin excepción por hash previo
  - **wrapper_docs_only_clean**: mismo contenido con --docs-only --clean; mismo oráculo parsed=N skipped=0
  - **catalog_generation_preserved**: generations.catalog de nav git-frontier permanece igual después de cada publicación documental y el mismo símbolo de código sigue visible por nav find
  - **docs_generation_published**: generations.docs de nav git-frontier coincide con generation_id del job succeeded después de cada corrida documental, incluido el wrapper
  - **missing_pointer**: null o generation_reasons no acreditan la generación esperada; conservar explicit_incomplete
  - **internal_memory_contract**: active_memory_generation_id se activa atómicamente con docs según el publisher; no es observable en estas superficies y permanece sin evidencia pública
  - **memory_evidence_limit**: memory o memory_pointer de workspace status no sustituyen un ID de generación; no declarar aceptación completa de TC-IDX-028 ni PASS por resolver solo contadores y docs
  - **forbidden_oracles**: sin SQL ni APIs internas, sin fixture Android, sin tests ficticios ni automatización propuesta
- **manual_plan**:
  - **paso**: usar el fixture propio preservado por FINAL_VERIFY1 si sigue disponible; el parent decide preparación aislada si falta; no tocar evidencia previa
  - **paso**: ejecutar mi-lsp index start --mode catalog --wait --no-daemon --workspace <fixture> --format json; guardar nav git-frontier y nav find del símbolo como referencia de catálogo y código
  - **paso**: ejecutar mi-lsp index start --mode docs --wait --no-daemon --workspace <fixture> --format json; guardar parsed=N skipped=0 y publicación exitosa
  - **paso**: repetir el comando documental anterior sin modificar contenido; guardar parsed=0 skipped=N
  - **paso**: ejecutar mi-lsp index start --mode docs --clean --wait --no-daemon --workspace <fixture> --format json; guardar parsed=N skipped=0
  - **paso**: ejecutar mi-lsp index --docs-only --clean --no-daemon --workspace <fixture> --format json; guardar parsed=N skipped=0; el wrapper espera implícitamente y no admite --wait
  - **paso**: tras cada corrida documental, guardar mi-lsp nav git-frontier --no-daemon --workspace <fixture> --format json y nav find del mismo símbolo; comparar generations.docs con el job y generations.catalog con la referencia inicial
  - **evidencia_pendiente**: guardar warnings de cada comando que espera, index status y workspace status --full como contexto; no consultas SQLite ni APIs internas
  - **evidencia_no_observable**: generación interna de memoria; registrar limitación separada sin inferirla de succeeded, snapshot_built_at o memory_pointer
- **execution_note**: FINAL_VERIFY1 parcial ya consumida; BUILD solo ajusta autoría code y wiki, sin corridas nuevas ni reescritura del packet; recheck1 reservado al verifier del parent
- **manual_evidence_owner**: parent_final_verify
- **manual_evidence_due**: FINAL_VERIFY del objetivo docs-clean-reparse-20261013, antes del cierre

```yaml
harness_protocol: SDD-HARNESS-v1
id: TP-IDX
kind: test-plan
audience: llm-first
imports:
  - .docs/wiki/00_governance.md
  - .docs/wiki/05_requirements/RF-IDX-001.md
  - .docs/wiki/05_requirements/RF-IDX-002.md
  - .docs/wiki/05_requirements/RF-IDX-003.md
exports:
  - TP-IDX
agent_must_read:
  - .docs/wiki/06_tests/TP-IDX.md
agent_may_edit:
  - .docs/wiki/06_tests/TP-IDX.md
agent_must_not_edit:
  - .docs/wiki/05_requirements/
  - internal/indexer/
verify:
  - .docs/wiki/06_tests/TP-IDX.md#Casos
stop_if:
  - harness_verdict=BLOCKED
  - manual_evidence_pending_is_reported_as_pass
  - test_oracle_or_acceptance_would_change
evidence:
  - .docs/wiki/06_tests/TP-IDX.md
  - .docs/wiki/05_requirements/RF-IDX-001.md
  - .docs/wiki/05_requirements/RF-IDX-002.md
  - .docs/wiki/05_requirements/RF-IDX-003.md
```
