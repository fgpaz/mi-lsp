---
wiki_contract: WIKI-CONTRACT-v1
id: CT-CLI-AXI-MODE
kind: contract
status: active
title: CT-CLI-AXI-MODE
---

## Consumidor

CLI, agentes y consumidores locales de las operaciones descritas; no se amplía la autoridad del consumidor.

## Operaciones

Se conservan requests, envelopes, campos, límites y semántica detallados a continuación.

Volver a [07_technical/contracts/CT-TECHNICAL-BASELINE.md](CT-TECHNICAL-BASELINE.md).

### Resumen
Define el overlay selectivo por superficie de onboarding/discovery AXI sobre la CLI publica de `mi-lsp`.

### Activacion y precedencia

- Defaults por superficie: algunas superficies entran en AXI por default.
- `--axi`: fuerza AXI por comando en cualquier superficie soportada.
- `MI_LSP_AXI=1`: fuerza AXI por sesion en cualquier superficie soportada.
- `--classic`: fuerza modo clasico y prevalece sobre defaults por superficie y sobre `MI_LSP_AXI=1`.
- `--axi` y `--classic` juntos son invalidos.
- `--axi=false`: anula el default AXI de la superficie actual; equivalente a `--classic` para esa invocacion.
- `--full`: expande disclosure solo cuando la superficie quedo en AXI efectivo.
- `--format`, `--max-items`, `--max-chars` y `--token-budget` explícitos ganan sobre defaults AXI.

### Superficies cubiertas

- AXI-default: `mi-lsp` sin subcomando, `init`, `workspace status`, `nav search`, `nav intent`, `nav pack`
- AXI-default condicional: `nav ask` para preguntas de onboarding/orientacion
- Classic-default: `nav workspace-map` y el resto de la CLI

### Reglas de contrato

- Por default, `mi-lsp` sin subcomando devuelve un home content-first; `--classic` restaura help generica.
- En AXI efectivo y sin `--format` explícito, discovery usa TOON cuando no aplica el despacho compacto de consumidores agente.
- Para consumidores agente (stdout no TTY, `MI_LSP_CLIENT_NAME` configurado o cliente cuyo nombre contiene `mcp`, p. ej. `mi-mcp`), la salida implícita de navegación usa formato `agent`, con el workspace como primera línea y resultado compacto. `--verbose` pide detalle ampliado; cualquier `--format` explícito prevalece y conserva el formato seleccionado.
- En una respuesta exitosa del formato `agent`, se priorizan filas breves de resultado y no se agregan bloques de coach/gobernanza ni continuaciones automáticas. Las omisiones o errores accionables sí permanecen visibles.
- En AXI preview, la respuesta puede anunciar expansion con `next_hint: rerun with --full for expanded detail`.
- En AXI preview, el envelope puede incluir `continuation.next`, pero `continuation.alternate` debe omitirse para no inflar la salida.
- `--full` no cambia routing, backend ni estructura base del envelope; solo expande disclosure.
- `nav ask` solo entra en AXI por default cuando la pregunta es claramente de orientacion; preguntas con paths, doc IDs, símbolos, comandos o lenguaje de implementacion deben quedar clasicas salvo `--axi`.
- Las `next_queries` y `next_steps` de superficies AXI-default no deben repetir `--axi` salvo cuando apunten a una superficie classic-default.

### Home AXI

El item principal del home puede incluir:

- `view=home`
- `mode=axi`
- `current_dir`
- `registered_workspaces`
- `workspace`, `workspace_root`, `workspace_kind`, `workspace_source` cuando se resolvio contexto
- `daemon_ready`
- `worker_ready`
- `next_steps`

### Discovery preview/full

- `workspace status`: `view=preview|full`, `docs_read_model`, `index_ready`, `next_steps`; en preview puede incluir `memory_pointer` y en `--full` expande `memory`
- `nav ask`: conserva `AskResult`, puede condensar evidencia en preview y usar `next_hint` para `--full` cuando la heuristica lo deja en AXI; `coach.trigger=anchor_drift` es aditivo y debe conservar al menos una acción hacia `nav pack --full`
- `nav pack`: conserva `PackResult`, entrega `mode=preview|full` y usa `--full` para materializar slices del mismo pack
- `nav workspace-map`: agrega `mode=preview|full` y `next_steps` solo cuando se fuerza AXI
- `nav search`: mantiene envelope estable y agrega guidance de expansion via `next_hint`; puede ademas exponer `continuation`/`memory_pointer` cuando ayudan a reentrar con pocos tokens
- `nav intent`: mantiene `backend=intent`, agrega `mode=docs|code|mixed`, usa preview/full sin mezclar docs y símbolos salvo en `mode=mixed` y anuncia `--full` via `next_hint`

## Errores

Se mantienen las razones, warnings, rechazos y fallbacks explícitos de cada operación.

## Compatibilidad

La migración cambia el contrato documental, no el protocolo runtime ni sus versiones. Los esquemas legacy que la herramienta consume continúan como compatibilidad de entrada, no como formato de autoría de esta wiki.
