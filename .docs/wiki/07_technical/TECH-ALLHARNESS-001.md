---
wiki_contract: WIKI-CONTRACT-v1
id: TECH-ALLHARNESS-001
kind: technical
status: draft
source_type: operator_decision
links:
  supports: [RF-ALLHARNESS-001]
---

# TECH-ALLHARNESS-001 — mi-lsp para todos los harness

## Resumen
Soporta [[RF-ALLHARNESS-001]]. Fuente: pedido explícito de Gabriel `home/ideas/mi-lsp-para-todos-los-harness.md`, 2026-10-20; persona:herramientas-pocas-enganchables-20261020. Tipo de fuente: decisión del operador; no evidencia runtime.

- Fuente: `internal/cli/provider_manifest.go`; snapshot distribuido: `integrations/mi-mcp/provider-manifest.json`. Regenerar snapshot con binario de la misma revisión: `mi-lsp provider-manifest > integrations/mi-mcp/provider-manifest.json`.
- `mi-lsp models` proyecta solamente operaciones con model_alias, sin otro router ni otra taxonomía pública. Presupuestos de navegación: 20 items / 2000 tokens aproximados. Find añade su presupuesto JSON propio.
- Gateway: C165/H3 es el único dueño de preservar model_alias y proyectar nombre JS con operación target, sin cambiar permisos ni argv/schema. El provider declara la metadata; su ejecución debe acreditarse contra un pin H3 publicado con soporte del campo, no CLI129. No hay implementación de loader de esta lane.
- Consumidor tras-edit: jefe-harness-web integra y acredita el adaptador automático. La skill solicita diagnostics, pero una llamada manual no demuestra enganche automático. H1Yoga binding39 de C165 es una frontera distinta del smoke de este objetivo.
- Skill canónica: `skills/mi-lsp/SKILL.md`. Distribución y configuración: `skills/mi-lsp/references/all-harnesses.md`.
- Implementaciones diagnostics/find entregadas por sus dueños; registrar constructors solo al integrar sus archivos. No asumir backends disponibles por instalar plugin.

## Mapa de componentes
- `internal/cli/provider_manifest.go` y `integrations/mi-mcp/provider-manifest.json`: manifiesto y snapshot distribuido.
- `mi-lsp models`, C165/H3, el consumidor tras-edit y `skills/mi-lsp/SKILL.md`: proyección, integración y distribución de la skill.
- Las implementaciones diagnostics/find pertenecen a sus dueños y se integran por sus archivos.

[[CT-ALLHARNESS-001]] · [[TP-ALLHARNESS-001]]

## Revisión de nav por uso declarado
No hay telemetría medida en esta lane. Evidencia de uso actual: antigua skill mi-lsp del repo recomienda intent, multi-read, search, refs/context y advanced continuations; provider preexistente publica intent/multi-read/q/wiki_graph/wiki_validate_contract. Esta revisión agrupa para lectura, no declara comandos sin uso runtime.

| Necesidad | Default / tratamiento |
|---|---|
| Orientación: intent, ask, route, pack, recall, wiki-root, overview, workspace-map | intent; el resto por help/continuación |
| Evidencia: search, find (símbolos), symbols, outline, context, multi-read, batch | search/read; fs.find busca nombres, no símbolos |
| Relaciones: refs, related, deps, callers, callees, neighbors, path, explain | refs; grafo por continuación exacta |
| Cambio: affected, diff-context, flow-slice, change-pack, explain-change, prepare, edit-plan | intent orienta; edit-plan conserva protección, no alias de lectura |
| Documentación/control: wiki, trace, evidence, governance, service, git-frontier, suggest | avanzado bajo help; wiki canónica sigue siendo autoridad |

Los subcomandos graph/wiki se conservan completos. Estas filas no agregan wrappers, gates ni flags de taxonomía.
