---
doc_id: DB-SYMBOL-EDGE-GRAPH
title: Grafo futuro de relaciones entre simbolos y wiki
layer: DB
family: EDGE-GRAPH
status: proposed
implements:
  - internal/store/schema.go
  - internal/store/queries.go
tests:
  - internal/store
---

```yaml
harness_protocol: SDD-HARNESS-v1
id: "DB-SYMBOL-EDGE-GRAPH"
kind: "support-doc"
audience: "llm-first"
imports:
  - '[[00_gobierno_documental]]'
  - '[[08_modelo_fisico_datos]]'
exports:
  - 'DB-SYMBOL-EDGE-GRAPH'
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/08_modelo_fisico_datos.md
  - .docs/wiki/08_db/DB-SYMBOL-EDGE-GRAPH.md
agent_may_edit:
  - .docs/wiki/08_modelo_fisico_datos.md
  - .docs/wiki/08_db/DB-SYMBOL-EDGE-GRAPH.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
evidence:
  - .docs/wiki/08_db/DB-SYMBOL-EDGE-GRAPH.md
```

Volver a [08_modelo_fisico_datos.md](../08_modelo_fisico_datos.md).

## Summary

`symbol_edges` es una propuesta de esquema para convertir el catalogo actual de archivos, simbolos y docs en un grafo consultable de impacto. Esta pagina no autoriza una migracion inmediata: el PR de `nav affected` solo consume diff, catalogo y heuristicas.

## Estado

- Estado: proposed.
- Tabla creada en este PR: no.
- Owner logico: workspace index DB.
- Primer consumidor esperado: futuras versiones de `nav related`, `nav callers`, `nav callees`, `nav impact`, `nav path`, `nav task-context` y `nav affected`.

## Esquema propuesto

Tabla: `symbol_edges`.

Columnas candidatas:

- `from_symbol_id`: simbolo origen cuando existe en `symbols`.
- `to_symbol_id`: simbolo destino cuando existe en `symbols`.
- `from_file_path`: archivo origen para edges sin simbolo resoluble.
- `to_file_path`: archivo destino para edges sin simbolo resoluble.
- `edge_kind`: tipo versionado de relacion.
- `source_backend`: extractor que produjo la relacion (`roslyn`, `tsserver`, `catalog`, `text`, `docgraph`).
- `confidence`: numero entre 0 y 1.
- `evidence`: JSON sanitizado con linea, rango, nombre o doc record minimo.
- `created_at`: timestamp de publicacion de la generacion.

Edge kinds iniciales:

- `contains`
- `calls`
- `imports`
- `references`
- `implements`
- `extends`
- `tests`
- `route_to_handler`
- `doc_mentions`

## Reglas

- `symbol_edges` debe ser una tabla separada. No sobrecargar `symbols` con columnas de relacion.
- La tabla es reconstruible desde indexacion, igual que `symbols` y `doc_records`.
- La publicacion debe ocurrir dentro de la generacion activa de `index.db`; un grafo parcial no debe quedar visible como definitivo.
- Las queries que usen esta tabla deben exponer `confidence` y `source_backend`, especialmente cuando el edge venga de heuristicas textuales.
- C# debe preferir Roslyn para edges semanticos. Tree-sitter o extractores textuales solo pueden actuar como fallback para lenguajes donde no haya backend canonico.
- Los edges `doc_mentions` conectan documentos gobernados con codigo, comandos o RF/TP mencionados; no reemplazan `doc_edges`.

## Fuera de alcance actual

- Crear la tabla en `schema.go`.
- Migrar `symbolsDDL`.
- Extraer llamadas completas para todos los lenguajes.
- Cambiar el contrato de `nav related` en este PR.
- Afirmar precision de impacto sin warning heuristico.
