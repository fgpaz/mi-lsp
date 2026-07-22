# Veredicto G9: BLOCKED

```toon
doc_id: G9-VERDICT-20260721
block_id: verdict
harness_protocol: SDD-HARNESS-v1
wiki_source_protocol: SDD-WIKI-SOURCE-v1
harness_readiness: ready_for_blocked_closure
wiki_source_readiness: ready_for_blocked_closure
harness_contract:
  id: HC-G9-VERDICT
  kind: independent-verdict
  audience: dual
  imports:
    - benchmark-summary.yaml
    - review-index.yaml
  exports:
    - traceability-closure.yaml
    - closure-packet.yaml
  agent_must_read:
    - benchmark-summary.yaml
    - review-index.yaml
  agent_may_edit:
    - g9-verdict.md
    - traceability-closure.yaml
    - closure-packet.yaml
  agent_must_not_edit:
    - ae-close-verdict.md
    - benchmark_fixture
    - comparator_source
  verify:
    - verdict_matches_summary
    - promoted_verdict_is_BLOCKED_after_ae_close
    - no_global_superiority_claim
  stop_if:
    - status_is_changed_from_BLOCKED
    - evidence_is_promoted_without_ae_close
  evidence:
    - g9-verdict.md
    - benchmark-summary.yaml
harness_verdict: BLOCKED
wiki_source_verdict: BLOCKED
status: BLOCKED
promoted_verdict: BLOCKED
repository_evidence_head: ec34074455f51af3289d14f4c4c408f002283766
product_benchmark_pin: 11ac8af870d4110b6b4333199b8a8343c52ce784
sample_shape: 240=8x30
outcomes: PASS195/NC42/BLOCKED3/FAIL0
hotpath: current 472.658515ms <= limit 1308.420881ms; baseline 1166.746255ms
current_quality: PASS 30/30 except callers-direct 29/30 network_indicator
graphify_quality:
  direct: 24 PASS/6 working_set NC
  transitive: 22 PASS/6 NC/2 metadata BLOCKED
semantic_tokens: current=Graphify direct65 transitive89 ratio1.0 target0.7 impossible_without_asymmetry
formal_comparison: NOT_COMPARABLE_for_incomplete_groups
scopes: build/index/incremental NC
security:
  graph_query_read_only: PASS
  no_implicit_mcp_or_network_core: PASS
  MILX_windows_positive_containment: NC_fail_closed
  benchmark_runtime: BLOCKED
independent_review: BLOCKED
observer_failure_note: v3/v4 repeat observer failures; another run would be retry storm
release_disposition: no_install_push_pr_deploy_publish
rollback: no_mutation
```

La corrida autoritativa conserva 240 muestras, organizadas como 8 variantes por 30 repeticiones. El resultado es **BLOCKED**, no un fallo de correctness: hay 195 `PASS`, 42 `NOT_COMPARABLE`, 3 `BLOCKED` y 0 `FAIL`.

El hotpath sí pasa el límite y la calidad corriente queda documentada como 30/30, con la excepción observada de `callers-direct` (29/30 por `network_indicator`). Graphify no ofrece una base formalmente completa para declarar superioridad global: sus grupos tienen casos `NOT_COMPARABLE` y bloqueos de metadata, mientras que build, index e incremental tampoco son comparables.

La comparación de tokens comunes tampoco autoriza una conclusión competitiva. Current y Graphify producen 65 unidades directas y 89 transitivas; la razón 1.0 no alcanza el objetivo 0.7 y no puede hacerse alcanzable sin introducir asimetría. Por eso la comparación formal queda en `NOT_COMPARABLE`.

La revisión independiente G9 permanece bloqueada porque v3 y v4 repiten fallos del observador. Otra corrida igual sería un retry storm, así que no se reintenta ni se cambia el veredicto. `ae-close` se ejecutó y emitió `ae-close-verdict.md` con veredicto `BLOCKED`; `promoted_verdict` queda `BLOCKED` y la disposición local es `HOLD`.

Referencias: [[benchmark-summary.yaml]], [[review-index.yaml]], [[traceability-closure.yaml]], [[closure-packet.yaml]], [[VICTORY_LAB_V2]].
