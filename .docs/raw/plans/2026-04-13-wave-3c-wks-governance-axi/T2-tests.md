# Task T2: Tests RF-WKS-004/005 invariants

## Shared Context
**Goal:** Cobertura de tests para: governance gate blocking en nav.pack + nav.route, --axi+--classic error, MI_LSP_AXI=1 activa AXI, --classic prevalece.
**Stack:** Go test, `internal/service/governance_test.go`, `internal/cli/axi_test.go`
**Architecture:** `governance_test.go` ya tiene tests de governance. `axi_test.go` ya tiene tests de AXI. Solo agregar tests faltantes para los invariants de RF-WKS-004/005 que no estén cubiertos.

## Task Metadata
```yaml
id: T2
depends_on: [T0]
agent_type: ps-worker
files:
  - modify: internal/service/governance_test.go  # gate blocking en nav.pack/nav.route
complexity: low
done_when: "go test ./internal/service -run Governance -count=1 EXIT:0"
```

## Reference
`internal/service/governance_test.go` — tests existentes como patrón.
`internal/service/governance_test_helpers_test.go:10-48` — fixture `writeSpecBackendGovernanceFixture`.
`internal/service/route_test.go:1-45` — patrón de setup de workspace para nav.route tests.

## Prompt
Agrega tests a `governance_test.go` para los invariantes no cubiertos.

**Busca primero** qué tests ya existen con `grep -n "func Test" internal/service/governance_test.go`. No duplicar.

**Test 1: `TestNavPackBlockedWhenGovernanceIsInvalid`** (si no existe):
- Setup: crear workspace con `writeWorkspaceFile` para `.docs/wiki/` pero SIN governance válida (sin `00_gobierno_documental.md`)
- Registrar workspace
- Ejecutar `nav.pack` con task cualquiera
- Assertar: `env.Ok == false` O el envelope contiene `governance_blocked=true` en Items
- O alternativamente: si `governanceGateEnvelope` retorna un envelope válido con blocked state, assertar que no ejecuta el pack real

**Test 2: `TestNavRouteBlockedWhenGovernanceIsInvalid`** (si no existe):
- Mismo setup sin governance
- Ejecutar `nav.route`
- Assertar: blocked state en respuesta

Patrón de setup para workspace sin governance válida:
```go
root := t.TempDir()
ensureWritableTestHome(t)
// No llamar writeSpecBackendGovernanceFixture — workspace sin governance
writeWorkspaceFile(t, root, "src/App.csproj", `<Project/>`)
alias := "gov-blocked-" + t.Name()
workspace.RegisterWorkspace(alias, model.WorkspaceRegistration{...})
defer workspace.RemoveWorkspace(alias)
app := New(root, nil)
env, err := app.Execute(ctx, model.CommandRequest{
    Operation: "nav.pack",
    Context:   model.QueryOptions{Workspace: alias},
    Payload:   map[string]any{"task": "test"},
})
// Si hay governance gate, env.Ok debe ser false o el envelope debe indicar blocked
```

Nota: si el governance gate retorna `nil, nil` cuando no hay governance doc (en vez de bloquearlo), el test puede ser diferente. Leer `governance.go` para entender cuándo retorna un blocked envelope vs nil.

## Skeleton
```go
func TestNavPackBlockedWhenGovernanceIsInvalid(t *testing.T) {
    ensureWritableTestHome(t)
    root := t.TempDir()
    writeWorkspaceFile(t, root, "src/App.csproj", "<Project/>")
    alias := "gov-blocked-pack-" + filepath.Base(t.TempDir())
    if _, err := workspace.RegisterWorkspace(alias, model.WorkspaceRegistration{
        Name: alias, Root: root, Languages: []string{"csharp"},
        Kind: model.WorkspaceKindSingle,
    }); err != nil {
        t.Fatalf("register: %v", err)
    }
    defer func() { _ = workspace.RemoveWorkspace(alias) }()

    app := New(root, nil)
    env, err := app.Execute(context.Background(), model.CommandRequest{
        Operation: "nav.pack",
        Context:   model.QueryOptions{Workspace: alias},
        Payload:   map[string]any{"task": "test"},
    })
    // El gate puede retornar error O un blocked envelope
    if err != nil {
        return // gate bloqueó via error
    }
    // Si retorna envelope, debe indicar blocked
    if env.Ok {
        t.Fatalf("expected blocked response when governance is invalid, got ok=true")
    }
}
```

## Verify
`go test ./internal/service -run "TestNav.*Blocked" -v -count=1` → PASS

## Commit
`test(governance): add gate blocking tests for nav.pack and nav.route`
