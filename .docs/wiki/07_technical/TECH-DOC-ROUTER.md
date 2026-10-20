---
wiki_contract: WIKI-CONTRACT-v1
id: TECH-DOC-ROUTER
kind: technical
status: active
title: TECH-DOC-ROUTER
links:
  supports:
  - RF-QRY-014
  - RF-QRY-015
  code:
  - path: internal/cli/axi_mode.go
    role: implementation
  - path: internal/cli/nav.go
    role: implementation
  - path: internal/docgraph/route.go
    role: implementation
  - path: internal/model/types.go
    role: implementation
  - path: internal/service/app.go
    role: implementation
  - path: internal/service/ask.go
    role: implementation
  - path: internal/service/pack.go
    role: implementation
  - path: internal/service/route.go
    role: implementation
---

## Resumen

Describe el diseño técnico del motor de routing de documentos de bajo token para `mi-lsp`. Este router alimenta `nav route`, `nav ask` y `nav pack` desde una única fuente de verdad canonica.

### Propósito

Describe el diseño técnico del motor de routing de documentos de bajo token para `mi-lsp`. Este router alimenta `nav route`, `nav ask` y `nav pack` desde una única fuente de verdad canonica.

## Mapa de componentes

Los componentes, paths exactos, responsabilidades y límites se mantienen en el detalle técnico siguiente.

### Arquitectura de dos tiers

#### Tier 1 - Resolución canónica sin índice

Tier 1 puede producir `anchor_doc + mini_pack_preview` sin requerir un índice de docs completo.

**Fuentes de Tier 1** (en orden de prioridad):
1. `governance/read-model.toml`: perfil efectivo, familia por defecto, jerarquia de capas
2. Doc ID explícito en la tarea (`RF-*`, `FL-*`, etc.) resuelto contra docs canónicos del filesystem
3. Filesystem de docs raiz: verifica que los docs existan fisicamente antes de anclar
4. Doc de gobernanza (`00_governance.md`) como fallback seguro
5. Solo como ultimo recurso si no hay wiki: `README.md`

**Flujo**:
```
question/task -> MatchFamily(profile) -> containingDocForExplicitID(root, profile, doc_id?)
                                       -> canonicalAnchorForFamily(family, profile, root)
                                       -> buildTier1PreviewPack(family, profile, root)
                                       -> RouteCanonicalLane{authoritative: true}
```

#### Tier 2 - Enriquecimiento con índice

Tier 2 enriquece la canonical lane con el índice de docs indexado.

**Fuentes de Tier 2**:
1. FTS5 BM25 sobre `doc_records`
2. `rankDocs(question, family, docs, ftsScores, profile, recentChanges)` owner-aware
3. `doc_edges` para traversal de evidencia

**Flujo**:
```
profile -> ListDocRecords -> FTSSearchDocs -> rankDocs -> enrich RouteCanonicalLane
                          -> buildDiscoveryAdvisory (advisory-only, docs-only by default)
```

### Separacion de lanes

- **Lane**: canonical; **Autoridad**: Autoritativa; **Fuente**: governance + index; **Puede sobreescribir la otra**: No aplica
- **Lane**: discovery; **Autoridad**: Advisory-only; **Fuente**: índice de docs / text search; **Puede sobreescribir la otra**: Nunca

**Regla de peso**: la canonical lane tiene al menos el doble del peso/prioridad de la discovery lane en cualquier ranking combinado.

### Fail-closed semantics

- Si Tier 1 no puede confiar en el routing canónico, devuelve `authoritative=false` con discovery advisory
- Nunca cae silenciosamente a `README.md` cuando governance y docs wiki existen
- Si governance esta bloqueada, toda la operación se bloquea antes de hacer routing

### Tipos canónicos

```go
type RouteDoc struct {
    Path   string
    Title  string
    DocID  string
    Layer  string
    Family string
    Stage  string
    Why    string
}

type RouteCanonicalLane struct {
    AnchorDoc     RouteDoc
    PreviewPack   []RouteDoc  // max 2-3 docs
    Family        string
    Authoritative bool
}

type RouteDiscoveryLane struct {
    Source   string     // "indexed_docs" | "text_search"
    Docs     []RouteDoc // advisory summary
    Advisory string
}

type RouteResult struct {
    Task      string
    Mode      string              // "preview" | "full"
    Canonical RouteCanonicalLane
    Discovery *RouteDiscoveryLane // omitempty, advisory only
    Why       []string
}
```

### Integracion con ask y pack

`nav ask` llama al route core para obtener `anchor_doc`, luego agrega `CodeEvidence` sobre el resultado.
`nav pack` llama al route core para obtener el `primary_doc`, luego construye el reading pack completo.
`nav route` llama al route core directamente y devuelve el `RouteResult` al usuario.
`nav.intent` reutiliza la misma logica documental cuando clasifica la query como `mode=docs`.

### Superficies AXI

`nav route` es AXI-default preview-first. `--full` expande canonical lane y discovery. Code discovery solo con `--full` o `--include-code-discovery`.

### Archivos de implementacion

- `internal/model/types.go`: tipos `RouteResult`, `RouteCanonicalLane`, `RouteDiscoveryLane`, `RouteDoc`
- `internal/docgraph/route.go`: `Tier1CanonicalRoute`, `canonicalAnchorForFamily`, helpers de filesystem
- `internal/service/route.go`: `resolveCanonicalRoute`, `route()` handler, `buildDiscoveryAdvisory`
- `internal/service/ask.go`: fallback Tier 1 cuando el índice esta vacio
- `internal/service/pack.go`: fallback Tier 1 cuando el índice esta vacio o stale
- `internal/service/app.go`: dispatch `nav.route`
- `internal/cli/nav.go`: comando `nav route` (`routeCommand`)
- `internal/cli/axi_mode.go`: `nav.route` en `supportsAXISurface` y `defaultAXIForOperation`; `buildWorkspaceAXINextSteps` incluye `nav route` antes de `nav ask`

### Notas de implementacion

- El router esta implementado y en produccion desde Wave 2.
- El scorer owner-aware queda `default-on`; `MI_LSP_DOC_RANKING=legacy` existe solo como override temporal de diagnostico.
- `Tier1CanonicalRoute` en `internal/docgraph/route.go` omite entradas de tipo glob pattern en el read-model; solo verifica paths concretos contra el filesystem antes de anclar.
- Cuando la tarea trae un RF concreto y el archivo canónico es agregado bajo `05_requirements/`, `Tier1CanonicalRoute` devuelve ese path como `anchor_doc` y conserva el `doc_id` pedido.
- `shouldUseDaemon` en el router de dispatch excluye `nav.route` — es liviano y no requiere daemon caliente.
- El fallback a `README.md` esta bloqueado cuando governance y wiki existen; solo aplica en repos sin `.docs/wiki`.

### RF asociados

- RF-QRY-014: comando publico `nav route`
- RF-QRY-015: reutilización interna desde ask/pack

### CT asociado

CT-NAV-ROUTE
