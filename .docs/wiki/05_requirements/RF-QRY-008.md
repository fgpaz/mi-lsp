---
wiki_contract: WIKI-CONTRACT-v1
id: RF-QRY-008
kind: requirement
status: active
title: RF-QRY-008 - Devolver contexto semantico de simbolos cambiados en un diff git
links:
  implements:
  - FL-QRY-01
  code:
  - path: internal/service/app.go
    role: behavior
---

## Propósito

RF-QRY-008 - Devolver contexto semántico de símbolos cambiados en un diff git

### 1. Hoja de ejecución
- **Campo**: ID; **Valor**: RF-QRY-008
- **Campo**: Título; **Valor**: Devolver contexto semántico de símbolos cambiados en un diff git
- **Campo**: Actores; **Valor**: Skill, Agente, CLI
- **Campo**: Prioridad; **Valor**: media
- **Campo**: Severidad; **Valor**: media
- **Campo**: FL origen; **Valor**: FL-QRY-01

## Comportamiento ordenado

### 2. Precondiciones detalladas
- **Condición**: Repositorio Git disponible; **Tipo**: técnica; **Estado requerido**: obligatorio
- **Condición**: Workspace indexado; **Tipo**: funcional; **Estado requerido**: preferido
- **Condición**: Ref base resoluble (default HEAD); **Tipo**: técnica; **Estado requerido**: obligatorio

### 4. Secuencia principal
1. La CLI recibe ref git (opcional).
2. El core obtiene diff entre HEAD y la ref.
3. Parsea archivos cambiados y extrae símbolos afectados.
4. Resuelve el contexto semántico de cada simbolo.
5. Si `--include-content`, incluye las líneas modificadas.
6. Devuelve DiffContextResult structurado.

### 7. Casos especiales y variantes
- Si git no esta disponible, devolver error o empty context según flag.
- `--include-content` por defecto false para evitar payloads grandes.
- Default a working tree si no se especifica ref.
- El working tree por defecto incluye staged, unstaged y archivos untracked relevantes contra `HEAD`; renames/copies deben mapearse sobre la ruta destino.

### 11. Decisiones cerradas
- Supuestos prohibidos:
  - no asumir que git siempre esta disponible
  - no obligar contenido en respuesta
- Decisiones cerradas:
  - default a working tree
  - content opcional
- TODO explicit = 0
- Fuera de alcance:
  - analisis de impacto dinamico
  - sugerencias de rollback
- Dependencias externas explícitas:
  - git binario local

## Superficies

CLI `mi-lsp`, con operaciones y selectores detallados en el comportamiento del requisito.

## Entradas/salidas tipadas

### 3. Entradas
- **Campo**: `git_ref`; **Tipo**: string; **Req.**: no; **Origen**: CLI; **Validacion**: git ref o branch; default working tree; **RN**: RF-QRY-008
- **Campo**: `--include-content`; **Tipo**: booleano; **Req.**: no; **Origen**: CLI; **Validacion**: si true, incluye líneas modificadas; **RN**: RF-QRY-008
- **Campo**: `--base-ref`; **Tipo**: string; **Req.**: no; **Origen**: CLI; **Validacion**: ref base para comparacion; default HEAD; **RN**: RF-QRY-008

### 5. Salidas
- **Campo**: `ok`; **Tipo**: bool; **Destino**: usuario/skill; **Efecto observable**: estado de la operación
- **Campo**: `diff_context`; **Tipo**: objeto; **Destino**: usuario/skill; **Efecto observable**: DiffContextResult con changed_files, changed_symbols, impact
- **Campo**: `stats`; **Tipo**: objeto; **Destino**: usuario/skill; **Efecto observable**: files_changed, symbols_affected, insertions, deletions
- **Campo**: `warnings`; **Tipo**: lista; **Destino**: usuario/skill; **Efecto observable**: git unavailable, parse failures

### 8. Impacto en el modelo de datos
- `DiffContextResult` (changed_files, changed_symbols, impact)
- `FileChange` (path, op, symbols_affected)
- `SymbolImpact` (symbol, impact_type, context)

## Errores

### 6. Errores tipados
- **Código**: `QRY_GIT_UNAVAILABLE`; **Causa**: git no disponible; **Trigger**: git command falla; **Respuesta esperada**: error explícito con sugerencia
- **Código**: `QRY_NO_CHANGES_DETECTED`; **Causa**: no hay cambios entre refs; **Trigger**: refs identicas; **Respuesta esperada**: warning amistoso, devolver empty result
- **Código**: `QRY_INVALID_REF`; **Causa**: ref no valida; **Trigger**: ref inexistente; **Respuesta esperada**: error explícito

## Criterios de aceptación (numerados)

1. Obtener contexto de cambios sin contenido: Dado cambios sin commitear en un repo git Cuando ejecuto "mi-lsp nav diff-context" Entonces devuelvo archivos cambiados, símbolos afectados, e impacto Y no incluyo líneas de código por defecto
2. Incluir contenido modificado con flag: Dado cambios con --include-content Cuando ejecuto la consulta Entonces devuelvo ademas las líneas modificadas alrededor del cambio Y trunco si payload excede presupuesto
3. Detectar cuando no hay cambios: Dado working tree limpio Cuando ejecuto la consulta Entonces devuelvo warning amistoso Y result vacio pero ok=true

### 9. Escenarios de aceptación (Gherkin)
```gherkin
Scenario: Obtener contexto de cambios sin contenido
  Given cambios sin commitear en un repo git
  When ejecuto "mi-lsp nav diff-context"
  Then devuelvo archivos cambiados, simbolos afectados, e impacto
  And no incluyo lineas de codigo por defecto

Scenario: Incluir contenido modificado con flag
  Given cambios con --include-content
  When ejecuto la consulta
  Then devuelvo ademas las lineas modificadas alrededor del cambio
  And trunco si payload excede presupuesto

Scenario: Detectar cuando no hay cambios
  Given working tree limpio
  When ejecuto la consulta
  Then devuelvo warning amistoso
  And result vacio pero ok=true
```

### 10. Trazabilidad de pruebas
- Positivo: `TP-QRY / TC-QRY-026`
- Positivo: `TP-QRY / TC-QRY-027`
- Negativo: `TP-QRY / TC-QRY-028`
