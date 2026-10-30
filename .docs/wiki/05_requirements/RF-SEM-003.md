---
wiki_contract: WIKI-CONTRACT-v1
id: RF-SEM-003
kind: requirement
status: active
title: RF-SEM-003 - Recuperar secciones por significado con nav recall (ungated, knowledge-wiki)
links:
  implements:
  - FL-SEM-01
  code:
  - path: internal/service/app.go
    role: behavior
---

## Propósito

RF-SEM-003 - Recuperar secciones por significado con nav recall (ungated, knowledge-wiki)

### 1. Hoja de ejecución
- **Campo**: ID; **Valor**: RF-SEM-003
- **Campo**: Título; **Valor**: Recuperar secciones por significado con nav recall (ungated, knowledge-wiki)
- **Campo**: Actores; **Valor**: Usuario, Skill, Agente, CLI/Core
- **Campo**: Prioridad; **Valor**: alta
- **Campo**: Severidad; **Valor**: alta
- **Campo**: FL origen; **Valor**: FL-SEM-01

## Comportamiento ordenado

### 2. Precondiciones detalladas
- **Condición**: Workspace con embeddings indexados (RF-SEM-002); **Tipo**: funcional; **Estado requerido**: obligatorio para ranking vectorial
- **Condición**: Config embeddings cargada (RF-SEM-001); **Tipo**: técnica; **Estado requerido**: obligatorio para ranking vectorial
- **Condición**: Perfil knowledge-wiki efectivo (no bloqueado por gobernanza); **Tipo**: operativa; **Estado requerido**: obligatorio
- **Condición**: Sin requirement de governance gate (ungated recall); **Tipo**: semántica; **Estado requerido**: obligatorio

### 4. Secuencia principal
1. CLI recibe `mi-lsp nav recall "<query>" --workspace <alias> [--intent formula|evidence|route|explore|learning]`.
2. Core carga config embeddings (RF-SEM-001).
3. Core normaliza `--intent` y enriquece el texto de query para sesgar el ranking.
4. Core solicita embedding de la query enriquecida al backend (timeout `timeout_ms`).
5. Si embedding disponible, busca TOP-K similitud cosine en tabla `wiki_chunk_embeddings`.
6. Aplica reranking por intención sobre metadata, path, doc title, heading y snippet.
7. Si RF-SEM-004 esta configurado, puede reordenar una ventana acotada mediante hook local externo antes del corte final.
8. Si backend no disponible, emite warning/hint y la guia documental segura es `mi-lsp nav wiki search`, no modelo local oculto.
9. Aplica `--max-items` y `--token-budget`.
10. Formatea resultados como `RecallResult` con `intent`, `archivo`, `heading`, `score`, `snippet`, `start_line`, `end_line` y `why`.
11. Devuelve envelope estable (no gated por gobernanza).

### 7. Casos especiales y variantes
- Si query es muy corta (<3 caracteres), sugiere expansion en `next_hint`.
- Si backend disponible pero timeout, devuelve warning/hint y recomienda `nav wiki search`.
- Intent desconocido se normaliza a `explore`.
- El modelo de embeddings descubre candidatos; no convierte material route-only en fuente final.
- `intent=route` prioriza perfiles/routing para despacho, pero el agente debe volver a `formula` o `evidence` para citar fuentes finales.
- Soporta bilingual: query ES se embebe directo o se traduce según backend capability.
- Si el hook externo de RF-SEM-004 falla, conserva el orden semántico e informa warning sanitizado.
- Sin gobernanza gate (ungated): mismo usuario puede recuperar spec docs, implementation docs, o knowledge wiki sin bloqueo.

### 11. Decisiones cerradas
- Supuestos prohibidos:
  - no asumir gobernanza gate (ungated recall)
  - no exponer tokens de embeddings raw en output
  - no filtrar resultados por RF existencia (knowledge-wiki es agnóstico)
- Decisiones cerradas:
  - fallback documental seguro es `nav wiki search`
  - TOP-K cosine similarity ranking
  - query embedding temporal (no cacheado globalmente)
  - soporte ES/EN bilingual (backend-dependent)
  - embeddings descubren candidatos; source-of-truth final sigue siendo wiki canonica/source-grounded
  - rerank externo es opcional, local y fail-open hacia el orden semántico original
- TODO explicit = 0
- Fuera de alcance:
  - multi-workspace recall (future, similar a FL-WIKI-01)
  - cache de query embeddings (future)

## Superficies

CLI `mi-lsp`, con operaciones y selectores detallados en el comportamiento del requisito.

## Entradas/salidas tipadas

### 3. Entradas
- **Campo**: `query`; **Tipo**: string; **Req.**: si; **Origen**: CLI; **Validacion**: natural language, ES o EN soportado; **RN**: RF-SEM-003
- **Campo**: `--workspace`; **Tipo**: string; **Req.**: si; **Origen**: CLI; **Validacion**: alias workspace registrado; **RN**: RF-SEM-003
- **Campo**: `--max-items`; **Tipo**: entero; **Req.**: no; **Origen**: CLI global; **Validacion**: límite de resultados; default global del CLI; **RN**: RF-SEM-003
- **Campo**: `--token-budget`; **Tipo**: entero; **Req.**: no; **Origen**: CLI global; **Validacion**: presupuesto aproximado de salida; **RN**: RF-SEM-003
- **Campo**: `--map`; **Tipo**: bool; **Req.**: no; **Origen**: CLI; **Validacion**: agrupa resultados en mapa compacto; **RN**: RF-SEM-003
- **Campo**: `--intent`; **Tipo**: enum; **Req.**: no; **Origen**: CLI; **Validacion**: `formula`, `evidence`, `route`, `explore`, `learning`; default `explore`; **RN**: RF-SEM-003
- **Campo**: `format`; **Tipo**: enum; **Req.**: no; **Origen**: CLI; **Validacion**: `compact`, `json`, `text`, `toon` o `yaml`; default `compact`; **RN**: RF-SEM-003

Guia de intenciones:

- `formula`: recuperar contratos, formulas, unidades, fixtures, rangos y stop conditions source-grounded.
- `evidence`: recuperar fuentes canónicas, source IDs, matrices de evidencia y punteros de seccion.
- `route`: recuperar perfiles o notas de ruteo para decidir quien atiende; estos hits no son fuente final.
- `explore`: explorar contexto amplio, sintesis, índices y decisiones previas.
- `learning`: recuperar aprendizajes durables, memoria operativa y reglas de mejora.

### 5. Salidas
- **Campo**: `ok`; **Tipo**: bool; **Destino**: usuario/skill; **Efecto observable**: operación exitosa
- **Campo**: `items`; **Tipo**: lista; **Destino**: usuario/skill; **Efecto observable**: array `RecallResult[]` con `intent`, `archivo`, `heading`, `snippet`, `score`, líneas y `why`
- **Campo**: `backend`; **Tipo**: string; **Destino**: usuario/skill; **Efecto observable**: `recall` si usa semantic; fallback documental recomendado `nav wiki search`
- **Campo**: `truncated`; **Tipo**: bool; **Destino**: usuario/skill; **Efecto observable**: hay más resultados disponibles
- **Campo**: `warnings`; **Tipo**: lista; **Destino**: usuario/skill; **Efecto observable**: diagnóstico de fallback activado, slow query, etc.
- **Campo**: `hint`; **Tipo**: string; **Destino**: usuario/skill; **Efecto observable**: explica intención efectiva o fallback
- **Campo**: `stats`; **Tipo**: objeto; **Destino**: usuario/skill; **Efecto observable**: conteos y presupuesto de salida

### 8. Impacto en el modelo de datos
- Query embedding temporal en memoria (no persistente)
- Consulta sobre `wiki_chunk_embeddings` con cosine similarity

## Errores

### 6. Errores tipados
- **Código**: `SEM_WORKSPACE_UNRESOLVED`; **Causa**: alias inválido; **Trigger**: `--workspace` no existe; **Respuesta esperada**: abortar con error explícito
- **Código**: `SEM_QUERY_TIMEOUT`; **Causa**: query embedding agota timeout; **Trigger**: timeout_ms excedido; **Respuesta esperada**: warning + guidance a `nav wiki search`
- **Código**: `SEM_NO_RESULTS`; **Causa**: ninguna seccion coincide; **Trigger**: query muy especifica o índice vacio; **Respuesta esperada**: retorna items=[] con hint accionable

## Criterios de aceptación (numerados)

1. Recuperar secciones por significado con backend: Dado un workspace con embeddings indexados Cuando ejecuto "mi-lsp nav recall 'como configurar embeddings' --workspace mi-lsp --max-items 5" Entonces retorna items con relevancia ordenada Y cada item tiene "intent", "heading", "archivo", "score" Y backend = "recall"
2. Recomendar fallback wiki si backend no disponible: Dado backend embeddings no disponible Cuando ejecuto "mi-lsp nav recall 'configurar embeddings' --workspace mi-lsp" Entonces warning indica provider no disponible Y hint recomienda `mi-lsp nav wiki search`
3. Soportar bilingual ES↔EN: Dado workspace con docs en Español e Ingles Cuando ejecuto "mi-lsp nav recall 'embeddings' --workspace mi-lsp" (EN) Entonces retorna sections de ambos idiomas si son semanticamente relacionadas Y cuando ejecuto misma query con termino ES equivalente, resultados similares
4. Diferenciar route de formula: Dado workspace con un contrato de formula y un perfil route-only Cuando ejecuto "mi-lsp nav recall 'formula hops' --workspace mi-lsp --intent formula" Entonces el contrato de formula queda por encima del perfil route-only Cuando ejecuto "mi-lsp nav recall 'formula hops' --workspace mi-lsp --intent route" Entonces el perfil route-only puede subir para despacho Y el hint indica que route no es fuente final

### 9. Escenarios de aceptación (Gherkin)
```gherkin
Scenario: Recuperar secciones por significado con backend
  Given un workspace con embeddings indexados
  When ejecuto "mi-lsp nav recall 'como configurar embeddings' --workspace mi-lsp --max-items 5"
  Then retorna items con relevancia ordenada
  And cada item tiene "intent", "heading", "archivo", "score"
  And backend = "recall"

Scenario: Recomendar fallback wiki si backend no disponible
  Given backend embeddings no disponible
  When ejecuto "mi-lsp nav recall 'configurar embeddings' --workspace mi-lsp"
  Then warning indica provider no disponible
  And hint recomienda `mi-lsp nav wiki search`

Scenario: Soportar bilingual ES↔EN
  Given workspace con docs en Español e Ingles
  When ejecuto "mi-lsp nav recall 'embeddings' --workspace mi-lsp" (EN)
  Then retorna sections de ambos idiomas si son semanticamente relacionadas
  And cuando ejecuto misma query con termino ES equivalente, resultados similares

Scenario: Diferenciar route de formula
  Given workspace con un contrato de formula y un perfil route-only
  When ejecuto "mi-lsp nav recall 'formula hops' --workspace mi-lsp --intent formula"
  Then el contrato de formula queda por encima del perfil route-only
  When ejecuto "mi-lsp nav recall 'formula hops' --workspace mi-lsp --intent route"
  Then el perfil route-only puede subir para despacho
  And el hint indica que route no es fuente final
```

### 10. Trazabilidad de pruebas
- Positivo: `TP-SEM / TC-SEM-007`
- Positivo: `TP-SEM / TC-SEM-008`
- Negativo: `TP-SEM / TC-SEM-009`

```yaml
harness_protocol: SDD-HARNESS-v1
id: RF-SEM-003
kind: requirement
audience: llm-first
imports:
- .docs/wiki/04_flows/FL-SEM-01.md
exports:
- RF-SEM-003
- RF-SEM-003 - Recuperar secciones por significado con nav recall (ungated, knowledge-wiki)
agent_must_read:
- .docs/wiki/05_requirements/RF-SEM-003.md
- .docs/wiki/04_flows/FL-SEM-01.md
- .docs/wiki/06_tests/TP-SEM.md
agent_may_edit:
- .docs/wiki/05_requirements/RF-SEM-003.md
agent_must_not_edit:
- .docs/wiki/00_governance.md
- AGENTS.md
- internal/service/app.go
verify:
- Validar el contrato y sus referencias con `mi-lsp nav wiki validate-harness --workspace
  <alias>`; contrastar los casos de .docs/wiki/06_tests/TP-SEM.md sin inferir que
  fueron ejecutados.
stop_if:
- 'Detenerse y escalar si el cambio contradice el límite documentado: - Supuestos
  prohibidos:'
evidence:
- .docs/wiki/05_requirements/RF-SEM-003.md
- .docs/wiki/06_tests/TP-SEM.md
```
