# CT-NAV-ASK

## Boundary

Usuario/agente -> CLI publica `mi-lsp nav ask`

## Forma de invocacion

```text
mi-lsp nav ask <question> [--workspace <alias>] [--all-workspaces] [--format compact|json|text]
```

La CLI acepta una pregunta libre y produce un envelope `backend=ask`.
El daemon es opcional; si no responde, el core ejecuta el mismo contrato en modo directo.
El hot path por default es directo; el daemon solo aporta warm state cuando una capa superior lo fuerza o cuando la operacion entra por un routeo semantico mas pesado.
El envelope puede agregar un bloque opcional `coach` query-level cuando existe un rerun/refinement claro.
El envelope puede agregar ademas `continuation` y `memory_pointer` como guidance estructurado de muy bajo costo.

## Payload logico

- `question`: string requerido
- `workspace`: alias o path resoluble
- `all_workspaces`: bool opcional; si vale `true`, la CLI itera workspaces registrados y mergea resultados docs-first en un solo envelope
- `max_items`, `token_budget`, `max_chars`: limites usuales del envelope

Cuando `workspace` se omite, el runtime resuelve primero el workspace registrado cuyo root contiene el `caller_cwd` real del invocador. Solo si no hay match puede caer a `last_workspace`, y ese caso debe quedar visible en `warnings`.

## Respuesta

Cada item de `backend=ask` contiene:
- `summary`
- `primary_doc`
- `doc_evidence`
- `code_evidence`
- `why`
- `next_queries`

El envelope puede contener ademas:
- `coach.trigger`
- `coach.message`
- `coach.confidence`
- `coach.actions[] { kind, label, command }`
- `continuation.reason`
- `continuation.next { op, query?, repo?, path?, symbol?, doc_id?, full? }`
- `continuation.alternate { ... }`
- `memory_pointer.doc_id`
- `memory_pointer.why`
- `memory_pointer.reentry_op`
- `memory_pointer.handoff`
- `memory_pointer.stale`

## Semantica observable

- `primary_doc` es el documento canonico elegido.
- El documento primario sale del scorer owner-aware compartido con `nav route` y `nav pack`; `owner_hints` opcionales desde `00` pueden sesgar ownership repo-especifico sin reemplazar la gobernanza. Si existe un match canonico positivo en `.docs/wiki/`, el scorer debe degradar `README`/generic y tambien artefactos de soporte en `.docs/raw/` para no usarlos como documento primario.
- `doc_evidence` agrega supporting docs, priorizando links y doc IDs explicitos.
- `code_evidence` muestra archivos, simbolos o snippets derivados desde docs o fallback textual.
- `why` explica por que la respuesta eligio ese camino.
- `next_queries` deja comandos concretos para profundizar.
- `coach` no reemplaza `next_queries`: es guidance query-level para la siguiente accion mas util cuando hay fallback textual, evidencia fina, preview recortado o un narrowing obvio.
- `continuation` no reemplaza `coach` ni `next_queries`: ofrece el mejor siguiente paso para el harness en forma machine-readable y con costo estable.
- `memory_pointer` no reemplaza evidencia: solo apunta a la mejor reentrada repo-local disponible, anclada en wiki/handoff reciente.
- Si ya existe un match canonico positivo, `README` y otros docs `generic` solo pueden quedar como evidencia secundaria; no pueden ganar `primary_doc`.
- con `read_model=default`, una wiki minima util bajo `07/08/09` debe seguir pudiendo rankear un `primary_doc` razonable cuando la pregunta comparte terminos claros con titulo/contenido del doc
- si varios aliases registrados comparten el mismo root, la seleccion automatica usa `project.name`, luego basename del root y deja warning visible con el alias elegido
- en AXI preview, el contrato reduce compute y salida a `primary_doc + 1 linked doc + 1 code evidence`
- en AXI preview, `coach.actions` se reduce a una sola accion para no inflar la respuesta
- en AXI preview, `continuation` conserva solo `next` y omite `alternate`

## Warnings esperables

- `read_model=project`
- `read_model=default`
- `documentation index is empty; using code fallback`
- `code evidence came from text fallback`
- `workspace omitted; multiple registry aliases share root ...`
- `workspace omitted; no registered workspace matched caller cwd ...; falling back to last_workspace=...`
- `<workspace>: ask failed: ...` cuando un workspace puntual falla dentro del fan-out cross-workspace

## Errores

- pregunta vacia -> error explicito
- workspace no resoluble -> error explicito
- `index.db` no accesible -> error explicito

## Relacion con `init`

`mi-lsp init` es el companion natural de este contrato: registra, indexa y deja visible el siguiente paso recomendado hacia `nav ask`.
No cambia el envelope de `nav ask`, pero mejora su discoverability.
