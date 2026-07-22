---
doc_id: TECH-GRAPH-NATIVE
title: Arquitectura del Graph Kernel nativo
layer: TECH
family: GRAPH
status: accepted-design
---

```yaml
harness_protocol: SDD-HARNESS-v1
source_protocol: SDD-WIKI-SOURCE-v1
id: "TECH-GRAPH-NATIVE"
doc_id: "TECH-GRAPH-NATIVE"
kind: "tech-spec"
audience: "llm-first"
imports:
  - '[[00_gobierno_documental]]'
  - '[[07_baseline_tecnica]]'
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
  - '[[DB-SYMBOL-EDGE-GRAPH]]'
  - '[[CT-GRAPH-CLI]]'
  - '[[CT-MILX-V1]]'
exports:
  - 'TECH-GRAPH-NATIVE'
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/07_baseline_tecnica.md
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
  - .docs/wiki/07_tech/TECH-GRAPH-NATIVE.md
  - .docs/wiki/08_db/DB-SYMBOL-EDGE-GRAPH.md
  - .docs/wiki/09_contratos/CT-GRAPH-CLI.md
  - .docs/wiki/09_contratos/CT-MILX-V1.md
agent_may_edit:
  - .docs/wiki/07_baseline_tecnica.md
  - .docs/wiki/07_tech/TECH-GRAPH-NATIVE.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --paths .docs/wiki/07_tech/TECH-GRAPH-NATIVE.md --format toon
  - mi-lsp nav wiki validate-source --workspace mi-lsp --paths .docs/wiki/07_tech/TECH-GRAPH-NATIVE.md --format toon
  - go test ./...
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
  - graph_identity_contract_unresolved=true
  - graph_schema_contract_unresolved=true
evidence:
  - .docs/wiki/07_tech/TECH-GRAPH-NATIVE.md
  - .docs/wiki/06_pruebas/TP-GPH.md
```

Volver a [07_baseline_tecnica.md](../07_baseline_tecnica.md).

## Resumen y boundary

Este documento expande la arquitectura objetivo del grafo nativo. El root `07_baseline_tecnica.md` conserva ownership y decisiones transversales; este detalle fija componentes, flujo de datos, dependencias y fallos. `accepted-design` no afirma implementacion: cada componente pasa a stable solo con su TP y Victory gate.

## Componentes y ownership

```toon
doc_id: TECH-GRAPH-NATIVE
block_id: TECH-GRAPH-NATIVE.components
kind: architecture
source_of_truth: this
verify:
  - mi-lsp nav wiki validate-source --workspace mi-lsp --format toon
evidence:
  - .docs/wiki/07_tech/TECH-GRAPH-NATIVE.md
  - .docs/wiki/06_pruebas/TP-GPH.md
status: accepted-design
components:
  graph_model:
    owner: Graph-Kernel
    planned_paths: [internal/model/graph_kernel.go]
    owns: [NodeKey-v1, GraphGeneration-records, typed-status-reasons, cross-RID]
  adapters:
    owner: Semantic-backends
    planned_paths: [internal/indexer/graph_adapters.go]
    inputs: [Roslyn, Go-parser-types-gopls, tsserver-gated, Pyright-gated]
    output: GraphObservationBatch-only
  generation_builder_validator:
    owner: Graph-Kernel
    planned_paths: [internal/indexer/graph_staging.go]
    owns: [owner-path-incrementality, staging, validation]
  graph_store_publisher:
    owner: Store
    planned_paths: [internal/store/graph_schema.go, internal/store/graph_persistence.go]
    authority: repo-local-SQLite
  query_engine:
    owner: Service
    planned_paths: [internal/graph, internal/service/graph_query.go]
    properties: [read-only, storage-backed, bounded, direct-daemon-parity]
  wiki_authority_bridge:
    owner: Docgraph-and-service
    rule: wiki-authority-before-code-evidence
  federation:
    owner: Core-with-optional-daemon-cache
    authority: derived-view-only
  context_optimizer:
    owner: Core
    output: ContextPack
  milx_host:
    owner: Extensions-runtime
    isolation: separate-process
    contract: MILX-v1
  victory_lab:
    owner: Quality-gate
    role: deterministic-comparison-not-runtime
```

## Pipeline y publicacion

```toon
doc_id: TECH-GRAPH-NATIVE
block_id: TECH-GRAPH-NATIVE.pipeline
kind: flow
source_of_truth: this
verify:
  - mi-lsp nav wiki validate-source --workspace mi-lsp --format toon
evidence:
  - .docs/wiki/07_tech/TECH-GRAPH-NATIVE.md
  - .docs/wiki/06_pruebas/TP-GPH.md
sequence:
  - resolve-workspace-repository-and-backend-identities
  - compute-source-and-config-fingerprints
  - collect-GraphObservationBatch-per-owner
  - derive-NodeKeys-edge-keys-evidence-and-unresolved
  - build-staged-generation-by-copy-forward-plus-owner-replacement
  - validate-collisions-endpoints-digests-cross-RIDs-and-coverage
  - persist-and-seal-staging-in-SQLite
  - atomically-swap-active-graph-generation-pointer
  - serve-read-only-bounded-queries
publication:
  visible_states: [active, explicitly-selected-retired]
  invisible_states: [staged, invalid]
  crash_rule: prior-valid-pointer-remains-or-is-restored
  query_time_migration: forbidden
```

Adapters no conocen SQLite ni publican. Store no inventa identidad. Service no reextrae ni repara durante query. El daemon solo conserva warm state/cache y no cambia semantica.

## Runtime y memoria

```toon
doc_id: TECH-GRAPH-NATIVE
block_id: TECH-GRAPH-NATIVE.runtime-constraints
kind: guardrails
source_of_truth: this
verify:
  - mi-lsp nav wiki validate-source --workspace mi-lsp --format toon
evidence:
  - .docs/wiki/07_tech/TECH-GRAPH-NATIVE.md
  - .docs/wiki/06_pruebas/TP-GPH.md
storage:
  authority: .mi-lsp/index.db
  adjacency: indexed-SQL-frontiers
  full-graph-in-memory: forbidden
  persisted-transitive-closures: forbidden
  networkx-core-dependency: forbidden
query:
  snapshot: fixed-at-request-start
  writes: forbidden
  bounds:
    depth-default: 1
    depth-max: 6
    path-depth-max: 12
    result-default: 50
    result-max: 500
    token-default: 4000
    token-max: 20000
cache:
  keys: [generation-digest, operation-and-parameters, authority-profile-digest, extension-or-pack-version-digest]
  stale-cache-behavior: discard-or-explicit-omission
```

## Backend maturity

```toon
doc_id: TECH-GRAPH-NATIVE
block_id: TECH-GRAPH-NATIVE.backend-maturity
kind: compatibility
source_of_truth: RF-GPH-008
verify:
  - mi-lsp nav wiki validate-source --workspace mi-lsp --format toon
evidence:
  - .docs/wiki/07_tech/TECH-GRAPH-NATIVE.md
  - .docs/wiki/06_pruebas/TP-GPH.md
stable-order: [csharp-roslyn, go-parser-types-gopls, typescript-tsserver, python-pyright]
rules:
  csharp: compiler-backed-relations-are-primary
  go: AST-is-extracted-and-types-or-gopls-required-for-semantic-promotion
  typescript: gated-until-lifecycle-and-edge-oracles-pass
  python: lexical-catalog-never-becomes-semantic-edge
  text: candidates-doc-mentions-or-unresolved-only
```

## Fallos y recovery

```toon
doc_id: TECH-GRAPH-NATIVE
block_id: TECH-GRAPH-NATIVE.failure-modes
kind: operations
source_of_truth: this
verify:
  - mi-lsp nav wiki validate-source --workspace mi-lsp --format toon
evidence:
  - .docs/wiki/07_tech/TECH-GRAPH-NATIVE.md
  - .docs/wiki/06_pruebas/TP-GPH.md
failures:
  identity-or-hash-collision: invalidate-generation
  dangling-or-stale-edge: unresolved-and-reject-exact-claim
  adapter-timeout-or-crash: discard-partial-batch
  incremental-fanout-unknown: full-rebuild-required
  SQLite-write-or-pointer-conflict: rollback-transaction
  process-crash-during-publish: recovery-keeps-prior-valid-generation
  daemon-unavailable: direct-mode
  graph-unavailable: legacy-or-docs-first-fallback-with-warning
  wiki-governance-blocked: diagnosis-and-repair-only
  MILX-timeout-crash-malformed: terminate-extension-preserve-core
rollback:
  unit: atomic-wave-commit-plus-immutable-prior-generation
  evidence: preserve-sanitized-reason-and-gate-result
```

## Implementacion por slices

```toon
doc_id: TECH-GRAPH-NATIVE
block_id: TECH-GRAPH-NATIVE.slices
kind: implementation-plan
source_of_truth: this
plan_anchor: RF-GPH-011
verify:
  - mi-lsp nav wiki validate-source --workspace mi-lsp --format toon
evidence:
  - .docs/wiki/07_tech/TECH-GRAPH-NATIVE.md
  - .docs/wiki/06_pruebas/TP-GPH.md
order: [G1-identity-records, G2-Roslyn-Go-adapters, G3-staging-validation, G4-SQLite-publish-recovery, G5-bounded-query, G6-explain-impact-wiki, G7-MILX-host, G8-CLI-no-MCP, G9-victory, G10-closure]
join_rule: next-slice-opens-only-after-current-TP-and-regression-gates-pass
planned_paths:
  status: slice-targets
  by_slice:
    G1-identity-records: [internal/model/graph.go, internal/model/graph_observation.go, internal/store/schema.go]
    G2-Roslyn-Go-adapters: [internal/indexer/graph_pipeline.go, internal/indexer/extractor_go.go, internal/service/graph_observer.go]
    G3-staging-validation: [internal/indexer/graph_staging.go, internal/model/graph.go]
    G4-SQLite-publish-recovery: [internal/store/schema.go, internal/store/graph.go, internal/store/graph_recovery.go, internal/indexer/graph_staging.go]
    G5-bounded-query: [internal/store/graph_query.go, internal/service/graph_query.go]
    G6-explain-impact-wiki: [internal/store/graph_impact.go, internal/service/graph_impact.go, internal/service/wiki_code_context.go]
    G7-MILX-host: [internal/milx/host.go, internal/milx/pack.go]
    G8-CLI-no-MCP: [internal/service/graph_query.go, internal/service/graph_impact.go]
    G9-victory: []
    G10-closure: []
implementation_paths:
  status: current-tree-verified
  files: [internal/model/graph.go, internal/model/graph_observation.go, internal/indexer/graph_pipeline.go, internal/indexer/extractor_go.go, internal/indexer/graph_staging.go, internal/store/schema.go, internal/store/graph.go, internal/store/graph_recovery.go, internal/store/graph_query.go, internal/store/graph_impact.go, internal/service/graph_observer.go, internal/service/graph_query.go, internal/service/graph_impact.go, internal/service/wiki_code_context.go, internal/milx/host.go, internal/milx/pack.go]
```

## Sync y documentos relacionados

- Modelo funcional: `[[05_modelo_datos]]`, `[[RF-GPH-001]]` a `[[RF-GPH-011]]`, `[[TP-GPH]]`.
- Persistencia: `[[DB-SYMBOL-EDGE-GRAPH]]`.
- CLI y envelopes: `[[CT-GRAPH-CLI]]`.
- Extensiones: `[[CT-MILX-V1]]`.
- Release: `[[AE-RELEASE-DISTRIBUTION]]`.

Cambiar ownership, pipeline, authority, backend maturity o degradacion visible exige sincronizar RF/TP. Cambiar tablas/migracion exige sincronizar `08`; cambiar comando/envelope/protocolo exige sincronizar `09`.
