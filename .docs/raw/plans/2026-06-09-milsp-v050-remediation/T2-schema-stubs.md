# Task T2: Schema + stubs land (desbloquea Wave 1)

## Shared Context
**Goal:** Materializar en un único commit los campos de struct nuevos y los stubs de interfaz cruzada, para que las 10 lanes compilen en paralelo sin tocar los mismos símbolos.
**Stack:** Go.
**Architecture:** Este commit va a `main` (o a una rama base `v050/schema` que todos los worktrees rebasan). Define el contrato; las lanes implementan el cuerpo.

## Locked Decisions
- T2 corre en el checkout principal sobre `main` y commitea; luego cada worktree de lane hace `git rebase main` (o `git merge main`) para recibir el schema. **Hacé este rebase como parte de T2** dejando los 10 worktrees actualizados.
- Los stubs devuelven `errors.New("not implemented: <symbol>")` o el zero-value; NO implementan lógica.
- Campos nuevos usan `omitempty` para no inflar tokens por defecto.

## Task Metadata
```yaml
id: T2
depends_on: [T1]
agent_type: general-purpose
goal_id: G4
github_issues: []
expected_outcome: "go build ./... verde con los campos y stubs nuevos; los 10 worktrees de lane contienen el schema."
files:
  - modify: internal/model/types.go
  - create: internal/indexer/job.go
  - modify: internal/store/state_store.go
complexity: medium
done_when:
  - "go build ./... exits 0"
  - "go vet ./... exits 0"
  - "all 10 v050 worktrees contain commit with schema (git log shows it)"
evidence_expected:
  - "build log captured in task report"
stop_if:
  - "adding fields breaks TOON/JSON marshaling tests (go test ./internal/output/...)"
```

## Reference
`internal/model/types.go` structs `AccessEvent`, `QueryEnvelope`, `GovernanceStatus` — seguir el estilo de tags JSON existente.

## Prompt
Agregá SOLO declaraciones (campos + stubs), sin lógica:
1. En `internal/model/types.go`:
   - `AccessEvent`: campo `DecisionHash string `json:"decision_hash,omitempty"`.
   - Nuevo tipo `type OutputProfile string` con consts `OutputProfileHuman OutputProfile = "human"` y `OutputProfileAgent OutputProfile = "agent"`.
   - `QueryEnvelope`: campo `Profile OutputProfile `json:"-"`.
2. Creá `internal/indexer/job.go` con el tipo `IndexMode`, `IndexJobState`, y los stubs:
   `func (e *Engine) StartBackgroundIndex(ctx context.Context, reg model.WorkspaceRegistration, mode IndexMode) (string, error)` → `return "", errors.New("not implemented: StartBackgroundIndex")`.
   `func (e *Engine) IndexJobStatus(jobID string) (IndexJobState, bool)` → `return IndexJobState{}, false`.
   (Si el indexer no usa el receiver `Engine`, usá el tipo real confirmado en discovery.yaml `cross_interfaces`.)
3. En `internal/store/state_store.go`: agregá el método stub `func (s *StateStore) PurgeAndVacuum(retentionDays int, maxBytes int64) (int, bool, error) { return 0, false, errors.New("not implemented: PurgeAndVacuum") }`.
4. `go build ./...` y `go vet ./...`. Si verde, `git add -A && git commit -m "feat(v050): land cross-lane schema and interface stubs"`.
5. Propagá a los worktrees: por cada `C:/wt/v050-<id>`, `git -C C:/wt/v050-<id> merge --no-edit main`.

## Execution Procedure
1. Leé `discovery.yaml` para el tipo real del receiver del indexer.
2. Aplicá los 3 cambios de archivo.
3. `go build ./... && go vet ./...`.
4. Commit en main.
5. Merge main en los 10 worktrees.

## Skeleton
```go
// internal/indexer/job.go
package indexer
import ("context"; "errors"; "github.com/.../internal/model")
type IndexMode int
const ( IndexModeFull IndexMode = iota; IndexModeIncremental )
type IndexJobState struct { JobID string; Phase string; Done bool; Err string }
func (e *Engine) StartBackgroundIndex(ctx context.Context, reg model.WorkspaceRegistration, mode IndexMode) (string, error) {
    return "", errors.New("not implemented: StartBackgroundIndex")
}
func (e *Engine) IndexJobStatus(jobID string) (IndexJobState, bool) { return IndexJobState{}, false }
```

## Verify
`go build ./...` → exit 0

## Commit
`feat(v050): land cross-lane schema and interface stubs`
