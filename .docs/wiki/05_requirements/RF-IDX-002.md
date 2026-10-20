---
wiki_contract: WIKI-CONTRACT-v1
id: RF-IDX-002
kind: requirement
status: active
title: RF-IDX-002 - Indexar incrementalmente usando git status para detectar archivos modificados
links:
  implements:
  - FL-IDX-01
  code:
  - path: internal/service/app.go
    role: behavior
---

## Propósito

RF-IDX-002 - Indexar incrementalmente usando git status para detectar archivos modificados

### 1. Hoja de ejecución
- **Campo**: ID; **Valor**: RF-IDX-002
- **Campo**: Título; **Valor**: Indexar incrementalmente usando git status para detectar archivos modificados
- **Campo**: Actores; **Valor**: Daemon, Skill, CLI
- **Campo**: Prioridad; **Valor**: alta
- **Campo**: Severidad; **Valor**: media
- **Campo**: FL origen; **Valor**: FL-IDX-01

## Comportamiento ordenado

### 2. Precondiciones detalladas
- **Condición**: Git disponible en workspace; **Tipo**: técnica; **Estado requerido**: preferido
- **Condición**: `index.db` existente; **Tipo**: funcional; **Estado requerido**: obligatorio
- **Condición**: Workspace resoluble; **Tipo**: funcional; **Estado requerido**: obligatorio

### 4. Secuencia principal
1. La CLI recibe workspace name.
2. El indexador verifica disponibilidad de git.
3. Si git disponible, ejecuta `git status --porcelain` para detectar archivos changed/added/deleted.
4. Si el delta toca `.docs/wiki`, `README*`, `docs/`, `.docs/` o `.docs/wiki/_mi-lsp/read-model.toml`, aborta el modo incremental y hace full re-index.
5. Si no, re-indexa solo archivos de código en la lista de cambios.
6. Archivos deletados se eliminan del indice.
7. Actualiza `index.db`, cierra el job y devuelve stats de cambios.

### 7. Casos especiales y variantes
- Si git no esta disponible, fallback a full index con warning.
- `--clean` siempre hace full re-index sin usar git status.
- Archivos nuevos se indexan normalmente.
- Archivos deletados en git se eliminan del indice.
- Cambios en docs o en el `read-model` no intentan incremental parcial; fuerzan full re-index para evitar grafo inconsistente.
- `index start --mode full` puede usar incremental si no hay `--clean`; si no publica generación full, marca la generación candidata como `skipped` y conserva los punteros activos previos.
- `index start --mode catalog` recompone solo catálogo y publica `active_catalog_generation_id`.
- `index start --mode docs` recompone solo docs + memoria y publica `active_docs_generation_id` / `active_memory_generation_id`.
- En `mode docs`, el skip-reparse por `content_hash` aplica solo cuando el job no trae `--clean`; con `--clean` se reparsea todo el contenido documental del alcance sin reutilizar filas previas por hash, manteniendo la publicación atomica y sin borrar el canon en disco.

#### Contrato observable del reparse documental

- **Marcador estable**: RF-IDX-002-DOCS-OBSERVABLE
- **requirement_id**: RF-IDX-002
- **resumen_publico**:
  - **campo**: warnings
  - **formato**: docs_skip_reparse parsed=<entero> skipped=<entero>
  - **disponibilidad**: toda lectura documental completada, incluso con skipped=0 o sin documentos
  - **parsed**: cantidad de rutas con parse exitoso en esta lectura
  - **skipped**: cantidad de rutas reutilizadas del snapshot previo por hash e identidad coincidentes
  - **exclusiones**: errores de lectura o parse y rutas sin documento no incrementan ninguno; conservan sus advertencias
  - **alcance**: resumen existente de docgraph, también visible en otros modos que lo consumen; no es un contador de símbolos ni bloques
  - **salida_docs**: index start --mode docs --wait e index --docs-only devuelven el resumen en warnings
  - **no_persistencia**: index status expone el job, no conserva este resumen terminal; guardar la salida de la ejecución que espera
  - **clean**: omite el snapshot previo; los contadores nunca se deducen del flag ni de docs=N
- **punteros_publicos**:
  - **nav_git_frontier**: generations.catalog y generations.docs leen los punteros activos de catálogo y docs; generations.graph es una identidad distinta
  - **ausencia**: valor null con generation_reasons cuando no está disponible; no equivale a publicación
  - **index_status**: generation_id identifica la generación candidata del job; succeeded no es lectura independiente del puntero activo
  - **wiki_code_context**: doc_generation_id y provenance.docs_generation cuando la superficie documental devuelve este contexto; code_generation_id identifica grafo, no catálogo
  - **workspace_status_full**: doc_count, docs_index_ready, index_symbols e index_ready; memory y memory_pointer pueden aparecer, sin ID de generación de memoria
- **limite_memoria**:
  - **contrato_interno**: el publisher activa active_docs_generation_id y active_memory_generation_id en la misma transacción documental y conserva active_catalog_generation_id
  - **no_observable**: active_memory_generation_id no está expuesto por estas superficies; memory_pointer es un puntero de reentrada, no de generación
  - **aceptacion**: comprobar publicación docs y preservación del catálogo en cada corrida mediante nav git-frontier y navegación del mismo símbolo; no declarar acreditado el puntero interno de memoria
  - **pendiente**: evidencia independiente de generación de memoria, dueño parent_final_verify, vencimiento antes del cierre del objetivo docs-clean-reparse-20261013

### 11. Decisiones cerradas
- Supuestos prohibidos:
  - no asumir que git siempre disponible
  - no fallar si git no esta
- Decisiones cerradas:
  - incremental con git, fallback full
  - docs/profile cambian -> full re-index
  - `--clean` fuerza full sin condiciones
- TODO explicit = 0

## Superficies

CLI `mi-lsp`, con operaciones y selectores detallados en el comportamiento del requisito.

## Entradas/salidas tipadas

### 3. Entradas
- **Campo**: `workspace`; **Tipo**: string; **Req.**: si; **Origen**: CLI; **Validacion**: nombre o path; **RN**: RF-IDX-002
- **Campo**: `--clean`; **Tipo**: booleano; **Req.**: no; **Origen**: CLI; **Validacion**: si true, fuerza full re-index; **RN**: RF-IDX-002
- **Campo**: `--mode`; **Tipo**: enum; **Req.**: no; **Origen**: CLI; **Validacion**: `full`, `docs`, `catalog` en `index start`; **RN**: RF-IDX-002

### 5. Salidas
- **Campo**: `ok`; **Tipo**: bool; **Destino**: usuario/skill; **Efecto observable**: estado de la indexación
- **Campo**: `stats`; **Tipo**: objeto; **Destino**: usuario/skill; **Efecto observable**: files_changed, files_skipped, files_deleted, duration
- **Campo**: `mode`; **Tipo**: string; **Destino**: usuario/skill; **Efecto observable**: `incremental` o `full` (fallback)
- **Campo**: `warnings`; **Tipo**: lista; **Destino**: usuario/skill; **Efecto observable**: git unavailable o docs change con fallback notice

### 8. Impacto en el modelo de datos
- `FileRecord`
- `DocRecord`
- `DocEdge`
- `DocMention`
- `index.db` schema

## Errores

### 6. Errores tipados
- **Código**: `IDX_GIT_UNAVAILABLE`; **Causa**: git no disponible; **Trigger**: git command falla; **Respuesta esperada**: degrade a full index con warning
- **Código**: `IDX_DATABASE_ERROR`; **Causa**: error escribiendo index.db; **Trigger**: lock o I/O error; **Respuesta esperada**: abortar con error explícito
- **Código**: `IDX_WORKSPACE_UNRESOLVED`; **Causa**: workspace no encontrado; **Trigger**: nombre inválido; **Respuesta esperada**: error explícito

## Criterios de aceptación (numerados)

1. Indexar incrementalmente con git disponible: Dado git disponible y archivos modificados Cuando ejecuto "mi-lsp index --workspace myapp" Entonces re-indexo solo archivos changed/added/deleted Y "mode" es "incremental" Y stats reportan números correctos
2. Fallback a full index si git no disponible: Dado git no disponible Cuando ejecuto la indexación Entonces degrade a full re-index con warning Y "mode" es "full" Y operación completa normalmente
3. Forzar full index por cambios documentales: Dado el delta incluye cambios en `.docs/wiki` o `read-model.toml` Cuando ejecuto la indexación incremental Entonces hago full re-index Y "mode" es "full"

### 9. Escenarios de aceptación (Gherkin)
```gherkin
Scenario: Indexar incrementalmente con git disponible
  Given git disponible y archivos modificados
  When ejecuto "mi-lsp index --workspace myapp"
  Then re-indexo solo archivos changed/added/deleted
  And "mode" es "incremental"
  And stats reportan numeros correctos

Scenario: Fallback a full index si git no disponible
  Given git no disponible
  When ejecuto la indexacion
  Then degrade a full re-index con warning
  And "mode" es "full"
  And operacion completa normalmente

Scenario: Forzar full index por cambios documentales
  Given el delta incluye cambios en `.docs/wiki` o `read-model.toml`
  When ejecuto la indexacion incremental
  Then hago full re-index
  And "mode" es "full"
```

### 10. Trazabilidad de pruebas
- Positivo: `TP-IDX / TC-IDX-007`
- Positivo: `TP-IDX / TC-IDX-008`
- Manual pendiente: `TP-IDX / TC-IDX-028` (contadores y punteros públicos; memoria interna no acreditada)
- Positivo: `TP-IDX / TC-IDX-009`
