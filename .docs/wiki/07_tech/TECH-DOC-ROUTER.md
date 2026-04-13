# TECH-DOC-ROUTER

## Proposito

Describe el diseno tecnico del motor de routing de documentos de bajo token para `mi-lsp`. Este router alimenta `nav route`, `nav ask` y `nav pack` desde una unica fuente de verdad canonica.

## Arquitectura de dos tiers

### Tier 1 - Resolucion canonica sin indice

Tier 1 puede producir `anchor_doc + mini_pack_preview` sin requerir un indice de docs completo.

**Fuentes de Tier 1** (en orden de prioridad):
1. `governance/read-model.toml`: perfil efectivo, familia por defecto, jerarquia de capas
2. Filesystem de docs raiz: verifica que los docs existan fisicamente antes de anclar
3. Doc de gobernanza (`00_gobierno_documental.md`) como fallback seguro
4. Solo como ultimo recurso si no hay wiki: `README.md`

**Flujo**:
```
question/task -> MatchFamily(profile) -> canonicalAnchorForFamily(family, profile, root)
                                       -> buildTier1PreviewPack(family, profile, root)
                                       -> RouteCanonicalLane{authoritative: true}
```

### Tier 2 - Enriquecimiento con indice

Tier 2 enriquece la canonical lane con el indice de docs indexado.

**Fuentes de Tier 2**:
1. FTS5 BM25 sobre `doc_records`
2. `rankDocs(question, family, docs, ftsScores)`
3. `doc_edges` para traversal de evidencia

**Flujo**:
```
profile -> ListDocRecords -> FTSSearchDocs -> rankDocs -> enrich RouteCanonicalLane
                          -> buildDiscoveryAdvisory (advisory-only, docs-only by default)
```

## Separacion de lanes

| Lane | Autoridad | Fuente | Puede sobreescribir la otra |
|---|---|---|---|
| canonical | Autoritativa | governance + index | No aplica |
| discovery | Advisory-only | indice de docs / text search | Nunca |

**Regla de peso**: la canonical lane tiene al menos el doble del peso/prioridad de la discovery lane en cualquier ranking combinado.

## Fail-closed semantics

- Si Tier 1 no puede confiar en el routing canonico, devuelve `authoritative=false` con discovery advisory
- Nunca cae silenciosamente a `README.md` cuando governance y docs wiki existen
- Si governance esta bloqueada, toda la operacion se bloquea antes de hacer routing

## Tipos canonicos

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

## Integracion con ask y pack

`nav ask` llama al route core para obtener `anchor_doc`, luego agrega `CodeEvidence` sobre el resultado.
`nav pack` llama al route core para obtener el `primary_doc`, luego construye el reading pack completo.
`nav route` llama al route core directamente y devuelve el `RouteResult` al usuario.

## Superficies AXI

`nav route` es AXI-default preview-first. `--full` expande canonical lane y discovery. Code discovery solo con `--full` o `--include-code-discovery`.

## Archivos de implementacion

- `internal/model/types.go`: tipos `RouteResult`, `RouteCanonicalLane`, `RouteDiscoveryLane`, `RouteDoc`
- `internal/docgraph/route.go`: `Tier1CanonicalRoute`, `canonicalAnchorForFamily`, helpers de filesystem
- `internal/service/route.go`: `resolveCanonicalRoute`, `route()` handler, `buildDiscoveryAdvisory`
- `internal/service/ask.go`: fallback Tier 1 cuando el indice esta vacio
- `internal/service/pack.go`: fallback Tier 1 cuando el indice esta vacio o stale
- `internal/service/app.go`: dispatch `nav.route`
- `internal/cli/nav.go`: comando `nav route` (`routeCommand`)
- `internal/cli/axi_mode.go`: `nav.route` en `supportsAXISurface` y `defaultAXIForOperation`; `buildWorkspaceAXINextSteps` incluye `nav route` antes de `nav ask`

## Notas de implementacion

- El router esta implementado y en produccion desde Wave 2.
- `Tier1CanonicalRoute` en `internal/docgraph/route.go` omite entradas de tipo glob pattern en el read-model; solo verifica paths concretos contra el filesystem antes de anclar.
- `shouldUseDaemon` en el router de dispatch excluye `nav.route` — es liviano y no requiere daemon caliente.
- El fallback a `README.md` esta bloqueado cuando governance y wiki existen; solo aplica en repos sin `.docs/wiki`.

## RF asociados

- RF-QRY-014: comando publico `nav route`
- RF-QRY-015: reutilizacion interna desde ask/pack

## CT asociado

CT-NAV-ROUTE
