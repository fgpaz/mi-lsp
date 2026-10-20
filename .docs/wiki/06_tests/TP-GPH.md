---
wiki_contract: WIKI-CONTRACT-v1
id: TP-GPH
kind: test
status: active
title: TP-GPH
links:
  verifies:
  - RF-GPH-001
  - RF-GPH-002
  - RF-GPH-003
  - RF-GPH-004
  - RF-GPH-005
  - RF-GPH-006
  - RF-GPH-007
  - RF-GPH-008
  - RF-GPH-009
  - RF-GPH-010
  - RF-GPH-011
  code:
  - path: internal/cli/git_frontier_test.go
    role: test
  - path: internal/indexer/incremental_graph_repair_test.go
    role: test
---

## Alcance

RF-GPH-001, RF-GPH-002, RF-GPH-003, RF-GPH-004, RF-GPH-005, RF-GPH-006, RF-GPH-007, RF-GPH-008, RF-GPH-009, RF-GPH-010, RF-GPH-011. Se conserva el inventario de escenarios, límites, disposiciones y evidencia histórica, sin reejecución.

## Casos

### Kotlin — oráculo futuro de la extensión fuente

- **Marcador estable**: TP-GPH.kotlin-compiler-export-v1
- **Estado del comportamiento**: manual_evidence_pending
- **owner**: coord-milsp
- **decision_due**: 2026-10-17
- **Evidencia referenciada**: FV1_FAIL_audit1_BLOCKED_recheck1_FAIL_graph_observe_pin529_SOURCE_ONLY
- **verification_basis**: counters_FV_audit_recheck_1_1_1_exhausted_no_retry_in_this_lane
- **fixture**: testdata/kotlin-graph-main-v1_sources_only_sandbox_local_JAR_network_disabled
- **bindings**:
  - testdata/kotlin-graph-main-v1/README.md
  - internal/indexer/extractor_kotlin.go
  - internal/model/graph_observation.go
- **raw_contract**: v2_obs2
- **admission**: ADR-KOTLIN-GRAPH-ADMISSION-V1
- **build_command**: bash_worker-kotlin/build.sh_pinned_local_checkout_new_absolute_output
- **Casos**:
  - **id**: TC-GPH-K01
  - RF-GPH-008 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: handshake fork honesto; BindingContext y ModuleDescriptor exportan overloads, receivers y módulos correctos; ninguna fuente PSI/text-only se etiqueta compiler. No acredita una ejecución de esta migración.
  - **type**: positivo
  - **Dado**: fork fwcd pin con descriptor main JVM completo y hashes reales
  - **Cuando**: se construye y consulta el método anunciado compiler-only
  - **Entonces**: handshake fork honesto; BindingContext y ModuleDescriptor exportan overloads, receivers y módulos correctos; ninguna fuente PSI/text-only se etiqueta compiler
  - **id**: TC-GPH-K02
  - RF-GPH-008 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: rechazo tipado; ningún batch ni fallback textual. No acredita una ejecución de esta migración.
  - **type**: negativo
  - **Dado**: stock fwcd, capability ausente o manifest/hash/version de fork falso
  - **Cuando**: se pide compiler export
  - **Entonces**: rechazo tipado; ningún batch ni fallback textual
  - **id**: TC-GPH-K03
  - RF-GPH-001 — Criterio 1; RF-GPH-002 — Criterio 1; RF-GPH-003 — Criterio 1; RF-GPH-004 — Criterio 1; RF-GPH-005 — Criterio 4; RF-GPH-006 — Criterio 1; RF-GPH-007 — Criterio 3; RF-GPH-008 — Criterio 1; RF-GPH-009 — Criterio 1; RF-GPH-010 — Criterio 3; RF-GPH-011 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: solo REFERENCE_TARGET resuelto; módulos aislados; configuración no certificada no acredita complete; sin NodeKey ni calls fake. No acredita una ejecución de esta migración.
  - **type**: negativo
  - **Dado**: homónimos/comentarios, FQN igual en otro módulo, descriptor fallback/incomplete/stdlib-only no acreditado
  - **Cuando**: se consulta references y admisión Graph
  - **Entonces**: solo REFERENCE_TARGET resuelto; módulos aislados; configuración no certificada no acredita complete; sin NodeKey ni calls fake
  - **id**: TC-GPH-K04
  - RF-GPH-001 — Criterio 1; RF-GPH-002 — Criterio 1; RF-GPH-003 — Criterio 1; RF-GPH-004 — Criterio 1; RF-GPH-005 — Criterio 1; RF-GPH-006 — Criterio 1; RF-GPH-007 — Criterio 1; RF-GPH-008 — Criterio 3; RF-GPH-009 — Criterio 1; RF-GPH-010 — Criterio 2; RF-GPH-011 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: se inválida todo export; no respuesta tardía ni partialbatch ready; siguiente caller crea runtime propio. No acredita una ejecución de esta migración.
  - **type**: negativo
  - **Dado**: cambio de sources/JAR/build/config, error compiler, cancel durante initialize/export o crash
  - **Cuando**: se termina o vence el request
  - **Entonces**: se inválida todo export; no respuesta tardía ni partialbatch ready; siguiente caller crea runtime propio
  - **id**: TC-GPH-K05
  - RF-GPH-001 — Criterio 1; RF-GPH-002 — Criterio 3; RF-GPH-003 — Criterio 3; RF-GPH-004 — Criterio 1; RF-GPH-005 — Criterio 1; RF-GPH-006 — Criterio 4; RF-GPH-007 — Criterio 1; RF-GPH-008 — Criterio 1; RF-GPH-009 — Criterio 3; RF-GPH-010 — Criterio 1; RF-GPH-011 — Criterio 4. Disposición: manual_evidence_pending. Evidencia esperada: backend_partial sin publicación; promotion_oracle_missing visible; generación previa intacta; stable/complete no se forjan. No acredita una ejecución de esta migración.
  - **type**: negativo
  - **Dado**: raw v2 partial experimental con registro Kotlin exacto y sin recibo del exporter pineado, o con backup/stdlib-only/cero diagnostics
  - **Cuando**: adapter emite NodeKey-v1 y batch canónico sin sello; core valida rebasa sella y exige Ready
  - **Entonces**: backend_partial sin publicación; promotion_oracle_missing visible; generación previa intacta; stable/complete no se forjan
  - **id**: TC-GPH-K08
  - RF-GPH-001 — Criterio 2; RF-GPH-002 — Criterio 1; RF-GPH-003 — Criterio 1; RF-GPH-004 — Criterio 1; RF-GPH-005 — Criterio 1; RF-GPH-006 — Criterio 1; RF-GPH-007 — Criterio 1; RF-GPH-008 — Criterio 1; RF-GPH-009 — Criterio 1; RF-GPH-010 — Criterio 1; RF-GPH-011 — Criterio 2. Disposición: manual_evidence_pending. Evidencia esperada: recibo certified ata JDK release, jvmTarget, sources, compileClasspath resolvedArtifacts y build inputs; no se versiona; no arranca el language server. No acredita una ejecución de esta migración.
  - **type**: positivo
  - **Dado**: módulo fixture copiado a sandbox, red off, Gradle 8.10 y JDK 11 ya presentes, sin wrapper
  - **Cuando**: MI_LSP_KOTLIN_EXPORT_MAIN_JVM=1 bash worker-kotlin/config/export-main-jvm.sh /sandbox/modulo
  - **Entonces**: recibo certified ata JDK release, jvmTarget, sources, compileClasspath resolvedArtifacts y build inputs; no se versiona; no arranca el language server
  - **id**: TC-GPH-K06
  - RF-GPH-001 — Criterio 1; RF-GPH-002 — Criterio 3; RF-GPH-003 — Criterio 1; RF-GPH-004 — Criterio 4; RF-GPH-005 — Criterio 1; RF-GPH-006 — Criterio 1; RF-GPH-007 — Criterio 1; RF-GPH-008 — Criterio 1; RF-GPH-009 — Criterio 4; RF-GPH-010 — Criterio 1; RF-GPH-011 — Criterio 3. Disposición: manual_evidence_pending. Evidencia esperada: unsupported/unavailable explícito sin full-platform PASS; no resolved-call export inventado. No acredita una ejecución de esta migración.
  - **type**: negativo
  - **Dado**: calls o scripts/AGP/variantes/K2 universal, CRLF o Windows sin Job Objects
  - **Cuando**: se solicita soporte fuera del dominio raw v2
  - **Entonces**: unsupported/unavailable explícito sin full-platform PASS; no resolved-call export inventado
  - **id**: TC-GPH-K07
  - RF-GPH-008 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: hashes launcher/patch/source/JARs y versiones fijadas; MIT fwcd conservada; inventario real de transitivas cotejado sin asumir licencia universal; compiler-only no ejecuta SQL/Gradle/indexación. No acredita una ejecución de esta migración.
  - **type**: positivo
  - **Dado**: distribución futura construida con Gradle 8.10/JDK 11 y caches offline
  - **Cuando**: owner inspecciona artefacto, licencias y source provenance
  - **Entonces**: hashes launcher/patch/source/JARs y versiones fijadas; MIT fwcd conservada; inventario real de transitivas cotejado sin asumir licencia universal; compiler-only no ejecuta SQL/Gradle/indexación

Estos casos son planes manuales, no unit tests nuevos ni evidencia ejecutada. Las siete fuentes fixture existen en `testdata/kotlin-graph-main-v1/`, sin runner nuevo ni ejecución en BUILD de esta ola wiki. Los casos TC-GPH-K01–K08 **no están acreditados íntegramente** como PASS del plan: en recheck1 histórico (consumido) solo TC-GPH-K08 observó exporter **certified** en sandbox (`init_script_sha256=ebccdf78…`); no afirmar PASS completo del caso ni de la serie. La cadena Go→`compilerObservations/v2` no observó payload y `graph_observe` terminó FAIL antes de los oráculos RF-GPH-008 (todos **NOTRUN**). Commit preview `52964ee` alinea pin Go↔init en fuente **sin** verificación runtime en esta lane. Los seis oráculos de RF-GPH-008 son exigidos para **admisión de grafo** y promoción; el bloqueo semántico actual no debe confundirse con la **publicación SOURCE** ya completada en PR [#113](https://github.com/fgpaz/mi-lsp/pull/113) / `origin/main` `8b7cdc9d…` (merge `2026-10-13T19:06:51Z`): línea `52964ee`, default-disabled en fuente, sin release binaria nueva. Comparar NodeKey/cross-RID tras relocation/range shift/parameter rename/config drift, overloads/receivers y módulos separados; corroborar cada denominador y omission. Aunque el recibo certifique la configuración, publicar grafo exige esos oráculos, ausentes. Una lista vacía, cero diagnostics o initialize exitoso no es PASS. Contadores FINAL_VERIFY/auditoría/recheck del objetivo Kotlin: **1/1/1 agotados** — historia FAIL/`graph_observe`, RF-GPH-008 **NOTRUN**×6 y TC-GPH-K08 partial/certified sandbox **sin reescritura**. Owner **coord-milsp**: deuda semántica y admisión runtime pendientes **sin nuevo presupuesto** (no reparar fixture, relajar guard ni correr QA).




### Aceptación pendiente de la colección fuente (B/C)

Los escenarios normativos no cambian ni crean tests ejecutables ficticios. El estado editorial de la evidencia consumida reside en el bloque TOON siguiente; no altera la fase del parent. Las evidencias históricas de A no se reejecutan ni validan este slice.

- **Marcador estable**: TP-GPH.canonical-source-store-acceptance
- **Estado del comportamiento**: manual_evidence_pending
- **owner**: coord-milsp
- **due**: 2026-10-15
- **fixture**: nueva_aislada_B_C_con_oráculos_fuente_y_WCB_separados
- **verification_wave**: single_recheck1_four_bounded_cases_after_repair_freeze_no_second_FINAL_VERIFY
- **execution_evidence**: FINAL_VERIFY1_PARTIAL_audit1_Changes_requested_preserved_repair_unique_RECHECK1_H1_H2_PASS_scoped_overall_PARTIAL
- **publication_scope**: parent_authorized_H1_H2_scoped_not_full_B_C
- **H1_evidence**: retired_absent_path_doc_ID_binding_ID_normal_excluded_empty_real_unsafe_blocked
- **H2_evidence**: exact_one_row_late_source_budget_no_primary_authority_direct_tests_supporting
- **incremental_evidence**: explicit_incomplete_Git_AM_not_consumed_processed_0_no_changes_no_owner_transition_no_PRODUCT_FAIL
- **q_evidence**: valid_pipeline_same_corpus_DB_one_governance_item_parity_only
- **q_uncovered**:
  - multi_item_sort
  - cursor_resume
  - native_digest
  - source_variation
- **legacy_source_owner_ranges**: partial_visible_zero_ranges_not_exact
- **global_readiness**: Graph_Harness_fullwiki_conformance_not_accredited
- **memory_atomicity_TC_IDX_028**: not_accredited_snapshot_docs_catalog_are_not_memory_ID
- **memory_debt**: owner_coord_milsp_due_before_full_acceptance_or_next_memory_sensitive_change
- **remaining_evidence**: explicit_incomplete_coord_milsp_due_2026_10_15
- **evidence_receipt**: artifacts/milsp-graphv1-store-history-recheck1-evidence-20261013.json
- **Registros**:
  - **id**: TC-GPH-SRC-001
  - RF-GPH-003 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: dos_declaraciones_un_agregado_kind_symbol_fuente_preservados_incluso_schema_contract_migration. No acredita una ejecución de esta migración.
  - **type**: manual-acceptance
  - **RF**: RF-GPH-003
  - **scenario**: dos_IDs_o_superficies_distintos_con_un_solo_WCB
  - **expected**: dos_declaraciones_un_agregado_kind_symbol_fuente_preservados_incluso_schema_contract_migration
  - **Estado del comportamiento**: manual_evidence_pending
  - **id**: TC-GPH-SRC-002
  - RF-GPH-003 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: ninguna_declaración_ambigua_omissions_preservadas_legacy_advisory_sin_ID_surface_inventados. No acredita una ejecución de esta migración.
  - **type**: manual-acceptance
  - **RF**: RF-GPH-003
  - **scenario**: ID_duplicado_document_wide_objeto_inválido_y_legacy
  - **expected**: ninguna_declaración_ambigua_omissions_preservadas_legacy_advisory_sin_ID_surface_inventados
  - **Estado del comportamiento**: manual_evidence_pending
  - **id**: TC-GPH-SRC-003
  - RF-GPH-003 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: docs_fuente_bridge_marker_coherentes_owners_ajenos_idénticos_rollback_conserva_snapshot_previo_RAM_sin_publicar. No acredita una ejecución de esta migración.
  - **type**: manual-acceptance
  - **RF**: RF-GPH-003
  - **scenario**: full_docs_only_incremental_snapshot_RAM_publicación_cancelada_y_owner_excluido_vivo
  - **expected**: docs_fuente_bridge_marker_coherentes_owners_ajenos_idénticos_rollback_conserva_snapshot_previo_RAM_sin_publicar
  - **Estado del comportamiento**: manual_evidence_pending
  - **id**: TC-GPH-SRC-004
  - RF-GPH-003 — Criterio 1; RF-GPH-007 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: lifecycles_independientes_historia_solo_explícita_inventario_retired_conservado_normal_excluido_empty_real_preservado_sin_primary_authority_direct_code_tests_supporting_target_histórico_no_evaluado_sucesor_declarado_separado_no_resurrección_no_versionado_de_ediciones. No acredita una ejecución de esta migración.
  - **type**: manual-acceptance
  - **RF**: RF-GPH-003
  - **scenario**: nav_trace_binding_sources_con_y_sin_historical_binding_deprecated_document_active_document_deprecated_retired_terminal_y_sucesor_incluye_retired_publicado_ausente_disco_por_path_doc_ID_binding_ID_y_owner_realmente_vacío
  - **expected**: lifecycles_independientes_historia_solo_explícita_inventario_retired_conservado_normal_excluido_empty_real_preservado_sin_primary_authority_direct_code_tests_supporting_target_histórico_no_evaluado_sucesor_declarado_separado_no_resurrección_no_versionado_de_ediciones
  - **Estado del comportamiento**: manual_evidence_pending
  - **id**: TC-GPH-SRC-005
  - RF-GPH-003 — Criterio 1; RF-GPH-007 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: explicit_incomplete_sin_DDL_backfill_bridge_disponible_empty_disponible_candidatos_sorted_bounded_truncation_sin_winner_primary_authority_direct_tests_supporting. No acredita una ejecución de esta migración.
  - **type**: manual-acceptance
  - **RF**: RF-GPH-003
  - **scenario**: DB_anterior_sin_tablas_owner_no_publicado_empty_parsed_DOC_ID_y_doc_path_AND_binding_id_ID_global_en_varios_owners_y_límite_200_incluye_truncación_tardía_outputbudget_tras_selección_activa
  - **expected**: explicit_incomplete_sin_DDL_backfill_bridge_disponible_empty_disponible_candidatos_sorted_bounded_truncation_sin_winner_primary_authority_direct_tests_supporting
  - **Estado del comportamiento**: manual_evidence_pending
  - **id**: TC-GPH-SRC-006
  - RF-GPH-003 — Criterio 1; RF-GPH-007 — Criterio 4. Disposición: manual_evidence_pending. Evidencia esperada: metadata_histórica_separada_sin_direct_code_ni_ranking_activos_q_v1_sort_digest_defaults_sin_delta_digest_fuente_separado_sin_cursor_rangos_desconocidos_no_exactos. No acredita una ejecución de esta migración.
  - **type**: manual-acceptance
  - **RF**: RF-GPH-007
  - **scenario**: consumidor_C_histórico_explícito_y_normal_direct_daemon
  - **expected**: metadata_histórica_separada_sin_direct_code_ni_ranking_activos_q_v1_sort_digest_defaults_sin_delta_digest_fuente_separado_sin_cursor_rangos_desconocidos_no_exactos
  - **Estado del comportamiento**: manual_evidence_pending
- **Comprobación prevista**:
  - parent_single_recheck1_H1_H2_incremental_real_q_v1_válido_other_gaps_explicit_incomplete
- **Evidencia referenciada**:
  - .docs/wiki/05_requirements/RF-GPH-003.md
  - .docs/wiki/07_technical/db/DB-DOC-INDEX.md
  - .docs/wiki/07_technical/contracts/CT-NAV-WIKI.md

La entrada pública real es `nav trace` o `nav wiki trace` con `--binding-sources`, documento ID posicional o `--doc <path>` y `--binding-id <ID>` opcional; `--historical` es la única habilitación de metadata fuente inactiva. Se inspecciona `wiki_code_context.binding_sources`, no se ejecuta un comando nuevo. TC-GPH-SRC-003 cubre mapa RAM ausente vs disponible vacío, tombstones owner/bloque, read/parse/concurrent-change y comparación contra lifecycle persistido cuando falta artifact-state. TC-GPH-SRC-005 exige AND exacto, candidatos bounded ante colisión y ninguna evidencia activa al truncar. El bloque TOON anterior registra el único recheck1 consumido y sus límites; los casos completos siguen manual_evidence_pending. No se habilita otra wave, audit/recheck2 ni conformance por esta alineación editorial.

El oráculo debe comparar los siete campos de cada objeto con su fuente y comprobar por separado la asociación WCB, incluidos cambios de bloque sin cambio de `(doc_path,binding_id)`. Debe cubrir owner vivo excluido, retiro sin bindings y colisión de ID entre documentos. Los rangos 0/0 permanecen desconocidos. Ni un lookup exitoso ni la asociación a un target actual otorgan conformance o evidencia compiler histórica.

### Contrato comun de prueba

- Fixtures versionadas: C#, Go, TypeScript, Python, wiki, mixed, cross-workspace, extensions y relations (`positive`, `negative`, `ambiguous`, `unresolved`, `not-comparable`).
- Oraculos: JSON canonico/goldens, hashes de fixture, invariantes SQLite, source digests, cross-RID y comparacion incremental-clean. Un LLM o screenshot nunca es el único juez.
- Toda medicion competitiva fija Graphify `9bf14a4931658152969586ace39eb965c010f0d1`, baseline mi-lsp `a251ab1f8db4e96f029926fbef275b078a20a111`, fixture digest, comando, entorno y raw samples.
- Benchmarks relevantes ejecutan 30 repeticiones por variante con el mismo protocolo cold/warm/incremental. Metrica unavailable es `BLOCKED/NOT_COMPARABLE`, no PASS.
- Evidencia durable es sanitizada: no secretos, raw compiler logs ni payloads arbitrarios. Cada fila incluye generation/cross-RID o explica por que no aplica.
- Gates fail-closed: governance, hash, comparator, baseline, cross-RID, determinismo, no-MCP/no-network, rollback y negative violations.

- **Marcador estable**: TP-GPH.native-hardening-cases
- **Evidencia referenciada**:
  - .docs/wiki/06_tests/TP-GPH.md
  - internal/indexer/graph_staging.go
  - internal/indexer/graph_staging.go
  - internal/indexer/incremental_graph_repair_test.go
  - internal/model/graph_observation.go
  - internal/service/graph_observer.go
  - worker-dotnet/MiLsp.Worker/RoslynService.cs
  - worker-dotnet/MiLsp.Worker.ContractTests/Program.cs
- **Registros**:
  - **id**: TP-GPH-001
  - RF-GPH-001 — Criterio 1; RF-GPH-002 — Criterio 1; RF-GPH-003 — Criterio 1; RF-GPH-004 — Criterio 1; RF-GPH-005 — Criterio 1; RF-GPH-006 — Criterio 1; RF-GPH-007 — Criterio 1; RF-GPH-008 — Criterio 1; RF-GPH-009 — Criterio 1; RF-GPH-010 — Criterio 1; RF-GPH-011 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
  - **type**: test-suite
  - **id**: TP-GPH-002
  - RF-GPH-001 — Criterio 1; RF-GPH-002 — Criterio 1; RF-GPH-003 — Criterio 1; RF-GPH-004 — Criterio 1; RF-GPH-005 — Criterio 1; RF-GPH-006 — Criterio 1; RF-GPH-007 — Criterio 1; RF-GPH-008 — Criterio 1; RF-GPH-009 — Criterio 1; RF-GPH-010 — Criterio 1; RF-GPH-011 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
  - **type**: test-suite
  - **id**: TP-GPH-003
  - RF-GPH-001 — Criterio 1; RF-GPH-002 — Criterio 1; RF-GPH-003 — Criterio 1; RF-GPH-004 — Criterio 1; RF-GPH-005 — Criterio 1; RF-GPH-006 — Criterio 1; RF-GPH-007 — Criterio 1; RF-GPH-008 — Criterio 1; RF-GPH-009 — Criterio 1; RF-GPH-010 — Criterio 1; RF-GPH-011 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
  - **type**: test-suite
  - **id**: TP-GPH-004
  - RF-GPH-001 — Criterio 1; RF-GPH-002 — Criterio 1; RF-GPH-003 — Criterio 1; RF-GPH-004 — Criterio 1; RF-GPH-005 — Criterio 1; RF-GPH-006 — Criterio 1; RF-GPH-007 — Criterio 1; RF-GPH-008 — Criterio 1; RF-GPH-009 — Criterio 1; RF-GPH-010 — Criterio 1; RF-GPH-011 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
  - **type**: test-suite
  - **id**: TP-GPH-005
  - RF-GPH-001 — Criterio 1; RF-GPH-002 — Criterio 1; RF-GPH-003 — Criterio 1; RF-GPH-004 — Criterio 1; RF-GPH-005 — Criterio 1; RF-GPH-006 — Criterio 1; RF-GPH-007 — Criterio 1; RF-GPH-008 — Criterio 1; RF-GPH-009 — Criterio 1; RF-GPH-010 — Criterio 1; RF-GPH-011 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
  - **type**: test-suite
  - **id**: TP-GPH-006
  - RF-GPH-001 — Criterio 1; RF-GPH-002 — Criterio 1; RF-GPH-003 — Criterio 1; RF-GPH-004 — Criterio 1; RF-GPH-005 — Criterio 1; RF-GPH-006 — Criterio 1; RF-GPH-007 — Criterio 1; RF-GPH-008 — Criterio 1; RF-GPH-009 — Criterio 1; RF-GPH-010 — Criterio 1; RF-GPH-011 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
  - **type**: test-suite
  - **id**: TP-GPH-007
  - RF-GPH-001 — Criterio 1; RF-GPH-002 — Criterio 1; RF-GPH-003 — Criterio 1; RF-GPH-004 — Criterio 1; RF-GPH-005 — Criterio 1; RF-GPH-006 — Criterio 1; RF-GPH-007 — Criterio 1; RF-GPH-008 — Criterio 1; RF-GPH-009 — Criterio 1; RF-GPH-010 — Criterio 1; RF-GPH-011 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
  - **type**: test-suite
  - **id**: TP-GPH-008
  - RF-GPH-001 — Criterio 1; RF-GPH-002 — Criterio 1; RF-GPH-003 — Criterio 1; RF-GPH-004 — Criterio 1; RF-GPH-005 — Criterio 1; RF-GPH-006 — Criterio 1; RF-GPH-007 — Criterio 1; RF-GPH-008 — Criterio 1; RF-GPH-009 — Criterio 1; RF-GPH-010 — Criterio 1; RF-GPH-011 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
  - **type**: test-suite
  - **id**: TP-GPH-009
  - RF-GPH-001 — Criterio 3; RF-GPH-002 — Criterio 1; RF-GPH-003 — Criterio 1; RF-GPH-004 — Criterio 1; RF-GPH-005 — Criterio 2; RF-GPH-006 — Criterio 4; RF-GPH-007 — Criterio 1; RF-GPH-008 — Criterio 1; RF-GPH-009 — Criterio 3; RF-GPH-010 — Criterio 1; RF-GPH-011 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
  - **type**: test-suite
- **Comprobación prevista**:
  - go test ./internal/model/... ./internal/indexer/... ./internal/service/... ./internal/daemon/...
  - dotnet run --project worker-dotnet/MiLsp.Worker.ContractTests/MiLsp.Worker.ContractTests.csproj
- **Condiciones de rechazo**:
  - worker_order_is_not_ref=true
  - worker_batch_sealed_before_core=true
  - unsupported_symbol_reported_as_unresolved=true
  - unresolved_ids_not_stable=true
  - candidate_bounds_or_normalization_wrong=true
- **Casos**:
  - **id**: TC-GPH-068
  - RF-GPH-001 — Criterio 1; RF-GPH-002 — Criterio 1; RF-GPH-003 — Criterio 1; RF-GPH-004 — Criterio 1; RF-GPH-005 — Criterio 1; RF-GPH-006 — Criterio 1; RF-GPH-007 — Criterio 3; RF-GPH-008 — Criterio 1; RF-GPH-009 — Criterio 1; RF-GPH-010 — Criterio 3; RF-GPH-011 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: nodes are ordered by Ref and the batch is canonical but unsealed. No acredita una ejecución de esta migración.
  - **type**: positivo
  - **Dado**: worker Roslyn emits a graph observation
  - **Cuando**: nodes are serialized
  - **Entonces**: nodes are ordered by Ref and the batch is canonical but unsealed
  - **Evidencia referenciada**: worker-dotnet/MiLsp.Worker.ContractTests/Program.cs
  - **id**: TC-GPH-069
  - RF-GPH-001 — Criterio 1; RF-GPH-002 — Criterio 1; RF-GPH-003 — Criterio 1; RF-GPH-004 — Criterio 1; RF-GPH-005 — Criterio 1; RF-GPH-006 — Criterio 1; RF-GPH-007 — Criterio 1; RF-GPH-008 — Criterio 1; RF-GPH-009 — Criterio 1; RF-GPH-010 — Criterio 1; RF-GPH-011 — Criterio 4. Disposición: manual_evidence_pending. Evidencia esperada: core executes ValidateCanonical, SealGraphObservationBatch and ReadyForStaging in that order. No acredita una ejecución de esta migración.
  - **type**: positivo
  - **Dado**: canonical unsealed worker batch reaches core
  - **Cuando**: graphObserver accepts it
  - **Entonces**: core executes ValidateCanonical, SealGraphObservationBatch and ReadyForStaging in that order
  - **Evidencia referenciada**: internal/service/graph_observer.go; internal/model/graph_observation.go
  - **id**: TC-GPH-070
  - RF-GPH-001 — Criterio 1; RF-GPH-002 — Criterio 1; RF-GPH-003 — Criterio 1; RF-GPH-004 — Criterio 1; RF-GPH-005 — Criterio 1; RF-GPH-006 — Criterio 1; RF-GPH-007 — Criterio 3; RF-GPH-008 — Criterio 1; RF-GPH-009 — Criterio 1; RF-GPH-010 — Criterio 3; RF-GPH-011 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: a typed omission is emitted instead of a false unresolved or semantic edge. No acredita una ejecución de esta migración.
  - **type**: negativo
  - **Dado**: local, lambda, anonymous or synthesized/implicit symbol is not an eligible graph endpoint
  - **Cuando**: Roslyn cannot represent it
  - **Entonces**: a typed omission is emitted instead of a false unresolved or semantic edge
  - **Evidencia referenciada**: worker-dotnet/MiLsp.Worker/RoslynService.cs; worker-dotnet/MiLsp.Worker.ContractTests/Program.cs
  - **id**: TC-GPH-071
  - RF-GPH-001 — Criterio 1; RF-GPH-002 — Criterio 1; RF-GPH-003 — Criterio 1; RF-GPH-004 — Criterio 1; RF-GPH-005 — Criterio 4; RF-GPH-006 — Criterio 1; RF-GPH-007 — Criterio 3; RF-GPH-008 — Criterio 2; RF-GPH-009 — Criterio 1; RF-GPH-010 — Criterio 3; RF-GPH-011 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: GraphUnresolved se ordena y deduplica por key antes de asignar IDs y CrossRID; source_document, source_block, target_kind y target_value son contexto nullable, acotado y sanitizado cuando se expone, no cambian UnresolvedKey/CrossRID, pero una modificación solo de provenance debe cambiar los digests de contenido/facts y los fingerprints de source/config/backend/generation.. No acredita una ejecución de esta migración.
  - **type**: negativo
  - **Dado**: un endpoint elegible está realmente ausente
  - **Cuando**: el ensamblado del grafo materializa registros unresolved
  - **Entonces**: GraphUnresolved se ordena y deduplica por key antes de asignar IDs y CrossRID; source_document, source_block, target_kind y target_value son contexto nullable, acotado y sanitizado cuando se expone, no cambian UnresolvedKey/CrossRID, pero una modificación solo de provenance debe cambiar los digests de contenido/facts y los fingerprints de source/config/backend/generation.
  - **Evidencia referenciada**: internal/indexer/graph_staging.go; internal/model/graph.go; internal/store/graph_query.go; internal/store/index_publish.go
  - **id**: TC-GPH-072
  - RF-GPH-001 — Criterio 1; RF-GPH-002 — Criterio 1; RF-GPH-003 — Criterio 3; RF-GPH-004 — Criterio 1; RF-GPH-005 — Criterio 4; RF-GPH-006 — Criterio 1; RF-GPH-007 — Criterio 3; RF-GPH-008 — Criterio 2; RF-GPH-009 — Criterio 2; RF-GPH-010 — Criterio 1; RF-GPH-011 — Criterio 3. Disposición: manual_evidence_pending. Evidencia esperada: trim, normalización a slash, deduplicación y orden léxico ocurren antes de los límites de 64 elementos y 4096 bytes; una mención TP-* se resuelve por coincidencia exacta de DocRecord.doc_id: un único target publica nodo/edge y el SourceBlock de la mención resuelta se normaliza antes de entrar al digest de claim y al mention portion del digest de facts; cambiar solo ese SourceBlock cambia content/facts y los fingerprints source/config/backend/generation, mientras whitespace y orden de entrada canónicos permanecen estables; un target faltante o duplicado queda GraphUnresolved con reason_code=missing_doc_target|ambiguous_doc_target, target_kind=document, target_value solicitado y candidatos bounded. La persistencia conserva source_block en reemplazos full e incremental y el round-trip de GraphUnresolved mantiene valores presentes y NULL.. No acredita una ejecución de esta migración.
  - **type**: negativo
  - **Dado**: candidatos documentales contienen whitespace, backslashes o duplicados, o una mención trae un ID TP-*
  - **Cuando**: se construyen los candidatos, menciones y diagnósticos GraphUnresolved
  - **Entonces**: trim, normalización a slash, deduplicación y orden léxico ocurren antes de los límites de 64 elementos y 4096 bytes; una mención TP-* se resuelve por coincidencia exacta de DocRecord.doc_id: un único target publica nodo/edge y el SourceBlock de la mención resuelta se normaliza antes de entrar al digest de claim y al mention portion del digest de facts; cambiar solo ese SourceBlock cambia content/facts y los fingerprints source/config/backend/generation, mientras whitespace y orden de entrada canónicos permanecen estables; un target faltante o duplicado queda GraphUnresolved con reason_code=missing_doc_target|ambiguous_doc_target, target_kind=document, target_value solicitado y candidatos bounded. La persistencia conserva source_block en reemplazos full e incremental y el round-trip de GraphUnresolved mantiene valores presentes y NULL.
  - **Evidencia referenciada**: internal/indexer/graph_staging.go; internal/indexer/graph_staging.go; internal/store/index_publish.go
  - **id**: TC-GPH-073
  - RF-GPH-001 — Criterio 1; RF-GPH-002 — Criterio 1; RF-GPH-003 — Criterio 1; RF-GPH-004 — Criterio 1; RF-GPH-005 — Criterio 1; RF-GPH-006 — Criterio 1; RF-GPH-007 — Criterio 4; RF-GPH-008 — Criterio 1; RF-GPH-009 — Criterio 1; RF-GPH-010 — Criterio 3; RF-GPH-011 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: ambos son canónicos; .docs/raw y src/main.go no lo son. No acredita una ejecución de esta migración.
  - **type**: positivo
  - **Dado**: paths wiki/10-chiamo.md y bibliotecas/memorias/ficha.md
  - **Cuando**: isCanonicalGraphDoc
  - **Entonces**: ambos son canónicos; .docs/raw y src/main.go no lo son
  - **Evidencia referenciada**: internal/indexer/indexer.go
  - **id**: TC-GPH-074
  - RF-GPH-001 — Criterio 1; RF-GPH-002 — Criterio 1; RF-GPH-003 — Criterio 1; RF-GPH-004 — Criterio 1; RF-GPH-005 — Criterio 3; RF-GPH-006 — Criterio 1; RF-GPH-007 — Criterio 1; RF-GPH-008 — Criterio 1; RF-GPH-009 — Criterio 1; RF-GPH-010 — Criterio 1; RF-GPH-011 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: publishGraph=true y se ensambla generation documental; sin identidad publicable, docs continúa y devuelve warning sanitizado. No acredita una ejecución de esta migración.
  - **type**: positivo
  - **Dado**: docs-only con Markdown canónico, identidad explícita o un único origin y sin batches de compilador
  - **Cuando**: documentationGraphRequest
  - **Entonces**: publishGraph=true y se ensambla generation documental; sin identidad publicable, docs continúa y devuelve warning sanitizado
  - **Evidencia referenciada**: internal/indexer/indexer.go; internal/indexer/indexer.go
  - **id**: TC-GPH-075
  - RF-GPH-001 — Criterio 1; RF-GPH-002 — Criterio 1; RF-GPH-003 — Criterio 1; RF-GPH-004 — Criterio 1; RF-GPH-005 — Criterio 4; RF-GPH-006 — Criterio 1; RF-GPH-007 — Criterio 4; RF-GPH-008 — Criterio 1; RF-GPH-009 — Criterio 4; RF-GPH-010 — Criterio 3; RF-GPH-011 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: la prueba de contrato deduplica solo registros contractualmente idénticos; una misma referencia con identidad distinta sigue fallando cerrado. No acredita una ejecución de esta migración.
  - **type**: negativo
  - **Estado del comportamiento**: verificado
  - **Dado**: el smoke de Gastos de v0.8.0 encuentra referencias de declaración Roslyn duplicadas con la misma identidad
  - **Cuando**: se normaliza el batch y se repite el workspace real
  - **Entonces**: la prueba de contrato deduplica solo registros contractualmente idénticos; una misma referencia con identidad distinta sigue fallando cerrado
  - **Oráculo**: prueba de contrato y rerun del workspace real de Gastos
  - **release_readback**: el artefacto publicado v0.8.1 linux-x64 en d0da7243fe012eba8bbb41b312ca292643ae8a59 indexó el clon temporal rastreado de Gastos en f9dd97e86888761e05ff9cf751c7b151f6854813 con 1278 archivos y 15656 símbolos; GPH_OBS_REF_DUPLICATE ausente
  - **Evidencia referenciada**: worker-dotnet/MiLsp.Worker/RoslynService.cs; worker-dotnet/MiLsp.Worker.ContractTests/Program.cs; .docs/auditoria/2026-08-26-live-wiki-code-bridge/release-readback-v0.8.1.yaml
  - **id**: TC-GPH-076
  - RF-GPH-007 — Criterio 4. Disposición: manual_evidence_pending. Evidencia esperada: el índice responde ok=true, publica catálogo y docs y omite el grafo con warning accionable (sin derivar identidad del path); con entrypoint explícito sigue siendo error; los errores de grafo salen tipados (code gph_*, stage graph_observation) y ningún error de index queda como index_generic (fallback index_failed). No acredita una ejecución de esta migración.
  - **type**: negativo
  - **Dado**: repo git con código Go, sin remote origin ni repository_identity declarada, con HOME y registry aislados
  - **Cuando**: mi-lsp index (selección automática) y, aparte, con entrypoint explícito
  - **Entonces**: el índice responde ok=true, publica catálogo y docs y omite el grafo con warning accionable (sin derivar identidad del path); con entrypoint explícito sigue siendo error; los errores de grafo salen tipados (code gph_*, stage graph_observation) y ningún error de index queda como index_generic (fallback index_failed)
  - **Evidencia referenciada**: internal/indexer/graph_identity_unavailable_test.go; internal/cli/index_error_classification_test.go; internal/daemon/error_classification_test.go
  - **id**: TC-GPH-077
  - RF-GPH-007 — Criterio 2. Disposición: manual_evidence_pending. Evidencia esperada: git_commit, git_base y git_frontier se resuelven; git_frontier es determinista e incorpora base, merge-base, HEAD, diff committed, diff tracked contra HEAD y contenidos untracked por ruta normalizada; cada clase de cambio altera el digest y la lectura no escribe ni repara el índice. No acredita una ejecución de esta migración.
  - **type**: positivo
  - **Estado del comportamiento**: partial
  - **Dado**: repo git con base y HEAD resolubles, cambios comprometidos, tracked staged/unstaged y archivos untracked no ignorados
  - **Cuando**: mi-lsp nav git-frontier se ejecuta dos veces en el mismo snapshot
  - **Entonces**: git_commit, git_base y git_frontier se resuelven; git_frontier es determinista e incorpora base, merge-base, HEAD, diff committed, diff tracked contra HEAD y contenidos untracked por ruta normalizada; cada clase de cambio altera el digest y la lectura no escribe ni repara el índice
  - **status_detail**: la prueba existente cubre repetibilidad, diff committed, tracked unstaged y untracked; faltan asserts del envelope público, staged, generaciones y ausencia de escrituras
  - **Evidencia referenciada**: internal/cli/git_frontier_test.go; internal/cli/git_frontier.go
  - **id**: TC-GPH-078
  - RF-GPH-007 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: campos sin evidencia permanecen null o unavailable, coherence/freshness no simulan PASS y unavailable/generation_reasons llevan códigos tipados; no se derivan IDs ni digests de timestamps, paths o valores por defecto. No acredita una ejecución de esta migración.
  - **type**: negativo
  - **Estado del comportamiento**: planned
  - **Dado**: workspace no resoluble, Git/HEAD/base inaccesible, DB ausente o generación/frescura no publicada
  - **Cuando**: mi-lsp nav git-frontier consulta la frontera pública
  - **Entonces**: campos sin evidencia permanecen null o unavailable, coherence/freshness no simulan PASS y unavailable/generation_reasons llevan códigos tipados; no se derivan IDs ni digests de timestamps, paths o valores por defecto
  - **Evidencia referenciada**: internal/cli/git_frontier.go
  - **id**: TC-GPH-079
  - RF-GPH-002 — Criterio 3. Disposición: manual_evidence_pending. Evidencia esperada: publica un grafo documental válido y graph freshness queda current contra la generación de catálogo activa. No acredita una ejecución de esta migración.
  - **type**: positivo
  - **Dado**: catálogo sin cambios, grafo stale, Markdown canónico e identidad de repositorio, sin batch semántico completo
  - **Cuando**: el reindex incremental repara el grafo
  - **Entonces**: publica un grafo documental válido y graph freshness queda current contra la generación de catálogo activa
  - **Evidencia referenciada**: internal/indexer/incremental.go; internal/indexer/incremental_graph_repair_test.go
  - **id**: TC-GPH-080
  - RF-GPH-002 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: devuelve GraphObservationError con code=graph_repair_incomplete; no termina como éxito sin cambios. No acredita una ejecución de esta migración.
  - **type**: negativo
  - **Dado**: grafo stale y ningún batch completo ni identidad para publicar el fallback documental
  - **Cuando**: el reindex incremental intenta repararlo
  - **Entonces**: devuelve GraphObservationError con code=graph_repair_incomplete; no termina como éxito sin cambios
  - **Evidencia referenciada**: internal/indexer/incremental.go; internal/indexer/incremental_graph_repair_test.go

### TP-GPH-001 - Identidad, NodeKey y cross-RID
  - RF-GPH-001 — Criterio 2; RF-GPH-002 — Criterio 1; RF-GPH-003 — Criterio 3; RF-GPH-004 — Criterio 1; RF-GPH-005 — Criterio 4; RF-GPH-006 — Criterio 4; RF-GPH-007 — Criterio 1; RF-GPH-008 — Criterio 1; RF-GPH-009 — Criterio 4; RF-GPH-010 — Criterio 1; RF-GPH-011 — Criterio 4. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.

> `wiki_source_table_exception: true`. Las tablas de casos heredadas se conservan como índice humano de cobertura; los bloques `toon` de este documento son la fuente normativa para los contratos LLM-first nuevos y sus gates.

**Cobertura:** RF-GPH-001 y modelo semantico.
- **Caso**: TC-GPH-001; **Tipo**: positivo; **Descripcion / oraculo**: golden vectors validan payload length-prefixed, SHA-256 BLOB(32), hex externo y cross-RID
  - RF-GPH-001 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
- **Caso**: TC-GPH-002; **Tipo**: positivo; **Descripcion / oraculo**: relocation de root, CRLF/LF y cambio solo de rango conservan NodeKey; cambio semántico lo cambia
  - RF-GPH-001 — Criterio 3. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
- **Caso**: TC-GPH-003; **Tipo**: positivo; **Descripcion / oraculo**: 30 reruns y RIDs soportados producen identidad/cross-RID byte-identical
  - RF-GPH-001 — Criterio 2. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
- **Caso**: TC-GPH-004; **Tipo**: negativo; **Descripcion / oraculo**: repository identity/campo requerido ausente, path absoluto/traversal o selector Go con backslash rechazado antes de normalización queda unresolved
  - RF-GPH-001 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
- **Caso**: TC-GPH-005; **Tipo**: negativo; **Descripcion / oraculo**: colision simulada del digest con tupla distinta inválida la generation y no rehasha con sal
  - RF-GPH-001 — Criterio 3. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
- **Caso**: TC-GPH-006; **Tipo**: negativo; **Descripcion / oraculo**: Unicode no NFC, version desconocida, case/path conflictivo o símbolo anonimo sin anchor no se publica
  - RF-GPH-001 — Criterio 3. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.

### TP-GPH-002 - Generation, publicación, migración y recovery
  - RF-GPH-001 — Criterio 1; RF-GPH-002 — Criterio 2; RF-GPH-003 — Criterio 1; RF-GPH-004 — Criterio 1; RF-GPH-005 — Criterio 3; RF-GPH-006 — Criterio 1; RF-GPH-007 — Criterio 1; RF-GPH-008 — Criterio 1; RF-GPH-009 — Criterio 1; RF-GPH-010 — Criterio 1; RF-GPH-011 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.

**Cobertura:** RF-GPH-002.
- **Caso**: TC-GPH-007; **Tipo**: positivo; **Descripcion / oraculo**: staging completo permanece invisible y pointer swap publica una única generation sellada
  - RF-GPH-002 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
- **Caso**: TC-GPH-008; **Tipo**: positivo; **Descripcion / oraculo**: readers concurrentes observan snapshot viejo o nuevo completo, nunca mezcla
  - RF-GPH-002 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
- **Caso**: TC-GPH-009; **Tipo**: negativo; **Descripcion / oraculo**: crash/cancel en cada ventana inválida staging y restaura/conserva pointer anterior
  - RF-GPH-002 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
- **Caso**: TC-GPH-010; **Tipo**: positivo; **Descripcion / oraculo**: migración additive mantiene tablas legacy y dual-read/write transaccional durante compatibilidad
  - RF-GPH-002 — Criterio 2. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
- **Caso**: TC-GPH-011; **Tipo**: positivo; **Descripcion / oraculo**: rollback/downgrade ejercitado vuelve a snapshot/schema compatible sin perdida del backup metadata
  - RF-GPH-002 — Criterio 2. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
- **Caso**: TC-GPH-012; **Tipo**: negativo; **Descripcion / oraculo**: pointer conflict reintenta explicitamente; publish del mismo ID/payload es idempotente
  - RF-GPH-002 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
- **Caso**: TC-GPH-013; **Tipo**: negativo; **Descripcion / oraculo**: mismo generation ID con digest/counts distintos bloquea como corrupcion
  - RF-GPH-002 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
- **Caso**: TC-GPH-014; **Tipo**: negativo; **Descripcion / oraculo**: query-time migration o repair implicito falla el test de solo lectura
  - RF-GPH-002 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
- **Caso**: TC-GPH-014A; **Tipo**: positivo; **Descripcion / oraculo**: `index --docs-only` publica GraphGeneration documental cuando hay markdown canónico en wiki/ o bibliotecas/ sin batches de compilador
  - RF-GPH-001 — Criterio 1; RF-GPH-002 — Criterio 1; RF-GPH-003 — Criterio 1; RF-GPH-004 — Criterio 1; RF-GPH-005 — Criterio 1; RF-GPH-006 — Criterio 1; RF-GPH-007 — Criterio 2; RF-GPH-008 — Criterio 1; RF-GPH-009 — Criterio 1; RF-GPH-010 — Criterio 3; RF-GPH-011 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
- **Caso**: TC-GPH-079; **Tipo**: positivo; **Descripcion / oraculo**: reindex incremental publica un grafo documental fresco cuando el catálogo no cambia, el grafo está stale y falta un batch semántico completo
  - RF-GPH-002 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
- **Caso**: TC-GPH-080; **Tipo**: negativo; **Descripcion / oraculo**: reindex incremental devuelve `graph_repair_incomplete` cuando no hay batch completo ni identidad para reparar el grafo stale
  - RF-GPH-002 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.

### TP-GPH-003 - Edges, adapters e incrementalidad
  - RF-GPH-001 — Criterio 1; RF-GPH-002 — Criterio 1; RF-GPH-003 — Criterio 3; RF-GPH-004 — Criterio 1; RF-GPH-005 — Criterio 1; RF-GPH-006 — Criterio 1; RF-GPH-007 — Criterio 1; RF-GPH-008 — Criterio 1; RF-GPH-009 — Criterio 1; RF-GPH-010 — Criterio 1; RF-GPH-011 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.

**Cobertura:** RF-GPH-003, RF-GPH-004 y RF-GPH-008.
- **Caso**: TC-GPH-015; **Tipo**: positivo; **Descripcion / oraculo**: Roslyn emite declarations/contains/references/calls/implements/extends exactos con provenance
  - RF-GPH-003 — Criterio 1; RF-GPH-008 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
- **Caso**: TC-GPH-016; **Tipo**: positivo; **Descripcion / oraculo**: Go emite declarations/contains/imports y solo promueve calls/references cuando types/gopls resuelve; `typeCheckAll` comparte importer/cache para conservar identidad de paquetes importados
  - RF-GPH-003 — Criterio 1; RF-GPH-008 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
- **Caso**: TC-GPH-016A; **Tipo**: positivo; **Descripcion / oraculo**: `TestObserveGoGraphSharesExportImporterIdentity`: imports de libreria estandar mantienen identidad coherente y el batch limpio permanece completo y stageable
  - RF-GPH-001 — Criterio 1; RF-GPH-002 — Criterio 1; RF-GPH-003 — Criterio 1; RF-GPH-004 — Criterio 1; RF-GPH-005 — Criterio 1; RF-GPH-006 — Criterio 1; RF-GPH-007 — Criterio 2; RF-GPH-008 — Criterio 1; RF-GPH-009 — Criterio 1; RF-GPH-010 — Criterio 1; RF-GPH-011 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
- **Caso**: TC-GPH-016B; **Tipo**: negativo; **Descripcion / oraculo**: `TestObserveGoGraphLocalTargetsAreUnsupported`: declaraciones locales no representadas se registran como omission `unsupported_symbol_kind`; no crean unresolved ni alteran completeness/ReadyForStaging
  - RF-GPH-001 — Criterio 1; RF-GPH-002 — Criterio 1; RF-GPH-003 — Criterio 1; RF-GPH-004 — Criterio 1; RF-GPH-005 — Criterio 1; RF-GPH-006 — Criterio 1; RF-GPH-007 — Criterio 2; RF-GPH-008 — Criterio 1; RF-GPH-009 — Criterio 1; RF-GPH-010 — Criterio 1; RF-GPH-011 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
- **Caso**: TC-GPH-016C; **Tipo**: positivo; **Descripcion / oraculo**: `TestObserveGoGraphTopLevelEmbeddedFieldIsStageable`: un campo embebido declarado por AST se materializa como endpoint `field` con evidencia y `contains`; la observación conserva `complete` y pasa `ReadyForStaging`, mientras type-load, parseo, cancelación y endpoints realmente ausentes siguen `partial` y fail-closed
  - RF-GPH-001 — Criterio 3; RF-GPH-002 — Criterio 1; RF-GPH-003 — Criterio 1; RF-GPH-004 — Criterio 1; RF-GPH-005 — Criterio 1; RF-GPH-006 — Criterio 1; RF-GPH-007 — Criterio 2; RF-GPH-008 — Criterio 1; RF-GPH-009 — Criterio 1; RF-GPH-010 — Criterio 1; RF-GPH-011 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
- **Caso**: TC-GPH-016D; **Tipo**: positivo; **Descripcion / oraculo**: `TestObserveGoGraphIgnoresTestOnlyPackages`: un directorio incluido por `go list` solo por `XTestGoFiles` no vuelve parcial el grafo ni impide publicar declaraciones del módulo
  - RF-GPH-001 — Criterio 1; RF-GPH-002 — Criterio 1; RF-GPH-003 — Criterio 1; RF-GPH-004 — Criterio 1; RF-GPH-005 — Criterio 4; RF-GPH-006 — Criterio 1; RF-GPH-007 — Criterio 2; RF-GPH-008 — Criterio 1; RF-GPH-009 — Criterio 1; RF-GPH-010 — Criterio 1; RF-GPH-011 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
- **Caso**: TC-GPH-017; **Tipo**: negativo; **Descripcion / oraculo**: tsserver ausente/experimental produce omission; texto no crea edge semántica
  - RF-GPH-003 — Criterio 1; RF-GPH-008 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
- **Caso**: TC-GPH-018; **Tipo**: negativo; **Descripcion / oraculo**: Pyright ausente/experimental y extractor lexical producen candidatos/unresolved, no compiler facts
  - RF-GPH-003 — Criterio 1; RF-GPH-008 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
- **Caso**: TC-GPH-019; **Tipo**: negativo; **Descripcion / oraculo**: ambiguous, stale o endpoint missing produce GraphUnresolved; validación confirma cero dangling edges
  - RF-GPH-003 — Criterio 3; RF-GPH-004 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
- **Caso**: TC-GPH-020; **Tipo**: positivo; **Descripcion / oraculo**: create/change/delete/rename reemplaza solo owners afectados y conserva snapshot equivalente al clean
  - RF-GPH-004 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
- **Caso**: TC-GPH-021; **Tipo**: positivo; **Descripcion / oraculo**: cambio de superficie recalcula fanout; uncertainty/config/backend drift fuerza full rebuild explicitamente
  - RF-GPH-004 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
- **Caso**: TC-GPH-022; **Tipo**: negativo; **Descripcion / oraculo**: cancel/crash/partial batch/provenance faltante descarta staging; stale_rate final es 0.0
  - RF-GPH-004 — Criterio 2; RF-GPH-008 — Criterio 3. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
- **Caso**: TC-GPH-022A; **Tipo**: negativo; **Descripcion / oraculo**: identidad explícita divergente u origin ausente/múltiple/no normalizable omite la publicación graph, conserva el índice docs y devuelve warning accionable sin fallback por path ni mutación de `project.toml`
  - RF-GPH-001 — Criterio 1; RF-GPH-002 — Criterio 1; RF-GPH-003 — Criterio 1; RF-GPH-004 — Criterio 1; RF-GPH-005 — Criterio 3; RF-GPH-006 — Criterio 4; RF-GPH-007 — Criterio 3; RF-GPH-008 — Criterio 1; RF-GPH-009 — Criterio 1; RF-GPH-010 — Criterio 3; RF-GPH-011 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
- **Caso**: TC-GPH-022B; **Tipo**: positivo; **Descripcion / oraculo**: container con Go/C#/TS/Python observa Go una sola vez desde el root, procesa cada `.csproj`, omite `.sln`, comparte identidad y rebasa paths al namespace global; un proyecto Roslyn partial queda omission `backend_partial` sin bloquear batches completos
  - RF-GPH-001 — Criterio 1; RF-GPH-002 — Criterio 1; RF-GPH-003 — Criterio 1; RF-GPH-004 — Criterio 1; RF-GPH-005 — Criterio 3; RF-GPH-006 — Criterio 2; RF-GPH-007 — Criterio 2; RF-GPH-008 — Criterio 1; RF-GPH-009 — Criterio 3; RF-GPH-010 — Criterio 2; RF-GPH-011 — Criterio 2. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
- **Caso**: TC-GPH-022C; **Tipo**: negativo; **Descripcion / oraculo**: backend elegible sin batch devuelve error y conserva el graph previo; workspace explicitamente non-graph devuelve `GraphNotApplicable`
  - RF-GPH-001 — Criterio 1; RF-GPH-002 — Criterio 1; RF-GPH-003 — Criterio 1; RF-GPH-004 — Criterio 1; RF-GPH-005 — Criterio 1; RF-GPH-006 — Criterio 3; RF-GPH-007 — Criterio 3; RF-GPH-008 — Criterio 2; RF-GPH-009 — Criterio 1; RF-GPH-010 — Criterio 3; RF-GPH-011 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
- **Caso**: TC-GPH-022D; **Tipo**: positivo; **Descripcion / oraculo**: clean e incremental equivalentes, con mismo contenido/origin/toolchain/config, producen el mismo `GenerationID` y digest; `CreatedAt` queda fuera del ID
  - RF-GPH-001 — Criterio 1; RF-GPH-002 — Criterio 1; RF-GPH-003 — Criterio 1; RF-GPH-004 — Criterio 2; RF-GPH-005 — Criterio 1; RF-GPH-006 — Criterio 1; RF-GPH-007 — Criterio 2; RF-GPH-008 — Criterio 3; RF-GPH-009 — Criterio 2; RF-GPH-010 — Criterio 1; RF-GPH-011 — Criterio 3. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
- **Caso**: TC-GPH-022G; **Tipo**: positivo; **Descripcion / oraculo**: `TestIndexStartNoChangeGraphRepairPreservesCatalogBinding`: un job owner-bound full que deriva en incremental sin cambios marca su generation candidata como `skipped` y no mueve el catálogo; con graph válido conserva pointer/binding y con graph stale repara solo el snapshot graph bajo el mismo fence, dejando disponibles las queries positivas; cancelación, fallo o pérdida de fence no publica
  - RF-GPH-001 — Criterio 1; RF-GPH-002 — Criterio 1; RF-GPH-003 — Criterio 1; RF-GPH-004 — Criterio 4; RF-GPH-005 — Criterio 3; RF-GPH-006 — Criterio 1; RF-GPH-007 — Criterio 3; RF-GPH-008 — Criterio 3; RF-GPH-009 — Criterio 2; RF-GPH-010 — Criterio 3; RF-GPH-011 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
- **Caso**: TC-GPH-022E; **Tipo**: positivo; **Descripcion / oraculo**: `TestExtractReferencesSupportsObsidianFormsAndSkipsFences`: wikilinks, embeds, Markdown links, anchors y alias preservan tipo/raw target; externos/fences/self-anchor no crean edges falsos
  - RF-GPH-001 — Criterio 1; RF-GPH-002 — Criterio 1; RF-GPH-003 — Criterio 3; RF-GPH-004 — Criterio 1; RF-GPH-005 — Criterio 1; RF-GPH-006 — Criterio 1; RF-GPH-007 — Criterio 2; RF-GPH-008 — Criterio 1; RF-GPH-009 — Criterio 1; RF-GPH-010 — Criterio 1; RF-GPH-011 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
- **Caso**: TC-GPH-022F; **Tipo**: positivo; **Descripcion / oraculo**: `TestAppendStructuralDocEdgesLinksGobiernoAndReadme`: `doc_hierarchy` enlaza wiki/00-gobierno.md y README de bibliotecas/memorias
  - RF-GPH-001 — Criterio 1; RF-GPH-002 — Criterio 1; RF-GPH-003 — Criterio 1; RF-GPH-004 — Criterio 1; RF-GPH-005 — Criterio 1; RF-GPH-006 — Criterio 1; RF-GPH-007 — Criterio 4; RF-GPH-008 — Criterio 1; RF-GPH-009 — Criterio 1; RF-GPH-010 — Criterio 3; RF-GPH-011 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.

- **Marcador estable**: TP-GPH-003.declared-artifact-bindings-parse
- **Estado del comportamiento**: manual_evidence_pending
- **fixture_isolation**: testdata/wiki-code-bidirectional
- **verification_basis**: owner_final_verify_single_wave
- **Aceptación**:
  - **id**: TC-GPH-003-AB-canonical-source
  - RF-GPH-001 — Criterio 2; RF-GPH-002 — Criterio 1; RF-GPH-003 — Criterio 1; RF-GPH-004 — Criterio 1; RF-GPH-005 — Criterio 3; RF-GPH-006 — Criterio 1; RF-GPH-007 — Criterio 3; RF-GPH-008 — Criterio 1; RF-GPH-009 — Criterio 1; RF-GPH-010 — Criterio 2; RF-GPH-011 — Criterio 2. Disposición: manual_evidence_pending. Evidencia esperada: cada binding válido conserva binding_id/kind/surface y ambos lifecycle en ParsedDoc.CanonicalBindings/ExtractBindings; SourceBindings aplica RF-GPH-003 sin exigir target_path ni alterar WCB/v1. No acredita una ejecución de esta migración.
  - **Dado**: definición GraphV1 RF|TP|CT|DB|TECH|TECH-DS|FL con kind documental plano, doc_id y document_lifecycle; declaraciones raíz/frontmatter/TOON/surfaces, repetidas y arrays inline
  - **Cuando**: parent ejecuta fixture manual aislada en su única FINAL_VERIFY
  - **Entonces**: cada binding válido conserva binding_id/kind/surface y ambos lifecycle en ParsedDoc.CanonicalBindings/ExtractBindings; SourceBindings aplica RF-GPH-003 sin exigir target_path ni alterar WCB/v1
  - **Evidencia referenciada**: pending
  - **id**: TC-GPH-003-AB-toon-braces
  - RF-GPH-001 — Criterio 2; RF-GPH-002 — Criterio 1; RF-GPH-003 — Criterio 1; RF-GPH-004 — Criterio 1; RF-GPH-005 — Criterio 1; RF-GPH-006 — Criterio 1; RF-GPH-007 — Criterio 1; RF-GPH-008 — Criterio 1; RF-GPH-009 — Criterio 1; RF-GPH-010 — Criterio 1; RF-GPH-011 — Criterio 3. Disposición: manual_evidence_pending. Evidencia esperada: una asociación declarada para src/worker.go; sin invalid_artifact_bindings_list por corte de sangría; se mantienen validaciones required/enums/ownership/path y exclusión de payload/projections. No acredita una ejecución de esta migración.
  - **Dado**: RF-BRACES con definición GraphV1 RF active y bloque TOON artifact_bindings abierto por corchete, objeto multilínea entre llaves y cierre de corchete; llaves y campos sin sangría ni comas YAML
  - **Cuando**: parent realiza recheck1 sobre la fixture y el oracle RF-BRACES preservados de FINAL_VERIFY1
  - **Entonces**: una asociación declarada para src/worker.go; sin invalid_artifact_bindings_list por corte de sangría; se mantienen validaciones required/enums/ownership/path y exclusión de payload/projections
  - **Evidencia referenciada**: pending_owner_recheck1
  - **id**: TC-GPH-003-AB-ownership-and-enums
  - RF-GPH-001 — Criterio 1; RF-GPH-002 — Criterio 2; RF-GPH-003 — Criterio 1; RF-GPH-004 — Criterio 1; RF-GPH-005 — Criterio 4; RF-GPH-006 — Criterio 1; RF-GPH-007 — Criterio 1; RF-GPH-008 — Criterio 1; RF-GPH-009 — Criterio 2; RF-GPH-010 — Criterio 1; RF-GPH-011 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: mappings de asociación correctos; owner prohibido, enums inválidos, obligatorios vacíos, campos extra, claves duplicadas, path inseguro o symbol multiline producen binding_omission sin promoción. No acredita una ejecución de esta migración.
  - **Dado**: siete kinds, once roles y seis surfaces; pares válidos según enums y ownership, incluidos schema/migration/contract y ui; owners RS|UJ|UI-RFC|HANDOFF o FL con behavior
  - **Cuando**: parent compara tabla oracle escrita antes de ejecutar
  - **Entonces**: mappings de asociación correctos; owner prohibido, enums inválidos, obligatorios vacíos, campos extra, claves duplicadas, path inseguro o symbol multiline producen binding_omission sin promoción
  - **Evidencia referenciada**: pending
  - **id**: TC-GPH-003-AB-lifecycle-and-identity
  - RF-GPH-001 — Criterio 1; RF-GPH-002 — Criterio 1; RF-GPH-003 — Criterio 1; RF-GPH-004 — Criterio 1; RF-GPH-005 — Criterio 3; RF-GPH-006 — Criterio 1; RF-GPH-007 — Criterio 3; RF-GPH-008 — Criterio 1; RF-GPH-009 — Criterio 1; RF-GPH-010 — Criterio 1; RF-GPH-011 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: ejes independientes legales; bridge excluido si cualquiera no active; declaraciones equivalentes conservadas en parseo y una fila agregada sin UNIQUE collision; IDs ambiguos rechazados todos; cambiar ID/surface no cambia WCB/v1. No acredita una ejecución de esta migración.
  - **Dado**: doc active y binding deprecated; doc deprecated o retired y binding active; dos binding_ids distintos proyectados al mismo WCB; IDs repetidos
  - **Cuando**: parent compara declaraciones fuente separadas y agregado legacy
  - **Entonces**: ejes independientes legales; bridge excluido si cualquiera no active; declaraciones equivalentes conservadas en parseo y una fila agregada sin UNIQUE collision; IDs ambiguos rechazados todos; cambiar ID/surface no cambia WCB/v1
  - **Evidencia referenciada**: pending
  - **id**: TC-GPH-003-AB-legacy-and-payload
  - RF-GPH-001 — Criterio 1; RF-GPH-002 — Criterio 1; RF-GPH-003 — Criterio 1; RF-GPH-004 — Criterio 1; RF-GPH-005 — Criterio 4; RF-GPH-006 — Criterio 1; RF-GPH-007 — Criterio 1; RF-GPH-008 — Criterio 1; RF-GPH-009 — Criterio 1; RF-GPH-010 — Criterio 1; RF-GPH-011 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: legacy conserva precedencias, defaults, conflictos y planned no navegable, sin required canónicos; hermanos no invalidan objetos; response/planned no crea asociaciones; ninguna heurística promueve documento a strict GraphV1. No acredita una ejecución de esta migración.
  - **Dado**: documento no adoptado con aliases implementation_anchors/code_links/test_links e histórico artifact_bindings derivado planned; campos hermanos owner/status; payload response/planned/projection
  - **Cuando**: parent verifica lectura legacy y default consumidor existentes
  - **Entonces**: legacy conserva precedencias, defaults, conflictos y planned no navegable, sin required canónicos; hermanos no invalidan objetos; response/planned no crea asociaciones; ninguna heurística promueve documento a strict GraphV1
  - **Evidencia referenciada**: pending
  - **id**: TC-GPH-003-AB-bridge-omissions
  - RF-GPH-001 — Criterio 1; RF-GPH-002 — Criterio 1; RF-GPH-003 — Criterio 1; RF-GPH-004 — Criterio 1; RF-GPH-005 — Criterio 3; RF-GPH-006 — Criterio 1; RF-GPH-007 — Criterio 1; RF-GPH-008 — Criterio 2; RF-GPH-009 — Criterio 1; RF-GPH-010 — Criterio 1; RF-GPH-011 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: provenance y resolution separados, file-only u omission honesta sin compiler fact; binding_omission informa explicit_incomplete de colección; B/C/K no se declaran completos. No acredita una ejecución de esta migración.
  - **Dado**: archivo ausente, catálogo ausente o símbolo no resuelto y colección fuente no persistida
  - **Cuando**: parent verifica forward/reverse existentes
  - **Entonces**: provenance y resolution separados, file-only u omission honesta sin compiler fact; binding_omission informa explicit_incomplete de colección; B/C/K no se declaran completos
  - **Evidencia referenciada**: pending
- **manual_evidence**: pending_owner_final_verify

### TP-GPH-004 - Navegacion bounded y determinista
  - RF-GPH-001 — Criterio 1; RF-GPH-002 — Criterio 1; RF-GPH-003 — Criterio 1; RF-GPH-004 — Criterio 1; RF-GPH-005 — Criterio 1; RF-GPH-006 — Criterio 1; RF-GPH-007 — Criterio 2; RF-GPH-008 — Criterio 1; RF-GPH-009 — Criterio 3; RF-GPH-010 — Criterio 1; RF-GPH-011 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.

**Cobertura:** RF-GPH-005.
- **Caso**: TC-GPH-023; **Tipo**: positivo; **Descripcion / oraculo**: neighbors respeta direction/relation/depth y ordering canónico
  - RF-GPH-005 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
- **Caso**: TC-GPH-024; **Tipo**: positivo; **Descripcion / oraculo**: callers usa calls entrante y callees calls saliente sin mezclar references
  - RF-GPH-005 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
- **Caso**: TC-GPH-025; **Tipo**: positivo; **Descripcion / oraculo**: path elige shortest path y resuelve empates lexicograficamente
  - RF-GPH-005 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
- **Caso**: TC-GPH-026; **Tipo**: positivo; **Descripcion / oraculo**: explain devuelve claim, endpoints, evidence y marca inferred sin elevarlo
  - RF-GPH-005 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
- **Caso**: TC-GPH-027; **Tipo**: positivo; **Descripcion / oraculo**: graph stats cuenta status/backend/unresolved; validate detecta schema/pointer/endpoint/cross-RID
  - RF-GPH-005 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
- **Caso**: TC-GPH-028; **Tipo**: negativo; **Descripcion / oraculo**: selector ausente/ambiguo y generation invalid/retired devuelven error/candidatos accionables
  - RF-GPH-005 — Criterio 3. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
- **Caso**: TC-GPH-029; **Tipo**: negativo; **Descripcion / oraculo**: depth/result/token ceiling y cursor stale producen reject o truncation determinista
  - RF-GPH-005 — Criterio 3. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
- **Caso**: TC-GPH-030; **Tipo**: positivo; **Descripcion / oraculo**: direct/daemon son semanticamente iguales y hash de tablas confirma cero writes/migrations
  - RF-GPH-005 — Criterio 2. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.

### TP-GPH-005 - Impacto y autoridad wiki-codigo
  - RF-GPH-001 — Criterio 1; RF-GPH-002 — Criterio 1; RF-GPH-003 — Criterio 1; RF-GPH-004 — Criterio 1; RF-GPH-005 — Criterio 1; RF-GPH-006 — Criterio 1; RF-GPH-007 — Criterio 2; RF-GPH-008 — Criterio 1; RF-GPH-009 — Criterio 1; RF-GPH-010 — Criterio 3; RF-GPH-011 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.

**Cobertura:** RF-GPH-006 y RF-GPH-007.
- **Caso**: TC-GPH-031; **Tipo**: positivo; **Descripcion / oraculo**: changed path resuelve seeds y direct impact con evidence path exacto
  - RF-GPH-006 — Criterio 4. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
- **Caso**: TC-GPH-031A; **Tipo**: positivo; **Descripcion / oraculo**: `nav.affected` inspecciona sólo paths explícitos respetando WAL; si cambiaron, publica catálogo/`doc_records` antes de resolver seeds y no reconstruye graph generation
  - RF-GPH-006 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
- **Caso**: TC-GPH-031B; **Tipo**: negativo; **Descripcion / oraculo**: En `nav.affected`, error/deadline antes del commit conserva evidencia publicada y emite warning sin escritura en background; graph stale produce solo heurísticas `confidence=0`, sin generation/evidencia graph-native, y el warning explícito `GPH_IMPACT_GRAPH_STALE` contiene `mi-lsp index --workspace "<alias>"`
  - RF-GPH-006 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
- **Caso**: TC-GPH-031C; **Tipo**: negativo; **Descripcion / oraculo**: Con graph stale, `nav.diff-context` propaga `GPH_IMPACT_GRAPH_STALE`; wrappers `change-pack` e `intent` no producen fallback heurístico stale y conservan warning/error sanitizado, incluido `INTENT_DIFF_UNAVAILABLE` y `graph_query_error` para intent
  - RF-GPH-006 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
- **Caso**: TC-GPH-032; **Tipo**: positivo; **Descripcion / oraculo**: transitive mode usa solo allowlist/direccion/costo y separa inferred
  - RF-GPH-006 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
- **Caso**: TC-GPH-033; **Tipo**: positivo; **Descripcion / oraculo**: tests/docs se agregan por edges/anchors; convenciones quedan heuristic
  - RF-GPH-006 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
- **Caso**: TC-GPH-034; **Tipo**: negativo; **Descripcion / oraculo**: basename/doc ID ambiguo produce `ambiguous_doc_target` con candidatos sorted/bounded; relation textual o unresolved no infla positivos ni viola fixture negative
  - RF-GPH-006 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
- **Caso**: TC-GPH-035; **Tipo**: comparativo; **Descripcion / oraculo**: precision/recall/F1/negative violations no regresan contra affected previo ni Graphify comparable
  - RF-GPH-006 — Criterio 2. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
- **Caso**: TC-GPH-036; **Tipo**: positivo; **Descripcion / oraculo**: docs canónicos siguen primeros y primary doc no cambia al enriquecer con código
  - RF-GPH-007 — Criterio 2. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
- **Caso**: TC-GPH-037; **Tipo**: negativo; **Descripcion / oraculo**: governance/projection stale bloquea; canon-code conflict produce drift, nunca override
  - RF-GPH-007 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
- **Caso**: TC-GPH-038; **Tipo**: positivo; **Descripcion / oraculo**: context pack conserva cadena obligatoria y reduce tokens solo con coverage/omissions explícitos
  - RF-GPH-007 — Criterio 3; RF-GPH-011 — Criterio 2. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.

### TP-GPH-006 - Federacion, MILX-v1 y packs
  - RF-GPH-001 — Criterio 1; RF-GPH-002 — Criterio 1; RF-GPH-003 — Criterio 1; RF-GPH-004 — Criterio 1; RF-GPH-005 — Criterio 1; RF-GPH-006 — Criterio 1; RF-GPH-007 — Criterio 1; RF-GPH-008 — Criterio 1; RF-GPH-009 — Criterio 1; RF-GPH-010 — Criterio 1; RF-GPH-011 — Criterio 4. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.

**Cobertura:** RF-GPH-009, RF-GPH-010 y seguridad de RF-GPH-011.
- **Caso**: TC-GPH-039; **Tipo**: positivo; **Descripcion / oraculo**: dos member generations producen snapshot global/cross-edge determinista por cross-RID
  - RF-GPH-009 — Criterio 2; RF-GPH-011 — Criterio 4. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
- **Caso**: TC-GPH-040; **Tipo**: negativo; **Descripcion / oraculo**: member/target ausente, schema incompatible o cross-RID conflictivo queda omission/unresolved
  - RF-GPH-009 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
- **Caso**: TC-GPH-041; **Tipo**: positivo; **Descripcion / oraculo**: query global direct/daemon mantiene budgets y no escribe member stores
  - RF-GPH-009 — Criterio 3. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
- **Caso**: TC-GPH-042; **Tipo**: negativo; **Descripcion / oraculo**: documento de un workspace no gobierna otro sin enlace canónico bilateral
  - RF-GPH-009 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
- **Caso**: TC-GPH-043; **Tipo**: positivo; **Descripcion / oraculo**: framing length-prefixed, describe/prepare/execute/cancel/health/shutdown y manifest golden
  - RF-GPH-010 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
- **Caso**: TC-GPH-044; **Tipo**: negativo; **Descripcion / oraculo**: capability desconocida, graph/wiki write, secret, network o MCP se deniega antes de ejecutar
  - RF-GPH-010 — Criterio 3. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
- **Caso**: TC-GPH-045; **Tipo**: negativo; **Descripcion / oraculo**: timeout/crash/cancel termina solo process tree de extension y cleanup queda probado
  - RF-GPH-010 — Criterio 2. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
- **Caso**: TC-GPH-046; **Tipo**: negativo; **Descripcion / oraculo**: frame malformed/oversize/schema inválido se descarta sin mutar graph/core
  - RF-GPH-010 — Criterio 2. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
- **Caso**: TC-GPH-047; **Tipo**: positivo; **Descripcion / oraculo**: Graphify import queda `external:graphify` advisory y no reemplaza NodeKey/evidence/wiki
  - RF-GPH-010 — Criterio 3. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
- **Caso**: TC-GPH-048; **Tipo**: positivo; **Descripcion / oraculo**: pack deterministic y cache key/invalidation incluyen generation, version, digest, params y authority
  - RF-GPH-011 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.

### TP-GPH-007 - Victory Lab, performance y cierre del slice
  - RF-GPH-001 — Criterio 1; RF-GPH-002 — Criterio 1; RF-GPH-003 — Criterio 1; RF-GPH-004 — Criterio 4; RF-GPH-005 — Criterio 1; RF-GPH-006 — Criterio 1; RF-GPH-007 — Criterio 1; RF-GPH-008 — Criterio 1; RF-GPH-009 — Criterio 1; RF-GPH-010 — Criterio 1; RF-GPH-011 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.

**Cobertura:** RF-GPH-011 y gates transversales RF-GPH-001..010.
- **Caso**: TC-GPH-049; **Tipo**: positivo; **Descripcion / oraculo**: manifest/revision/fixture hashes se verifican; CRLF/LF equivalen sin ocultar mutación semántica
  - RF-GPH-011 — Criterio 3. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
- **Caso**: TC-GPH-050; **Tipo**: comparativo; **Descripcion / oraculo**: 30 repeticiones por current, Graphify y baseline previo con mismo fixture/protocolo
  - RF-GPH-011 — Criterio 3. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
- **Caso**: TC-GPH-051; **Tipo**: comparativo; **Descripcion / oraculo**: correctness, precision, recall, F1 y negative violations reportan numerador/denominador
  - RF-GPH-011 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
- **Caso**: TC-GPH-052; **Tipo**: comparativo; **Descripcion / oraculo**: token count canónico mide objetivo <=0.70x sin omitir autoridad/evidence requerida
  - RF-GPH-011 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
- **Caso**: TC-GPH-053; **Tipo**: comparativo; **Descripcion / oraculo**: warm p95 mide objetivo <=0.80x y hot-path guard rail baseline x1.10 +25 ms
  - RF-GPH-011 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
- **Caso**: TC-GPH-054; **Tipo**: comparativo; **Descripcion / oraculo**: peak RSS mide objetivo <=0.50x sin materializar grafo completo
  - RF-GPH-011 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
- **Caso**: TC-GPH-055; **Tipo**: comparativo; **Descripcion / oraculo**: incremental mide objetivo <=0.70x y stale_rate=0.0 contra clean oracle
  - RF-GPH-011 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
- **Caso**: TC-GPH-056; **Tipo**: positivo; **Descripcion / oraculo**: 30 outputs canónicos, NodeKeys, cross-RIDs y digests son identicos por variante/RID
  - RF-GPH-011 — Criterio 4. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
- **Caso**: TC-GPH-057; **Tipo**: negativo; **Descripcion / oraculo**: security scan/runtime prueba no-MCP, no-network, no secret leak, no query write y rollback
  - RF-GPH-011 — Criterio 2. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
- **Caso**: TC-GPH-058; **Tipo**: negativo; **Descripcion / oraculo**: metrica/comparator/raw sample unavailable, threshold fallido o claim global desde subset bloquea PASS
  - RF-GPH-011 — Criterio 3. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.

### Comandos de verificacion mínimos

```text
python -m unittest discover -s scripts/bench/victory_lab -p "test_*.py"
python scripts/bench/victory_lab/runner.py --manifest benchmarks/victory-lab/v1/manifest.json --repetitions 30 --output <evidence-dir>
python scripts/bench/victory_lab/report.py --input <evidence-dir> --output <report.json>
gofmt -l cmd internal
go vet ./...
go test ./...
mi-lsp nav governance --workspace mi-lsp --format toon
mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
```

### TP-GPH-008 - T5 Grupo E: routing, explain-change y analisis advisory
  - RF-GPH-001 — Criterio 3; RF-GPH-002 — Criterio 1; RF-GPH-003 — Criterio 1; RF-GPH-004 — Criterio 1; RF-GPH-005 — Criterio 1; RF-GPH-006 — Criterio 1; RF-GPH-007 — Criterio 4; RF-GPH-008 — Criterio 1; RF-GPH-009 — Criterio 1; RF-GPH-010 — Criterio 3; RF-GPH-011 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.

- **Marcador estable**: tp-gph-t5-group-e
- **Evidencia referenciada**:
  - .docs/wiki/06_tests/TP-GPH.md
  - internal/service/intent.go
  - internal/service/graph_query_test.go
- **Comprobación prevista**:
  - go test ./internal/service/... ./cmd/mi-lsp/...
  - go run ./cmd/mi-lsp nav explain-change --path internal/service/intent.go --workspace <alias> --format toon
  - go run ./cmd/mi-lsp nav graph stats --workspace <alias> --format toon
  - go run ./cmd/mi-lsp nav graph validate --workspace <alias> --format toon
- **Condiciones de rechazo**:
  - governance_blocked=true
  - timeout_without_typed_diagnostic=true
  - graph_claim_without_freshness_current=true
- **Casos**:
  - **id**: TC-GPH-059
  - RF-GPH-005 — Criterio 3. Disposición: manual_evidence_pending. Evidencia esperada: mi-lsp es el primer planner sin opt-out; devuelve intent, operation, confidence, freshness, preview, expansions y telemetry sanitizada. No acredita una ejecución de esta migración.
  - **type**: positivo
  - **Dado**: pregunta soportada con callers, callees, affected-change, tests, contracts y wiki
  - **Cuando**: mi-lsp nav intent '<question>' --workspace <alias> --format toon
  - **Entonces**: mi-lsp es el primer planner sin opt-out; devuelve intent, operation, confidence, freshness, preview, expansions y telemetry sanitizada
  - **id**: TC-GPH-060
  - RF-GPH-005 — Criterio 3. Disposición: manual_evidence_pending. Evidencia esperada: section_count=7 y las secciones son change, affected, callers, callees, tests, contracts y wiki. No acredita una ejecución de esta migración.
  - **type**: positivo
  - **Dado**: path de cambio y catálogo documental gobernado
  - **Cuando**: mi-lsp nav explain-change --path <path> --workspace <alias> --format toon
  - **Entonces**: section_count=7 y las secciones son change, affected, callers, callees, tests, contracts y wiki
  - **id**: TC-GPH-061
  - RF-GPH-005 — Criterio 3. Disposición: manual_evidence_pending. Evidencia esperada: cada expansion tiene command ejecutable y reason; la preview declara omisiones y no afirma completitud. No acredita una ejecución de esta migración.
  - **type**: positivo
  - **Dado**: preview con evidencia parcial
  - **Cuando**: se inspecciona expansions[] y next_hint
  - **Entonces**: cada expansion tiene command ejecutable y reason; la preview declara omisiones y no afirma completitud
  - **id**: TC-GPH-062
  - RF-GPH-005 — Criterio 4. Disposición: manual_evidence_pending. Evidencia esperada: fallback tipado y omisiones visibles; no se presenta heuristica como prueba graph-native. No acredita una ejecución de esta migración.
  - **type**: negativo
  - **Dado**: backend graph no disponible
  - **Cuando**: explain-change intenta completar affected, callers o callees
  - **Entonces**: fallback tipado y omisiones visibles; no se presenta heuristica como prueba graph-native
  - **id**: TC-GPH-063
  - RF-GPH-005 — Criterio 1; RF-GPH-006 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: resultado BLOCKED; no se permite sustituir silenciosamente la consulta por rg, Grep, Glob o Read. No acredita una ejecución de esta migración.
  - **type**: negativo
  - **Dado**: nav ask excede el deadline
  - **Cuando**: el runtime devuelve context deadline exceeded sin diagnóstico tipado
  - **Entonces**: resultado BLOCKED; no se permite sustituir silenciosamente la consulta por rg, Grep, Glob o Read
  - **id**: TC-GPH-064
  - RF-GPH-006 — Criterio 1; RF-GPH-007 — Criterio 3; RF-GPH-011 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: rank usa 0.45 authority + 0.25 impact + 0.20 centrality + 0.10 boundary; communities y digest son deterministas. No acredita una ejecución de esta migración.
  - **type**: positivo
  - **Dado**: analysis con freshness current y nodos exact/extracted
  - **Cuando**: se calcula graph rank y communities
  - **Entonces**: rank usa 0.45 authority + 0.25 impact + 0.20 centrality + 0.10 boundary; communities y digest son deterministas
  - **id**: TC-GPH-065
  - RF-GPH-006 — Criterio 1; RF-GPH-007 — Criterio 1; RF-GPH-011 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: claims exactos quedan omitidos o degradados con warning; solo current autoriza afirmaciones exactas. No acredita una ejecución de esta migración.
  - **type**: negativo
  - **Dado**: freshness lagging, stale, invalid o unknown
  - **Cuando**: se solicitan claims exactos del grafo
  - **Entonces**: claims exactos quedan omitidos o degradados con warning; solo current autoriza afirmaciones exactas
  - **id**: TC-GPH-066
  - RF-GPH-011 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: solo metadata derivada permitida; query, prompt, argv, payload, content, snippets, secrets y raw_error no aparecen. No acredita una ejecución de esta migración.
  - **type**: negativo
  - **Dado**: eventos de utility o intent telemetry
  - **Cuando**: se serializa evidencia operacional
  - **Entonces**: solo metadata derivada permitida; query, prompt, argv, payload, content, snippets, secrets y raw_error no aparecen
  - **id**: TC-GPH-067
  - RF-GPH-005 — Criterio 3. Disposición: manual_evidence_pending. Evidencia esperada: stats/validate no escriben ni reparan index.db y exponen generation/status/omissions de forma estable. No acredita una ejecución de esta migración.
  - **type**: positivo
  - **Dado**: comando graph read-only publicado
  - **Cuando**: mi-lsp nav graph stats|validate --workspace <alias> --format toon
  - **Entonces**: stats/validate no escriben ni reparan index.db y exponen generation/status/omissions de forma estable

PASS final requiere los siete TP, comparadores fijados, raw evidence, cross-RID, rollback, seguridad y autoridad wiki. Un caso `planned` no cuenta como ejecutado hasta enlazar test automatizado o evidencia manual determinista.

### TP-GPH-009 - Mapa Harness-first F1-F11
  - RF-GPH-001 — Criterio 1; RF-GPH-002 — Criterio 2; RF-GPH-003 — Criterio 1; RF-GPH-004 — Criterio 1; RF-GPH-005 — Criterio 2; RF-GPH-006 — Criterio 4; RF-GPH-007 — Criterio 4; RF-GPH-008 — Criterio 1; RF-GPH-009 — Criterio 3; RF-GPH-010 — Criterio 1; RF-GPH-011 — Criterio 3. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.

- **Marcador estable**: tp-gph-harness-first-f1-f11
- **Evidencia referenciada**:
  - .docs/wiki/06_tests/TP-GPH.md
  - internal/service/intent.go
  - scripts/bench/harness_first/tests/test_runner.py
- **Estado del comportamiento**: implemented_tests_and_campaign
- **campaign_status**: PASS
- **campaign_record**:
  - **schema**: harness-first-campaign/v1
  - **campaign_id**: harness-first-final-9bb3163
  - **source_revision**: 9bb3163dc8840c76a6acfdccdbc798973f05bd49
  - **binary_sha256**: d4dbcf3981ae4569cf92b52d2300eae0cd583436683067db4ed746f1ec1da35a
  - **Estado del comportamiento**: PASS
  - **correctness_percent**: 100.0
  - **parity**: PASS_exact_digest
  - **parity_digest**: faae133a60e5e31f0b951867b7477b112cebf49daf463b092e83294b781a072e
  - **retry_amplification**: 1.0
  - **latency_p95_ms**: 8009.6
  - **latency_p99_ms**: 8591.1
  - **latency_budget_p95_ms**: 15000
  - **latency_budget_p99_ms**: 15000
  - **peak_rss_bytes**: 252096512
  - **preview_usefulness**: PASS
  - **Evidencia referenciada**: .docs/auditoria/2026-07-21-milsp-harness-first-roadmap/traceability-closure.yaml
- **Comprobación prevista**:
  - go test ./internal/model/... ./internal/service/... ./internal/store/... ./internal/telemetry/...
  - python -m unittest discover -s scripts/bench/harness_first/tests -p "test_*.py"
- **Condiciones de rechazo**:
  - campaign_status=PASS_without_real_run=true
  - raw_campaign_output_promoted=true
- **features**:
  - **F1_intent_routing**:
    - **Estado del comportamiento**: implemented
    - **Evidencia referenciada**: internal/service/intent.go::TestClassifySupportedIntentRoutesAllT3Operations
  - **F2_all_workspaces_intent_plan**:
    - **Estado del comportamiento**: implemented
    - **Evidencia referenciada**: internal/service/ask_test.go::TestNavAskAllWorkspacesPreservesSupportedIntentPlansDeterministically
    - **invariant**: every_plan_preserves_workspace_and_determinism_digest
  - **F3_explain_change_preview**:
    - **Estado del comportamiento**: implemented
    - **Evidencia referenciada**: internal/service/intent.go::TestExplainChangePreviewHasSevenSectionsWikiEvidenceAndExpansions
    - **sections**:
      - change
      - affected
      - callers
      - callees
      - tests
      - contracts
      - wiki
  - **F4_executable_expansions**:
    - **Estado del comportamiento**: implemented
    - **Evidencia referenciada**:
      - internal/service/intent.go::TestIntentExpansionCommandsUseExecutableCLIOperations
      - internal/service/intent.go::TestExplainChangeExpansionPreservesNormalizedPathsAndRef
      - internal/service/intent.go::TestIncompletePathExpansionUsesExecutableDiscovery
    - **shape**:
      - **command**: string
      - **reason**: string
  - **F5_rank_freshness_communities**:
    - **Estado del comportamiento**: implemented
    - **Evidencia referenciada**:
      - internal/service/graph_query_test.go::TestGraphRankQueryCachesCompleteAnalysisAndReusesIt
      - internal/model/graph_query_test.go::TestDeterminismDigestExcludesElapsedTime
    - **exact_claim_gate**: freshness=current
  - **F6_complete_only_cache_and_digests**:
    - **Estado del comportamiento**: implemented
    - **Evidencia referenciada**:
      - internal/service/graph_query_test.go::TestGraphRankCacheBridgeSingleConnectionCoversMissHitAndUtility
      - internal/store/graph_analysis_test.go::TestGetGraphAnalysisIgnoresDigestMismatch
  - **F7_bounded_snapshot_queries**:
    - **Estado del comportamiento**: implemented
    - **Evidencia referenciada**:
      - internal/service/graph_query_test.go::TestGraphQueryRejectsInvalidBudget
      - internal/service/graph_query_test.go::TestFinalizeGraphItemsAlwaysAdvancesCursorAtTinyBudget
  - **F8_utility_caps_and_decay**:
    - **Estado del comportamiento**: implemented
    - **Evidencia referenciada**:
      - internal/store/utility_test.go::TestUtilityGlobalScopeCapBoundsManyCandidateSignals
      - internal/store/utility_test.go::TestUtilityEventsApplyScopeRetentionAndDecay
    - **signals**:
      - continuation_followed
      - feedback_positive
      - feedback_negative
      - result_selected
  - **F9_sqlite_connection_invariants**:
    - **Estado del comportamiento**: implemented
    - **Evidencia referenciada**:
      - internal/service/graph_query_test.go::TestGraphRankCacheBridgeSingleConnectionCoversMissHitAndUtility
      - internal/store/db.go
      - internal/daemon/state_store.go
    - **write_max_open_conns**: 1
  - **F10_sanitized_telemetry**:
    - **Estado del comportamiento**: implemented
    - **Evidencia referenciada**:
      - internal/telemetry/access_events_test.go::TestEnrichAccessEventRedactsRawTelemetryInputs
      - internal/telemetry/access_events_test.go::TestNormalizeAccessEventRejectsArbitraryTelemetryCodesEverywhere
  - **F11_campaign_contract_and_release_gate**:
    - **Estado del comportamiento**: implemented_and_executed
    - **Evidencia referenciada**:
      - scripts/bench/harness_first/tests/test_runner.py
      - docs/benchmarks/HARNESS_FIRST_CAMPAIGN.json
      - docs/benchmarks/HARNESS_FIRST.md
    - **campaign_execution**: PASS_harness-first-final-9bb3163

El mapa F1-F11 demuestra cobertura automatizada del árbol actual, y la campaña Harness-first fue ejecutada una única vez sobre `9bb3163` con binario real, provenance limpia (`vcs.modified=false`), RSS acotado y paridad direct/daemon por digest exacto. El registro sanitizado vive en `campaign_record` y en el paquete de cierre trazado; la salida raw permanece como evidencia externa temporal y no se promueve.

### TP-GPH-T10 - Cierre ejecutado del puente wiki ↔ código
  - RF-GPH-001 — Criterio 1; RF-GPH-002 — Criterio 2; RF-GPH-003 — Criterio 1; RF-GPH-004 — Criterio 2; RF-GPH-005 — Criterio 1; RF-GPH-006 — Criterio 1; RF-GPH-007 — Criterio 4; RF-GPH-008 — Criterio 3; RF-GPH-009 — Criterio 1; RF-GPH-010 — Criterio 1; RF-GPH-011 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.

- **Marcador estable**: TP-GPH.t10-live-wiki-code-bridge
- **Estado del comportamiento**: implemented_and_executed
- **verification_basis**:
  - **branch**: main
  - **code_commit**: e1835ee
  - **implementation_commits**:
    - d1c8e4a
    - badedb8
    - a910ac0
    - edf09dc
    - e2361a8
  - **repair_commit**: e1835ee
  - **go_test_all**: PASS
  - **go_test_packages**: 28
  - **git_diff_check**: PASS
  - **binary_sha256**: 2f7d94cd1eec05b0055184cc05452725831e5b65404c4a095b7bc01717c36a1a
- **campaign**:
  - **schema**: wiki-code-bridge-runner/v1
  - **Estado del comportamiento**: PASS
  - **case_inventory_count**: 37
  - **case_inventory**:
    - full_index_fixture_baseline
    - reverse_lookup
    - supporting_only
    - graph_stale
    - edit_binding_overlay
    - remove_binding_tombstone
    - add_binding_reverse
    - delete_rename_target
    - fail_closed_inputs
    - raw_audit_decoys
    - unmapped_changed_code
    - modern_js_extensions
    - lost_watcher_event
    - direct_daemon_parity
    - no_query_writes
    - same_tick_rewrite
    - unknown_state_omission
    - stable_digest_30
    - cost_counters
    - graph_v1_alias_normalization
    - planned_binding
    - retired_exclusion_redirect
    - duplicate_doc_ids
    - active_retired_transition
    - governance_imports_self_export
    - graph_v1_phase_ordering
    - legacy_advisory
    - mjs_find_related
    - wiki_to_code_direct_precision
    - code_to_wiki_reverse_recall
    - false_direct_implementation_edges
    - raw_audit_primary_results
    - warm_mixed_neighbors_p95
    - warm_direct_binding_lookup_p95
    - stable_digest_runs
    - dirty_single_file_overlay_target
    - latency_campaign_complete
  - **stable_digest_runs**: 30
  - **stable_digest**: 0f32b7e85bb0bcbd727424321e0a272c27e6a94aca6c11c78bd12541bd9255a2
  - **residual_risks**:
- **Aceptación**:
  - **overlay_read_your_writes**: PASS
  - **overlay_cases**:
    - edit_binding_overlay
    - remove_binding_tombstone
    - add_binding_reverse
    - delete_rename_target
    - lost_watcher_event
    - same_tick_rewrite
  - **stale_graph**: PASS_direct_evidence_preserved_with_typed_omission
  - **modern_js_extensions**: PASS
  - **no_query_writes**: PASS
  - **direct_daemon_parity**: PASS
  - **deterministic_output**: PASS_30_runs
  - **direct_precision**:
    - **value**: 1.0
    - **Estado del comportamiento**: PASS
  - **reverse_recall**:
    - **value**: 1.0
    - **Estado del comportamiento**: PASS
  - **planned_non_navigable**: PASS
  - **retired_default_exclusion_and_historical_redirect**: PASS
  - **raw_audit_exclusion**: PASS
  - **fail_closed_inputs_and_unmapped_classification**: PASS
- **cost**:
  - **bindings_examined**: 3
  - **bytes_read**: 3047
  - **catalog_queries**: 5
  - **files_checked**: 3
  - **files_hashed**: 4
  - **files_parsed**: 1
  - **metadata_checked**: 1
  - **output_token_estimate**: 210
  - **semantic_backend_calls**: 0
  - **symbols_checked**: 1
- **latency_ms**:
  - **samples**: 30
  - **warm_direct_binding_lookup_p95**:
    - **value**: 80
    - **target**: 100
    - **Estado del comportamiento**: PASS
  - **warm_mixed_neighbors_p95**:
    - **value**: 69
    - **target**: 1000
    - **Estado del comportamiento**: PASS
  - **dirty_single_file_overlay_p95**:
    - **value**: 75
    - **target**: 250
    - **Estado del comportamiento**: PASS
  - **cold_direct_lookup**: 85.657
  - **cold_reverse_lookup**: 79.018
  - **warm_reverse_lookup_p95**: 83
- **validator_boundary**:
  - **governance**: PASS_in_sync_valid
  - **full_workspace_validate_harness**: PASS_124_contracts_858_links_0_blockers
  - **full_workspace_validate_source**: PASS_31_artifacts_71_records_0_blockers
  - **canonical_drift_repaired**: true
  - **verification_scope**: full_workspace
  - **next_action**: none
- **verification_semantics**:
  - **sanitized_runner_metrics_are_evidence**: true
  - **raw_prompt_plan_and_host_paths_are_not_authority**: true
  - **graph_v1**: consumed_read_only_subset_only
- **Comprobación prevista**:
  - FINAL_VERIFY code basis e1835ee: go test ./... PASS; git diff --check PASS
  - sanitized FINAL_VERIFY result: wiki-code-bridge-runner/v1 PASS; inventory=37
- **Evidencia referenciada**:
  - internal/service/wiki_code_context.go
  - internal/livecontext/overlay_test.go
  - internal/livecontext/manifest_test.go
  - internal/indexer/incremental.go
  - internal/store/index_publish_test.go
  - internal/store/queries_incremental.go
  - internal/service/wiki_code_bindings.go
  - .docs/wiki/06_tests/TP-GPH.md
