# Comparación baseline vs final

- baseline: `mi-lsp version (devel) protocol=mi-lsp-v1.1`
- final: `mi-lsp version (devel) protocol=mi-lsp-v1.1`

| Tipo | Lenguaje | Precisión base→final | Recall base→final | hit@5 base→final | Falsos vacíos base→final | Errores base→final | ms mi-lsp base→final | Bytes compact base→final |
|---|---|---|---|---|---|---|---|---|
| definition | csharp | - → 100% | 0% → 100% | - → - | 4 → 0 | 4 → 0 | 13 → 16 | 625 → 308 |
| definition | go | - → 100% | 0% → 100% | - → - | 8 → 0 | 8 → 0 | 12 → 22 | 606 → 467 |
| definition | typescript | - → 100% | 0% → 100% | - → - | 4 → 0 | 4 → 0 | 65 → 70 | 704 → 334 |
| intent | csharp | - → - | - → - | 0% → 100% | 0 → 0 | 0 → 0 | 27 → 47 | 1272 → 2738 |
| intent | go | - → - | - → - | 0% → 50% | 0 → 0 | 0 → 0 | 22 → 31 | 846 → 1755 |
| intent | typescript | - → - | - → - | 0% → 100% | 0 → 0 | 0 → 0 | 73 → 346 | 617 → 1954 |
| literal | csharp | 100% → 100% | 17% → 17% | - → - | 0 → 0 | 0 → 0 | 31 → 31 | 1927 → 2294 |
| literal | go | 100% → 100% | 59% → 59% | - → - | 0 → 0 | 0 → 0 | 29 → 32 | 1637 → 1762 |
| literal | typescript | 100% → 100% | 36% → 36% | - → - | 0 → 0 | 0 → 0 | 81 → 86 | 1984 → 2111 |
| refs | csharp | - → 100% | 0% → 100% | - → - | 4 → 0 | 4 → 0 | 211 → 682 | 371 → 5734 |
| refs | go | 83% → 100% | 50% → 84% | - → - | 4 → 0 | 0 → 0 | 566 → 1380 | 999 → 2239 |
| refs | typescript | 78% → 100% | 100% → 100% | - → - | 0 → 0 | 0 → 0 | 125 → 8140 | 6857 → 5827 |

**Totales:** falsos vacíos 24 → 0, errores 20 → 0.

## Casos que dejaron de ser falso vacío (24)

`mi-lsp.def.LoadRegistry`, `mi-lsp.def.SaveRegistry`, `mi-lsp.def.GarbageCollectRegistry`, `mi-lsp.def.ResolveWorkspace`, `mi-lsp.refs.LoadRegistry`, `mi-lsp.refs.SaveRegistry`, `mi-lsp.refs.GarbageCollectRegistry`, `mi-lsp.refs.ResolveWorkspace`, `mi-gateway.def.NewTranscriptTracker`, `mi-gateway.def.NewHTTPAdapter`, `mi-gateway.def.NewAllowlist`, `mi-gateway.def.ExternalKey`, `pi-subagents.def.resolvePiLaunchToolPlan`, `pi-subagents.def.supervisorChannelDir`, `pi-subagents.def.ChildSessionEvent`, `pi-subagents.def.PiLaunchToolPlan`, `tedi-memory.def.IEmbeddingProvider`, `tedi-memory.def.WorkspaceStore`, `tedi-memory.def.ProjectionRebuilder`, `tedi-memory.def.JournalRepository`, `tedi-memory.refs.IEmbeddingProvider`, `tedi-memory.refs.WorkspaceStore`, `tedi-memory.refs.ProjectionRebuilder`, `tedi-memory.refs.JournalRepository`

## Regresiones: casos que pasaron a falso vacío (0)

-

