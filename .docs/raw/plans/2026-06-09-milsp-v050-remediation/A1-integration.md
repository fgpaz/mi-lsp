# Task A1: Integration aggregator

## Shared Context
**Goal:** Mergear las 10 ramas de lane en una rama de integración, resolver conflictos por unión de intención bloqueada, y dejar el árbol verde (build/vet/test Go + dotnet build).
**Stack:** Go, .NET, git.
**Architecture:** Único agregador. Crea `v050/integration` desde `main` (post-T2) y mergea L1..L10 en orden. Por la propiedad exclusiva de archivos, los conflictos esperados son mínimos (solo `model/types.go`/`go.mod` si alguna lane lo tocó indebidamente).

## Locked Decisions
- Orden de merge: L7 (posee types.go) primero, luego L3, L2, L1, L5, L6, L4, L9, L8, L10.
- Si dos ramas tocaron el mismo archivo Go (no debería pasar): es defecto de planificación → registrar y resolver por unión solo si la intención no se contradice; si se contradice, STOP.
- Tras cada merge, `go build ./...` debe quedar verde; si un merge rompe, resolver antes del siguiente.

## Task Metadata
```yaml
id: A1
depends_on: [L1, L2, L3, L4, L5, L6, L7, L8, L9, L10]
agent_type: general-purpose
goal_id: G6
github_issues: []
expected_outcome: "rama v050/integration con todas las lanes, go build/vet/test verde y dotnet build verde."
files:
  - read: "all lane branches"
complexity: high
done_when:
  - "go build ./... exits 0"
  - "go vet ./... exits 0"
  - "go test ./... passes"
  - "dotnet build worker-dotnet/MiLsp.Worker exits 0 (or .NET SDK absence recorded)"
evidence_expected:
  - ".docs/auditoria/2026-06-09-milsp-v050-remediation/A1-integration.yaml"
stop_if:
  - "two lane branches modify the same Go file with contradictory intent — STOP, report planning defect"
  - "go test fails after integration and the failing test crosses two lanes — STOP, escalate"
```

## Reference
Verdicts de cada lane: `.docs/auditoria/2026-06-09-milsp-v050-remediation/L*-verdict.yaml`. Matriz de propiedad en el plan principal.

## Prompt
Sos el agregador. Leé primero los verdicts de cada lane (no los transcripts). Creá `git worktree add C:/wt/v050-integration -b v050/integration main`. Mergeá las ramas en el orden bloqueado. Tras cada merge corré `go build ./...`. Al terminar, `go vet ./...`, `go test ./...`, y `dotnet build worker-dotnet/MiLsp.Worker` (si el SDK existe). Resolvé conflictos solo por unión de intención no contradictoria. Registrá todo en `A1-integration.yaml` (orden, conflictos, resoluciones, exit codes). No hagas push.

## Execution Procedure
1. `git worktree add C:/wt/v050-integration -b v050/integration main`.
2. `cd C:/wt/v050-integration`.
3. Para cada lane en orden: `git merge --no-edit v050/<lane>`; resolvé conflictos; `go build ./...`.
4. `go vet ./... && go test ./...`.
5. `dotnet build worker-dotnet/MiLsp.Worker` (o registrá ausencia de SDK).
6. Commit de la integración si hubo resoluciones. `A1-integration.yaml`.

## Skeleton
```bash
git worktree add C:/wt/v050-integration -b v050/integration main
for L in l7 l3 l2 l1 l5 l6 l4 l9 l8 l10; do git merge --no-edit v050/$L && go build ./... || break; done
go vet ./... && go test ./...
```

## Verify
`go build ./... && go vet ./... && go test ./...` → todo exit 0

## Commit
`chore(v050): integrate all remediation lanes`
