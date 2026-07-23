# TP-GPH

```yaml
harness_protocol: SDD-HARNESS-v1
source_protocol: SDD-WIKI-SOURCE-v1
doc_id: "TP-GPH"
id: "TP-GPH"
kind: "support-doc"
audience: "llm-first"
imports:
  - '[[00_gobierno_documental]]'
  - '[[FL-GPH-01]]'
  - '[[FL-GPH-02]]'
  - '[[FL-GPH-03]]'
  - '[[RF-GPH-001]]'
  - '[[RF-GPH-002]]'
  - '[[RF-GPH-003]]'
  - '[[RF-GPH-004]]'
  - '[[RF-GPH-005]]'
  - '[[RF-GPH-006]]'
  - '[[RF-GPH-007]]'
  - '[[RF-GPH-008]]'
  - '[[RF-GPH-009]]'
  - '[[RF-GPH-010]]'
  - '[[RF-GPH-011]]'
exports:
  - 'TP-GPH'
  - 'TP-GPH-001'
  - 'TP-GPH-002'
  - 'TP-GPH-003'
  - 'TP-GPH-004'
  - 'TP-GPH-005'
  - 'TP-GPH-006'
  - 'TP-GPH-007'
  - 'TP-GPH-008'
  - 'TP-GPH-009'
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/03_FL/FL-GPH-01.md
  - .docs/wiki/03_FL/FL-GPH-02.md
  - .docs/wiki/03_FL/FL-GPH-03.md
  - .docs/wiki/04_RF/RF-GPH-001.md
  - .docs/wiki/04_RF/RF-GPH-002.md
  - .docs/wiki/04_RF/RF-GPH-003.md
  - .docs/wiki/04_RF/RF-GPH-004.md
  - .docs/wiki/04_RF/RF-GPH-005.md
  - .docs/wiki/04_RF/RF-GPH-006.md
  - .docs/wiki/04_RF/RF-GPH-007.md
  - .docs/wiki/04_RF/RF-GPH-008.md
  - .docs/wiki/04_RF/RF-GPH-009.md
  - .docs/wiki/04_RF/RF-GPH-010.md
  - .docs/wiki/04_RF/RF-GPH-011.md
  - .docs/wiki/06_pruebas/TP-GPH.md
agent_may_edit:
  - .docs/wiki/06_pruebas/TP-GPH.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
evidence:
  - .docs/wiki/06_pruebas/TP-GPH.md
```

## Contrato comun de prueba

- Fixtures versionadas: C#, Go, TypeScript, Python, wiki, mixed, cross-workspace, extensions y relations (`positive`, `negative`, `ambiguous`, `unresolved`, `not-comparable`).
- Oraculos: JSON canonico/goldens, hashes de fixture, invariantes SQLite, source digests, cross-RID y comparacion incremental-clean. Un LLM o screenshot nunca es el unico juez.
- Toda medicion competitiva fija Graphify `9bf14a4931658152969586ace39eb965c010f0d1`, baseline mi-lsp `a251ab1f8db4e96f029926fbef275b078a20a111`, fixture digest, comando, entorno y raw samples.
- Benchmarks relevantes ejecutan 30 repeticiones por variante con el mismo protocolo cold/warm/incremental. Metrica unavailable es `BLOCKED/NOT_COMPARABLE`, no PASS.
- Evidencia durable es sanitizada: no secretos, raw compiler logs ni payloads arbitrarios. Cada fila incluye generation/cross-RID o explica por que no aplica.
- Gates fail-closed: governance, hash, comparator, baseline, cross-RID, determinismo, no-MCP/no-network, rollback y negative violations.

```toon
harness_protocol: SDD-HARNESS-v1
source_protocol: SDD-WIKI-SOURCE-v1
doc_id: TP-GPH
block_id: TP-GPH.native-hardening-cases
kind: test-contract
audience: llm-first
source_of_truth: this
imports:
  - .docs/wiki/07_tech/TECH-GRAPH-NATIVE.md
  - .docs/wiki/09_contratos/CT-GRAPH-CLI.md
  - .docs/wiki/09_contratos/CT-DAEMON-WORKER.md
evidence:
  - .docs/wiki/06_pruebas/TP-GPH.md
  - internal/indexer/graph_staging.go
  - internal/indexer/graph_staging_test.go
  - internal/model/graph_observation.go
  - internal/service/graph_observer.go
  - worker-dotnet/MiLsp.Worker/RoslynService.cs
  - worker-dotnet/MiLsp.Worker.ContractTests/Program.cs
records:
  - id: TP-GPH-001
    type: test-suite
  - id: TP-GPH-002
    type: test-suite
  - id: TP-GPH-003
    type: test-suite
  - id: TP-GPH-004
    type: test-suite
  - id: TP-GPH-005
    type: test-suite
  - id: TP-GPH-006
    type: test-suite
  - id: TP-GPH-007
    type: test-suite
  - id: TP-GPH-008
    type: test-suite
  - id: TP-GPH-009
    type: test-suite
verify:
  - go test ./internal/model/... ./internal/indexer/... ./internal/service/... ./internal/daemon/...
  - dotnet run --project worker-dotnet/MiLsp.Worker.ContractTests/MiLsp.Worker.ContractTests.csproj
stop_if:
  - worker_order_is_not_ref=true
  - worker_batch_sealed_before_core=true
  - unsupported_symbol_reported_as_unresolved=true
  - unresolved_ids_not_stable=true
  - candidate_bounds_or_normalization_wrong=true
cases:
  - id: TC-GPH-068
    type: positivo
    given: worker Roslyn emits a graph observation
    when: nodes are serialized
    then: nodes are ordered by Ref and the batch is canonical but unsealed
    evidence: worker-dotnet/MiLsp.Worker.ContractTests/Program.cs
  - id: TC-GPH-069
    type: positivo
    given: canonical unsealed worker batch reaches core
    when: graphObserver accepts it
    then: core executes ValidateCanonical, SealGraphObservationBatch and ReadyForStaging in that order
    evidence: internal/service/graph_observer.go; internal/model/graph_observation.go
  - id: TC-GPH-070
    type: negativo
    given: local, lambda, anonymous or synthesized/implicit symbol is not an eligible graph endpoint
    when: Roslyn cannot represent it
    then: a typed omission is emitted instead of a false unresolved or semantic edge
    evidence: worker-dotnet/MiLsp.Worker/RoslynService.cs; worker-dotnet/MiLsp.Worker.ContractTests/Program.cs
  - id: TC-GPH-071
    type: negativo
    given: an eligible endpoint is genuinely missing
    when: graph assembly materializes unresolved records
    then: GraphUnresolved is sorted and deduplicated by key before IDs and CrossRID are assigned
    evidence: internal/indexer/graph_staging.go
  - id: TC-GPH-072
    type: negativo
    given: documentation candidates contain whitespace, backslashes or duplicates
    when: GraphUnresolved candidates are built
    then: trim, slash normalization, dedupe and lexical sort run before limits of 64 items and 4096 bytes
    evidence: internal/indexer/graph_staging.go; internal/indexer/graph_staging_test.go
```

## TP-GPH-001 - Identidad, NodeKey y cross-RID

> `wiki_source_table_exception: true`. Las tablas de casos heredadas se conservan como índice humano de cobertura; los bloques `toon` de este documento son la fuente normativa para los contratos LLM-first nuevos y sus gates.

**Cobertura:** RF-GPH-001 y modelo semantico.

| Caso | Tipo | Descripcion / oraculo |
|---|---|---|
| TC-GPH-001 | positivo | golden vectors validan payload length-prefixed, SHA-256 BLOB(32), hex externo y cross-RID |
| TC-GPH-002 | positivo | relocation de root, CRLF/LF y cambio solo de rango conservan NodeKey; cambio semantico lo cambia |
| TC-GPH-003 | positivo | 30 reruns y RIDs soportados producen identidad/cross-RID byte-identical |
| TC-GPH-004 | negativo | repository identity/campo requerido ausente, path absoluto/traversal o backend inestable queda unresolved |
| TC-GPH-005 | negativo | colision simulada del digest con tupla distinta invalida la generation y no rehasha con sal |
| TC-GPH-006 | negativo | Unicode no NFC, version desconocida, case/path conflictivo o simbolo anonimo sin anchor no se publica |

## TP-GPH-002 - Generation, publicacion, migracion y recovery

**Cobertura:** RF-GPH-002.

| Caso | Tipo | Descripcion / oraculo |
|---|---|---|
| TC-GPH-007 | positivo | staging completo permanece invisible y pointer swap publica una unica generation sellada |
| TC-GPH-008 | positivo | readers concurrentes observan snapshot viejo o nuevo completo, nunca mezcla |
| TC-GPH-009 | negativo | crash/cancel en cada ventana invalida staging y restaura/conserva pointer anterior |
| TC-GPH-010 | positivo | migracion additive mantiene tablas legacy y dual-read/write transaccional durante compatibilidad |
| TC-GPH-011 | positivo | rollback/downgrade ejercitado vuelve a snapshot/schema compatible sin perdida del backup metadata |
| TC-GPH-012 | negativo | pointer conflict reintenta explicitamente; publish del mismo ID/payload es idempotente |
| TC-GPH-013 | negativo | mismo generation ID con digest/counts distintos bloquea como corrupcion |
| TC-GPH-014 | negativo | query-time migration o repair implicito falla el test de solo lectura |

## TP-GPH-003 - Edges, adapters e incrementalidad

**Cobertura:** RF-GPH-003, RF-GPH-004 y RF-GPH-008.

| Caso | Tipo | Descripcion / oraculo |
|---|---|---|
| TC-GPH-015 | positivo | Roslyn emite declarations/contains/references/calls/implements/extends exactos con provenance |
| TC-GPH-016 | positivo | Go emite declarations/contains/imports y solo promueve calls/references cuando types/gopls resuelve; `typeCheckAll` comparte importer/cache para conservar identidad de paquetes importados |
| TC-GPH-016A | positivo | `TestObserveGoGraphSharesExportImporterIdentity`: imports de libreria estandar mantienen identidad coherente y el batch limpio permanece completo y stageable |
| TC-GPH-016B | negativo | `TestObserveGoGraphLocalTargetsAreUnsupported`: declaraciones locales no representadas se registran como omission `unsupported_symbol_kind`; no crean unresolved ni alteran completeness/ReadyForStaging |
| TC-GPH-016C | negativo | `TestObserveGoGraphTopLevelEmbeddedFieldRemainsUnresolved`: un endpoint local elegible pero ausente conserva `partial` y el rechazo de `ReadyForStaging`; no se promueve ni se relaja el gate |
| TC-GPH-017 | negativo | tsserver ausente/experimental produce omission; texto no crea edge semantica |
| TC-GPH-018 | negativo | Pyright ausente/experimental y extractor lexical producen candidatos/unresolved, no compiler facts |
| TC-GPH-019 | negativo | ambiguous, stale o endpoint missing produce GraphUnresolved; validacion confirma cero dangling edges |
| TC-GPH-020 | positivo | create/change/delete/rename reemplaza solo owners afectados y conserva snapshot equivalente al clean |
| TC-GPH-021 | positivo | cambio de superficie recalcula fanout; uncertainty/config/backend drift fuerza full rebuild explicitamente |
| TC-GPH-022 | negativo | cancel/crash/partial batch/provenance faltante descarta staging; stale_rate final es 0.0 |
| TC-GPH-022A | negativo | identidad explicita divergente, origin local ausente/multiple/no normalizable o git ausente falla cerrado sin fallback ni mutacion de `project.toml` |
| TC-GPH-022B | positivo | container con Go/C#/TS/Python observa Go una sola vez desde el root, procesa cada `.csproj`, omite `.sln`, comparte identidad y rebasa paths al namespace global; un proyecto Roslyn partial queda omission `backend_partial` sin bloquear batches completos |
| TC-GPH-022C | negativo | backend elegible sin batch devuelve error y conserva el graph previo; workspace explicitamente non-graph devuelve `GraphNotApplicable` |
| TC-GPH-022D | positivo | clean e incremental equivalentes, con mismo contenido/origin/toolchain/config, producen el mismo `GenerationID` y digest; `CreatedAt` queda fuera del ID |

## TP-GPH-004 - Navegacion bounded y determinista

**Cobertura:** RF-GPH-005.

| Caso | Tipo | Descripcion / oraculo |
|---|---|---|
| TC-GPH-023 | positivo | neighbors respeta direction/relation/depth y ordering canonico |
| TC-GPH-024 | positivo | callers usa calls entrante y callees calls saliente sin mezclar references |
| TC-GPH-025 | positivo | path elige shortest path y resuelve empates lexicograficamente |
| TC-GPH-026 | positivo | explain devuelve claim, endpoints, evidence y marca inferred sin elevarlo |
| TC-GPH-027 | positivo | graph stats cuenta status/backend/unresolved; validate detecta schema/pointer/endpoint/cross-RID |
| TC-GPH-028 | negativo | selector ausente/ambiguo y generation invalid/retired devuelven error/candidatos accionables |
| TC-GPH-029 | negativo | depth/result/token ceiling y cursor stale producen reject o truncation determinista |
| TC-GPH-030 | positivo | direct/daemon son semanticamente iguales y hash de tablas confirma cero writes/migrations |

## TP-GPH-005 - Impacto y autoridad wiki-codigo

**Cobertura:** RF-GPH-006 y RF-GPH-007.

| Caso | Tipo | Descripcion / oraculo |
|---|---|---|
| TC-GPH-031 | positivo | changed path resuelve seeds y direct impact con evidence path exacto |
| TC-GPH-032 | positivo | transitive mode usa solo allowlist/direccion/costo y separa inferred |
| TC-GPH-033 | positivo | tests/docs se agregan por edges/anchors; convenciones quedan heuristic |
| TC-GPH-034 | negativo | relation textual, ambiguous o unresolved no infla positivos ni viola fixture negative |
| TC-GPH-035 | comparativo | precision/recall/F1/negative violations no regresan contra affected previo ni Graphify comparable |
| TC-GPH-036 | positivo | docs canonicos siguen primeros y primary doc no cambia al enriquecer con codigo |
| TC-GPH-037 | negativo | governance/projection stale bloquea; canon-code conflict produce drift, nunca override |
| TC-GPH-038 | positivo | context pack conserva cadena obligatoria y reduce tokens solo con coverage/omissions explicitos |

## TP-GPH-006 - Federacion, MILX-v1 y packs

**Cobertura:** RF-GPH-009, RF-GPH-010 y seguridad de RF-GPH-011.

| Caso | Tipo | Descripcion / oraculo |
|---|---|---|
| TC-GPH-039 | positivo | dos member generations producen snapshot global/cross-edge determinista por cross-RID |
| TC-GPH-040 | negativo | member/target ausente, schema incompatible o cross-RID conflictivo queda omission/unresolved |
| TC-GPH-041 | positivo | query global direct/daemon mantiene budgets y no escribe member stores |
| TC-GPH-042 | negativo | documento de un workspace no gobierna otro sin enlace canonico bilateral |
| TC-GPH-043 | positivo | framing length-prefixed, describe/prepare/execute/cancel/health/shutdown y manifest golden |
| TC-GPH-044 | negativo | capability desconocida, graph/wiki write, secret, network o MCP se deniega antes de ejecutar |
| TC-GPH-045 | negativo | timeout/crash/cancel termina solo process tree de extension y cleanup queda probado |
| TC-GPH-046 | negativo | frame malformed/oversize/schema invalido se descarta sin mutar graph/core |
| TC-GPH-047 | positivo | Graphify import queda `external:graphify` advisory y no reemplaza NodeKey/evidence/wiki |
| TC-GPH-048 | positivo | pack deterministic y cache key/invalidation incluyen generation, version, digest, params y authority |

## TP-GPH-007 - Victory Lab, performance y cierre del slice

**Cobertura:** RF-GPH-011 y gates transversales RF-GPH-001..010.

| Caso | Tipo | Descripcion / oraculo |
|---|---|---|
| TC-GPH-049 | positivo | manifest/revision/fixture hashes se verifican; CRLF/LF equivalen sin ocultar mutacion semantica |
| TC-GPH-050 | comparativo | 30 repeticiones por current, Graphify y baseline previo con mismo fixture/protocolo |
| TC-GPH-051 | comparativo | correctness, precision, recall, F1 y negative violations reportan numerador/denominador |
| TC-GPH-052 | comparativo | token count canonico mide objetivo <=0.70x sin omitir autoridad/evidence requerida |
| TC-GPH-053 | comparativo | warm p95 mide objetivo <=0.80x y hot-path guard rail baseline x1.10 +25 ms |
| TC-GPH-054 | comparativo | peak RSS mide objetivo <=0.50x sin materializar grafo completo |
| TC-GPH-055 | comparativo | incremental mide objetivo <=0.70x y stale_rate=0.0 contra clean oracle |
| TC-GPH-056 | positivo | 30 outputs canonicos, NodeKeys, cross-RIDs y digests son identicos por variante/RID |
| TC-GPH-057 | negativo | security scan/runtime prueba no-MCP, no-network, no secret leak, no query write y rollback |
| TC-GPH-058 | negativo | metrica/comparator/raw sample unavailable, threshold fallido o claim global desde subset bloquea PASS |

## Comandos de verificacion minimos

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

## TP-GPH-008 - T5 Grupo E: routing, explain-change y analisis advisory

```toon
block_id: tp-gph-t5-group-e
kind: test-cases
source_of_truth: this
evidence:
  - .docs/wiki/06_pruebas/TP-GPH.md
  - internal/service/intent_test.go
  - internal/service/graph_query_test.go
verify:
  - go test ./internal/service/... ./cmd/mi-lsp/...
  - go run ./cmd/mi-lsp nav explain-change --path internal/service/intent.go --workspace <alias> --format toon
  - go run ./cmd/mi-lsp nav graph stats --workspace <alias> --format toon
  - go run ./cmd/mi-lsp nav graph validate --workspace <alias> --format toon
stop_if:
  - governance_blocked=true
  - timeout_without_typed_diagnostic=true
  - graph_claim_without_freshness_current=true
cases:
  - id: TC-GPH-059
    type: positivo
    given: "pregunta soportada con callers, callees, affected-change, tests, contracts y wiki"
    when: "mi-lsp nav intent '<question>' --workspace <alias> --format toon"
    then: "mi-lsp es el primer planner sin opt-out; devuelve intent, operation, confidence, freshness, preview, expansions y telemetry sanitizada"
  - id: TC-GPH-060
    type: positivo
    given: "path de cambio y catalogo documental gobernado"
    when: "mi-lsp nav explain-change --path <path> --workspace <alias> --format toon"
    then: "section_count=7 y las secciones son change, affected, callers, callees, tests, contracts y wiki"
  - id: TC-GPH-061
    type: positivo
    given: "preview con evidencia parcial"
    when: "se inspecciona expansions[] y next_hint"
    then: "cada expansion tiene command ejecutable y reason; la preview declara omisiones y no afirma completitud"
  - id: TC-GPH-062
    type: negativo
    given: "backend graph no disponible"
    when: "explain-change intenta completar affected, callers o callees"
    then: "fallback tipado y omisiones visibles; no se presenta heuristica como prueba graph-native"
  - id: TC-GPH-063
    type: negativo
    given: "nav ask excede el deadline"
    when: "el runtime devuelve context deadline exceeded sin diagnostico tipado"
    then: "resultado BLOCKED; no se permite sustituir silenciosamente la consulta por rg, Grep, Glob o Read"
  - id: TC-GPH-064
    type: positivo
    given: "analysis con freshness current y nodos exact/extracted"
    when: "se calcula graph rank y communities"
    then: "rank usa 0.45 authority + 0.25 impact + 0.20 centrality + 0.10 boundary; communities y digest son deterministas"
  - id: TC-GPH-065
    type: negativo
    given: "freshness lagging, stale, invalid o unknown"
    when: "se solicitan claims exactos del grafo"
    then: "claims exactos quedan omitidos o degradados con warning; solo current autoriza afirmaciones exactas"
  - id: TC-GPH-066
    type: negativo
    given: "eventos de utility o intent telemetry"
    when: "se serializa evidencia operacional"
    then: "solo metadata derivada permitida; query, prompt, argv, payload, content, snippets, secrets y raw_error no aparecen"
  - id: TC-GPH-067
    type: positivo
    given: "comando graph read-only publicado"
    when: "mi-lsp nav graph stats|validate --workspace <alias> --format toon"
    then: "stats/validate no escriben ni reparan index.db y exponen generation/status/omissions de forma estable"

```

PASS final requiere los siete TP, comparadores fijados, raw evidence, cross-RID, rollback, seguridad y autoridad wiki. Un caso `planned` no cuenta como ejecutado hasta enlazar test automatizado o evidencia manual determinista.

## TP-GPH-009 - Mapa Harness-first F1-F11

```toon
doc_id: TP-GPH
block_id: tp-gph-harness-first-f1-f11
kind: implemented-test-map
source_of_truth: this
evidence:
  - .docs/wiki/06_pruebas/TP-GPH.md
  - internal/service/intent_test.go
  - scripts/bench/harness_first/tests/test_runner.py
status: implemented_tests_and_campaign
campaign_status: PASS
campaign_record:
  schema: harness-first-campaign/v1
  campaign_id: harness-first-final-9bb3163
  source_revision: 9bb3163dc8840c76a6acfdccdbc798973f05bd49
  binary_sha256: d4dbcf3981ae4569cf92b52d2300eae0cd583436683067db4ed746f1ec1da35a
  status: PASS
  correctness_percent: 100.0
  parity: PASS_exact_digest
  parity_digest: faae133a60e5e31f0b951867b7477b112cebf49daf463b092e83294b781a072e
  retry_amplification: 1.0
  latency_p95_ms: 8009.6
  latency_p99_ms: 8591.1
  latency_budget_p95_ms: 15000
  latency_budget_p99_ms: 15000
  peak_rss_bytes: 252096512
  preview_usefulness: PASS
  evidence: .docs/auditoria/2026-07-21-milsp-harness-first-roadmap/traceability-closure.yaml
verify:
  - go test ./internal/model/... ./internal/service/... ./internal/store/... ./internal/telemetry/...
  - python -m unittest discover -s scripts/bench/harness_first/tests -p "test_*.py"
stop_if:
  - campaign_status=PASS_without_real_run=true
  - raw_campaign_output_promoted=true
features:
  F1_intent_routing:
    status: implemented
    evidence: internal/service/intent_test.go::TestClassifySupportedIntentRoutesAllT3Operations
  F2_all_workspaces_intent_plan:
    status: implemented
    evidence: internal/service/ask_test.go::TestNavAskAllWorkspacesPreservesSupportedIntentPlansDeterministically
    invariant: every_plan_preserves_workspace_and_determinism_digest
  F3_explain_change_preview:
    status: implemented
    evidence: internal/service/intent_test.go::TestExplainChangePreviewHasSevenSectionsWikiEvidenceAndExpansions
    sections: [change, affected, callers, callees, tests, contracts, wiki]
  F4_executable_expansions:
    status: implemented
    evidence: [internal/service/intent_test.go::TestIntentExpansionCommandsUseExecutableCLIOperations, internal/service/intent_test.go::TestExplainChangeExpansionPreservesNormalizedPathsAndRef, internal/service/intent_test.go::TestIncompletePathExpansionUsesExecutableDiscovery]
    shape: {command: string, reason: string}
  F5_rank_freshness_communities:
    status: implemented
    evidence: [internal/service/graph_query_test.go::TestGraphRankQueryCachesCompleteAnalysisAndReusesIt, internal/model/graph_query_test.go::TestDeterminismDigestExcludesElapsedTime]
    exact_claim_gate: freshness=current
  F6_complete_only_cache_and_digests:
    status: implemented
    evidence: [internal/service/graph_query_test.go::TestGraphRankCacheBridgeSingleConnectionCoversMissHitAndUtility, internal/store/graph_analysis_test.go::TestGetGraphAnalysisIgnoresDigestMismatch]
  F7_bounded_snapshot_queries:
    status: implemented
    evidence: [internal/service/graph_query_test.go::TestGraphQueryRejectsInvalidBudget, internal/service/graph_query_test.go::TestFinalizeGraphItemsAlwaysAdvancesCursorAtTinyBudget]
  F8_utility_caps_and_decay:
    status: implemented
    evidence: [internal/store/utility_test.go::TestUtilityGlobalScopeCapBoundsManyCandidateSignals, internal/store/utility_test.go::TestUtilityEventsApplyScopeRetentionAndDecay]
    signals: [continuation_followed, feedback_positive, feedback_negative, result_selected]
  F9_sqlite_connection_invariants:
    status: implemented
    evidence: [internal/service/graph_query_test.go::TestGraphRankCacheBridgeSingleConnectionCoversMissHitAndUtility, internal/store/db.go, internal/daemon/state_store.go]
    write_max_open_conns: 1
  F10_sanitized_telemetry:
    status: implemented
    evidence: [internal/telemetry/access_events_test.go::TestEnrichAccessEventRedactsRawTelemetryInputs, internal/telemetry/access_events_test.go::TestNormalizeAccessEventRejectsArbitraryTelemetryCodesEverywhere]
  F11_campaign_contract_and_release_gate:
    status: implemented_and_executed
    evidence: [scripts/bench/harness_first/tests/test_runner.py, docs/benchmarks/HARNESS_FIRST_CAMPAIGN.json, docs/benchmarks/HARNESS_FIRST.md]
    campaign_execution: PASS_harness-first-final-9bb3163
```

El mapa F1-F11 demuestra cobertura automatizada del árbol actual, y la campaña Harness-first fue ejecutada una única vez sobre `9bb3163` con binario real, provenance limpia (`vcs.modified=false`), RSS acotado y paridad direct/daemon por digest exacto. El registro sanitizado vive en `campaign_record` y en el paquete de cierre trazado; la salida raw permanece como evidencia externa temporal y no se promueve.
