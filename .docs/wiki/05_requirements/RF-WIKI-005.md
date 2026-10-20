---
wiki_contract: WIKI-CONTRACT-v1
id: RF-WIKI-005
kind: requirement
status: active
title: RF-WIKI-005 - Empaquetar documentacion canon para tarea en todos los workspaces
links:
  implements:
  - FL-WIKI-01
  code:
  - path: internal/service/app.go
    role: behavior
---

## Propósito

RF-WIKI-005 - Empaquetar documentacion canon para tarea en todos los workspaces

### 1. Hoja de ejecución
- **Campo**: ID; **Valor**: RF-WIKI-005
- **Campo**: Título; **Valor**: Empaquetar documentacion canon para tarea en todos los workspaces
- **Campo**: Actores; **Valor**: Usuario, Skill, Agente, CLI/Core
- **Campo**: Prioridad; **Valor**: alta
- **Campo**: Severidad; **Valor**: media
- **Campo**: FL origen; **Valor**: FL-WIKI-01

## Comportamiento ordenado

### 2. Precondiciones detalladas
- **Condición**: Al menos un workspace registrado con `docs_ready=true`; **Tipo**: funcional; **Estado requerido**: obligatorio
- **Condición**: `~/.mi-lsp/registry.toml` accesible; **Tipo**: operativa; **Estado requerido**: obligatorio
- **Condición**: Flag `--all-workspaces` presente; **Tipo**: técnica; **Estado requerido**: obligatorio

### 4. Secuencia principal
1. La CLI recibe `nav wiki pack <task_id|scope> --all-workspaces [--rf|--fl|--doc] [--format X]`.
2. El core itera sobre workspaces registrados con semaphore=4.
3. Para cada workspace, ejecuta el packager local que colecta documentos canónicos para la tarea.
4. Packager respeta filtros opcionales (`--rf`, `--fl`, `--doc`) si estan presentes.
5. Retorna N mini-packs en el envelope, uno por workspace.
6. **Importante**: `pack --all-workspaces` devuelve N items (packs separados), NO un super-pack mergeado.
7. Hermes (consumidor) decide si los une, mantiene separados, o filtra.
8. La CLI devuelve resultado en formato solicitado.

### 7. Casos especiales y variantes
- Devuelve N items en el envelope, **NO un super-pack**, cada uno es independiente.
- Con `--rf`, `--fl`, `--doc`, packager filtra documentos dentro de cada workspace (no globaliza el filtro).
- Si dos workspaces tienen el mismo task_id, ambos retornan sus propios packs en el array `items[]`.
- Si un pack esta vacio, continuar con otros y reportar warning, no error fatal.
- Mini-pack payload nominal 5-50KB por workspace, según cantidad de documentos.

### 11. Decisiones cerradas
- Supuestos prohibidos:
  - no fusionar packs entre workspaces
  - no omitir campo `workspace` en items
  - no cambiar semántica de filtros sin actualizar CT
- Decisiones cerradas:
  - `pack --all-workspaces` retorna N mini-packs, nunca un super-pack
  - CT-NAV-WIKI documenta esto explicitamente
  - semaphore=4 para fan-out
  - timeout 30s por workspace
- TODO explicit = 0
- Fuera de alcance:
  - fusion de packs (responsabilidad de consumidor Hermes)
  - sincronización de contenido entre workspaces

## Superficies

CLI `mi-lsp`, con operaciones y selectores detallados en el comportamiento del requisito.

## Entradas/salidas tipadas

### 3. Entradas
- **Campo**: `task_id` o `scope`; **Tipo**: string; **Req.**: si; **Origen**: CLI; **Validacion**: identificador de tarea o scope (RF-*, TP-*, etc.); **RN**: RF-WIKI-005
- **Campo**: `--all-workspaces`; **Tipo**: bool; **Req.**: si; **Origen**: CLI; **Validacion**: flag presente; **RN**: RF-WIKI-005
- **Campo**: `--rf`, `--fl`, `--doc`; **Tipo**: enum; **Req.**: no; **Origen**: CLI; **Validacion**: filtros opcionales de scope; **RN**: RF-WIKI-005
- **Campo**: `format`; **Tipo**: enum; **Req.**: no; **Origen**: CLI; **Validacion**: `compact`, `json`, `text`, `toon` o `yaml`; default `compact`; **RN**: RF-WIKI-005

### 5. Salidas
- **Campo**: `ok`; **Tipo**: bool; **Destino**: usuario/skill; **Efecto observable**: estado de la operación
- **Campo**: `items`; **Tipo**: lista; **Destino**: usuario/skill; **Efecto observable**: array con N mini-packs (uno por workspace)
- **Campo**: `workspace`; **Tipo**: string; **Destino**: usuario/skill; **Efecto observable**: workspace de origen del pack
- **Campo**: `task_id`; **Tipo**: string; **Destino**: usuario/skill; **Efecto observable**: tarea identificada
- **Campo**: `pack`; **Tipo**: objeto; **Destino**: usuario/skill; **Efecto observable**: documentos coleccionados: {root_docs[], related_docs[], evidence_paths[]}
- **Campo**: `pack_size_bytes`; **Tipo**: número; **Destino**: usuario/skill; **Efecto observable**: tamaño aproximado del mini-pack
- **Campo**: `warnings`; **Tipo**: lista; **Destino**: usuario/skill; **Efecto observable**: workspaces donde pack estuvo vacio o incompleto
- **Campo**: `stats`; **Tipo**: objeto; **Destino**: usuario/skill; **Efecto observable**: `workspaces_queried`, `workspaces_with_pack`, `workspaces_failed`, `total_items`

### 8. Impacto en el modelo de datos
- `QueryEnvelope` con array de items; cada item contiene: workspace, task_id, pack{root_docs[], related_docs[], evidence_paths[]}, pack_size_bytes
- Estadisticas globales: `workspaces_queried`, `workspaces_with_pack`, `workspaces_failed`, `total_items`

## Errores

### 6. Errores tipados
- **Código**: `WIKI_PACK_NOT_FOUND`; **Causa**: task_id no resolvio a documentos; **Trigger**: tarea inexistente; **Respuesta esperada**: incluir en `workspaces_failed[]` con razon
- **Código**: `WIKI_PACK_EMPTY`; **Causa**: packager colecciono cero documentos; **Trigger**: scope muy restrictivo; **Respuesta esperada**: warning en item, pack vacio pero continuar
- **Código**: `WIKI_PACK_TIMEOUT`; **Causa**: empaquetado > 30s; **Trigger**: workspace muy grande; **Respuesta esperada**: timeout error, workspace en `workspaces_failed[]`

## Criterios de aceptación (numerados)

1. Empaquetar documentacion de N workspaces por separado: Dado una tarea RF-WIKI-001 en dos workspaces Cuando ejecuto "mi-lsp nav wiki pack RF-WIKI-001 --all-workspaces --format toon" Entonces retorna items con dos packs distintos Y cada pack contiene documentos del workspace respectivo Y NO hay fusion ni super-pack
2. Respetar filtros por scope: Dado una tarea compleja con multiples capas Cuando ejecuto "nav wiki pack <task> --all-workspaces --rf" Entonces cada pack contiene solo documentos RF Y root_docs[] incluye solo RF-* Y related_docs[] puede incluir FL si hay imports
3. Reportar pack vacio sin error: Dado una tarea con scope muy restrictivo Cuando ejecuto "nav wiki pack <task> --all-workspaces --doc" Entonces si algun workspace tiene pack vacio, entra a warnings Y stats.workspaces_with_pack refleja cuantos tuvieron contenido

### 9. Escenarios de aceptación (Gherkin)
```gherkin
Scenario: Empaquetar documentacion de N workspaces por separado
  Given una tarea RF-WIKI-001 en dos workspaces
  When ejecuto "mi-lsp nav wiki pack RF-WIKI-001 --all-workspaces --format toon"
  Then retorna items con dos packs distintos
  And cada pack contiene documentos del workspace respectivo
  And NO hay fusion ni super-pack

Scenario: Respetar filtros por scope
  Given una tarea compleja con multiples capas
  When ejecuto "nav wiki pack <task> --all-workspaces --rf"
  Then cada pack contiene solo documentos RF
  And root_docs[] incluye solo RF-*
  And related_docs[] puede incluir FL si hay imports

Scenario: Reportar pack vacio sin error
  Given una tarea con scope muy restrictivo
  When ejecuto "nav wiki pack <task> --all-workspaces --doc"
  Then si algun workspace tiene pack vacio, entra a warnings
  And stats.workspaces_with_pack refleja cuantos tuvieron contenido
```

### 10. Trazabilidad de pruebas
- Positivo: `TP-WIKI / TC-WIKI-014`
- Positivo: `TP-WIKI / TC-WIKI-015`
- Negativo: `TP-WIKI / TC-WIKI-016`
