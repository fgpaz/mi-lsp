# Resultado parcial — mi-lsp wiki v1.1 / C229

**Estado:** implementación de servicio/modelo integrada localmente; Working sigue abierto. No se tocaron los seams de G (`internal/cli/nav.go`, `internal/cli/provider_manifest.go`) ni el archivo final compartido de Root39.

## Trabajo

- Añadida la proyección `wiki-graph/v1` sobre la generación de grafo existente: vecinos, rutas, explicación, impacto, change-pack, matriz RF×TP, unresolved, roadmap opcional y Mermaid derivado por respuesta.
- Los detalles v1.1 de `validate-contract` se agregan a la respuesta existente sin reemplazar su resultado/counters legacy. Hallazgos de secciones/criterios siguen no bloqueantes; las ausencias nuevas de diagramas son advisories no bloqueantes.
- Se clasifican RF/TP por rutas legacy existentes; la matriz deriva sus pares de aristas `verifies` y resuelve el idioma desde governance.
- La consulta `mi-decidir consultar` falló tipada: `forbidden` / `authenticated execution unavailable; provider not invoked`. Seguí según el pedido y la ontología aplicable: `persona:wiki-links-en-grafo-unico-milsp-20261019`, `persona:wiki-diagramas-c4-secuencia-y-derivados-20261019`, `persona:wiki-idioma-en-governance-20261019`, `persona:roadmap-afuera-con-capa-fina-20261019`.

## Verificación realizada

- `go build ./...`: pasó.
- `git diff --check`: pasó.
- Ejecución real de servicio sobre el workspace registrado `mi-lsp`: `validate-contract` devolvió 134 checks, los 146 warnings legacy, 9 advisories de diagramas y 0 hallazgos nuevos bloqueantes; `wiki-graph neighbors` devolvió 4 nodos/5 aristas, freshness `current` y 5 unresolved; `matrix` devolvió 66 filas y freshness `current`.
- La generación activa consultada pertenece al workspace publicado y solo registra relaciones `doc_*`; no se reindexó esta rama. Por eso la ejecución no demuestra todavía las nuevas aristas tipadas `verifies`/`code` de esta implementación.
- No se ejecutaron unit tests ni gates adicionales. No hay commit de esta integración, publicación en `main`, tag/release, instalación Tesla ni readback.

## Paths tocados

- `.docs/planificacion/2026-10-19-mi-lsp-wiki-v1-1-design.md`
- `internal/docgraph/docgraph.go`
- `internal/indexer/graph_staging.go`
- `internal/model/graph.go`
- `internal/model/graph_impact.go`
- `internal/model/types.go`
- `internal/model/wiki_graph.go`
- `internal/service/app.go`
- `internal/service/contract_validate.go`
- `internal/service/wiki_graph.go`
- `internal/wikisource/parser.go`
- `internal/wikisource/wiki_contract.go`
- `.orchestration/results/milsp-wiki-v1-1-implementation-20261019.md` (esta nota)

BLOQUEADO: G integration sigue pendiente y el pedido prohíbe tocar sus seams; falta la integración CLI/manifiesto antes de publicar/taggear/instalar y leer la versión.
REPORTE-FINAL: /home/tesla/repos/mios-wt/mimcp39-milsp-wiki-v11-design-20261019/.orchestration/results/milsp-wiki-v1-1-implementation-20261019.md
TRABADO
