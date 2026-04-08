# CT-NAV-ASK

## Boundary

Usuario/agente -> CLI publica `mi-lsp nav ask`

## Forma de invocacion

```text
mi-lsp nav ask <question> [--workspace <alias>] [--all-workspaces] [--format compact|json|text]
```

La CLI acepta una pregunta libre y produce un envelope `backend=ask`.
El daemon es opcional; si no responde, el core ejecuta el mismo contrato en modo directo.

## Payload logico

- `question`: string requerido
- `workspace`: alias o path resoluble
- `all_workspaces`: bool opcional; si vale `true`, la CLI itera workspaces registrados y mergea resultados docs-first en un solo envelope
- `max_items`, `token_budget`, `max_chars`: limites usuales del envelope

## Respuesta

Cada item de `backend=ask` contiene:
- `summary`
- `primary_doc`
- `doc_evidence`
- `code_evidence`
- `why`
- `next_queries`

## Semantica observable

- `primary_doc` es el documento canonico elegido.
- `doc_evidence` agrega supporting docs, priorizando links y doc IDs explicitos.
- `code_evidence` muestra archivos, simbolos o snippets derivados desde docs o fallback textual.
- `why` explica por que la respuesta eligio ese camino.
- `next_queries` deja comandos concretos para profundizar.
- con `read_model=default`, una wiki minima util bajo `07/08/09` debe seguir pudiendo rankear un `primary_doc` razonable cuando la pregunta comparte terminos claros con titulo/contenido del doc

## Warnings esperables

- `read_model=project`
- `read_model=default`
- `documentation index is empty; using code fallback`
- `code evidence came from text fallback`
- `<workspace>: ask failed: ...` cuando un workspace puntual falla dentro del fan-out cross-workspace

## Errores

- pregunta vacia -> error explicito
- workspace no resoluble -> error explicito
- `index.db` no accesible -> error explicito

## Relacion con `init`

`mi-lsp init` es el companion natural de este contrato: registra, indexa y deja visible el siguiente paso recomendado hacia `nav ask`.
No cambia el envelope de `nav ask`, pero mejora su discoverability.
