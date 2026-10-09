---
doc_id: CT-Q-V1
title: Contrato congelado de consultas q-v1
layer: CT
family: QUERY-Q-V1
status: implemented
source_schema: SDD-WIKI-SOURCE-v1
wiki_source_protocol: SDD-WIKI-SOURCE-v1
source_kind: canonical-contract
normative_format: toon
harness_protocol: SDD-HARNESS-v1
id: CT-Q-V1
kind: support-doc
audience: llm-first
imports:
  - '[[00_gobierno_documental]]'
  - '[[RF-QRY-022]]'
  - '[[RF-QRY-023]]'
  - '[[RF-QRY-025]]'
  - '[[TP-QRY]]'
exports:
  - CT-Q-V1
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/09_contratos/CT-Q-V1.md
  - .docs/wiki/04_RF/RF-QRY-022.md
  - .docs/wiki/04_RF/RF-QRY-023.md
  - .docs/wiki/04_RF/RF-QRY-025.md
agent_may_edit:
  - .docs/wiki/09_contratos/CT-Q-V1.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-source --workspace mi-lsp --ids CT-Q-V1 --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
evidence:
  - .docs/wiki/09_contratos/CT-Q-V1.md
  - .docs/wiki/04_RF/RF-QRY-022.md
  - .docs/wiki/04_RF/RF-QRY-023.md
---

# CT-Q-V1 — Contrato congelado de consultas q-v1

```toon
doc_id: CT-Q-V1
block_id: CT-Q-V1.q-contract
kind: normative
source_of_truth: this
verify:
  - go test ./internal/query ./internal/service
  - mi-lsp nav wiki validate-source --workspace mi-lsp --ids CT-Q-V1 --format toon
evidence:
  - .docs/wiki/09_contratos/CT-Q-V1.md
  - internal/query/parser_test.go
  - internal/service/primitives_q_test.go
```

Estado: congelado; cualquier cambio incompatible requiere `q-v2`. Este contrato precede y gobierna las implementaciones F1, F5, F2 y F4 de mi-lsp. `contract_version` es exactamente `q-v1`.

## 1. Frontera y superficies

mi-lsp es el motor local de consultas y adapta la misma ejecución al daemon, CLI y MCP Go. El sandbox y `execute` son propiedad de mi-mcp; mi-lsp es proveedor y expone `milsp.q(...)` por su manifiesto de proveedor. Este contrato no agrega ni modifica código de mi-mcp. La consulta es de solo lectura: no indexa, registra workspaces, escribe archivos ni usa red. Se preservan las herramientas `nav_*` existentes.

- CLI: `mi-lsp q "<pipeline>" [--workspace <alias|ruta>] [--format json|compact|toon]`.
- MCP Go: una herramienta `milsp` con `q`, `workspace`, `budget`, `max_bytes`, `timeout_ms`, `session_id`, `page` y `fresh` opcionales; `contract_version: "q-v1"` siempre está presente en el envelope y en `tools/list`.
- Helper del host: `milsp.q(pipeline, {workspace, budget, max_bytes, timeout_ms, session_id, page, fresh, signal})` devuelve el envelope q-v1. El host es responsable de propagar su cancelación a `signal`.

## 2. Manifiesto de proveedor mi-mcp

El comando `mi-lsp provider-manifest` (alias explícito `--format json`; JSON por defecto) imprime un único objeto JSON limpio por stdout. No inicia daemon, no registra workspaces, no escribe archivos ni usa red. `provider_version` deriva de la versión del binario que expone `mi-lsp --version`, sin el prefijo `v`; en builds de desarrollo conserva el valor de esa fuente. `min_provider_version` es `0.10.0`.

El contrato fijo es `format: mi-mcp-provider/v1`, `namespace: milsp`, `contracts.q: q-v1`, `auth: none`, y una operación `q` de efecto `read`. La invocación usa `mi-lsp` con `--format json --client-name mi-mcp --no-auto-register` antes del subcomando. `input_schema` requiere `pipeline` y permite únicamente `pipeline`, `workspace`, `budget`, `max_bytes`, `timeout_ms`, `session_id`, `page`, `fresh` y `dedupe`; los campos opcionales corresponden a opciones de q-v1 disponibles en CLI (workspace puede ser global). `signal` no forma parte del schema porque se procesa en el host y no es argv. La salida referencia este contrato como q-v1.

El archivo publicado `integrations/mi-mcp/provider-manifest.json` mantiene el mismo objeto salvo `provider_version`, fijado en `0.10.1` para esa distribución. El test compara ambos tras normalizar ese campo y descubre las flags desde Cobra para que una divergencia del CLI falle.

## 3. Identidad y revisión de ítems

`id` es identidad, no versión: `ws:<workspace_id>/<kind>/<path>#<symbol>`. `rev` es un campo separado con forma `rev:<64 hex minúsculas>` (SHA-256 del contenido normalizado a LF). No se adjunta `@rev` al id q-v1.

- `workspace_id` es el SHA-256 hexadecimal completo de la raíz absoluta canónica del workspace, con separadores `/`; alias distintos de la misma raíz producen el mismo id. No se usa rowid ni alias mutable.
- `kind` es `symbol`, `range` o `doc`. `path` es ruta relativa a la raíz con `/` y `symbol` es `qualified_name`, `start-end` para rangos, o `doc_id` para documentos. Para docs sin archivo se usa `_wiki/<doc_id>` como path.
- Cada segmento se codifica por bytes UTF-8 con percent-encoding RFC 3986: solo `A-Z a-z 0-9 - . _ ~` quedan literales; escapes hexadecimales en mayúsculas. El separador entre ruta y símbolo es el único `#` literal. `~<signature_hash completo>` se añade al símbolo cuando nombres cualificados colisionan en el mismo archivo. La identidad incluye siempre workspace y kind; sin esa desambiguación no se emite el ítem.
- La revisión es SHA-256 del texto del rango/símbolo o del documento, normalizado CRLF/CR a LF y sin terminador final. Una identidad sin contenido disponible no recibe revisión ficticia.
- Un id q-v1 recibido se resuelve solo dentro del workspace seleccionado; el workspace dentro del id debe coincidir o se devuelve `stage_failed` (sin acceso cruzado implícito). Si cambió el contenido, se re-resuelve por identidad y devuelve `stale: true`, `id` sin cambios y `rev` actual. Si no existe devuelve `{id, missing:true}` como resultado tipado, no como texto de fallback.
- IDs legacy `s1:`, `r1:` y `d1:` se aceptan únicamente como entrada transicional: se resuelven por su ruta/nombre/doc_id en el workspace explícito, se emite el id q-v1 canónico y `stale:true`. No se generan IDs legacy.

## 4. Gramática y primitivas

```text
pipeline := stage ("|" stage)*                 ; 1–8 etapas
stage := verb arg* option*
option := key "=" value | flag
global := budget=N | max_bytes=N | ws=alias|ruta | fresh | page=cursor
```

Fuentes: `sym <nombre|glob> [kind=] [exact] [repo=]`, `text <patrón> [regex] [path=glob] [type=go]`, `docs [query] [layer=RF]`, `id <id>[,<id>…]`, `diff [ref=HEAD]`, `changed since=<mark>`.

Transformaciones: `edges <refs|callers|callees|impl> [depth=1..3]`, `docs` sin query, `where <campo><op><valor>` (`=`, `!=`, `~`, `!~`; campos `path`, `name`, `kind`, `origin`, `lang`, `layer`), `limit N`, `uniq`, `sort <campo>`.

Salidas: `read [±N|ctx=N|full]`, `fields a,b,…`, `count`. Meta: `describe [verbo|@receta]` con gramática/recetas en ≤600 tokens. Máximo ocho etapas. Las fuentes mantienen su origen y nunca degradan un vacío semántico a resultado de texto no relacionado. Se aceptan alias inequívocos: `edges caller`/`callee`, `read context=N` y `max-bytes=N` equivalen a `callers`/`callees`, `ctx=N` y `max_bytes=N`.

Recetas de uso canónicas (también se prueban literalmente en Go):

- `sym "App.Execute" exact | edges callers depth=2 | read ±3`
- `text "MI_LSP_REFS_TIMEOUT" type=go | limit 5 | read ±2`
- `docs "RF-QRY-001" | limit 3 | read`

Presupuesto por defecto `budget=2000` tokens; máximo 12000, estimado como bytes serializados/4. `fields` por defecto es `id,kind,name,file,line,origin`; `text` solo aparece después de `read`. `edges` limita fan-out a 50 por fuente y 200 por etapa. Orden final/paginable determinista por ruta y línea, con desempate por id.

## 5. Envelope, etapas y errores

Todo resultado contiene `contract_version:"q-v1"`, `operation:"q"`, `ok`, `workspace`, `items`, `stages`, `budget`, `session_mark` y, cuando corresponda, `partial`, `reason`, `fallback_used`, `error`, `continuation` y `truncated`. Cada etapa informa `{verb,in,out,ms,truncated,reason?}`.

El envelope también publica `generation_id`: el snapshot de la generación ya publicada (`last_index`, `active_catalog`, `active_docs`, `active_memory`), el mismo que usan `nav intent` y `nav multi-read`, para que el consumidor cachee contra esa generación. Cada ítem agrega `sensibilidad` desde el frontmatter del archivo; va vacía si no hay marca. mi-lsp no filtra por ese campo. El detalle compartido está en [[RF-QRY-025]].

Los únicos `error.code` terminales q-v1 son `timeout`, `cancelled`, `cursor_invalid`, `cursor_expired`, `snapshot_changed`, `stage_failed` y `max_bytes_item_exceeded`. El error incluye `stage` cuando aplica y detalle acotado/sanitizado. No se inventan códigos alternativos para estas condiciones. Errores de sintaxis/opción inválida usan `stage_failed` con posición de carácter, expectativa concreta y un único ejemplo canónico: `sym "App.Execute" exact | edges callers depth=2 | read ±3`.

- Una falla después de producir resultados preservables devuelve `ok:true`, `partial:true`, `reason` igual al código de la primera falla y `items` de la última salida completa; `stages` registra las etapas realizadas. Sin resultado preservable devuelve `ok:false`. `fallback_used` refleja solo fallback realmente ejecutado.
- Workspace se resuelve en modo de solo lectura. Si falla, se sugiere el alias registrado probable por cwd/ruta; si no está registrado, el detalle indica `mi-lsp workspace add <ruta> --name <alias> --no-index`; si falta el índice, indica `mi-lsp index --workspace <alias>`. No se registra ni reindexa en silencio.
- La precedencia de fallas simultáneas es `cancelled` > `timeout` > `stage_failed`; una falla de cursor o snapshot impide aplicar esa página y se devuelve su código específico. Una etapa degradada registrable no borra resultados previos ni falsea `ok`.
- Cancelación y deadline son end-to-end desde el caller, daemon y etapa; se revisan antes/después de cada etapa y durante fan-out. Default de reloj 10 s; `timeout_ms` admite 1–30000 ms. Al faltar el daemon, el fallback directo mantiene el mismo contrato y marca `route=direct_fallback`.

## 6. Bytes, paginación y snapshot

`max_bytes` (MCP/helper) se mide sobre el envelope JSON UTF-8 completo serializado, incluidos metadatos, etapas, errores y continuación; no solo sobre `items`. Default 262144 bytes, mínimo admitido 1024. La respuesta jamás supera el límite.

El cursor es opaco, autenticado con HMAC y ligado a consulta canónica, workspace_id, generación del índice, orden y posición. TTL: 10 minutos. Alteración o firma incorrecta produce `cursor_invalid`; cursor vencido `cursor_expired`; generación distinta `snapshot_changed`. `page` solo continúa la misma consulta y snapshot. Sin cursor, el primer resultado es página 1.

Si el envelope proyectado supera `max_bytes`, se quitan ítems completos del final y se emite continuación. Si un solo ítem excede el límite, se recorta primero su campo `text` en límite UTF-8, se agrega `truncated_item:true` y se reserva espacio para envelope/diagnósticos. Si aun así no cabe, se omite y se informa `max_bytes_item_exceeded` en `reason`/`error` sin devolver una respuesta mayor al límite. No se corta JSON a mitad de ítem.

## 7. Sesiones y caché

`session_id` explícito identifica el estado; MCP genera uno por proceso solo cuando el caller no lo proporciona. `session_mark` es monotónico por sesión. Se guardan por sesión (LRU máximo 200, idle 30 min) las identidades/revisiones de ítems entregados con texto. `read` repetido puede devolver `{id,rev,seen:true}` sin texto; `fresh` fuerza texto. `changed since=<mark>` lista identidades vistas cuyo contenido cambió, con revisión actual.

La caché/dedupe es opt-in (`dedupe=true`); por defecto nunca altera contenido, orden, estado de sesión ni respuesta. Si se activa, su clave contiene session_id, generación y etapa canónica. Cancelación no deja un resultado parcial cacheado.

## 8. Recetas y compatibilidad

Las recetas son pipelines versionadas `@name`; cada una conserva su expansión declarada y puede referenciar `contract_version`. Las recetas iniciales son `@who-calls`, `@trace`, `@find-def`, `@explain`, `@impact`/`@explain-change`; las `nav_*` existentes permanecen compatibles. La herramienta nueva Go publica `milsp` y su gramática/seis ejemplos en la descripción; no altera respuestas de herramientas anteriores.

Los RF/TP de q-v1 describen esta versión congelada. Cualquier cambio de grammar, identidad, envelope, códigos, cursor, presupuestos o semántica de ejecución exige nueva versión de contrato y negociación explícita; no se corrige silenciosamente bajo q-v1.
