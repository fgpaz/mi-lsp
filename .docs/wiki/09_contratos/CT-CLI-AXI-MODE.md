# CT-CLI-AXI-MODE

Volver a [09_contratos_tecnicos.md](../09_contratos_tecnicos.md).

## Summary

Define el overlay selectivo por superficie de onboarding/discovery AXI sobre la CLI publica de `mi-lsp`.

## Activacion y precedencia

- Defaults por superficie: algunas superficies entran en AXI por default.
- `--axi`: fuerza AXI por comando en cualquier superficie soportada.
- `MI_LSP_AXI=1`: fuerza AXI por sesion en cualquier superficie soportada.
- `--classic`: fuerza modo clasico y prevalece sobre defaults por superficie y sobre `MI_LSP_AXI=1`.
- `--axi` y `--classic` juntos son invalidos.
- `--axi=false`: anula el default AXI de la superficie actual; equivalente a `--classic` para esa invocacion.
- `--full`: expande disclosure solo cuando la superficie quedo en AXI efectivo.
- `--format`, `--max-items`, `--max-chars` y `--token-budget` explicitos ganan sobre defaults AXI.

## Superficies cubiertas

- AXI-default: `mi-lsp` sin subcomando, `init`, `workspace status`, `nav search`, `nav intent`, `nav pack`
- AXI-default condicional: `nav ask` para preguntas de onboarding/orientacion
- Classic-default: `nav workspace-map` y el resto de la CLI

## Reglas de contrato

- Por default, `mi-lsp` sin subcomando devuelve un home content-first; `--classic` restaura help generica.
- En AXI efectivo y sin `--format` explicito, la salida por defecto de discovery es TOON.
- En AXI preview, la respuesta puede anunciar expansion con `next_hint: rerun with --full for expanded detail`.
- `--full` no cambia routing, backend ni estructura base del envelope; solo expande disclosure.
- `nav ask` solo entra en AXI por default cuando la pregunta es claramente de orientacion; preguntas con paths, doc IDs, simbolos, comandos o lenguaje de implementacion deben quedar clasicas salvo `--axi`.
- Las `next_queries` y `next_steps` de superficies AXI-default no deben repetir `--axi` salvo cuando apunten a una superficie classic-default.

## Home AXI

El item principal del home puede incluir:

- `view=home`
- `mode=axi`
- `current_dir`
- `registered_workspaces`
- `workspace`, `workspace_root`, `workspace_kind`, `workspace_source` cuando se resolvio contexto
- `daemon_ready`
- `worker_ready`
- `next_steps`

## Discovery preview/full

- `workspace status`: `view=preview|full`, `docs_read_model`, `index_ready`, `next_steps`
- `nav ask`: conserva `AskResult`, puede condensar evidencia en preview y usar `next_hint` para `--full` cuando la heuristica lo deja en AXI
- `nav pack`: conserva `PackResult`, entrega `mode=preview|full` y usa `--full` para materializar slices del mismo pack
- `nav workspace-map`: agrega `mode=preview|full` y `next_steps` solo cuando se fuerza AXI
- `nav search` / `nav intent`: mantienen envelope estable y agregan guidance de expansion via `next_hint`
