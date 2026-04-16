# CT-NAV-INTENT

## Boundary

Usuario/agente -> CLI publica `mi-lsp nav intent`

## Forma de invocacion

```text
mi-lsp nav intent <question> [--workspace <alias>] [--repo <name>] [--top N] [--offset N] [--full]
```

## Semantica

`nav intent` conserva `backend=intent`, pero expone `mode=docs|code`.

- `mode=docs`: consultas capability-like, contract-like, flow-like o docs-first. Usa el scorer documental owner-aware compartido con `nav route`, `nav ask` y `nav pack`.
- `mode=code`: consultas symbol-like o implementation-like. Conserva el ranking BM25 actual sobre `search_text`.

El contrato no mezcla docs y simbolos en la misma lista.

## Payload logico

- `question`: string requerido
- `workspace`: alias o path resoluble
- `repo`: selector opcional de repo para workspaces `container`
- `top`: entero opcional
- `offset`: entero opcional

## Respuesta

El envelope incluye:

- `backend=intent`
- `mode=docs|code`
- `items`
- `warnings`
- `stats`

En `mode=docs`, cada item contiene:

- `doc_path`
- `doc_id`
- `title`
- `family`
- `layer`
- `score`
- `evidence`
- `next_queries`

En `mode=code`, cada item contiene:

- `file`
- `line`
- `symbol`
- `kind`
- `qualified_name`
- `score`
- `evidence`
- `snippet`

## Reglas observables

- Si `mode=docs`, `--repo` se valida pero no redefine el lane documental; puede quedar warning visible.
- Si `mode=code`, `--repo` filtra el universo de simbolos en workspaces `container`.
- Si ya existe un candidato documental canonico positivo, `README` y otros docs `generic` no deben liderar la lista.
- En AXI preview, `nav intent` mantiene `backend` y `mode`, y anuncia expansion via `next_hint` hacia `--full`.

## Diagnostico

- `MI_LSP_DOC_RANKING=owner|legacy` permite comparar el scorer owner-aware contra el camino legacy sin cambiar el contrato publico.
- La telemetria puede registrar solo metadata derivada: `doc_ranker` e `intent_mode`.

## RF asociados

- RF-QRY-001
- RF-QRY-011
- RF-QRY-014
- RF-QRY-015
