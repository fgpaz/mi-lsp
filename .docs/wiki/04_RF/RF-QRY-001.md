---
doc_id: RF-QRY-001
id: RF-QRY-001
title: Emitir envelope estable y truncacion determinista
implements:
  - internal/model/types.go
  - internal/output/formatter.go
  - internal/output/truncator.go
  - internal/cli/reason.go
tests:
  - internal/output/formatter_test.go
  - internal/output/truncator_test.go
  - internal/cli/reason_test.go
  - internal/service/app_test.go
  - internal/service/autoindex_test.go
---

# RF-QRY-001 - Emitir envelope estable y truncacion determinista

```yaml
harness_protocol: SDD-HARNESS-v1
id: "RF-QRY-001"
kind: "support-doc"
audience: "llm-first"
imports:
  - '[[00_gobierno_documental]]'
  - '[[RF-QRY-001]]'
exports:
  - 'RF-QRY-001'
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/04_RF/RF-QRY-001.md
agent_may_edit:
  - .docs/wiki/04_RF/RF-QRY-001.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
evidence:
  - .docs/wiki/04_RF/RF-QRY-001.md
```

## 1. Execution Sheet

| Campo | Valor |
|---|---|
| ID | RF-QRY-001 |
| Titulo | Emitir envelope estable y truncacion determinista |
| Actores | Usuario, Skill, Agente, CLI/Core |
| Prioridad | alta |
| Severidad | alta |
| FL origen | FL-QRY-01 |

## 2. Detailed Preconditions

| Condicion | Tipo | Estado requerido |
|---|---|---|
| Workspace resoluble | funcional | obligatorio |
| Comando `nav` soportado | funcional | obligatorio |
| Presupuestos numericos validos | tecnica | obligatorio |

## 3. Inputs

| Campo | Tipo | Req. | Origen | Validacion | RN |
|---|---|---|---|---|---|
| `format` | enum | no | CLI | `compact`, `json`, `text`, `toon` o `yaml`; default `compact` | RF-QRY-001 |
| `token_budget` | entero | no | CLI | mayor que cero | RF-QRY-001 |
| `max_items` | entero | no | CLI | mayor que cero | RF-QRY-001 |
| `max_chars` | entero | no | CLI | mayor que cero | RF-QRY-001 |
| `axi` | bool | no | CLI/env/default | modo AXI efectivo por default de superficie, `--axi` o `MI_LSP_AXI=1` | RF-QRY-001 |
| `classic` | bool | no | CLI | `--classic`; fuerza salida clasica y gana sobre defaults/env | RF-QRY-001 |
| `full` | bool | no | CLI | solo relevante cuando la superficie queda en AXI efectivo | RF-QRY-001 |

## 4. Process Steps (Happy Path)

1. La CLI recibe una consulta `nav`.
2. El core ejecuta la operacion y obtiene `items` normalizados.
3. El truncador aplica `max_items`, `max_chars` y `token_budget` en orden determinista.
4. Si la superficie queda en AXI efectivo y no hubo `--format` explicito, el formatter usa TOON como default para superficies cubiertas.
5. Cuando el formato efectivo es TOON, el formatter sanitiza recursivamente los strings del envelope despues de `envelopeToMap` y antes de `toon.Marshal`.
6. El formatter emite un unico envelope estable.
7. La CLI devuelve el resultado en el formato solicitado o normalizado.

## 5. Outputs

| Campo | Tipo | Destino | Efecto observable |
|---|---|---|---|
| `ok` | bool | usuario/skill | estado de la operacion |
| `backend` | string | usuario/skill | origen semantico o sintactico de la respuesta |
| `items` | lista | usuario/skill | resultado truncado o completo |
| `truncated` | bool | usuario/skill | explicita recorte |
| `warnings` | lista | usuario/skill | contexto de degradacion o frescura |
| `hint` | string/null | usuario/skill | diagnóstico accionable cuando `items=[]`, el índice no está listo o el daemon no está disponible (omitempty) |
| `next_hint` | string/null | usuario/skill | sugerencia para pedir mas detalle |
| `coach` | objeto/null | usuario/skill | guidance explicito y machine-readable para rerun, refine, narrow o expand |
| `continuation` | objeto/null | usuario/skill | siguiente paso tiny y machine-readable para el harness |
| `memory_pointer` | objeto/null | usuario/skill | puntero de reentrada wiki-aware con costo minimo |
| `mode` | string/null | usuario/skill | subtipo publico de la respuesta cuando la superficie expone variantes como `nav.intent (docs|code|mixed)` |
| `degraded` | bool | usuario/skill | `true` cuando respondio una via de menor fidelidad (texto en lugar de catalogo o semantica); omitido cuando es `false` (omitempty) |
| `reason` | string/null | usuario/skill | razon tipificada de la degradacion o del vacio, del conjunto cerrado de la seccion 6.1 (omitempty) |
| `fallback_used` | string/null | usuario/skill | via que respondio en lugar de la primaria: `catalog` o `text` (omitempty) |
| `items[].origin` | string/null | usuario/skill | procedencia por item: `semantic`, `catalog`, `text` o `wiki`; aditivo por item |

## 6. Typed Errors

| Codigo | Causa | Trigger | Respuesta esperada |
|---|---|---|---|
| `QRY_WORKSPACE_UNRESOLVED` | workspace invalido | alias/path no resoluble | abortar con `ok=false` |
| `QRY_INVALID_BUDGET` | flags invalidos | algun presupuesto es `<= 0` | abortar con error tipado |
| `QRY_RENDER_FAILED` | fallo de serializacion | formatter no puede construir output | abortar con error explicito |
| `workspace_resolution_failed` | path existente todavía no registrado y sin catálogo listo | `nav.find` recibe el path explícito con consulta cross-workspace habilitada y no hay evidencia de catálogo completo | `ok=false`; error `Kind=workspace`, `Code=workspace_resolution_failed`, `Stage=workspace_resolution`, `HintCode=workspace_resolution_failed`, `ReasonCode=invalid_workspace`; `Detail` y `NextHint` instruyen registrar e indexar ese root y reintentar |
| `index_not_ready` | workspace sin evidencia de catálogo completo | `nav.find` no encuentra `active_catalog_generation_id` ni el par completo y atómico `indexed_at` + `total_files` | ya no es error terminal: `nav.find` responde `ok=true`, `backend=text`, `degraded=true`, `reason=index_not_ready`, `fallback_used=text` con items `origin=text` y dispara un único reindex completo en segundo plano (ver [[RF-IDX-001]]) |
| `workspace_db_open_failed` | no se pudo leer el estado de generación | el store no puede determinar readiness | ya no es error terminal: misma respuesta degradada a texto con `reason=index_not_ready` (o `index_schema_broken` si la base está corrupta o con esquema roto); nunca se devuelven cero coincidencias sin marcar `degraded` |

### 6.1 Razones tipificadas (contrato `primitives-v1`)

El campo `reason` pertenece a un conjunto cerrado para que scripts y agentes ramifiquen sin parsear `warnings`. Es aditivo: los consumidores previos que ignoran `degraded`, `reason`, `fallback_used` y `items[].origin` siguen funcionando.

| `reason` | Significado |
|---|---|
| `index_not_ready` | no hay catálogo publicado o no se pudo leer; respondió texto |
| `index_schema_broken` | `index.db` corrupta o con esquema roto; respondió texto y la base se pone en cuarentena al reindexar |
| `index_stale` | el catálogo existe pero está desfasado respecto del disco |
| `lsp_unavailable` | falta el binario o runtime del backend semántico (roslyn/tsserver/pyright/gopls) |
| `lsp_error` | el backend semántico falló o devolvió error |
| `semantic_empty_text_hits` | el backend semántico devolvió vacío pero la verificación textual encontró coincidencias |
| `language_unsupported` | el archivo o símbolo no tiene backend semántico para su lenguaje |
| `no_matches` | ninguna vía encontró coincidencias; el vacío está verificado y es un resultado válido |
| `workspace_not_found` | el workspace no resuelve |

`fallback_used` toma `catalog` o `text`; `items[].origin` toma `semantic`, `catalog`, `text` o `wiki`. Las superficies que no degradan omiten `degraded`, `reason` y `fallback_used`.

## 7. Special Cases and Variants

- Si `format` es invalido, la respuesta se normaliza a `compact`.
- Si se alcanza un limite, `truncated=true` y `next_hint` debe indicar como pedir mas precision.
- Si la superficie queda en AXI efectivo y soporta preview-first, `next_hint` puede sugerir `--full` aun sin truncation dura.
- `coach` es aditivo y opcional: no reemplaza `warnings`, `hint` ni `next_hint`.
- En AXI preview, `coach.actions` se reduce a una sola accion para limitar costo de salida.
- `continuation` es aditivo y opcional: no reemplaza `coach`, `next_hint` ni `next_queries`.
- `memory_pointer` es aditivo y opcional: nunca persiste texto largo ni reemplaza `workspace status --full`.
- Cuando `nav.find` no puede usar el catálogo, no devuelve un vacío falso: responde `ok=true` con items de texto (`origin=text`, declaraciones primero, patrón acotado por límites de palabra), `backend=text`, `degraded=true`, `reason` tipificado y `fallback_used=text`, más un warning `catalog unavailable (<reason>); served from text; <resultado del reindex>`. Si el texto tampoco encuentra nada, devuelve `ok=true`, `items=[]` y `reason=no_matches` con `degraded=true`, porque sin catálogo el vacío no está verificado por el índice. Hay catálogo listo si existe `active_catalog_generation_id` o metadata transaccional completa `indexed_at` + `total_files`, escrita por `ReplaceCatalog`; esta última también cubre catálogos sin generación versionada y `total_files=0`. Un esquema sin esos campos o metadata parcial no basta, y no se inspeccionan conteos de filas para inferir completitud. Con catálogo listo, un vacío es un resultado válido sin `degraded`.
- Razón de la degradación de `nav.find`: `index_schema_broken` cuando la apertura o consulta falla por corrupción o esquema roto (`no such table`, `malformed`, `not a database`, etc.); `index_not_ready` en el resto (catálogo ausente, bloqueado o ilegible). Los items del catálogo llevan `origin=catalog`.
- Un path existente todavía no registrado se permite si su catálogo publicado aporta una de esas señales de readiness. Si no está registrado ni tiene catálogo listo, devuelve `workspace_resolution_failed`, `Kind=workspace`, `Stage=workspace_resolution`, `HintCode=workspace_resolution_failed` y `ReasonCode=invalid_workspace`; su `Detail` y `NextHint` usan el root y patrón recibidos para sugerir el registro, indexación y reintento exactos. Este es el único caso de `nav.find` sin catálogo que sigue siendo `ok=false`.
- El reindex de autosanación es un único job completo en segundo plano, deduplicado y con backoff tras fallo; nunca escribe el registry. Detalle operativo en [[RF-IDX-001]]. La readiness por metadata no garantiza frescura respecto de cambios posteriores en disco; se mantiene la política previa de warnings stale.
- Para un path no registrado, `Detail` y `NextHint` dicen: “Workspace is not registered. Run `mi-lsp workspace add '<root shell-quoted>'` to register and index it, then retry `mi-lsp nav find '<pattern shell-quoted>' --workspace '<root shell-quoted>'`.” Los argumentos se citan dinámicamente según las reglas de la CLI.
- La CLI JSON conserva intacto el envelope de error aunque salga con código distinto de cero. MCP conserva el mismo diagnóstico en `structuredContent` y presenta error más `next` en el texto renderizado. En perfiles compactos, `NextHint` deriva del origen accionable del error.
- `--classic` prevalece sobre defaults por superficie y sobre `MI_LSP_AXI=1`.
- `--axi` y `--classic` juntos deben rechazarse antes de ejecutar la query.
- `compact` usa keys cortos y JSON sin whitespace innecesario.
- `toon` serializa el envelope en TOON (Token-Oriented Object Notation); ~20-40% menos tokens que JSON en arrays grandes.
- `toon` no debe fallar por controles no imprimibles dentro de strings: reemplaza todo control excepto tab, newline y carriage-return por escapes ASCII visibles (`\u0000`, `\u001f`, etc.) y agrega una unica advertencia `toon output sanitized unsafe control characters` cuando ocurre. El comportamiento `compact`/JSON queda compatible y no comparte esta sanitizacion.
- `yaml` serializa el envelope en YAML estándar; útil para lectura humana o parsers YAML.
- Si `items=[]`, el envelope emite `hint` con diagnóstico de causa (patron no encontrado, timeout, regex-like sin `--regex`).
- Cada item de `nav search` incluye `origin=text` y `col` (columna de la primera coincidencia, base 1 en runas) cuando la puede calcular. En formato agent implícito (salida no interactiva o cliente agente), `nav search` usa tope por defecto de 20 items (`--max-items` explícito gana; AXI y los demás `nav.*` conservan 5). El renderer agent muestra un snippet de hasta 160 runas con ventana centrada en la coincidencia (marca el recorte con `…`), agrupa en un encabezado de archivo los hits consecutivos del mismo archivo (`  <línea>: <texto>`), agrega `[in <caller>]` cuando el item trae `caller` y `(origin=<x>)` cuando el origen no es `text`. Con `degraded=true` el encabezado agrega `backend=<b> degraded=true reason=<r> fallback_used=<f>`.
- Si `nav search` agota presupuesto o timeout interno despues de encontrar resultados parciales seguros, debe devolver `ok=true`, preservar los `items` parciales, agregar warning tipado de timeout, `next_hint` accionable para acotar/reintentar y `coach.trigger=search_timeout`.
- Si el daemon falla y el fallback directo responde, el envelope emite `hint: "daemon_unavailable; served from local text index"`.
- `--format`, `--max-items`, `--max-chars` y `--token-budget` explicitos ganan sobre defaults AXI.
- `--max-chars` es global. `0` o ausente no es tope explicito; si hay `token_budget` mayor que cero, el truncador puede derivar `token_budget * 4`. Un valor explicito mayor que cero es el tope de caracteres. El recorte deja `truncated=true`, un marcador de truncacion y `continuation.next` cuando esa continuacion existia. Si el bloque de `continuation` por si solo excede `--max-chars`, se conserva completo en vez de cortarlo a medias.
- `mi-lsp version` usa el mismo envelope estable cuando se pasa `--format compact|json|toon|yaml`; sin `--format` explicito usa `text` legible y no requiere workspace ni daemon.
- Un fallo terminal externo agrega `error.reason_code` desde la allowlist cerrada en [[CT-NAV-INTENT]] y [[CT-GRAPH-CLI]] (`unsupported_operation`, `unavailable_binary`, `invalid_workspace`, `explicit_incomplete`), mas `error.detail` sanitizado y acotado a 300 caracteres en un campo separado; un `reason_code` fuera de la allowlist se reemplaza por la clasificacion derivada del mensaje/codigo/kind. Un fallo de proceso que nunca llego a formar un envelope imprime la linea humana primero y agrega un unico trailer `reason_code=... detail=...`; el detalle nunca hace eco de tokens, secrets ni argv crudos.

## 8. Data Model Impact

- `QueryEnvelope`

## 9. Expanded Acceptance Criteria (Gherkin)

```gherkin
Scenario: Responder con envelope estable en formato compact
  Given una consulta valida y un workspace resoluble
  When ejecuto "mi-lsp nav find Repository --workspace gastos --format compact"
  Then la respuesta incluye "ok", "backend", "items", "warnings", "stats" y "truncated"

Scenario: Truncar de forma determinista
  Given una consulta valida que excede el presupuesto
  When ejecuto la misma consulta dos veces con el mismo "token_budget"
  Then ambas respuestas tienen el mismo orden y el mismo punto de corte
  And "truncated" es "true"

Scenario: Rechazar presupuestos invalidos
  Given un "token_budget" igual a cero
  When ejecuto una consulta "nav"
  Then la operacion falla con "QRY_INVALID_BUDGET"

Scenario: Escapar controles inseguros en TOON
  Given un envelope con "code_evidence.snippet" que contiene NUL u otros controles no imprimibles
  When renderizo la respuesta con "--format toon"
  Then la operacion no falla por serializacion
  And la salida contiene escapes visibles como "\u0000"
  And la salida no contiene bytes NUL crudos
  And "warnings" incluye una sola advertencia de sanitizacion TOON

Scenario: Exponer provenance del binario
  Given un binario mi-lsp instalado
  When ejecuto "mi-lsp version --format toon"
  Then la respuesta incluye "backend=version"
  And "items[0]" incluye "goos", "goarch", "protocol_version", "worker_rid", "cli_path", "executable_sha256" y metadata VCS cuando esta disponible
  And no requiere workspace registrado ni daemon activo

Scenario: Mapear un fallo terminal a reason_code y detail separados
  Given un fallo terminal externo con un mensaje que menciona un workspace invalido
  When la CLI construye el envelope de error
  Then "error.reason_code" es uno de "unsupported_operation", "unavailable_binary", "invalid_workspace" o "explicit_incomplete"
  And "error.detail" es un campo distinto, sanitizado y acotado a 300 caracteres
  And un "reason_code" fuera de esa lista se reemplaza por la clasificacion derivada

Scenario: Degradar a texto sin falso vacio cuando falta el catalogo
  Given un workspace registrado sin catalogo publicado
  When ejecuto "mi-lsp nav find HelloWorld --workspace gastos --format json"
  Then "ok" es "true" y "backend" es "text"
  And "degraded" es "true", "reason" es "index_not_ready" y "fallback_used" es "text"
  And cada item incluye "origin" igual a "text"
  And "warnings" indica que se lanzo o se omitio el reindex en segundo plano

Scenario: Distinguir un vacio verificado de un vacio sin indice
  Given un workspace registrado sin catalogo publicado
  When ejecuto "mi-lsp nav find SimboloInexistente --workspace gastos --format json"
  Then "ok" es "true", "items" es vacio, "degraded" es "true" y "reason" es "no_matches"
```

## 10. Test Traceability

- Positivo: `TP-QRY / TC-QRY-001`
- Positivo: `TP-QRY / TC-QRY-002`
- Positivo: `TP-QRY / TC-QRY-042`
- Positivo: `TP-QRY / TC-QRY-106`
- Positivo: `TP-QRY / TC-QRY-108`
- Positivo: `TP-QRY / TC-QRY-109`
- Positivo: `TP-QRY / TC-QRY-147`
- Positivo: `TP-QRY / TC-QRY-148`
- Positivo: `TP-QRY / TC-QRY-151`
- Positivo: `TP-QRY / TC-QRY-171`
- Positivo: `TP-QRY / TC-QRY-172`
- Positivo: `TP-QRY / TC-QRY-182`
- Positivo: `TP-QRY / TC-QRY-183`
- Positivo: `TP-QRY / TC-QRY-184`
- Negativo: `TP-QRY / TC-QRY-003`
- Negativo: `TP-QRY / TC-QRY-149`
- Negativo: `TP-QRY / TC-QRY-150`
- Negativo: `TP-QRY / TC-QRY-152`
- Negativo: `TP-QRY / TC-QRY-176`

## 11. No Ambiguities Left

- Supuestos prohibidos:
  - no asumir formato libre por comando
  - no omitir `backend`, `warnings` o `truncated`
- Decisiones cerradas:
  - envelope unico para toda respuesta `nav`
  - truncacion determinista y visible
- TODO explicit = 0
- Fuera de alcance:
  - streaming parcial de respuestas
- Dependencias externas explicitas:
  - solo formatter/truncator locales
