---
wiki_contract: WIKI-CONTRACT-v1
id: RF-QRY-006
kind: requirement
status: active
title: RF-QRY-006 - Devolver vecindario semantico de un simbolo
links:
  implements:
  - FL-QRY-01
  code:
  - path: internal/service/app.go
    role: behavior
---

## Propósito

RF-QRY-006 - Devolver vecindario semántico de un símbolo

### 1. Hoja de ejecución
- **Campo**: ID; **Valor**: RF-QRY-006
- **Campo**: Título; **Valor**: Devolver vecindario semántico de un símbolo (definicion, callers, implementors, tests)
- **Campo**: Actores; **Valor**: Skill, Agente, CLI
- **Campo**: Prioridad; **Valor**: alta
- **Campo**: Severidad; **Valor**: media
- **Campo**: FL origen; **Valor**: FL-QRY-01

## Comportamiento ordenado

### 2. Precondiciones detalladas
- **Condición**: Workspace indexado; **Tipo**: funcional; **Estado requerido**: obligatorio
- **Condición**: Símbolo existe en catálogo; **Tipo**: funcional; **Estado requerido**: obligatorio
- **Condición**: Backend semántico disponible; **Tipo**: técnica; **Estado requerido**: preferido; graceful degradation

### 4. Secuencia principal
1. La CLI recibe un nombre de simbolo.
2. El core busca en el catálogo sintactico.
3. Resuelve la definicion del simbolo.
4. Encuentra callers (referencias), implementors (subclases/interfaces), tests (referencias en test files).
5. Aplica `--depth` para limitar la expansion transitive.
6. Devuelve `SymbolNeighborhood` con todos los vecinos.

### 7. Casos especiales y variantes
- Si el backend semántico falla, continua con sintactico.
- Profundidad 0 = solo definicion.
- Profundidad 1 = definicion + directos callers/implementors.
- Profundidad 2+ = expansion transitive limitada.

### 11. Decisiones cerradas
- Supuestos prohibidos:
  - no garantizar transitividad completa
  - no asumir que backend semántico esta disponible
- Decisiones cerradas:
  - profundidad limitada para evitar explosion
  - graceful degrade a sintactico
- TODO explicit = 0
- Fuera de alcance:
  - analisis de flujo de datos
  - rastreo transitive completo
- Dependencias externas explícitas:
  - catálogo sintactico local
  - backend semántico opcional (Roslyn, tsserver, pyright, gopls, rust-analyzer); Go conserva catálogo AST nativo y Rust un catálogo lexical cuando sus backends semánticos no estan disponibles

## Superficies

CLI `mi-lsp`, con operaciones y selectores detallados en el comportamiento del requisito.

## Entradas/salidas tipadas

### 3. Entradas
- **Campo**: `symbol`; **Tipo**: string; **Req.**: si; **Origen**: CLI; **Validacion**: nombre valido de símbolo; **RN**: RF-QRY-006
- **Campo**: `--depth`; **Tipo**: entero; **Req.**: no; **Origen**: CLI; **Validacion**: 0-3; default 1; **RN**: RF-QRY-006
- **Campo**: `--language`; **Tipo**: enum; **Req.**: no; **Origen**: CLI; **Validacion**: `python`, `typescript`, `csharp`; infer si no se informa; **RN**: RF-QRY-006

### 5. Salidas
- **Campo**: `ok`; **Tipo**: bool; **Destino**: usuario/skill; **Efecto observable**: estado de la operación
- **Campo**: `neighborhood`; **Tipo**: objeto; **Destino**: usuario/skill; **Efecto observable**: SymbolNeighborhood con definition, callers, implementors, tests
- **Campo**: `depth_applied`; **Tipo**: entero; **Destino**: usuario/skill; **Efecto observable**: profundidad efectiva usada
- **Campo**: `backend`; **Tipo**: string; **Destino**: usuario/skill; **Efecto observable**: sintactico o semántico
- **Campo**: `warnings`; **Tipo**: lista; **Destino**: usuario/skill; **Efecto observable**: degradaciones si backend semántico no disponible

### 8. Impacto en el modelo de datos
- `SymbolNeighborhood` (definition, callers, implementors, tests)
- `SymbolRecord`

## Errores

### 6. Errores tipados
- **Código**: `QRY_SYMBOL_NOT_FOUND`; **Causa**: símbolo no en catálogo; **Trigger**: nombre inexistente; **Respuesta esperada**: devolver error con sugerencia de busqueda
- **Código**: `QRY_SEMANTIC_UNAVAILABLE`; **Causa**: backend semántico no disponible; **Trigger**: backend proceso no responde; **Respuesta esperada**: degrade a sintactico con warning
- **Código**: `QRY_INVALID_DEPTH`; **Causa**: profundidad inválida; **Trigger**: depth < 0 o > 3; **Respuesta esperada**: rechazar con valor default 1

## Criterios de aceptación (numerados)

1. Obtener vecindario semántico con profundidad 1: Dado un símbolo valido en el catálogo Cuando ejecuto "mi-lsp nav related --symbol MyClass --depth 1" Entonces la respuesta incluye definicion, callers directos, implementors directos, tests Y no expansion transitive Y "backend" indica semántico o sintactico
2. Degrade graceful si semantic backend falla: Dado backend semántico no disponible Cuando ejecuto la consulta Entonces la respuesta incluye definicion sintactica Y warning explico degradacion Y callers/implementors basados en texto
3. Rechazar símbolo no encontrado: Dado nombre de símbolo inexistente Cuando ejecuto la consulta Entonces fallo con "QRY_SYMBOL_NOT_FOUND" Y sugerir busqueda textual alternativa

### 9. Escenarios de aceptación (Gherkin)
```gherkin
Scenario: Obtener vecindario semantico con profundidad 1
  Given un simbolo valido en el catalogo
  When ejecuto "mi-lsp nav related --symbol MyClass --depth 1"
  Then la respuesta incluye definicion, callers directos, implementors directos, tests
  And no expansion transitive
  And "backend" indica semantico o sintactico

Scenario: Degrade graceful si semantic backend falla
  Given backend semantico no disponible
  When ejecuto la consulta
  Then la respuesta incluye definicion sintactica
  And warning explico degradacion
  And callers/implementors basados en texto

Scenario: Rechazar simbolo no encontrado
  Given nombre de simbolo inexistente
  When ejecuto la consulta
  Then fallo con "QRY_SYMBOL_NOT_FOUND"
  And sugerir busqueda textual alternativa
```

### 10. Trazabilidad de pruebas
- Positivo: `TP-QRY / TC-QRY-020`
- Positivo: `TP-QRY / TC-QRY-021`
- Negativo: `TP-QRY / TC-QRY-022`
