---
wiki_contract: WIKI-CONTRACT-v1
id: RF-QRY-001
kind: requirement
status: active
title: RF-QRY-001 - Emitir envelope estable y truncacion determinista
links:
  implements:
  - FL-QRY-01
  code:
  - path: internal/cli/reason.go
    role: behavior
  - path: internal/model/types.go
    role: behavior
  - path: internal/output/formatter.go
    role: behavior
  - path: internal/output/truncator.go
    role: behavior
---

## Propósito

RF-QRY-001 - Emitir envelope estable y truncación determinista

### 1. Hoja de ejecución
- **Campo**: ID; **Valor**: RF-QRY-001
- **Campo**: Título; **Valor**: Emitir envelope estable y truncación determinista
- **Campo**: Actores; **Valor**: Usuario, Skill, Agente, CLI/Core
- **Campo**: Prioridad; **Valor**: alta
- **Campo**: Severidad; **Valor**: alta
- **Campo**: FL origen; **Valor**: FL-QRY-01

## Comportamiento ordenado

### 2. Precondiciones detalladas
- **Condición**: Workspace resoluble; **Tipo**: funcional; **Estado requerido**: obligatorio
- **Condición**: Comando `nav` soportado; **Tipo**: funcional; **Estado requerido**: obligatorio
- **Condición**: Presupuestos numericos válidos; **Tipo**: técnica; **Estado requerido**: obligatorio

### 4. Secuencia principal
1. La CLI recibe una consulta `nav`.
2. El core ejecuta la operación y obtiene `items` normalizados.
3. El truncador aplica `max_items`, `max_chars` y `token_budget` en orden determinista.
4. Si la superficie queda en AXI efectivo y no hubo `--format` explícito, el formatter usa TOON como default para superficies cubiertas.
5. Cuando el formato efectivo es TOON, el formatter sanitiza recursivamente los strings del envelope después de `envelopeToMap` y antes de `toon.Marshal`.
6. El formatter emite un único envelope estable.
7. La CLI devuelve el resultado en el formato solicitado o normalizado.

### 7. Casos especiales y variantes
- Si `format` es inválido, la respuesta se normaliza a `compact`.
- Si se alcanza un límite, `truncated=true` y `next_hint` debe indicar como pedir más precision.
- Si la superficie queda en AXI efectivo y soporta preview-first, `next_hint` puede sugerir `--full` aun sin truncation dura.
- `coach` es aditivo y opcional: no reemplaza `warnings`, `hint` ni `next_hint`.
- En AXI preview, `coach.actions` se reduce a una sola acción para limitar costo de salida.
- `continuation` es aditivo y opcional: no reemplaza `coach`, `next_hint` ni `next_queries`. En TOON aparece antes del resto del envelope y `continuation.next` es un comando CLI ejecutable de una sola línea; JSON y YAML conservan el destino estructurado.
- `memory_pointer` es aditivo y opcional: nunca persiste texto largo ni reemplaza `workspace status --full`.
- Cuando `nav.find` no puede usar el catálogo, no devuelve un vacío falso: responde `ok=true` con items de texto (`origin=text`, declaraciones primero, patrón acotado por límites de palabra), `backend=text`, `degraded=true`, `reason` tipificado y `fallback_used=text`, más un warning `catalog unavailable (<reason>); served from text; <resultado del reindex>`. Si el texto tampoco encuentra nada, devuelve `ok=true`, `items=[]` y `reason=no_matches` con `degraded=true`, porque sin catálogo el vacío no está verificado por el índice. Hay catálogo listo si existe `active_catalog_generation_id` o metadata transaccional completa `indexed_at` + `total_files`, escrita por `ReplaceCatalog`; esta última también cubre catálogos sin generación versionada y `total_files=0`. Un esquema sin esos campos o metadata parcial no basta, y no se inspeccionan conteos de filas para inferir completitud. Con catálogo listo, un vacío es un resultado válido sin `degraded`.
- Razón de la degradación de `nav.find`: `index_schema_broken` cuando la apertura o consulta falla por corrupción o esquema roto (`no such table`, `malformed`, `not a database`, etc.); `index_not_ready` en el resto (catálogo ausente, bloqueado o ilegible). Los items del catálogo llevan `origin=catalog`.
- Un path existente todavía no registrado se permite si su catálogo publicado aporta una de esas señales de readiness. Si no está registrado ni tiene catálogo listo, devuelve `workspace_resolution_failed`, `Kind=workspace`, `Stage=workspace_resolution`, `HintCode=workspace_resolution_failed` y `ReasonCode=invalid_workspace`; su `Detail` y `NextHint` usan el root y patrón recibidos para sugerir el registro, indexación y reintento exactos. Este es el único caso de `nav.find` sin catálogo que sigue siendo `ok=false`.
- El reindex de autosanación es un único job completo en segundo plano, deduplicado y con backoff tras fallo; nunca escribe el registry. Detalle operativo en [[RF-IDX-001]]. La readiness por metadata no garantiza frescura respecto de cambios posteriores en disco; se mantiene la política previa de warnings stale.
- Para un path no registrado, `Detail` y `NextHint` dicen: “Workspace is not registered. Run `mi-lsp workspace add '<root shell-quoted>'` to register and index it, then retry `mi-lsp nav find '<pattern shell-quoted>' --workspace '<root shell-quoted>'`.” Los argumentos se citan dinámicamente según las reglas de la CLI.
- La CLI JSON conserva intacto el envelope de error aunque salga con código distinto de cero. [RETIRADO 2026-10-15] MCP conserva el mismo diagnóstico en `structuredContent` y presenta error más `next` en el texto renderizado. En perfiles compactos, `NextHint` deriva del origen accionable del error.
- `--classic` prevalece sobre defaults por superficie y sobre `MI_LSP_AXI=1`.
- `--axi` y `--classic` juntos deben rechazarse antes de ejecutar la query.
- `compact` usa keys cortos y JSON sin whitespace innecesario.
- `toon` serializa el envelope en TOON (Token-Oriented Object Notation); ~20-40% menos tokens que JSON en arrays grandes.
- `toon` no debe fallar por controles no imprimibles dentro de strings: reemplaza todo control excepto tab, newline y carriage-return por escapes ASCII visibles (`\u0000`, `\u001f`, etc.) y agrega una única advertencia `toon output sanitized unsafe control characters` cuando ocurre. El comportamiento `compact`/JSON queda compatible y no comparte esta sanitizacion.
- `yaml` serializa el envelope en YAML estándar; útil para lectura humana o parsers YAML.
- Si `items=[]`, el envelope emite `hint` con diagnóstico de causa (patron no encontrado, timeout, regex-like sin `--regex`).
- Cada item de `nav search` incluye `origin=text` y `col` (columna de la primera coincidencia, base 1 en runas) cuando la puede calcular. En formato agent implícito (salida no interactiva o cliente agente), `nav search` usa tope por defecto de 20 items (`--max-items` explícito gana; AXI y los demás `nav.*` conservan 5). El renderer agent muestra un snippet de hasta 160 runas con ventana centrada en la coincidencia (marca el recorte con `…`), agrupa en un encabezado de archivo los hits consecutivos del mismo archivo (`  <línea>: <texto>`), agrega `[in <caller>]` cuando el item trae `caller` y `(origin=<x>)` cuando el origen no es `text`. Con `degraded=true` el encabezado agrega `backend=<b> degraded=true reason=<r> fallback_used=<f>`.
- Si `nav search` agota presupuesto o timeout interno después de encontrar resultados parciales seguros, debe devolver `ok=true`, preservar los `items` parciales, agregar warning tipado de timeout, `next_hint` accionable para acotar/reintentar y `coach.trigger=search_timeout`.
- Si el daemon falla y el fallback directo responde, el envelope emite `hint: "daemon_unavailable; served from local text index"`.
- `--format`, `--max-items`, `--max-chars` y `--token-budget` explícitos ganan sobre defaults AXI.
- `--max-chars` es global. `0` o ausente no es tope explícito; si hay `token_budget` mayor que cero, el truncador puede derivar `token_budget * 4`. Un valor explícito mayor que cero es el tope de caracteres. El recorte deja `truncated=true`, un marcador de truncación y `continuation.next` cuando esa continuacion existia. Si el bloque de `continuation` por si solo excede `--max-chars`, se conserva completo en vez de cortarlo a medias.
- `mi-lsp version` usa el mismo envelope estable cuando se pasa `--format compact|json|toon|yaml`; sin `--format` explícito usa `text` legible y no requiere workspace ni daemon.
- Un fallo terminal externo agrega `error.reason_code` desde la allowlist cerrada en [[CT-NAV-INTENT]] y [[CT-GRAPH-CLI]] (`unsupported_operation`, `unavailable_binary`, `invalid_workspace`, `explicit_incomplete`), más `error.detail` sanitizado y acotado a 300 caracteres en un campo separado; un `reason_code` fuera de la allowlist se reemplaza por la clasificación derivada del mensaje/codigo/kind. Un fallo de proceso que nunca llego a formar un envelope, sin `--format json`, imprime la línea humana primero y agrega un único trailer `reason_code=... detail=...`. Con `--format json` imprime un único envelope `ok:false` y `error.reason_code` de la misma allowlist, más `error.detail`. El detalle nunca hace eco de tokens, secrets ni argv crudos.

### 11. Decisiones cerradas
- Supuestos prohibidos:
  - no asumir formato libre por comando
  - no omitir `backend`, `warnings` o `truncated`
- Decisiones cerradas:
  - envelope único para toda respuesta `nav`
  - truncación determinista y visible
- TODO explicit = 0
- Fuera de alcance:
  - streaming parcial de respuestas
- Dependencias externas explícitas:
  - solo formatter/truncator locales

## Superficies

CLI `mi-lsp`, con operaciones y selectores detallados en el comportamiento del requisito.

## Entradas/salidas tipadas

### 3. Entradas
- **Campo**: `format`; **Tipo**: enum; **Req.**: no; **Origen**: CLI; **Validacion**: `compact`, `json`, `text`, `toon` o `yaml`; default `compact`; **RN**: RF-QRY-001
- **Campo**: `token_budget`; **Tipo**: entero; **Req.**: no; **Origen**: CLI; **Validacion**: mayor que cero; **RN**: RF-QRY-001
- **Campo**: `max_items`; **Tipo**: entero; **Req.**: no; **Origen**: CLI; **Validacion**: mayor que cero; **RN**: RF-QRY-001
- **Campo**: `max_chars`; **Tipo**: entero; **Req.**: no; **Origen**: CLI; **Validacion**: mayor que cero; **RN**: RF-QRY-001
- **Campo**: `axi`; **Tipo**: bool; **Req.**: no; **Origen**: CLI/env/default; **Validacion**: modo AXI efectivo por default de superficie, `--axi` o `MI_LSP_AXI=1`; **RN**: RF-QRY-001
- **Campo**: `classic`; **Tipo**: bool; **Req.**: no; **Origen**: CLI; **Validacion**: `--classic`; fuerza salida clasica y gana sobre defaults/env; **RN**: RF-QRY-001
- **Campo**: `full`; **Tipo**: bool; **Req.**: no; **Origen**: CLI; **Validacion**: solo relevante cuando la superficie queda en AXI efectivo; **RN**: RF-QRY-001

### 5. Salidas
- **Campo**: `ok`; **Tipo**: bool; **Destino**: usuario/skill; **Efecto observable**: estado de la operación
- **Campo**: `backend`; **Tipo**: string; **Destino**: usuario/skill; **Efecto observable**: origen semántico o sintactico de la respuesta
- **Campo**: `items`; **Tipo**: lista; **Destino**: usuario/skill; **Efecto observable**: resultado truncado o completo
- **Campo**: `truncated`; **Tipo**: bool; **Destino**: usuario/skill; **Efecto observable**: explícita recorte
- **Campo**: `warnings`; **Tipo**: lista; **Destino**: usuario/skill; **Efecto observable**: contexto de degradacion o frescura
- **Campo**: `hint`; **Tipo**: string/null; **Destino**: usuario/skill; **Efecto observable**: diagnóstico accionable cuando `items=[]`, el índice no está listo o el daemon no está disponible (omitempty)
- **Campo**: `next_hint`; **Tipo**: string/null; **Destino**: usuario/skill; **Efecto observable**: sugerencia para pedir más detalle
- **Campo**: `coach`; **Tipo**: objeto/null; **Destino**: usuario/skill; **Efecto observable**: guidance explícito y machine-readable para rerun, refine, narrow o expand
- **Campo**: `continuation`; **Tipo**: objeto/null; **Destino**: usuario/skill; **Efecto observable**: siguiente paso tiny y machine-readable para el harness
- **Campo**: `memory_pointer`; **Tipo**: objeto/null; **Destino**: usuario/skill; **Efecto observable**: puntero de reentrada wiki-aware con costo mínimo
- **Campo**: `mode`; **Tipo**: string/null; **Destino**: usuario/skill; **Efecto observable**: subtipo publico de la respuesta cuando la superficie expone variantes como `nav.intent (docs|code|mixed)`
- **Campo**: `degraded`; **Tipo**: bool; **Destino**: usuario/skill; **Efecto observable**: `true` cuando respondio una via de menor fidelidad (texto en lugar de catálogo o semántica); omitido cuando es `false` (omitempty)
- **Campo**: `reason`; **Tipo**: string/null; **Destino**: usuario/skill; **Efecto observable**: razon tipificada de la degradacion o del vacio, del conjunto cerrado de la seccion 6.1 (omitempty)
- **Campo**: `fallback_used`; **Tipo**: string/null; **Destino**: usuario/skill; **Efecto observable**: via que respondio en lugar de la primaria: `catalog` o `text` (omitempty)
- **Campo**: `items[].origin`; **Tipo**: string/null; **Destino**: usuario/skill; **Efecto observable**: procedencia por item: `semantic`, `catalog`, `text` o `wiki`; aditivo por item

### 8. Impacto en el modelo de datos
- `QueryEnvelope`

## Errores

### 6. Errores tipados
- **Código**: `QRY_WORKSPACE_UNRESOLVED`; **Causa**: workspace inválido; **Trigger**: alias/path no resoluble; **Respuesta esperada**: abortar con `ok=false`
- **Código**: `QRY_INVALID_BUDGET`; **Causa**: flags invalidos; **Trigger**: algun presupuesto es `<= 0`; **Respuesta esperada**: abortar con error tipado
- **Código**: `QRY_RENDER_FAILED`; **Causa**: fallo de serializacion; **Trigger**: formatter no puede construir output; **Respuesta esperada**: abortar con error explícito
- **Código**: `workspace_resolution_failed`; **Causa**: path existente todavía no registrado y sin catálogo listo; **Trigger**: `nav.find` recibe el path explícito con consulta cross-workspace habilitada y no hay evidencia de catálogo completo; **Respuesta esperada**: `ok=false`; error `Kind=workspace`, `Code=workspace_resolution_failed`, `Stage=workspace_resolution`, `HintCode=workspace_resolution_failed`, `ReasonCode=invalid_workspace`; `Detail` y `NextHint` instruyen registrar e indexar ese root y reintentar
- **Código**: `index_not_ready`; **Causa**: workspace sin evidencia de catálogo completo; **Trigger**: `nav.find` no encuentra `active_catalog_generation_id` ni el par completo y atómico `indexed_at` + `total_files`; **Respuesta esperada**: ya no es error terminal: `nav.find` responde `ok=true`, `backend=text`, `degraded=true`, `reason=index_not_ready`, `fallback_used=text` con items `origin=text` y dispara un único reindex completo en segundo plano (ver [[RF-IDX-001]])
- **Código**: `workspace_db_open_failed`; **Causa**: no se pudo leer el estado de generación; **Trigger**: el store no puede determinar readiness; **Respuesta esperada**: ya no es error terminal: misma respuesta degradada a texto con `reason=index_not_ready` (o `index_schema_broken` si la base está corrupta o con esquema roto); nunca se devuelven cero coincidencias sin marcar `degraded`

#### 6.1 Razones tipificadas (contrato `primitives-v2`)

El campo `reason` pertenece a un catálogo versionado (`primitives-v2`) para que scripts y agentes ramifiquen sin parsear `warnings`; un consumidor mapea cualquier valor desconocido a `unknown`. Es independiente de `error.reason_code` (allowlist cerrada `unsupported_operation|unavailable_binary|invalid_workspace|explicit_incomplete`), que solo aparece con `ok=false` y no se mezcla con `reason`. `reason` vacío significa resultado de fidelidad completa; `reason=no_matches` con `degraded` ausente significa que la vía completa corrió y no encontró nada. Es aditivo: los consumidores previos que ignoran `degraded`, `reason`, `fallback_used` y `items[].origin` siguen funcionando.

- **`reason`**: `index_not_ready`; **Significado**: no hay catálogo publicado o no se pudo leer; respondió texto
- **`reason`**: `index_schema_broken`; **Significado**: `index.db` corrupta o con esquema roto; respondió texto y la base se pone en cuarentena al reindexar
- **`reason`**: `lsp_unavailable`; **Significado**: falta el binario o runtime del backend semántico (roslyn/tsserver/pyright/gopls/rust-analyzer)
- **`reason`**: `lsp_error`; **Significado**: el backend semántico falló o devolvió error
- **`reason`**: `semantic_empty_text_hits`; **Significado**: el backend semántico devolvió vacío pero la verificación textual encontró coincidencias
- **`reason`**: `language_unsupported`; **Significado**: el archivo o símbolo no tiene backend semántico para su lenguaje
- **`reason`**: `no_matches`; **Significado**: ninguna vía encontró coincidencias; el vacío está verificado y es un resultado válido (`degraded=true` solo si faltaba el catálogo y respondió texto)

`fallback_used` toma `catalog` o `text`; `items[].origin` toma `semantic`, `catalog`, `text` o `wiki`. Las superficies que no degradan omiten `degraded`, `reason` y `fallback_used`.

## Criterios de aceptación (numerados)

1. Responder con envelope estable en formato compact: Dado una consulta valida y un workspace resoluble Cuando ejecuto "mi-lsp nav find Repository --workspace gastos --format compact" Entonces la respuesta incluye "ok", "backend", "items", "warnings", "stats" y "truncated"
2. Truncar de forma determinista: Dado una consulta valida que excede el presupuesto Cuando ejecuto la misma consulta dos veces con el mismo "token_budget" Entonces ambas respuestas tienen el mismo orden y el mismo punto de corte Y "truncated" es "true"
3. Rechazar presupuestos invalidos: Dado un "token_budget" igual a cero Cuando ejecuto una consulta "nav" Entonces la operación falla con "QRY_INVALID_BUDGET"
4. Escapar controles inseguros en TOON: Dado un envelope con "code_evidence.snippet" que contiene NUL u otros controles no imprimibles Cuando renderizo la respuesta con "--format toon" Entonces la operación no falla por serializacion Y la salida contiene escapes visibles como "\u0000" Y la salida no contiene bytes NUL crudos Y "warnings" incluye una sola advertencia de sanitizacion TOON
5. Exponer provenance del binario: Dado un binario mi-lsp instalado Cuando ejecuto "mi-lsp version --format toon" Entonces la respuesta incluye "backend=version" Y "items[0]" incluye "goos", "goarch", "protocol_version", "worker_rid", "cli_path", "executable_sha256" y metadata VCS cuando esta disponible Y no requiere workspace registrado ni daemon activo
6. Mapear un fallo terminal a reason_code y detail separados: Dado un fallo terminal externo con un mensaje que menciona un workspace inválido Cuando la CLI construye el envelope de error Entonces "error.reason_code" es uno de "unsupported_operation", "unavailable_binary", "invalid_workspace" o "explicit_incomplete" Y "error.detail" es un campo distinto, sanitizado y acotado a 300 caracteres Y un "reason_code" fuera de esa lista se reemplaza por la clasificación derivada
7. Degradar a texto sin falso vacio cuando falta el catálogo: Dado un workspace registrado sin catálogo publicado Cuando ejecuto "mi-lsp nav find HelloWorld --workspace gastos --format json" Entonces "ok" es "true" y "backend" es "text" Y "degraded" es "true", "reason" es "index_not_ready" y "fallback_used" es "text" Y cada item incluye "origin" igual a "text" Y "warnings" indica que se lanzo o se omitio el reindex en segundo plano
8. Distinguir un vacio verificado de un vacio sin índice: Dado un workspace registrado sin catálogo publicado Cuando ejecuto "mi-lsp nav find SimboloInexistente --workspace gastos --format json" Entonces "ok" es "true", "items" es vacio, "degraded" es "true" y "reason" es "no_matches"

### 9. Escenarios de aceptación (Gherkin)
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

### 10. Trazabilidad de pruebas
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
