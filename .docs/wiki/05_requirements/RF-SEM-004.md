---
wiki_contract: WIKI-CONTRACT-v1
id: RF-SEM-004
kind: requirement
status: active
title: RF-SEM-004 - Reordenar candidatos de recall mediante hook local externo y seguro
links:
  implements:
  - FL-SEM-01
  code:
  - path: internal/model/types.go
    role: behavior
  - path: internal/rerank/extension.go
    role: behavior
  - path: internal/service/recall.go
    role: behavior
  - path: internal/service/workspace_ops.go
    role: behavior
---

## Propósito

RF-SEM-004 - Reordenar candidatos de recall mediante hook local externo y seguro

### 1. Hoja de ejecución
- **Campo**: ID; **Valor**: RF-SEM-004
- **Campo**: Título; **Valor**: Reordenar candidatos de recall mediante hook local externo y seguro
- **Campo**: Actores; **Valor**: Usuario, Skill, Agente, CLI/Core, comando local externo
- **Campo**: Prioridad; **Valor**: alta
- **Campo**: Severidad; **Valor**: alta
- **Campo**: FL origen; **Valor**: FL-SEM-01

## Comportamiento ordenado

### 2. Precondiciones detalladas
- **Condición**: Embeddings configurados e indexados; **Tipo**: funcional; **Estado requerido**: obligatorio para invocar rerank
- **Condición**: `[recall.rerank_extension] enabled=true`; **Tipo**: operativa; **Estado requerido**: obligatorio
- **Condición**: `command` local resuelto por el host; **Tipo**: técnica; **Estado requerido**: obligatorio
- **Condición**: Sin dependencia privada en core; **Tipo**: arquitectura; **Estado requerido**: obligatorio

### 4. Secuencia
1. `nav recall` ejecuta primero RF-SEM-003: embedding de query, cosine similarity e intent reranking local.
2. Si embeddings estan deshabilitados o fallan, no invoca el hook y conserva el fallback a `nav wiki search`.
3. Si el hook esta activo, Core serializa stdin JSON versionado con query, `top_n`, candidatos acotados y metadata segura.
4. Core ejecuta `command` con `args` mediante subprocess local sin shell y con timeout.
5. Core parsea stdout JSON versionado con `indices` o `results[].index`.
6. Si la salida es valida, reordena la ventana de candidatos antes del corte final de `max_items`.
7. Si la salida es parcial, coloca primero los índices devueltos y completa faltantes en orden semántico original.
8. Si hay cualquier falla, conserva el orden semántico original y emite warning sanitizado.

### 11. Decisiones cerradas
- El core no implementa clientes privados ni proxies especificos de proveedor para rerank.
- El adaptador privado, si existe, vive fuera de este repositorio y habla el protocolo local.
- `nav search` y `nav recall` no rerankeado siguen siendo fallbacks confiables.
- Cambiar modelo o dimensión de embeddings requiere reindex/reembedding completo de los chunks afectados.

## Superficies

CLI `mi-lsp`, con operaciones y selectores detallados en el comportamiento del requisito.

## Entradas/salidas tipadas

### 3. Entradas
- **Campo**: `enabled`; **Tipo**: bool; **Req.**: si; **Origen**: `.mi-lsp/project.toml [recall.rerank_extension]`; **Validacion**: default false; solo true activa hook; **RN**: RF-SEM-004
- **Campo**: `command`; **Tipo**: string; **Req.**: si; **Origen**: `.mi-lsp/project.toml`; **Validacion**: path o binario local; no se ejecuta via shell; **RN**: RF-SEM-004
- **Campo**: `args`; **Tipo**: lista string; **Req.**: no; **Origen**: `.mi-lsp/project.toml`; **Validacion**: argumentos estaticos, sin interpolacion de secretos; **RN**: RF-SEM-004
- **Campo**: `timeout_ms`; **Tipo**: entero; **Req.**: no; **Origen**: `.mi-lsp/project.toml`; **Validacion**: >0; default 2000; **RN**: RF-SEM-004
- **Campo**: `candidate_count`; **Tipo**: entero; **Req.**: no; **Origen**: `.mi-lsp/project.toml`; **Validacion**: ventana maxima de candidatos enviados; default 50; **RN**: RF-SEM-004
- **Campo**: `top_n`; **Tipo**: entero; **Req.**: no; **Origen**: `.mi-lsp/project.toml`; **Validacion**: cantidad solicitada al hook; clamp a candidatos; **RN**: RF-SEM-004
- **Campo**: `max_snippet_chars`; **Tipo**: entero; **Req.**: no; **Origen**: `.mi-lsp/project.toml`; **Validacion**: límite local de snippet por candidato; default 500; **RN**: RF-SEM-004

### 5. Protocol

Stdin:

```json
{
  "protocol_version": "mi-lsp-rerank-extension-v1",
  "query": "freeform user query",
  "top_n": 5,
  "candidates": [
    {
      "index": 0,
      "archivo": ".docs/wiki/07_technical/TECH-SEMANTIC-RECALL.md",
      "heading": "Algoritmo de busqueda",
      "snippet": "bounded snippet",
      "score": 0.82,
      "start_line": 70,
      "end_line": 90,
      "why": ["semantic_match", "intent_formula"]
    }
  ],
  "metadata": {"source": "nav.recall"}
}
```

Stdout:

```json
{
  "protocol_version": "mi-lsp-rerank-extension-v1",
  "indices": [2, 0, 1],
  "warnings": ["optional provider-local warning"]
}
```

`results: [{"index": 2, "score": 0.91}]` es equivalente cuando el adaptador quiere devolver score externo. Scores no finitos o índices duplicados/fuera de rango invalidan la respuesta.

### 6. Salidas
- **Campo**: `RecallResult.why += external_rerank`; **Tipo**: lista; **Destino**: usuario/skill; **Efecto observable**: indica que el hook reordeno ese candidato
- **Campo**: `warnings`; **Tipo**: lista; **Destino**: usuario/skill; **Efecto observable**: degradaciones sanitizadas, sin payloads ni secretos
- **Campo**: `workspace.status.rerank_extension_enabled`; **Tipo**: bool; **Destino**: usuario/skill; **Efecto observable**: diagnóstico de configuración
- **Campo**: `workspace.status.rerank_extension_mode`; **Tipo**: string; **Destino**: usuario/skill; **Efecto observable**: `local-command` cuando activo

### 8. Impacto en el modelo de datos
- Config local nueva: `[recall.rerank_extension]` dentro de `.mi-lsp/project.toml`.
- Sin tabla SQLite nueva.
- Sin persistencia de payloads de rerank.
- `AccessEvent` conserva solo conteos/warnings existentes; no persiste query/snippets/stdin/stdout del hook.

## Errores

### 7. Failure Semantics

- Missing command, start failure, timeout, exit no cero, stdout vacio, JSON inválido, version inválida, índices duplicados/fuera de rango o scores no finitos preservan el orden semántico original.
- Warnings no incluyen query text, snippets, stdin/stdout completo, stderr, provider responses, tokens, API keys ni rutas secretas.
- El hook nunca se invoca cuando `[embeddings]` esta inactivo o el embedding de query degrada a lexical fallback.

## Criterios de aceptación (numerados)

1. Rerank externo reordena candidatos: Dado embeddings configurados e indexados Y `[recall.rerank_extension] enabled=true` Cuando ejecuto `mi-lsp nav recall "consulta" --workspace mi-lsp --max-items 1` Y el hook devuelve un índice valido distinto del primer candidato semántico Entonces el resultado visible usa ese orden Y `why` incluye `external_rerank`
2. Falla del hook preserva orden: Dado un hook activo que responde JSON inválido Cuando ejecuto `mi-lsp nav recall "consulta sensible" --workspace mi-lsp` Entonces el orden semántico original se conserva Y `warnings` contiene solo una razon sanitizada Y `warnings` no contiene query, snippet, stdout, stderr, tokens ni secretos
3. Embeddings inactivos no invocan rerank: Dado `[embeddings] enabled=false` Y `[recall.rerank_extension] enabled=true` Cuando ejecuto `mi-lsp nav recall "consulta" --workspace mi-lsp` Entonces `items=[]` con hint de embeddings Y no se ejecuta el comando externo

### 9. Criterios de aceptación
```gherkin
Scenario: Rerank externo reordena candidatos
  Given embeddings configurados e indexados
  And `[recall.rerank_extension] enabled=true`
  When ejecuto `mi-lsp nav recall "consulta" --workspace mi-lsp --max-items 1`
  And el hook devuelve un indice valido distinto del primer candidato semantico
  Then el resultado visible usa ese orden
  And `why` incluye `external_rerank`

Scenario: Falla del hook preserva orden
  Given un hook activo que responde JSON invalido
  When ejecuto `mi-lsp nav recall "consulta sensible" --workspace mi-lsp`
  Then el orden semantico original se conserva
  And `warnings` contiene solo una razon sanitizada
  And `warnings` no contiene query, snippet, stdout, stderr, tokens ni secretos

Scenario: Embeddings inactivos no invocan rerank
  Given `[embeddings] enabled=false`
  And `[recall.rerank_extension] enabled=true`
  When ejecuto `mi-lsp nav recall "consulta" --workspace mi-lsp`
  Then `items=[]` con hint de embeddings
  And no se ejecuta el comando externo
```

### 10. Trazabilidad de pruebas
- Positivo: `TP-SEM / TC-SEM-010`
- Positivo: `TP-SEM / TC-SEM-011`
- Negativo: `TP-SEM / TC-SEM-012`
- Negativo: `TP-SEM / TC-SEM-013`
- Negativo: `TP-SEM / TC-SEM-014`
