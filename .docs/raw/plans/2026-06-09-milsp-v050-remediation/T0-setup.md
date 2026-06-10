# Task T0: Worktrees + branches + build baseline

## Shared Context
**Goal:** Preparar 10 worktrees git aislados (uno por lane) + rama de integración, y confirmar baseline verde antes de mutar.
**Stack:** Go 1.2x, git worktrees, PowerShell en Windows ARM64.
**Architecture:** Cada lane Wave 1 trabaja en su propio worktree/branch desde `main` (268f512). El agregador (A1) hace merge por commits.

## Locked Decisions
- Base ref = `main` @ `268f512`. Ningún worktree parte de otra rama.
- Naming de ramas: `v050/<lane-id>` (ej. `v050/l1-async-indexing`). Worktrees bajo `C:/wt/v050-<lane-id>`.
- Ningún subagente hace `git push`; solo commits locales en su branch.

## Task Metadata
```yaml
id: T0
depends_on: []
agent_type: ps-worker
goal_id: G6
github_issues: []
expected_outcome: "10 worktrees+branches creados desde main y un build baseline verde registrado."
files:
  - read: go.mod
complexity: low
done_when:
  - "git worktree list shows 10 v050-* worktrees"
  - "go build ./... exits 0 in the main checkout (baseline)"
evidence_expected:
  - ".docs/auditoria/2026-06-09-milsp-v050-remediation/T0-worktrees.yaml"
stop_if:
  - "git worktree add fails (dirty index, name collision)"
  - "baseline go build ./... is NOT 0 — abort, the repo is already broken"
```

## Reference
`git worktree add <path> -b <branch> main` — patrón estándar; ver memoria de carrera de git: worktrees evitan que subagentes paralelos se pisen.

## Prompt
Sos un operador de git. Creá exactamente estos 10 worktrees, cada uno con su rama nueva desde `main`. No toques el checkout principal salvo para el build baseline. No hagas push. Registrá el resultado en YAML.

Lanes y ramas:
`l1-async-indexing, l2-daemon-core, l3-telemetry-store, l4-security-surface, l5-daemon-runtime, l6-search-sqlite-docs, l7-governance-tokens-profile, l8-dotnet-worker, l9-doctor, l10-docs-scripts-release`.

## Execution Procedure
1. `cd C:/repos/mios/mi-lsp`; confirmá `git status` limpio y `git rev-parse main` == `268f5125d70c12db67ef53bfed997c10f49589f6` (si difiere, registralo pero continuá desde el main actual).
2. Para cada lane `<id>`: `git worktree add C:/wt/v050-<id> -b v050/<id> main`.
3. En el checkout principal: `go build ./...`; capturá exit code. Si != 0, STOP.
4. Escribí `.docs/auditoria/2026-06-09-milsp-v050-remediation/T0-worktrees.yaml` con: base_sha, lista de {lane, branch, worktree_path}, baseline_build_exit.

## Skeleton
```yaml
# T0-worktrees.yaml
base_sha: "268f512..."
baseline_build_exit: 0
worktrees:
  - { lane: l1-async-indexing, branch: v050/l1-async-indexing, path: C:/wt/v050-l1-async-indexing }
  # ...resto
```

## Verify
`git worktree list` → muestra los 10 worktrees `v050-*`

## Commit
(no commit; T0 solo crea worktrees y registra evidencia, no modifica archivos versionados del repo principal)
