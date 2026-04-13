# Task T6: workspace_map — add frontend_apps struct and field

## Shared Context
**Goal:** Add `frontendAppEntry` struct and `FrontendApps []frontendAppEntry` field to `workspaceMapEntry`.
**Stack:** Go, `internal/service/workspace_map.go`
**Architecture:** New struct is independent from `serviceMapEntry` — separate semantic surface.

## Task Metadata
```yaml
id: T6
depends_on: [T5]
agent_type: ps-worker
files:
  - modify: internal/service/workspace_map.go:18-55
complexity: low
done_when: "go build ./... exits 0"
```

## Reference
`internal/service/workspace_map.go:18-55` — existing structs:
```go
type workspaceMapEntry struct {
    Mode         string              `json:"mode,omitempty"`
    NextSteps    []string            `json:"next_steps,omitempty"`
    Repos        []repoMapEntry      `json:"repos"`
    Services     []serviceMapEntry   `json:"services"`
    Dependencies []serviceDependency `json:"dependencies,omitempty"`
    Stats        workspaceMapStats   `json:"stats"`
}
```

## Prompt
Open `internal/service/workspace_map.go`.

1. Add a new struct after the `serviceDependency` struct (around line 53):
```go
type frontendAppEntry struct {
    Path       string `json:"path"`
    Name       string `json:"name"`
    Language   string `json:"language"`
    PageCount  int    `json:"page_count"`
}
```

2. Add `FrontendApps []frontendAppEntry \`json:"frontend_apps,omitempty"\`` to `workspaceMapEntry` struct:
```go
type workspaceMapEntry struct {
    Mode         string              `json:"mode,omitempty"`
    NextSteps    []string            `json:"next_steps,omitempty"`
    Repos        []repoMapEntry      `json:"repos"`
    Services     []serviceMapEntry   `json:"services"`
    FrontendApps []frontendAppEntry  `json:"frontend_apps,omitempty"`
    Dependencies []serviceDependency `json:"dependencies,omitempty"`
    Stats        workspaceMapStats   `json:"stats"`
}
```

## Skeleton
```go
type frontendAppEntry struct {
    Path      string `json:"path"`
    Name      string `json:"name"`
    Language  string `json:"language"`
    PageCount int    `json:"page_count"`
}

// workspaceMapEntry gets new field:
// FrontendApps []frontendAppEntry `json:"frontend_apps,omitempty"`
```

## Verify
`go build ./...` -> `Build succeeded`

## Commit
`feat(workspace-map): add frontendAppEntry struct and FrontendApps field`
