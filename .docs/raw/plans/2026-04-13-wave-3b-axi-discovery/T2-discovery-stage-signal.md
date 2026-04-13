# Task T2: Stage signal en discovery lane (anchor|preview|discovery)

## Shared Context
**Goal:** Asegurar que `RouteDoc.Stage` se puebla correctamente en anchor, preview pack y discovery lane.
**Stack:** Go, `internal/service/route.go`, `internal/docgraph/route.go`
**Architecture:** `RouteDoc.Stage` ya existe en el modelo. `buildDiscoveryAdvisory` en `route.go` no lo puebla. `buildTier1PreviewPack` en `docgraph/route.go` lo puebla desde el `stage` del `canonicalPathsForStage`. Solo falta: anchor debe tener `stage="anchor"`, y `buildDiscoveryAdvisory` debe poner `stage="discovery"`.

## Task Metadata
```yaml
id: T2
depends_on: [T0]
agent_type: ps-worker
files:
  - modify: internal/service/route.go:20-100   # resolveCanonicalRoute y buildDiscoveryAdvisory
  - modify: internal/docgraph/route.go:15-40   # Tier1CanonicalRoute — stage del anchor
complexity: low
done_when: "go build ./... EXIT:0"
```

## Reference
`internal/docgraph/route.go:15-40` — `Tier1CanonicalRoute` donde se construye `anchorDoc`.
`internal/service/route.go:56-95` — enriquecimiento Tier 2 y `buildDiscoveryAdvisory`.
`internal/service/route.go:100-122` — `buildDiscoveryAdvisory`.

## Prompt
Dos cambios pequeños para poblar `Stage` correctamente.

**Cambio 1 — Anchor stage en Tier1CanonicalRoute (`docgraph/route.go:20-25`)**

En `Tier1CanonicalRoute`, cuando se construye `anchorDoc`, agregar `Stage: "anchor"`:
```go
anchorDoc := model.RouteDoc{
    Path:   anchorPath,
    Family: family,
    Why:    "canonical_anchor",
    Stage:  "anchor",   // ← agregar esta línea
}
```

**Cambio 2 — Discovery stage en buildDiscoveryAdvisory (`route.go:100-122`)**

En `buildDiscoveryAdvisory`, cuando se construye cada `RouteDoc`, agregar `Stage: "discovery"`:
```go
docs = append(docs, model.RouteDoc{
    Path:   candidate.record.Path,
    Title:  candidate.record.Title,
    DocID:  candidate.record.DocID,
    Layer:  candidate.record.Layer,
    Family: candidate.record.Family,
    Why:    strings.Join(candidate.reason, ","),
    Stage:  "discovery",   // ← agregar esta línea
})
```

**Cambio 3 — Preview pack stage en resolveCanonicalRoute Tier 2 (`route.go:69-88`)**

Cuando se construye el `previewPack` en el Tier 2 (líneas ~69-88), asegurarse de que cada doc tenga `Stage: "preview"`:
```go
preview = append(preview, model.RouteDoc{
    Path:   candidate.record.Path,
    Title:  candidate.record.Title,
    DocID:  candidate.record.DocID,
    Layer:  candidate.record.Layer,
    Family: candidate.record.Family,
    Why:    strings.Join(candidate.reason, ","),
    Stage:  "preview",   // ← asegurar que está presente
})
```

**NO hacer:**
- No cambiar `buildTier1PreviewPack` — ya puebla `Stage` desde `canonicalPathsForStage`
- No cambiar el modelo `RouteDoc`

## Skeleton
```go
// docgraph/route.go — anchorDoc
anchorDoc := model.RouteDoc{
    Path:  anchorPath,
    Stage: "anchor",
    // ...
}

// route.go — buildDiscoveryAdvisory
docs = append(docs, model.RouteDoc{
    Stage: "discovery",
    // ...
})

// route.go — Tier 2 preview pack
preview = append(preview, model.RouteDoc{
    Stage: "preview",
    // ...
})
```

## Verify
`go build ./...` → EXIT:0

## Commit
`feat(route): populate Stage field in anchor, preview, and discovery docs`
