# Task T1: Añadir governanceGateEnvelope a operaciones faltantes

## Shared Context
**Goal:** RF-WKS-005 requiere que todas las operaciones spec-driven usen el governance gate.
**Stack:** Go, `internal/service/*.go`
**Architecture:** Si T0 encontró gaps, este task los cierra. Si T0 reportó "ningún gap", este task es no-op. Patrón a seguir: `internal/service/route.go:160-165`.

## Task Metadata
```yaml
id: T1
depends_on: [T0]
agent_type: ps-worker
files:
  - modify: internal/service/search.go     # solo si T0 lo marcó como gap
  - modify: internal/service/intent.go     # solo si T0 lo marcó como gap
  - modify: internal/service/workspace_map.go  # solo si T0 lo marcó como gap
complexity: low
done_when: "go build ./... EXIT:0 — o 'no-op si T0 no encontró gaps'"
```

## Reference
`internal/service/route.go:160-165` — patrón exacto para agregar gate:
```go
func (a *App) route(ctx context.Context, request model.CommandRequest) (model.Envelope, error) {
    if blockedEnv, err := a.governanceGateEnvelope(ctx, request, "nav.route"); err != nil {
        return model.Envelope{}, err
    } else if blockedEnv != nil {
        return *blockedEnv, nil
    }
    // ... resto del handler
```

## Prompt
Lee el reporte de T0. Si no hay gaps, este task es no-op — commitea solo un no-op comment.

Si hay gaps, para cada operación faltante:
1. Abrir el archivo del handler
2. Encontrar la función que maneja esa operación
3. Agregar el gate al inicio de la función siguiendo EXACTAMENTE el patrón de `route.go:160-165`
4. Reemplazar `"nav.route"` por el nombre de la operación correspondiente (ej. `"nav.search"`)

El gate retorna un `*model.Envelope` cuando governance está blocked — esto significa que la operación devuelve un envelope válido con `ok=false` y el estado de governance, en vez de ejecutar la operación real.

**Operaciones que NUNCA deben tener gate** (no modificar):
- `workspace.init`, `workspace.status`, `workspace.list`, `workspace.add`, `workspace.remove`
- `nav.governance`
- `root.home`, `workspace.init`

## Skeleton
```go
// Al inicio de cada handler con gap:
if blockedEnv, err := a.governanceGateEnvelope(ctx, request, "nav.OPERATION"); err != nil {
    return model.Envelope{}, err
} else if blockedEnv != nil {
    return *blockedEnv, nil
}
```

## Verify
`go build ./...` → EXIT:0

## Commit
`feat(governance): apply governance gate to remaining nav operations`
(o `chore: no-op, T0 confirmed full gate coverage`)
