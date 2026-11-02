---
wiki_contract: WIKI-CONTRACT-v1
id: TECH-WIKI-AWARE-SEARCH
kind: technical
status: active
title: TECH-WIKI-AWARE-SEARCH
links:
  code:
  - path: internal/docidentity/identity.go
    role: implementation
  - path: internal/indexer/indexer.go
    role: implementation
  - path: internal/service/wiki_search.go
    role: implementation
  - path: internal/store/doc_snapshot.go
    role: implementation
  - path: internal/store/index_publish.go
    role: implementation
  - path: internal/store/meta.go
    role: implementation
  - path: internal/store/queries_incremental.go
    role: implementation
---

```yaml
harness_protocol: SDD-HARNESS-v1
id: TECH-WIKI-AWARE-SEARCH
kind: technical-architecture
audience: dual
imports: [.docs/wiki/07_technical/TECH-DOC-ROUTER.md, .docs/wiki/07_technical/contracts/CT-NAV-WIKI.md]
exports: [TECH-WIKI-AWARE-SEARCH]
agent_must_read: [.docs/wiki/07_technical/TECH-WIKI-AWARE-SEARCH.md, internal/service/wiki_search.go, internal/indexer/indexer.go]
agent_may_edit: [.docs/wiki/07_technical/TECH-WIKI-AWARE-SEARCH.md]
agent_must_not_edit: [internal/store/, .docs/wiki/_mi-lsp/read-model.toml, .mi-lsp/index.db]
verify: ["Leer .docs/wiki/07_technical/TECH-WIKI-AWARE-SEARCH.md y .docs/wiki/07_technical/TECH-DOC-ROUTER.md; cotejar las afirmaciones con la fuente publicada internal/service/wiki_search.go y sus límites. Registrar únicamente comandos y resultados efectivamente ejecutados; esta especificación no afirma una ejecución."]
stop_if: [La propuesta trate índices, raw/auditoría, snippets o materiales no canónicos como fuente primaria, o sugiera indexación durante BUILD.]
evidence: [.docs/wiki/07_technical/TECH-WIKI-AWARE-SEARCH.md, .docs/wiki/07_technical/TECH-DOC-ROUTER.md, internal/service/wiki_search.go]
```

## Resumen

Describir el pipeline técnico de `nav wiki`, `nav ask`, `nav pack` y del índice documental repo-local.
Esta capa existe para que la respuesta docs-first y los reading packs canónicos sean reproducibles, explainable/traceable y baratos en tiempo de ejecucion.

### Propósito

Describir el pipeline técnico de `nav wiki`, `nav ask`, `nav pack` y del índice documental repo-local.
Esta capa existe para que la respuesta docs-first y los reading packs canónicos sean reproducibles, explainable/traceable y baratos en tiempo de ejecucion.

## Mapa de componentes

Los componentes, paths exactos, responsabilidades y límites se mantienen en el detalle técnico siguiente.

### Pipeline

1. `workspace init` o `index` construye `doc_records`, `doc_edges`, `doc_mentions`, `doc_source_blocks` y `doc_source_records`; `index --docs-only` reconstruye solo esas tablas y la memoria de reentrada. Las publicaciones respaldadas por el extractor vigente estampan `workspace_meta.doc_identity_snapshot_version` dentro de la misma transacción documental; escritores documentales no versionados lo invalidan.
2. `docgraph.LoadProfile()` carga `.docs/wiki/_mi-lsp/read-model.toml` si existe; si no, usa el perfil embebido.
3. `nav wiki search` rankea `doc_records` y filtra por capas documentales explícitas (`RS`, `RF`, `FL`, `TP`, `CT`, `TECH`, `DB`).
4. `nav ask`, `nav route` y `nav pack` normalizan preguntas de inventario de anclas para quitar meta-terminos SDD (`RS`, `RF`, `FL`, `CT`, `TECH`, `DB`, `TP`) del ranking cuando esos tokens expresan formato/capa y no dominio funcional.
5. `nav ask` clasifica la pregunta por familia (`functional`, `technical`, `ux` o fallback generico).
6. El ranker pondera familia, capa (`01-09`/`10-16`), `doc_id` explícito y tokens de pregunta.
7. El documento primario se completa con supporting docs via `doc_edges` antes de volver al ranking textual.
8. La evidencia de código se deriva desde `doc_mentions` (`file_path`, `symbol`) y solo después usa fallback textual.
9. `nav pack` reutiliza familia, capas, `doc_edges` y el bloque `reading_pack` del perfil para construir un reading pack ordenado con stages y slices on-demand.
10. `nav wiki validate-source` abre los Markdown que declaran `wiki_source_protocol: SDD-WIKI-SOURCE-v1`, valida `doc_id`, fences `toon`, `block_id`, records referenciables y excepciones de tablas, y compara contra las filas typed publicadas.

### Reglas clave

- Docs primero, código despues.
- Links y doc IDs explícitos tienen prioridad sobre similitud textual.
- El `read-model` solo gobierna selección y orden; no persiste dentro de SQLite.
- Si no hay docs indexados pero existe wiki canónica, `nav ask` y `nav pack` usan el fallback Tier 1 del route core (anchor canónico desde governance/read-model), no README.md.
- Si la tarea incluye un `RF-*` explícito que vive dentro de un documento agregado, Tier 1 debe anclar el documento contenedor en `.docs/wiki/05_requirements/` aunque el índice documental este vacio.
- Si no hay docs indexados y no existe wiki canónica, `nav ask` degrada a search textual del workspace.
- Si existen docs pero el match es debil, la respuesta igual debe ser explainable con `why` y `next_queries`.
- Si la pregunta de `nav ask` necesito normalizar meta-terminos de anclas, el resultado puede emitir `anchor_drift` como coach/why preventivo y orientar la continuacion a `nav pack` para que el agente confirme el pack canonico.
- `nav wiki search` debe cortar por governance bloqueada y por docgraph vacio antes de ofrecer candidatos.
- `nav wiki search` debe resolver coincidencias exactas de `doc_id`, `block_id` y `record_id` fuente antes de aplicar ranking textual.
- `nav wiki search|route|pack|trace` debe adjuntar `lookup_status` para que un agente distinga identidad canónica indexada, alias/read-model, fallback por menciones/contenido, preview truncada, índice vacio/stale, filtros, ambigüedad y ausencia real.
- Los documentos no migrados a `SDD-WIKI-SOURCE-v1` no bloquean `validate-source`; solo bloquean los artefactos que declaran el protocolo y fallan su shape.
- `nav ask|route|pack --repo docs` es compatibilidad para agentes: se acepta, se ignora como filtro documental y se guia a `nav wiki`.
- `.docs/raw/**`, `.docs/auditoria/**`, matrices, índices y cualquier `.docs/**` no deben emitirse como `code_evidence`; siguen participando como documentos secundarios o soporte documental cuando corresponda.

### nav wiki como primer paso recomendado

Cuando una tarea es claramente documental, el primer paso recomendado es `nav wiki search`:

```
nav wiki search "workflow masterformularios" --workspace idp --layer RF,FL,CT,TP --format toon
nav wiki pack "workflow con masterformularios" --workspace idp --format toon
```

El resultado de `wiki search` debe traer `next_queries` hacia `wiki pack`, `wiki trace`, `multi-read` o `ask`, de modo que el agente no invente la escalera de exploracion.

### nav route como primer paso de orientacion

`nav route` es el punto de entrada preferido para agentes y skills antes de lanzar `nav ask` o `nav pack`. Su ventaja: resuelve el anchor canónico y el mini reading pack usando el mínimo de tokens posibles (Tier 1 no requiere índice, Tier 2 enriquece si esta disponible). El patron recomendado es:

```
nav route <task>          # obtiene anchor + mini preview
nav ask <question>        # pregunta docs-first enriquecida con code evidence
nav pack <task>           # reading pack canonico completo
```

Ver TECH-DOC-ROUTER para el diseño del motor de routing de dos tiers.

### Fallback generico

Cuando el repo no usa `.docs/wiki`, el indexador considera como corpus documental:
- `README.md`
- `README*.md`
- `docs/`
- `.docs/`

Ese modo permite usar `nav ask` en repos sin gobierno documental estricto, aunque con menor calidad de ranking.

### Costos y tradeoffs

- Persistir el grafo documental evita volver a parsear toda la wiki en cada pregunta.
- El incremental por archivo no alcanza para docs: cambios documentales se tratan como disparador de full re-index.
- La recuperacion operacional de un corpus documental vacio debe preferir `mi-lsp index --docs-only`, porque preserva el catálogo de código y recompone `memory_pointer`.
- No se usan embeddings ni servicios externos en esta version.

### Boundary de snapshot documental

La confianza de `doc_mentions` de tipo `doc_id` depende del marker de versión del extractor. Cuando falta o es antiguo, `nav wiki search` confirma la mención contra el Markdown canónico con ruta segura, límite de lectura y `content_hash`; si no puede confirmar, omite la mención y emite advertencia. El snapshot documental es atómico para sus familias documentales, pero no promete que la activación foreground del Graph comparta la misma transacción. Las variantes fenced de job pueden recibir una publicación graph explícita dentro de su transacción. Docs-only no valida por sí mismo graph, recall, embeddings ni índice instalado/live.

- **Marcador estable**: tech-wiki-aware-search-snapshot-boundary
- **Implementación**:
  - internal/docidentity/identity.go
  - internal/service/wiki_search.go
  - internal/store/index_publish.go
  - internal/store/meta.go
  - internal/store/doc_snapshot.go
  - internal/store/queries_incremental.go
  - internal/indexer/indexer.go
- **oracles**:
  - internal/service/wiki_search_test.go
  - internal/docidentity/identity_test.go
  - internal/docgraph/identity_test.go
  - internal/docgraph/reference_identity_test.go
  - internal/wikisource/parser_test.go
  - internal/indexer/indexer_test.go
  - internal/store/doc_identity_snapshot_test.go
- **non_claims**:
  - graph_foreground_atomicity
  - installed_index
  - live_refresh
  - embeddings
