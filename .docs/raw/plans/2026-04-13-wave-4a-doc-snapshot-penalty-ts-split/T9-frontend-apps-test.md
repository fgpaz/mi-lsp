# Task T9: Test — frontend_apps in workspace-map output

## Shared Context
**Goal:** Write a test that verifies `nav workspace-map` output includes `frontend_apps` when the project has TypeScript repos.
**Stack:** Go test, `internal/service/workspace_map_test.go` (or add to existing test file)
**Architecture:** Test validates that the workspace-map JSON output includes `frontend_apps` array field with correct structure.

## Task Metadata
```yaml
id: T9
depends_on: [T7]
agent_type: ps-worker
files:
  - modify: internal/service/workspace_map_test.go (or create)
complexity: medium
done_when: "go test ./internal/service/... -run FrontendApps -count=1 exits 0"
```

## Reference
If `internal/service/workspace_map_test.go` does not exist, check `internal/service/app_test.go` for the testing pattern used in this package.

## Prompt
Check if `internal/service/workspace_map_test.go` exists. If not, check the pattern in `internal/service/app_test.go` to understand how tests are structured in this package.

Write a test function `TestWorkspaceMapFrontendApps` (or add to an existing test file) that:
1. Mocks or uses a real workspace that has a repo with `"typescript"` in Languages
2. Calls the `workspaceMap` function
3. Asserts that the result contains a non-nil `FrontendApps` array
4. If `FrontendApps` has entries, validates each has `Path`, `Name`, `Language == "typescript"`, `PageCount >= 0`

Note: If there is no existing workspace_map_test.go and the existing test infrastructure is complex, add a simpler unit test that tests the `discoverServices` function directly with a mock `model.ProjectFile` that has a repo with `Languages: ["typescript"]` and verify the returned `frontendApps` has the expected entry.

```go
func TestWorkspaceMapFrontendApps(t *testing.T) {
    // Test that a project with a TypeScript repo produces frontend_apps
    project := model.ProjectFile{
        Repos: []model.WorkspaceRepo{
            {
                ID:        "ts-repo",
                Name:      "my-next-app",
                Root:      "frontend/app",
                Languages: []string{"typescript"},
            },
        },
        Entrypoints: []model.WorkspaceEntrypoint{}, // no .csproj
    }
    
    // Call discoverServices (or workspaceMap if easier)
    services, frontendApps, _, _ := discoverServices(context.Background(), nil, registration, project)
    
    if len(frontendApps) == 0 {
        t.Fatal("expected at least one frontend app for TypeScript repo, got 0")
    }
    
    fa := frontendApps[0]
    if fa.Language != "typescript" {
        t.Errorf("Language = %q, want %q", fa.Language, "typescript")
    }
    if fa.Name != "my-next-app" {
        t.Errorf("Name = %q, want %q", fa.Name, "my-next-app")
    }
}
```

## Skeleton
```go
func TestWorkspaceMapFrontendApps(t *testing.T) {
    // Mock project with TypeScript repo
    project := model.ProjectFile{
        Repos: []model.WorkspaceRepo{
            {ID: "ts-repo", Name: "my-next-app", Root: "frontend", Languages: []string{"typescript"}},
        },
        Entrypoints: []model.WorkspaceEntrypoint{},
    }
    
    // Call discoverServices or workspaceMap
    // Assert frontendApps is not empty
    // Assert Language == "typescript"
}
```

## Verify
`go test ./internal/service/... -run FrontendApps -count=1` -> `PASS`

## Commit
`test(workspace-map): add TestWorkspaceMapFrontendApps`
