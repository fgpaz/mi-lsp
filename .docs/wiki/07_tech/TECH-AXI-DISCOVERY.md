# TECH-AXI-DISCOVERY

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
3. Si no hubo `--format` explicito, AXI usa TOON por default en las superficies cubiertas.
4. `nav search` y `nav intent` arrancan con una first page mas estrecha cuando el usuario no fijo `--max-items`.
5. `nav ask` usa una allowlist corta de intents de orientacion y blockers conservadores de implementacion para decidir si entra en AXI por default.
6. Las respuestas preview-first deben anunciar expansion via `next_hint` hacia `--full` solo cuando la preview realmente recorta la first page o la evidencia inicial.
7. `init` y `workspace status` conservan el bootstrap/base summary actual; solo agregan `view` y `next_steps`.
8. El home AXI resuelve contexto por `--workspace`, `cwd` o ultimo workspace registrado y agrega readiness barata de daemon/worker.
9. Las `next_queries` o `next_steps` de superficies AXI-default no deben repetir `--axi` salvo cuando apunten a una superficie que sigue classic-default, como `nav workspace-map`.
10. Cuando AXI esta en modo efectivo y el usuario no paso `--format` explicito, el format por defecto escala a `toon`. El override explicito `--format compact` siempre gana.
11. `--axi=false` permite anular explicitamente el default AXI de una superficie cuando el usuario quiere salida clasica sin escribir `--classic`.

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
