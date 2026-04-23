# Close Remaining `mi-lsp` Hardening (2026-04-23)

## Intent

Cerrar los dos slices pendientes ya abiertos en el dirty set congelado:

- docs/ranking/tokenization/fallback documentation
- index jobs cancelation force path + `nav trace` legacy disk fallback

Mantener sin reabrir:

- gobernanza `spec_backend` sana
- branch/PR policy: una rama, un PR, sin push directo a `main`
- guardrail de binarios: todos los smokes de Windows con `C:\Users\fgpaz\bin\mi-lsp.exe` desde cwd neutral; todos los smokes de WSL con ruta explicita
- `RF-GAS-09/10` como evidencia externa de fallback, no como canon nuevo de `mi-lsp`
- dirty set permitido: los 18 paths congelados del plan mas esta plan tree y el artefacto final de trazabilidad/auditoria

## Frozen dirty set at baseline

```text
M  .docs/wiki/03_FL/FL-IDX-01.md
M  .docs/wiki/04_RF/RF-QRY-010.md
M  .docs/wiki/06_pruebas/TP-QRY.md
M  .docs/wiki/07_baseline_tecnica.md
M  .docs/wiki/09_contratos/CT-CLI-DAEMON-ADMIN.md
M  .docs/wiki/09_contratos/CT-NAV-ASK.md
M  .docs/wiki/09_contratos_tecnicos.md
M  internal/cli/index.go
M  internal/docgraph/docgraph.go
M  internal/service/doc_ranking.go
M  internal/service/index_jobs.go
M  internal/service/owner_ranking_test.go
M  internal/service/trace.go
M  internal/service/trace_test.go
M  internal/store/index_jobs.go
?? internal/store/index_jobs_test.go
?? internal/store/process_terminate_unix.go
?? internal/store/process_terminate_windows.go
```

## Baseline repo truth

- Branch: `main`
- `HEAD`: `335388d8c767c882e686d04a1b71231876a68e11`
- Governance: healthy, projection in sync
- Open GitHub PRs in `fgpaz/mi-lsp`: `0`

## Execution order

1. Materialize Wave 0 evidence and stop conditions.
2. Inspect and finish the docs/ranking owned slice only.
3. Inspect and finish the index/trace owned slice only.
4. Run required verification.
5. Produce durable closure artifact and assess branch/PR/merge readiness from verified state.
