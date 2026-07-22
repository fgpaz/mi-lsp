---
linear_parent: not_applicable_user_authorized
linear_child: not_applicable_user_authorized
anchors: [AE-RELEASE-DISTRIBUTION, TP-GPH, TP-WIKI, CT-GRAPH-CLI, CT-NAV-WIKI]
allowed_paths: [scripts/release/**, scripts/bench/**, docs/benchmarks/**, .github/workflows/**, .docs/auditoria/2026-07-21-milsp-harness-first-roadmap/**]
forbidden_paths: [.env*, secrets/**, comparator source trees, C:/repos/mios/mi-pi/**]
verify: [go build ./..., go vet ./..., go test ./..., pre-push guard, CI checks, installed arm64 readback]
stop_if: [dirty provenance, final benchmark repeated, CI red, release asset mismatch]
secret_scan: {required: true, evidence: names-only-no-values}
---
# Task T6: Integración, benchmark único, binarios y release

## Shared Context
**Goal:** G6, cerrar ciclo AE e integrar un release verificable.
**Stack:** Go build/test, PowerShell release, GitHub CLI.
**Architecture:** una única campaña final sobre candidato limpio; source SHA, binaries, installed readback y release assets deben coincidir.

## Locked Decisions
- Sólo un benchmark final end-to-end; no Victory Lab 30x repetido.
- Windows arm64 es plataforma obligatoria; `go test -race` queda NOT_RUN_HOST_UNSUPPORTED.

## Task Metadata
```yaml
id: T6
depends_on: [T5]
agent_type: ps-worker
goal_id: G6
github_issues: []
expected_outcome: "candidato limpio integrado en origin/main y release publicado con binarios verificados."
files:
  - modify: scripts/release/**
  - modify: scripts/bench/**
  - modify: docs/benchmarks/**
  - modify: .github/workflows/**
  - create: .docs/auditoria/2026-07-21-milsp-harness-first-roadmap/implementation-summary.yaml
complexity: high
done_when:
  - "full deterministic suite exits 0"
  - "single final benchmark meets correctness/latency/RSS budgets"
  - "release readback SHA equals integrated main SHA"
evidence_expected:
  - "benchmark summary"
  - "binary SHA256 and go version -m"
  - "CI/PR/release URLs"
stop_if:
  - "candidate tree is dirty at provenance capture"
  - "benchmark lacks terminal correctness or RSS"
```

## Reference
`scripts/release/ae-release-binaries.ps1`, `scripts/release/regression-smoke.ps1`, existing Victory Lab child metrics only where reusable.

## Prompt
Integra commits por wave, corre una sola suite completa y un único benchmark E2E que mida correctness, p50/p95/p99, RSS peak, tokens/preview usefulness, direct/daemon parity y no retry amplification. Construye/publica binarios requeridos incluyendo windows-arm64, verifica `go version -m`, checksums, worker status, daemon stop/start, governance, wiki pack y explain-change desde binario real. Después de trace/audit/pre-push verdes, push branch, crea PR, espera CI, merge protegido a main, crea tag/release nuevo y verifica assets/readback.

## Execution Procedure
1. Confirma árbol limpio y commits atómicos de T1-T5.
2. Ejecuta `go build ./...`, `go vet ./...`, `go test ./...` una vez.
3. Ejecuta el benchmark final una vez; un fallo se diagnostica y bloquea, no se reintenta ciegamente.
4. Ejecuta release script para arm64 y RIDs canónicos; captura hashes/provenance.
5. Corre smokes del binario real e instalado.
6. Delega cierre a T7/T8; sólo después ejecuta PR/CI/merge/release y readback.

## Skeleton
```yaml
final_benchmark:
  repetitions: 1_campaign
  requires: [correctness, p50, p95, p99, peak_rss, direct_daemon_parity, usefulness]
release_readback:
  expected_sha: integrated_origin_main_sha
  windows_arm64: required
```

## Test
Suite completa única, benchmark único y smokes de binario real para wiki pack, explain-change, graph freshness/rank y worker status.

## Verify
`go build ./... && go vet ./... && go test ./...` + release/readback gates -> PASS

## Commit
`chore(release): publish Harness-first graph-native release - Gabriel Paz -`
