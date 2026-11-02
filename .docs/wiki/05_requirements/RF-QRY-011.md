---
wiki_contract: WIKI-CONTRACT-v1
id: RF-QRY-011
kind: requirement
status: active
title: RF-QRY-011 - Resolver intencion en modo hibrido docs|code con scope opcional de repo
links:
  implements:
  - FL-QRY-01
  code:
  - path: internal/service/app.go
    role: behavior
  - path: internal/service/doc_query_context.go
    role: behavior
  - path: internal/service/intent.go
    role: behavior
  - path: internal/service/affected.go
    role: behavior
---

## Propósito

RF-QRY-011 - Resolver intención en modo hibrido docs|code con scope opcional de repo

### 1. Hoja de ejecución
- **Campo**: ID; **Valor**: RF-QRY-011
- **Campo**: Título; **Valor**: Resolver intención en modo hibrido docs; **Detalle**: code con scope opcional de repo
- **Campo**: Actores; **Valor**: Usuario, Skill, Agente, CLI/Core
- **Campo**: Prioridad; **Valor**: media
- **Campo**: Severidad; **Valor**: media
- **Campo**: FL origen; **Valor**: FL-QRY-01

## Comportamiento ordenado

### 2. Precondiciones detalladas
- **Condición**: Workspace resoluble; **Tipo**: funcional; **Estado requerido**: obligatorio
- **Condición**: Catálogo repo-local disponible; **Tipo**: técnica; **Estado requerido**: obligatorio
- **Condición**: Pregunta no vacia; **Tipo**: funcional; **Estado requerido**: obligatorio

### 3. Secuencia principal
1. La CLI recibe `mi-lsp nav intent <question>`.
2. El core clasifica la pregunta en `mode=docs` o `mode=code`. En un proyecto cinético (par `modelo.md`/`acciones.md` bajo `wiki/`, sin manifiesto de producto) `nav intent` permanece en `mode=docs`.
3. Si la pregunta trae señales de código (ver sección 5) y el catálogo contiene símbolos o archivos que coinciden con sus tokens, el modo es `mixed`: los matches de código fuertes van primero, luego los documentos y al final los matches de código débiles, todo acotado a `top`. El código se rankea por archivo: cada token pesa por su rareza (IDF) y según dónde coincide (nombre de archivo, segmento de ruta, símbolo, padre), con bonus por cubrir más tokens de la pregunta y penalización para archivos de test. Si la pregunta tiene señales de código y el catálogo no está publicado, falta, está vacío o roto, `nav intent` no responde con un vacío silencioso ni con `ok=false`: devuelve `ok=true` con los documentos que encuentre, `degraded=true`, `reason=index_not_ready` (o `index_schema_broken` si la base está corrupta) y un warning que sugiere `mi-lsp nav search <identificador>`. Las preguntas sin señales de código no se marcan.
4. Si el usuario envio `--repo`, el core valida el selector; en `mode=code` acota el universo al repo hijo seleccionado del workspace `container`, y en `mode=docs` puede ignorarlo con warning visible.
5. En `mode=docs`, el sistema usa el scorer owner-aware documental compartido con `nav route/ask/pack`.
6. En `mode=code`, el sistema mantiene el ranking BM25 actual sobre `search_text` enriquecido del catálogo con boosts por nombre/kind.
7. Devuelve un envelope `backend=intent` con `mode=docs|code|mixed`.
8. Como `nav intent` pertenece a la superficie AXI-default, la primera page puede ser más estrecha por default y debe incluir guidance de expansion via `--full` salvo `--classic`.
9. Para una salida implícita consumida por agentes, la CLI presenta el workspace elegido primero y una forma compacta; `--verbose` amplía el detalle. Un `--format` explícito conserva el formato solicitado.
10. Para los candidatos Markdown de `nav.intent`, se permite refrescar hasta cinco paths bajo presupuesto de 500 ms. Si se publica el catálogo, se recarga y vuelve a puntuar una sola vez; ante error/deadline se devuelven los resultados calculados en memoria con warning. El refresco no reconstruye el grafo.

### 5. Casos especiales y variantes
- Si la normalización no produce tokens utiles, responde `ok=true`, `items=[]` y warning.
- Si `mode=docs` y no hay docs indexados fuertes, puede responder desde Tier 1 canonical route con items documentales owner-aware.
- Si `mode=code` y no hay símbolos compatibles, responde `ok=true`, `items=[]` y warning.
- La operación es catalog-first y directa; no depende del daemon.
- En workspaces `container`, `--repo` acota el resultado solo en `mode=code`; en `mode=docs` se valida pero no cambia el lane documental.
- `mode=mixed` (aditivo; `docs` y `code` se mantienen): se activa cuando la pregunta tiene señales léxicas de código (camelCase, snake_case, `a.b`, rutas o archivos `*.go|cs|ts|tsx|js|jsx|py|rs|java`, sustantivos como `function|func|method|struct|class|handler|interface|type`, o patrones `where|donde ... implemented|defined|called|implementa|define|llama`) o cuando un token de la pregunta coincide con un nombre de símbolo o de archivo del catálogo. Sin señales ni coincidencia de catálogo, o sin catálogo, la respuesta de `mode=docs` queda intacta.
- Cada item de `nav.intent` lleva `result_kind=code|doc`; `kind` conserva el tipo de símbolo (`function`, `struct`, etc.) y `origin` es `catalog` para código y `wiki` para documentos. Un match de código es fuerte si su nombre iguala un identificador de la pregunta, o si es `name_match` y cubre al menos dos tokens en nombre, padre o ruta; los fuertes ocupan como máximo `max(1, 2/3 de top)` posiciones antes de los documentos.
- Tokenización de lenguaje natural para código: separa identificadores camelCase y snake_case, descarta stopwords en inglés y español y palabras de menos de tres caracteres, y agrega un stem liviano (`ing`, `ed`, `es`, `s`) como token adicional sin reemplazar la palabra original.
- En AXI efectivo, la semántica del ranking no cambia; solo cambia la disclosure inicial y el guidance de expansion.

## Superficies

CLI `mi-lsp`, con operaciones y selectores detallados en el comportamiento del requisito.

## Entradas/salidas tipadas

### 6. Impacto en el modelo de datos
- `SymbolRecord`
- `QueryEnvelope`
- `QueryOptions`

## Errores

### 4. Errores tipados
- **Código**: `QRY_INTENT_QUESTION_REQUIRED`; **Causa**: falta pregunta; **Trigger**: argumento vacio; **Respuesta esperada**: abortar con error explícito
- **Código**: `QRY_INTENT_WORKSPACE_NOT_FOUND`; **Causa**: workspace inválido; **Trigger**: alias/path no resoluble; **Respuesta esperada**: abortar con `ok=false`
- **Código**: `QRY_INTENT_SCOPE_UNKNOWN`; **Causa**: repo selector inválido; **Trigger**: `--repo` no coincide con un repo hijo; **Respuesta esperada**: devolver `backend=router`, candidatos y `next_hint`

## Criterios de aceptación (numerados)

1. La CLI recibe `mi-lsp nav intent <question>`.
2. El core clasifica la pregunta en `mode=docs` o `mode=code`. En un proyecto cinético (par `modelo.md`/`acciones.md` bajo `wiki/`, sin manifiesto de producto) `nav intent` permanece en `mode=docs`.
3. Si la pregunta trae señales de código (ver sección 5) y el catálogo contiene símbolos o archivos que coinciden con sus tokens, el modo es `mixed`: los matches de código fuertes van primero, luego los documentos y al final los matches de código débiles, todo acotado a `top`. El código se rankea por archivo: cada token pesa por su rareza (IDF) y según dónde coincide (nombre de archivo, segmento de ruta, símbolo, padre), con bonus por cubrir más tokens de la pregunta y penalización para archivos de test. Si la pregunta tiene señales de código y el catálogo no está publicado, falta, está vacío o roto, `nav intent` no responde con un vacío silencioso ni con `ok=false`: devuelve `ok=true` con los documentos que encuentre, `degraded=true`, `reason=index_not_ready` (o `index_schema_broken` si la base está corrupta) y un warning que sugiere `mi-lsp nav search <identificador>`. Las preguntas sin señales de código no se marcan.
4. Si el usuario envio `--repo`, el core valida el selector; en `mode=code` acota el universo al repo hijo seleccionado del workspace `container`, y en `mode=docs` puede ignorarlo con warning visible.
5. En `mode=docs`, el sistema usa el scorer owner-aware documental compartido con `nav route/ask/pack`.
6. En `mode=code`, el sistema mantiene el ranking BM25 actual sobre `search_text` enriquecido del catálogo con boosts por nombre/kind.
7. Devuelve un envelope `backend=intent` con `mode=docs|code|mixed`.
8. Como `nav intent` pertenece a la superficie AXI-default, la primera page puede ser más estrecha por default y debe incluir guidance de expansion via `--full` salvo `--classic`.
9. Para una salida implícita consumida por agentes, la CLI presenta el workspace elegido primero y una forma compacta; `--verbose` amplía el detalle. Un `--format` explícito conserva el formato solicitado.
10. Para los candidatos Markdown de `nav.intent`, se permite refrescar hasta cinco paths bajo presupuesto de 500 ms. Si se publica el catálogo, se recarga y vuelve a puntuar una sola vez; ante error/deadline se devuelven los resultados calculados en memoria con warning. El refresco no reconstruye el grafo.

### 7. Trazabilidad de pruebas
- Positivo: `TP-QRY / TC-QRY-040`
- Positivo: `TP-QRY / TC-QRY-044`
- Positivo: `TP-QRY / TC-QRY-045`
- Positivo: `TP-QRY / TC-QRY-179`
- Positivo: `TP-QRY / TC-QRY-181`
- Negativo: `TP-QRY / TC-QRY-041`
- Negativo: `TP-QRY / TC-QRY-180`

```yaml
harness_protocol: SDD-HARNESS-v1
id: RF-QRY-011
kind: requirement
audience: llm-first
imports:
- .docs/wiki/04_flows/FL-QRY-01.md
exports:
- RF-QRY-011
- RF-QRY-011 - Resolver intencion en modo hibrido docs|code con scope opcional de
  repo
agent_must_read:
- .docs/wiki/05_requirements/RF-QRY-011.md
- .docs/wiki/04_flows/FL-QRY-01.md
- .docs/wiki/06_tests/TP-QRY.md
agent_may_edit:
- .docs/wiki/05_requirements/RF-QRY-011.md
agent_must_not_edit:
- .docs/wiki/00_governance.md
- AGENTS.md
- internal/service/app.go
- internal/service/doc_query_context.go
- internal/service/intent.go
- internal/service/affected.go
verify:
- 'Para los casos de este requisito en .docs/wiki/06_tests/TP-QRY.md, ejecutar el
  comando publicado: go test ./internal/model ./internal/service ./internal/cli ./internal/indexer
  (la evidencia de ejecución debe registrarse por separado).'
stop_if:
- Detenerse si la propuesta requiere alterar criterios, decisiones o comportamiento
  más allá de RF-QRY-011.
evidence:
- .docs/wiki/05_requirements/RF-QRY-011.md
- .docs/wiki/06_tests/TP-QRY.md
```
