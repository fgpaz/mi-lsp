# Harness vertical wiki ↔ código

Este directorio contiene el runner suplementario de la Wave 5 T9. Las
pruebas Go son las oráculos primarias de corrección; el runner Python solo
reproduce la campaña local de rendimiento, digest y no-escritura sobre copias
temporales del fixture T3.

## Ejecución

FINAL_VERIFY debe construir un binario temporal fuera del repositorio y
entregar su ruta al runner:

```text
python3 scripts/bench/wiki_code_bridge/runner.py \
  --binary /ruta/al/binario-temporal/mi-lsp \
  --fixture testdata/wiki-code-bidirectional \
  --output /ruta/de/salida/wiki-code-bridge.json
```

El runner usa únicamente la biblioteca estándar de Python. Cada escenario
copia el fixture a un workspace temporal, crea solamente la configuración
mínima de detección en ese workspace y ejecuta el binario con
`--no-daemon --no-auto-daemon --format json`. No instala, publica ni libera el
binario; tampoco usa red, secretos, Graphify ni un daemon externo.

La cantidad autorizada de muestras es exactamente 30. La salida es un único
JSON sanitizado: no conserva stdout/stderr, prompts, snippets completos,
secretos, rutas absolutas ni logs arbitrarios. La identidad registra revisión
(`MI_LSP_REVISION`, o `unknown`), digest SHA-256 del binario, digest del fixture
y plataforma. El runner no narra como PASS los casos que pertenecen a las
pruebas Go primarias; los deja en `primary_only_cases` y publica el inventario
cerrado completo.

Si Python no está disponible, no se reemplaza la campaña por un skip. Debe
registrarse el resultado tipado:

```yaml
status: BLOCKED
reason_code: python_unavailable
```

La función `blocked_summary` y la salida de error del runner mantienen ese
mismo contrato sin fabricar tiempos ni digests.

## Forma de salida

```yaml
schema: wiki-code-bridge-runner/v1
status: PASS|FAIL|BLOCKED
acceptance_case_inventory: [37 ids]
primary_oracle: go_tests
acceptance_results: {}
cost:
  metadata_checked: 0
  files_hashed: 0
  files_parsed: 0
  semantic_backend_calls: 0
  output_token_estimate: 0
performance:
  cold_direct_lookup_ms: null
  cold_reverse_lookup_ms: null
  warm_direct_binding_lookup_p95_ms: null
  dirty_single_file_overlay_p95_ms: null
  warm_reverse_lookup_p95_ms: null
  warm_mixed_neighbors_p95_ms: null
provenance: {}
residual_risks: []
```

Un objetivo de latencia no demostrado o incumplido nunca se convierte en
PASS por narración: se conserva como `FAIL` con un residual risk. Los fallos
de corrección, digest estable o no-escritura producen código de salida no cero.

## Inventario T9 (37 casos cerrados)

1. `full_index_fixture_baseline`
2. `reverse_lookup`
3. `supporting_only`
4. `graph_stale`
5. `edit_binding_overlay`
6. `remove_binding_tombstone`
7. `add_binding_reverse`
8. `delete_rename_target`
9. `fail_closed_inputs`
10. `raw_audit_decoys`
11. `unmapped_changed_code`
12. `modern_js_extensions`
13. `lost_watcher_event`
14. `direct_daemon_parity`
15. `no_query_writes`
16. `same_tick_rewrite`
17. `unknown_state_omission`
18. `stable_digest_30`
19. `cost_counters`
20. `graph_v1_alias_normalization`
21. `planned_binding`
22. `retired_exclusion_redirect`
23. `duplicate_doc_ids`
24. `active_retired_transition`
25. `governance_imports_self_export`
26. `graph_v1_phase_ordering`
27. `legacy_advisory`
28. `mjs_find_related`
29. `wiki_to_code_direct_precision`
30. `code_to_wiki_reverse_recall`
31. `false_direct_implementation_edges`
32. `raw_audit_primary_results`
33. `warm_mixed_neighbors_p95`
34. `warm_direct_binding_lookup_p95`
35. `stable_digest_runs`
36. `dirty_single_file_overlay_target`
37. `latency_campaign_complete`

Los umbrales son: precisión directa 1.0, recall inverso 1.0, cero aristas
directas falsas, cero resultados raw/auditoría primarios, p95 de vecinos mixtos
como máximo 1000 ms, p95 de lookup directo caliente como máximo 100 ms,
30/30 digests idénticos y overlay de un archivo sucio como máximo 250 ms.

No se ejecutan las pruebas ni este runner durante la autoría T9. FINAL_VERIFY
es la única ola que produce resultados observados.
