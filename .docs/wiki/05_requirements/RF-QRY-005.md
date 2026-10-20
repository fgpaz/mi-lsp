---
wiki_contract: WIKI-CONTRACT-v1
id: RF-QRY-005
kind: requirement
status: active
title: RF-QRY-005 - Ejecutar N operaciones heterogeneas en batch con dispatch paralelo
links:
  implements:
  - FL-QRY-01
  code:
  - path: internal/service/app.go
    role: behavior
---

## Propósito

RF-QRY-005 - Ejecutar N operaciones heterogeneas en batch con dispatch paralelo

### 1. Hoja de ejecución
- **Campo**: ID; **Valor**: RF-QRY-005
- **Campo**: Título; **Valor**: Ejecutar N operaciones heterogeneas en batch con dispatch paralelo
- **Campo**: Actores; **Valor**: Skill, Agente, CLI
- **Campo**: Prioridad; **Valor**: alta
- **Campo**: Severidad; **Valor**: media
- **Campo**: FL origen; **Valor**: FL-QRY-01

## Comportamiento ordenado

### 2. Precondiciones detalladas
- **Condición**: Workspace resoluble; **Tipo**: funcional; **Estado requerido**: obligatorio
- **Condición**: Backend semántico disponible; **Tipo**: técnica; **Estado requerido**: obligatorio
- **Condición**: Operaciones soportadas en RF; **Tipo**: funcional; **Estado requerido**: obligatorio

### 4. Secuencia principal
1. La CLI recibe JSON array con N operaciones.
2. Valida que cada operación sea un tipo soportado.
3. Dispatch paralelo hasta `parallelism` workers.
4. Cada operación ejecuta independientemente.
5. Colecta resultados y devuelve BatchResult array.
6. Si algun contexto se cancela, interrumpe gracefully.

### 7. Casos especiales y variantes
- Si una operación falla, devuelve su error pero continua con las demas.
- `ok=true` si al menos una operación completa exitosamente.
- Timeout por operación (default 30s) para evitar bloqueos.

### 11. Decisiones cerradas
- Supuestos prohibidos:
  - no asumir orden de ejecución
  - no garantizar atomicidad transacional
- Decisiones cerradas:
  - dispatch paralelo fino pero sin transacciones globales
  - resultados ordenados por índice original
- TODO explicit = 0
- Fuera de alcance:
  - garantias ACID
  - rollback distribuido
- Dependencias externas explícitas:
  - pool de workers local

## Superficies

CLI `mi-lsp`, con operaciones y selectores detallados en el comportamiento del requisito.

## Entradas/salidas tipadas

### 3. Entradas
- **Campo**: `operations`; **Tipo**: array JSON; **Req.**: si; **Origen**: stdin; **Validacion**: array de objetos {op, params}; **RN**: RF-QRY-005
- **Campo**: `parallelism`; **Tipo**: entero; **Req.**: no; **Origen**: CLI/config; **Validacion**: enteros positivos; default CPU count; **RN**: RF-QRY-005
- **Campo**: `stdin_max_size`; **Tipo**: entero; **Req.**: no; **Origen**: config; **Validacion**: limit en bytes; default 10MB; **RN**: RF-QRY-005

### 5. Salidas
- **Campo**: `ok`; **Tipo**: bool; **Destino**: usuario/skill; **Efecto observable**: estado general (false si algunas operaciones fallan)
- **Campo**: `batch_results`; **Tipo**: array; **Destino**: usuario/skill; **Efecto observable**: array de resultados para cada operación
- **Campo**: `stats`; **Tipo**: objeto; **Destino**: usuario/skill; **Efecto observable**: timing, parallelism_actual, operaciones completadas
- **Campo**: `warnings`; **Tipo**: lista; **Destino**: usuario/skill; **Efecto observable**: errores parciales o degradaciones

### 8. Impacto en el modelo de datos
- `QueryEnvelope`
- `BatchOperation` (op, params)
- `BatchResult` (operation_index, ok, result, error)

## Errores

### 6. Errores tipados
- **Código**: `QRY_INVALID_OPERATION`; **Causa**: tipo de operación no soportado; **Trigger**: op type desconocido; **Respuesta esperada**: rechazar con error tipado
- **Código**: `QRY_STDIN_TOO_LARGE`; **Causa**: payload excede límite; **Trigger**: stdin > 10MB; **Respuesta esperada**: abortar con error
- **Código**: `QRY_CONTEXT_CANCELLED`; **Causa**: contexto cancelado durante ejecución; **Trigger**: timeout/señal; **Respuesta esperada**: interrumpir y devolver resultados parciales

## Criterios de aceptación (numerados)

1. Ejecutar batch con multiples operaciones: Dado un JSON array con 3 operaciones validas diferentes Cuando envio por stdin "mi-lsp nav batch < operations.json" Entonces la respuesta incluye 3 resultados paralelos en "batch_results" Y "stats" reporta tiempo total < suma de tiempos secuenciales
2. Continuar si una operación falla: Dado un batch con 1 operación valida y 1 inválida Cuando ejecuto el batch Entonces "ok" es "true" (al menos una completo) Y devuelvo error tipado para la operación inválida Y la operación valida completa normalmente
3. Rechazar payload demasiado grande: Dado JSON stdin > 10MB Cuando ejecuto batch Entonces fallo con "QRY_STDIN_TOO_LARGE" Y no proceso ninguna operación

### 9. Escenarios de aceptación (Gherkin)
```gherkin
Scenario: Ejecutar batch con multiples operaciones
  Given un JSON array con 3 operaciones validas diferentes
  When envio por stdin "mi-lsp nav batch < operations.json"
  Then la respuesta incluye 3 resultados paralelos en "batch_results"
  And "stats" reporta tiempo total < suma de tiempos secuenciales

Scenario: Continuar si una operacion falla
  Given un batch con 1 operacion valida y 1 invalida
  When ejecuto el batch
  Then "ok" es "true" (al menos una completo)
  And devuelvo error tipado para la operacion invalida
  And la operacion valida completa normalmente

Scenario: Rechazar payload demasiado grande
  Given JSON stdin > 10MB
  When ejecuto batch
  Then fallo con "QRY_STDIN_TOO_LARGE"
  And no proceso ninguna operacion
```

### 10. Trazabilidad de pruebas
- Positivo: `TP-QRY / TC-QRY-017`
- Positivo: `TP-QRY / TC-QRY-018`
- Negativo: `TP-QRY / TC-QRY-019`
