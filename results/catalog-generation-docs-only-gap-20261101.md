# Tantalo Android: catalog_generation bajo alcance docs-only

**Hallazgo: BLOQUEADO; no ejecutar ni aceptar la generación faltante.** Revisión documental y de fuente, READONLY salvo este informe. No hice indexación, build, release, cambio de código/config/permisos, alta de workspace ni escritura en el índice destino.

## Alcance y evidencia

- Fuente publicada consultada: `/home/tesla/repos/mios-release/mi-lsp-v01611-final-20261101`, `HEAD=8b2977c5d0f7eab34a05d21fe72e59d66d69be7a`; árbol limpio. No usé el checkout de integración como fuente de producto.
- Navegación documental: wiki gobernada `WIKI-CONTRACT-v1`, `RF-IDX-002`, `TP-IDX`, `CT-INDEX-JOB-OWNERSHIP` y `CT-GRAPH-CLI`; contrastada con implementación publicada abajo.
- Decisión/persona consultada con `mi-decidir`: `persona:tantalo-un-solo-servidor-20261030` (Tantalo/Android usa el servidor nativo; el ciclo AE completo lo instala y prueba Gabriel). No contiene autorización para indexación catalog-only. La consulta tuvo `ontology_source:adapter_absent`; por tanto no infiero autoridad adicional.
- El CLI público de `mi-lsp` y el catálogo de operaciones de `mi-mcp` fueron consultados en modo de descubrimiento. No hay un publicador separado y read-only de `active_catalog_generation_id`.

## Contrato y respuesta

**No existe un camino permitido dentro de docs-only para crear un `catalog_generation` ausente.** El contrato dice que `index --docs-only` reconstruye documentos/memoria sin reemplazar el catálogo de código. En `RF-IDX-002`:

- líneas 50–52: `mode catalog` recompone el catálogo y publica `active_catalog_generation_id`; `mode docs` recompone docs+memoria y publica solo `active_docs_generation_id` / `active_memory_generation_id`.
- líneas 69–78: `nav git-frontier.generations.catalog` y `.docs` leen punteros distintos; `.graph` es una identidad distinta. La publicación docs conserva `active_catalog_generation_id`; `memory_pointer` no prueba una generación.
- líneas 154 y `TP-IDX / TC-IDX-028`: la evidencia está **`manual_evidence_pending`**, no es una aceptación completa. El caso exige un fixture aislado propio, publica primero su catálogo mediante CLI, y luego verifica que docs-mode preserve ese puntero. Prohíbe el fixture Android y marca `null`/`generation_reasons` como `explicit_incomplete`, no como éxito.

La implementación coincide: `internal/store/index_publish.go` (`publishGenerationForJobTx`) actualiza en modo `docs` solo docs y memoria; en modo `catalog` actualiza catálogo, y `full` actualiza los tres. `internal/service/index_jobs.go` enruta `docs` y `catalog` a indexadores diferentes; `internal/indexer/indexer.go` muestra que el catálogo se forma a partir de archivos y símbolos. Así, un `index start --mode catalog` podría producir el puntero por una operación pública, pero **indexa código, crea un job durable y escribe el estado**: queda fuera de la autorización literal recibida. No propongo ejecutarlo ni lo trato como autorizado; tampoco propongo un full index.

Separación importante: `CT-GRAPH-CLI.docs-only-generation-availability` permite que docs-only publique una generación documental del grafo si hay Markdown canónico y afirma expresamente `does_not_replace_code_catalog: true`. Una `generations.graph` presente no equivale a `generations.catalog`. Del mismo modo, el `generation_id` del job docs identifica su generación candidata/documental, no el puntero activo de catálogo.

Por tanto, el `catalog_generation=null` observado con `docs_generation` y/o una generación graph presente es **compatible con el contrato** cuando no existía un catálogo previo. No prueba que el catálogo exista, no valida el catálogo, y no permite aceptar la brecha como resuelta. El alcance docs-only sí está publicado como operación, pero `TC-IDX-028` sigue pendiente de evidencia manual y su oráculo exige preservar un catálogo de referencia; no demuestra que docs-only pueda originarlo.

## Owner, decisión y siguiente evento

Único owner nombrado por el test plan: `parent_final_verify`, para la evidencia manual del fixture aislado; el documento no lo designa como dueño del catálogo Android. `BOTSD_PADRE` está vacío; la búsqueda read-only de botsd no encontró candidato de coordinación, así que no envié aviso ni atribuí identidad. No consta aquí un owner/Task que pueda autorizar la operación excluida, ni ETA.

**Decisión requerida:** el coordinador/owner real debe decidir expresamente si abre un alcance separado para publicar un catálogo de código; hasta entonces, dejar el estado Android como `catalog_generation` ausente y el gap en HOLD. Próximo evento: recibir esa decisión y el owner real; sin ella no se ejecuta ninguna operación mutante. No se altera el criterio `null` a PASS ni se modifica schema/contrato.

## Verificaciones y paths

- Comprobé `HEAD` y `git status` de la fuente publicada: commit indicado y árbol limpio.
- Leí la ruta de publicación de generaciones, la ruta docs-only, la ruta catalog-only y el contrato/prueba pertinentes; no ejecuté tests, build, indexación ni comandos de escritura.
- El diagnóstico automático post-escritura de mi-lsp devolvió `unsupported_operation` (sin backend diagnóstico para este lenguaje); no cuenta como validación.
- Paths tocados por esta sesión: `results/catalog-generation-docs-only-gap-20261101.md` únicamente. Sin cambios en el checkout de fuente publicada ni en código/wiki/config.
- Mi-decidir: persona consultada `persona:tantalo-un-solo-servidor-20261030`; la respuesta fue parcial y sin decisión aplicable al permiso de indexación.

BLOQUEADO: pregunta: ¿el owner real autoriza un alcance separado de publicación catalog-only para Tantalo Android? Mientras tanto, no ejecutar ni aceptar catalog_generation.
REPORTE-FINAL: results/catalog-generation-docs-only-gap-20261101.md
TRABADO
