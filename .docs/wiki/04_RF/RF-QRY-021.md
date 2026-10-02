---
doc_id: RF-QRY-021
title: Exponer navegación local por el protocolo MCP
layer: RF
status: implemented
source_schema: SDD-WIKI-SOURCE-v1
wiki_source_protocol: SDD-WIKI-SOURCE-v1
source_kind: canonical-requirement
normative_format: toon
harness_protocol: SDD-HARNESS-v1
id: RF-QRY-021
kind: requirement
audience: llm-first
imports:
  - '[[00_gobierno_documental]]'
  - '[[FL-QRY-01]]'
  - '[[TP-QRY]]'
exports:
  - RF-QRY-021
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/03_FL/FL-QRY-01.md
  - .docs/wiki/04_RF/RF-QRY-021.md
  - .docs/wiki/06_pruebas/TP-QRY.md
agent_may_edit:
  - .docs/wiki/04_RF/RF-QRY-021.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-source --workspace mi-lsp --ids RF-QRY-021 --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
  - stdout_contains_non_protocol_output=true
evidence:
  - .docs/wiki/04_RF/RF-QRY-021.md
  - .docs/wiki/06_pruebas/TP-QRY.md
---

# RF-QRY-021 - Exponer navegación local por el protocolo MCP

## Resultado requerido

Permitir que un cliente MCP local invoque las operaciones de navegación de `mi-lsp` mediante una puerta nativa, stateless y opcional. La puerta ejecuta el binario CLI para cada llamada, no mantiene índice ni caché propios, y conserva a la CLI como autoridad de navegación y resolución de workspace.

## Interfaz y transporte

- Comando: `mi-lsp mcp [--workspace <alias|ruta>]`.
- Transporte: JSON-RPC delimitado por líneas NDJSON sobre stdin/stdout. stdout contiene exclusivamente mensajes del protocolo; logs y diagnósticos van a stderr.
- El proceso soporta `initialize`, la notificación `notifications/initialized`, `ping`, `tools/list` y `tools/call`. EOF cierra el proceso de forma limpia.
- El parsing de una línea está limitado a 8 MiB. No se agrega un timeout propio del servidor a cada solicitud; la operación usa el contexto de la consulta.
- Una solicitud con JSON/JSON-RPC inválido recibe un error de protocolo estructurado y no debe cerrar el proceso para las solicitudes válidas posteriores.
- `tools/list` publica estas trece herramientas: `nav_intent`, `nav_route`, `nav_pack`, `nav_wiki`, `nav_search`, `nav_find`, `nav_refs`, `nav_related`, `nav_flow_slice`, `nav_change_pack`, `nav_affected`, `nav_multi_read` y `nav_overview`.
- `nav_refs` acepta `symbol`, `file`, `repo`, `entrypoint`, `project`, `solution` y `context` (entero 0 a 5: líneas de contexto antes y después de cada referencia, equivalente a `nav refs --context`). Su envelope conserva los campos aditivos `degraded`, `reason`, `fallback_used` e `items[].origin`; una referencia nunca vuelve como falso vacío sino con el fallback de texto tipificado ([[RF-QRY-002]]).
- Las llamadas ejecutan el binario CLI y devuelven su envelope; la puerta no mantiene un proceso de consulta stateful entre llamadas. Los esquemas publicados describen los argumentos admitidos por cada operación y no duplican una implementación alternativa de navegación.

## Workspace y salida

- `--workspace` acepta alias o ruta. Si el selector se omite tanto en el comando como en la llamada a herramienta, el proceso usa su `CallerCWD`; la regla de resolución fail-closed sigue vigente.
- La respuesta de una llamada incluye `structuredContent` con el envelope JSON completo (`Format=json`) y `content[0].text` con la forma compacta de agente, que empieza con `workspace=<selección>`.
- `content[0].text` prioriza resultados breves (en búsqueda, archivo, línea y fragmento); la respuesta exitosa no agrega coach/gobernanza ni continuación automática. Los errores y omisiones accionables se conservan.
- `--format` de la CLI no cambia el contrato de salida de `tools/call`. La selección de formato y el texto legible por agente pertenecen a la respuesta de herramienta.
- Un workspace inválido devuelve un hint accionable, por ejemplo `mi-lsp workspace add '<path>' --name '<alias>' --no-index`; si no hay CWD disponible, la continuación indica `mi-lsp workspace scan`.

## Límites de aceptación

El proceso MCP es local y opcional; no abre un listener HTTP, no realiza llamadas de red y no requiere daemon. El objetivo de aceptación para una primera llamada fría a `nav_search` es menos de 5 s medidos desde el mismo checkout, host y condiciones reproducibles. Ese umbral requiere evidencia medida y no se considera satisfecho por definir este requisito.

## Compatibilidad y trazabilidad

El transporte MCP expone operaciones existentes y no altera su semántica. Los formatos explícitos del CLI y consumidores JSON de scripts mantienen compatibilidad. `TP-QRY` cubre handshake, inventario de herramientas, respuesta compacta/estructurada, resolución de workspace, limpieza de stdout, latencia fría y el argumento `context` de `nav_refs` (TC-QRY-177).
