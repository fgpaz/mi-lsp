# Task T7: discoverServices — add TypeScript repo loop for frontend_apps

## Shared Context
**Goal:** After the existing entrypoint loop in `discoverServices`, add a loop over `project.Repos` that detects repos with `"typescript"` in `Languages` and generates `frontendAppEntry` entries.
**Stack:** Go, `internal/service/workspace_map.go`
**Architecture:** TS repos are detected via `WorkspaceRepo.Languages` already populated by topology.go. No re-scan needed. No entrypoints needed.

## Task Metadata
```yaml
id: T7
depends_on: [T6]
agent_type: ps-worker
files:
  - modify: internal/service/workspace_map.go:155-280
complexity: medium
done_when: "go build ./... exits 0"
```

## Reference
`internal/service/workspace_map.go:245-280` — end of discoverServices with return:
```go
    return services, serviceEvents, warnings
}

func detectDependencies(services []serviceMapEntry, serviceEvents []serviceEventData) []serviceDependency {
```
The function returns `services, serviceEvents, warnings`. We need to also return `frontendApps`.

## Prompt
Open `internal/service/workspace_map.go`. 

1. Change the signature of `discoverServices` from:
```go
func discoverServices(ctx context.Context, db *sql.DB, registration model.WorkspaceRegistration, project model.ProjectFile) ([]serviceMapEntry, []serviceEventData, []string)
```
to:
```go
func discoverServices(ctx context.Context, db *sql.DB, registration model.WorkspaceRegistration, project model.ProjectFile) ([]serviceMapEntry, []frontendAppEntry, []serviceEventData, []string)
```

2. Add a `frontendApps := make([]frontendAppEntry, 0)` variable after the `services` declaration.

3. At the end of `discoverServices`, after the entrypoint loop but before the return, add:
```go
// Detect TypeScript/Next.js repos that have no entrypoints (not .csproj-based)
for _, repo := range project.Repos {
    hasTS := false
    for _, lang := range repo.Languages {
        if lang == "typescript" {
            hasTS = true
            break
        }
    }
    if !hasTS {
        continue
    }
    
    // Count pages/ app directories for page_count
    pageCount := 0
    absRoot := registration.Root
    if repo.Root != "" {
        absRoot = filepath.Join(absRoot, filepath.FromSlash(repo.Root))
    }
    pagesDir := filepath.Join(absRoot, "pages")
    appDir := filepath.Join(absRoot, "app")
    if info, err := os.Stat(pagesDir); err == nil && info.IsDir() {
        entries, _ := os.ReadDir(pagesDir)
        for _, e := range entries {
            if !e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
                pageCount++
            } else if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
                pageCount++
            }
        }
    }
    if info, err := os.Stat(appDir); err == nil && info.IsDir() {
        entries, _ := os.ReadDir(appDir)
        for _, e := range entries {
            if !e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
                pageCount++
            } else if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
                pageCount++
            }
        }
    }
    
    name := repo.Name
    if name == "" {
        name = filepath.Base(repo.Root)
    }
    
    frontendApps = append(frontendApps, frontendAppEntry{
        Path:      repo.Root,
        Name:      name,
        Language:  "typescript",
        PageCount: pageCount,
    })
}
```

4. Update the return statement from:
```go
return services, serviceEvents, warnings
```
to:
```go
return services, frontendApps, serviceEvents, warnings
```

5. In `workspaceMap` function (caller of `discoverServices`), update the call site:
```go
services, frontendApps, serviceEvents, svcWarnings := discoverServices(ctx, db, registration, project)
```
and populate `mapEntry.FrontendApps = frontendApps`.

6. Add `os` to the imports if not already present.

## Skeleton
```go
// Signature change:
func discoverServices(...) ([]serviceMapEntry, []frontendAppEntry, []serviceEventData, []string)

// New variable:
frontendApps := make([]frontendAppEntry, 0)

// New loop over project.Repos (before return)
for _, repo := range project.Repos {
    // check Languages for "typescript"
    // count pages/ and app/ entries
    // append frontendAppEntry
}

// Return updated:
return services, frontendApps, serviceEvents, warnings
```

## Verify
`go build ./...` -> `Build succeeded`

## Commit
`feat(workspace-map): add TypeScript repo detection and frontend_apps population`
