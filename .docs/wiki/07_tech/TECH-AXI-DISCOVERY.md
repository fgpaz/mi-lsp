# TECH-AXI-DISCOVERY

```yaml
harness_protocol: SDD-HARNESS-v1
id: "TECH-AXI-DISCOVERY"
kind: "support-doc"
audience: "llm-first"
imports:
  - '[[00_gobierno_documental]]'
  - '[[TECH-AXI-DISCOVERY]]'
exports:
  - 'TECH-AXI-DISCOVERY'
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/07_tech/TECH-AXI-DISCOVERY.md
agent_may_edit:
  - .docs/wiki/07_tech/TECH-AXI-DISCOVERY.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
evidence:
  - .docs/wiki/07_tech/TECH-AXI-DISCOVERY.md
```

## Proposito

Describir la capa tecnica del modo AXI selectivo por superficie para onboarding y discovery del CLI.
Su objetivo es mejorar el primer paso del agente sin alterar la semantica base de `mi-lsp`.

## Activacion

- Defaults por superficie: AXI se activa por default solo donde ya demostro reducir round-trips.
- `--axi` fuerza AXI por comando en cualquier superficie soportada.
- `MI_LSP_AXI=1` fuerza AXI por sesion en cualquier superficie soportada.
- `--classic` fuerza salida clasica y prevalece sobre defaults por superficie y sobre `MI_LSP_AXI=1`.
- `--axi` y `--classic` juntos son invalidos.
- `--full` expande la disclosure solo en las superficies que quedaron en AXI efectivo.

## Superficies cubiertas en v1

- AXI-default: root command sin subcomando, `init`, `workspace status`, `nav search`, `nav intent`, `nav pack`
- AXI-default condicional: `nav ask` solo para preguntas de onboarding/orientacion
- Classic-default: `nav workspace-map` y el resto de la CLI

## Reglas tecnicas

1. AXI vive en el borde del CLI y viaja al core como `QueryOptions{AXI, Full}`.
2. El daemon/core nunca ve `classic`; la CLI resuelve primero el modo efectivo y envia solo `QueryOptions{AXI, Full}`.
3. Para una sesión interactiva humana, AXI usa TOON por default en las superficies cubiertas cuando no se especifica `--format`. Para consumidores agente (stdout no TTY, `MI_LSP_CLIENT_NAME` configurado o MCP), el dispatch implícito usa formato `agent`.
4. `nav search` y `nav intent` arrancan con una first page mas estrecha cuando el usuario no fijo `--max-items`. `nav.search` ejecuta el scan en paralelo y limita la selección con orden estable por ruta y línea, evitando ordenar el conjunto completo de coincidencias. `nav.find` refresca sólo los archivos candidatos dentro del presupuesto de 250 ms (máximo 32 archivos, 4 MiB cada uno y 16 MiB agregados); si publica cambios, repite la consulta y, ante error/deadline, conserva el snapshot disponible con warning. `nav.route` y `nav.intent` pueden refrescar hasta cinco candidatos Markdown en 500 ms; sólo una publicación confirmada causa recarga y un único reranking, y ante fallo/deadline mantienen la respuesta en memoria con warning. El lector extrae bytes bajo writer lock cancelable; hashes de dialecto no reconocido preservan las filas legacy. Estos refresh síncronos no ejecutan Roslyn ni dejan escrituras en segundo plano.
5. `nav ask` usa una allowlist corta de intents de orientacion y blockers conservadores de implementacion para decidir si entra en AXI por default.
6. Las respuestas preview-first deben anunciar expansion via `next_hint` hacia `--full` solo cuando la preview realmente recorta la first page o la evidencia inicial.
7. `init` y `workspace status` conservan el bootstrap/base summary actual; solo agregan `view` y `next_steps`.
8. El home AXI resuelve contexto por `--workspace`, `cwd` o ultimo workspace registrado y agrega readiness barata de daemon/worker.
9. Las `next_queries` o `next_steps` de superficies AXI-default no deben repetir `--axi` salvo cuando apunten a una superficie que sigue classic-default, como `nav workspace-map`.
10. Cuando AXI está en modo efectivo para un consumidor humano y no se pasó `--format` explícito, el formato por defecto es `toon`. En consumidores agente aplica el formato `agent`; cualquier `--format` explícito siempre gana.
11. `--axi=false` permite anular explicitamente el default AXI de una superficie cuando el usuario quiere salida clasica sin escribir `--classic`.

## Consumidores agente y MCP local

El CLI selecciona `agent` por defecto cuando stdout no es TTY, `MI_LSP_CLIENT_NAME` está configurado o el cliente identifica MCP. La respuesta compacta comienza con el workspace y limita `find`, `search` y `multi-read` a cinco resultados. `search` presenta filas breves de archivo, línea y fragmento; en éxito se omiten bloques de coach/gobernanza y continuaciones automáticas. `--verbose` amplía el detalle; un `--format` explícito conserva su contrato.

`mi-lsp mcp` sirve JSON-RPC/NDJSON sobre stdin/stdout como proceso local. El servidor publica las operaciones `nav_*` mediante `tools/list` y las ejecuta dentro del proceso con el mismo servicio de la CLI. Para una llamada de herramienta, `structuredContent` contiene el envelope JSON completo y `content[0].text` lleva la vista compacta con el workspace primero; la bandera de formato de CLI no cambia ese contrato. Los logs se emiten fuera de stdout para preservar el framing.

## Stage signal en discovery lane

Cada `RouteDoc` en la discovery lane lleva el campo `stage` con uno de:
- `anchor` — doc canonico de anclaje (siempre el primero)
- `preview` — doc del mini preview pack (Tier 1 canonical)
- `discovery` — doc de discovery advisory (Tier 2, non-authoritative)

El stage permite a los agentes distinguir la fuente de cada doc sin necesidad de session state.

## No objetivos de esta version

- No convierte toda la CLI en AXI-default.
- No instala hooks ni escribe contexto persistente del agente.
- No altera routing directo vs daemon.
- No redefine envelopes por comando; reutiliza `hint`, `next_hint`, `next_steps` y `next_queries`.
