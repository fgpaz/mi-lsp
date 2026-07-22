---
doc_id: VICTORY-LAB
title: Victory Lab competitivo
layer: TP
family: GPH
status: active
---

# Victory Lab

## v1 histórico

```yaml
harness_protocol: SDD-HARNESS-v1
source_protocol: SDD-WIKI-SOURCE-v1
id: VICTORY-LAB
doc_id: VICTORY-LAB
kind: benchmark-doc
audience: llm-first
imports:
  - '[[TP-GPH]]'
  - '[[TECH-GRAPH-NATIVE]]'
  - '[[AE-RELEASE-DISTRIBUTION]]'
exports:
  - VICTORY-LAB
agent_must_read:
  - docs/benchmarks/VICTORY_LAB.md
  - .docs/wiki/06_pruebas/TP-GPH.md
  - .docs/wiki/07_tech/TECH-GRAPH-NATIVE.md
agent_may_edit:
  - docs/benchmarks/VICTORY_LAB.md
  - benchmarks/victory-lab/v1/**
  - scripts/bench/victory_lab/**
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - python -m unittest discover -s scripts/bench/victory_lab -p "test_*.py"
  - python scripts/bench/victory_lab/runner.py --manifest benchmarks/victory-lab/v1/manifest.json --repetitions 30 --output <evidence-dir>
  - python scripts/bench/victory_lab/report.py --input <evidence-dir> --output <report.json>
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
stop_if:
  - manifest_hash_mismatch=true
  - comparator_missing=true
  - repetitions_below_30=true
  - harness_verdict=BLOCKED
evidence:
  - docs/benchmarks/VICTORY_LAB.md
  - benchmarks/victory-lab/v1/manifest.json
  - scripts/bench/victory_lab/runner.py
  - scripts/bench/victory_lab/report.py
```

Victory Lab v1 es el fundamento determinista y sin dependencias del laboratorio. El manifiesto fija la revisión de Graphify, los hashes, los casos C#/Go/TypeScript/Python/wiki/mixed/cross-workspace/extension/relationship y 30 repeticiones por defecto.

El runner v1 conserva cada caso y repetición en JSONL. El reporte calcula precisión, recall, F1, p50/p95, MAD, intervalos bootstrap deterministas y unidades de RSS, disco, salida y tokens. Los goldens son fixtures escritos a mano; distinguen positivos, negativos, ambiguos, no resueltos y `NOT_COMPARABLE`.

La sección incremental mide estado inicial, mutaciones deterministas y una reconstrucción limpia. No usa selección best-of, red, instalación ni juez de modelo.

## v2 autoritativo para G9

v2 conserva v1 como histórico y agrega attestation, grupos completos de 30 muestras, observación del árbol de procesos, controles anti-gaming, seguridad estática y separación explícita entre el pin del producto medido y el HEAD del repositorio que contiene la evidencia. La fuente de evidencia promovible es el bundle externo con alias `<external-evidence-root>/victory-g9-authoritative-v4-20260721`.

```toon
doc_id: VICTORY-LAB
block_id: v2-harness-contract
source_protocol: SDD-WIKI-SOURCE-v1
source_of_truth: this
harness_protocol: SDD-HARNESS-v1
kind: benchmark-doc
audience: dual
imports:
  - '[[TP-GPH]]'
  - '[[TECH-GRAPH-NATIVE]]'
  - '[[AE-RELEASE-DISTRIBUTION]]'
exports:
  - VICTORY-LAB
agent_must_read:
  - docs/benchmarks/VICTORY_LAB.md
  - benchmarks/victory-lab/v2/manifest.json
  - .docs/auditoria/2026-07-21-milsp-graph-native-g9-g10/benchmark-summary.yaml
agent_may_edit:
  - docs/benchmarks/VICTORY_LAB.md
  - .docs/auditoria/2026-07-21-milsp-graph-native-g9-g10/**
agent_must_not_edit:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/_mi-lsp/read-model.toml
  - ae-close-verdict.md
  - comparator_source
verify:
  - python -m unittest discover -s scripts/bench/victory_lab -p "test_*.py"
  - python scripts/bench/victory_lab/runner_v2.py --manifest benchmarks/victory-lab/v2/manifest.json --repetitions 30 --output <external-evidence-root>/victory-g9-authoritative-v4-20260721/run
  - python scripts/bench/victory_lab/report_v2.py --manifest benchmarks/victory-lab/v2/manifest.json --input <external-evidence-root>/victory-g9-authoritative-v4-20260721/run --repetitions 30 --output <external-evidence-root>/victory-g9-authoritative-v4-20260721/report.json
  - git diff --check
stop_if:
  - manifest_hash_mismatch=true
  - fixture_or_oracle_hash_mismatch=true
  - repetitions_below_30=true
  - observer_failure=true
  - comparator_group_incomplete=true
  - status_is_promoted_without_ae_close=true
evidence:
  - '<external-evidence-root>/victory-g9-authoritative-v4-20260721/run'
  - '<external-evidence-root>/victory-g9-authoritative-v4-20260721/samples.jsonl'
  - '<external-evidence-root>/victory-g9-authoritative-v4-20260721/report.json'
  - '<external-evidence-root>/victory-g9-authoritative-v4-20260721/manifest.json'
```

```toon
doc_id: VICTORY-LAB
block_id: v2-wiki-source
source_protocol: SDD-WIKI-SOURCE-v1
kind: wiki-source-contract
wiki_source_protocol: SDD-WIKI-SOURCE-v1
wiki_source_readiness: ready_for_blocked_closure
navigation_readiness: ready_for_blocked_closure
source_of_truth: this
authority:
  normative_format: fenced_toon
  human_container: docs/benchmarks/VICTORY_LAB.md
  evidence_summary: .docs/auditoria/2026-07-21-milsp-graph-native-g9-g10/benchmark-summary.yaml
  verdict: .docs/auditoria/2026-07-21-milsp-graph-native-g9-g10/g9-verdict.md
  closure: .docs/auditoria/2026-07-21-milsp-graph-native-g9-g10/closure-packet.yaml
required_fields:
  - doc_id
  - block_id
  - imports
  - exports
  - verification
  - stop_conditions
  - durable_evidence
  - traceability_links
traceability_links:
  - '[[.docs/auditoria/2026-07-21-milsp-graph-native-g9-g10/benchmark-summary.yaml]]'
  - '[[.docs/auditoria/2026-07-21-milsp-graph-native-g9-g10/review-index.yaml]]'
  - '[[.docs/auditoria/2026-07-21-milsp-graph-native-g9-g10/g9-verdict.md]]'
  - '[[.docs/auditoria/2026-07-21-milsp-graph-native-g9-g10/traceability-closure.yaml]]'
  - '[[.docs/auditoria/2026-07-21-milsp-graph-native-g9-g10/closure-packet.yaml]]'
wiki_source_blocks_reviewed:
  - v1-harness-contract
  - v2-harness-contract
  - v2-wiki-source
verify:
  - mi-lsp nav wiki validate-source --workspace <workspace> --format toon
  - mi-lsp nav wiki validate-harness --workspace <workspace> --format toon
evidence:
  - docs/benchmarks/VICTORY_LAB.md
  - .docs/auditoria/2026-07-21-milsp-graph-native-g9-g10/benchmark-summary.yaml
  - .docs/auditoria/2026-07-21-milsp-graph-native-g9-g10/g9-verdict.md
wiki_source_verdict: BLOCKED
```

### Comandos exactos de v2

Ejecutar desde la raíz del repositorio, reemplazando solo los placeholders documentados y conservando el alias externo. El benchmark se ejecuta con el pin de producto declarado por el manifiesto; la evidencia del repositorio debe registrar el HEAD real observado al ejecutar.

```text
PYTHONDONTWRITEBYTECODE=1 python scripts/bench/victory_lab/runner_v2.py --manifest benchmarks/victory-lab/v2/manifest.json --repetitions 30 --output <external-evidence-root>/victory-g9-authoritative-v4-20260721/run
PYTHONDONTWRITEBYTECODE=1 python scripts/bench/victory_lab/report_v2.py --manifest benchmarks/victory-lab/v2/manifest.json --input <external-evidence-root>/victory-g9-authoritative-v4-20260721/run --repetitions 30 --output <external-evidence-root>/victory-g9-authoritative-v4-20260721/report.json
```

### Resultado G9/G10 preservado

El resultado autoritativo queda **BLOCKED** y no se reinterpreta como `FAIL`: 240 muestras son exactamente 8x30, con `PASS195/NC42/BLOCKED3/FAIL0`. El pin del producto benchmark es `11ac8af870d4110b6b4333199b8a8343c52ce784`; el HEAD real del repositorio que conserva esta documentación y su evidencia es `2e89564b6a3033101e9cfbb7a7610acaa2acfb54`.

El hotpath pasa: `472.658515ms <= 1308.420881ms`, con baseline `1166.746255ms`. La calidad current es PASS 30/30 excepto `callers-direct` 29/30 por `network_indicator`. Graphify directo queda en 24 PASS y 6 `NOT_COMPARABLE` por `working_set`; transitive queda en 22 PASS, 6 `NOT_COMPARABLE` y 2 bloqueos de metadata.

Los tokens semánticos comunes son current=Graphify: direct 65, transitive 89, razón 1.0 frente a target 0.7. El objetivo no es alcanzable sin asimetría; como los grupos están incompletos, la comparación formal es `NOT_COMPARABLE`. Build, index e incremental también son `NOT_COMPARABLE`. No se declara superioridad global.

G10 conserva `go test -count=1 ./... PASS`, `go vet PASS`, Python 161 PASS, `.NET Release PASS 0/0` y diff check PASS; `go test -race` queda `NOT_RUN_HOST_UNSUPPORTED`. La evidencia de seguridad conserva graph query read-only PASS, ausencia de MCP/red implícitos en core PASS, MILX Windows positive containment `NOT_COMPARABLE` con fail-closed y benchmark runtime BLOCKED.

No hay instalación, push, PR, deploy ni publish. El hash del ejecutable global `16b5...ed16` permanece sin cambios y no es provenance del benchmark ni de release. Worker status es timeout, no PASS. Los hashes del benchmark son evidence-only; rollback es `no_mutation`.

La revisión independiente G9 queda BLOCKED porque v3 y v4 repiten fallos del observer; otra corrida sería retry storm. `promoted_verdict` permanece `pending_ae_close` y no se crea `ae-close-verdict.md` en esta etapa.
