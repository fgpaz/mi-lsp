# Task L9: mi-lsp doctor (self-check command)

## Shared Context
**Goal:** Comando read-only `mi-lsp doctor` que detecta de forma recurrente los problemas de la auditoría.
**Stack:** Go.
**Architecture:** Worktree `C:/wt/v050-l9-doctor`, branch `v050/l9-doctor`. Único dueño de archivos NUEVOS `internal/cli/doctor.go` y `internal/service/doctor.go`. No modifica archivos de otras lanes; consume APIs públicas existentes (daemon status, registry, version).

## Locked Decisions
- 100% read-only: NO muta daemon, índice ni registry. Solo lee.
- Chequeos: (1) aliases stale en registry, (2) drift de versión daemon vs CLI (daemon!=dev y ==cli), (3) tamaño de daemon.db > umbral, (4) dirs vigilados cerca del límite de handles, (5) binario `+dirty`, (6) governance_blocked, (7) tasa de truncación/recall alta si disponible.
- Salida con severidad P1/P2/P3 y exit code: 0 si no hay P1, !=0 si hay P1.
- Si una API que doctor necesita pertenece a otra lane y aún no existe, usar lo disponible en `main` (post-T2) y marcar el chequeo como `skipped` con razón.

## Task Metadata
```yaml
id: L9
depends_on: [T2]
agent_type: general-purpose
goal_id: G5
github_issues: []
expected_outcome: "mi-lsp doctor corre read-only y reporta hallazgos con severidad y exit code."
files:
  - create: internal/cli/doctor.go
  - create: internal/service/doctor.go
complexity: medium
done_when:
  - "go build ./... exits 0"
  - "go test ./internal/service/... passes (doctor unit tests)"
  - "mi-lsp doctor runs without mutating state (manual smoke in verdict)"
evidence_expected:
  - ".docs/auditoria/2026-06-09-milsp-v050-remediation/L9-verdict.yaml"
stop_if:
  - "a check would require mutating state to evaluate — mark it skipped, never mutate"
```

## Reference
Auditoría `.docs/auditoria/2026-06-09-post-release-audit-v042/informe-auditoria.md` (cada hallazgo es un chequeo candidato). Comando existente `mi-lsp version`/`daemon status` para reusar lectura.

## Prompt
Creá `internal/cli/doctor.go` (registro del subcomando `doctor` en el árbol cobra/flag existente) y `internal/service/doctor.go` (lógica). Implementá los 7 chequeos read-only listados en Locked Decisions, cada uno devolviendo {id, severity, ok, detail}. Salida en formato envelope estándar (`--format toon`); exit 0 si no hay P1, !=0 si hay P1. No mutes nada. Registrá tests unitarios de la lógica de severidad/exit.

## Execution Procedure
1. `cd C:/wt/v050-l9-doctor`; `git merge --no-edit main`.
2. Creá los 2 archivos; registrá el subcomando.
3. `go build ./... && go vet ./... && go test ./internal/service/...`.
4. Smoke: `mi-lsp doctor --format toon`; confirmá que no muta (registry/daemon iguales antes/después).
5. Commit. `L9-verdict.yaml`.

## Skeleton
```go
type Check struct { ID, Severity, Detail string; OK bool }
func RunDoctor(ctx context.Context) ([]Check, error) {
    checks := []Check{ checkStaleAliases(), checkDaemonVersion(), checkDbSize(), checkWatchedDirs(), checkBinaryDirty(), checkGovernance(), checkTruncationRate() }
    return checks, nil
}
// exit: if any P1 not ok -> os.Exit(1)
```

## Verify
`go test ./internal/service/...` (doctor) → PASS

## Commit
`feat(cli): add read-only 'mi-lsp doctor' self-check command`
