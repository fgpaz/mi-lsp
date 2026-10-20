---
wiki_contract: WIKI-CONTRACT-v1
id: RF-DAE-003
kind: requirement
status: active
title: RF-DAE-003 - Auto-iniciar daemon en la primera consulta semantica
links:
  implements:
  - FL-DAE-01
  code:
  - path: internal/service/app.go
    role: behavior
---

## Propósito

RF-DAE-003 - Auto-iniciar daemon en la primera consulta semántica

### 1. Hoja de ejecución
- **Campo**: ID; **Valor**: RF-DAE-003
- **Campo**: Título; **Valor**: Auto-iniciar daemon en la primera consulta semántica si no esta corriendo
- **Campo**: Actores; **Valor**: CLI, Daemon, Skill, Agente
- **Campo**: Prioridad; **Valor**: alta
- **Campo**: Severidad; **Valor**: media
- **Campo**: FL origen; **Valor**: FL-DAE-01

## Comportamiento ordenado

### 2. Precondiciones detalladas
- **Condición**: Binario daemon disponible; **Tipo**: técnica; **Estado requerido**: obligatorio
- **Condición**: Consulta semántica valida; **Tipo**: funcional; **Estado requerido**: obligatorio
- **Condición**: Home del usuario escribible; **Tipo**: operativa; **Estado requerido**: obligatorio

### 4. Secuencia principal
1. La CLI recibe una consulta semántica (refs, context, etc).
2. Verifica si daemon esta vivo (health check).
3. Si daemon vivo, enruta la consulta al daemon.
4. Si no vivo y sin `--no-auto-daemon`, intenta iniciar daemon en background.
5. Espera hasta `daemon_start_timeout` (default 3s) a que daemon quede saludable.
6. Enruta la consulta al daemon.
7. Si timeout o fallo, fallback a direct mode y devuelve warning.

### 7. Casos especiales y variantes
- `--no-auto-daemon` salta auto-start completamente, intenta direct o falla.
- Timeout de 3s es configurable pero recomendado para UX.
- Auto-start no bloquea interactivamente; procede en background.

### 11. Decisiones cerradas
- Supuestos prohibidos:
  - no bloquear usuario en auto-start
  - no fallar por daemon timeout
- Decisiones cerradas:
  - auto-start en background + fallback
  - 3s timeout por defecto
  - --no-auto-daemon deshabilita completamente
- TODO explicit = 0
- Fuera de alcance:
  - auto-start remoto
  - cluster mode
- Dependencias externas explícitas:
  - daemon binario local
  - home directory escribible

## Superficies

CLI `mi-lsp`, con operaciones y selectores detallados en el comportamiento del requisito.

## Entradas/salidas tipadas

### 3. Entradas
- **Campo**: `query_type`; **Tipo**: enum; **Req.**: si; **Origen**: CLI; **Validacion**: `refs`, `context`, `deps`, `related` (operaciones semánticas); **RN**: RF-DAE-003
- **Campo**: `--no-auto-daemon`; **Tipo**: booleano; **Req.**: no; **Origen**: CLI; **Validacion**: si true, salta auto-start; **RN**: RF-DAE-003
- **Campo**: `daemon_start_timeout`; **Tipo**: duracion; **Req.**: no; **Origen**: config; **Validacion**: segundos; default 3s; **RN**: RF-DAE-003

### 5. Salidas
- **Campo**: `ok`; **Tipo**: bool; **Destino**: usuario/skill; **Efecto observable**: true si consulta completa
- **Campo**: `result`; **Tipo**: objeto; **Destino**: usuario/skill; **Efecto observable**: resultado de la consulta
- **Campo**: `backend`; **Tipo**: string; **Destino**: usuario/skill; **Efecto observable**: `daemon` o `direct` (fallback)
- **Campo**: `warnings`; **Tipo**: lista; **Destino**: usuario/skill; **Efecto observable**: si fallback a direct

### 8. Impacto en el modelo de datos
- `DaemonState`
- `RuntimeSnapshot`

## Errores

### 6. Errores tipados
- **Código**: `DAE_AUTO_START_TIMEOUT`; **Causa**: daemon no se levanta en tiempo; **Trigger**: timeout > 3s; **Respuesta esperada**: fallback a direct mode con warning
- **Código**: `DAE_AUTO_START_FAILED`; **Causa**: fallo al iniciar daemon; **Trigger**: error durante boot; **Respuesta esperada**: fallback a direct mode con warning
- **Código**: `QRY_DIRECT_MODE_FAILED`; **Causa**: fallback directo también falla; **Trigger**: error sin daemon; **Respuesta esperada**: abortar con error explícito

## Criterios de aceptación (numerados)

1. Auto-iniciar daemon en primera consulta semántica: Dado daemon no corriendo Cuando ejecuto "mi-lsp nav refs --symbol MyClass" Entonces auto-inicio daemon en background Y espero hasta 3s a que levante Y enruto la consulta al daemon una vez saludable Y "backend" indica "daemon"
2. Fallback a direct si auto-start timeout: Dado daemon no corriendo y startup toma > 3s Cuando ejecuto la consulta Entonces timeout en auto-start Y fallback a direct mode Y devuelvo warning indicando degradacion Y "backend" es "direct"
3. Saltar auto-start con flag: Dado --no-auto-daemon presente Cuando intento semántica query Entonces no intento auto-start Y procedo directo o error

### 9. Escenarios de aceptación (Gherkin)
```gherkin
Scenario: Auto-iniciar daemon en primera consulta semantica
  Given daemon no corriendo
  When ejecuto "mi-lsp nav refs --symbol MyClass"
  Then auto-inicio daemon en background
  And espero hasta 3s a que levante
  And enruto la consulta al daemon una vez saludable
  And "backend" indica "daemon"

Scenario: Fallback a direct si auto-start timeout
  Given daemon no corriendo y startup toma > 3s
  When ejecuto la consulta
  Then timeout en auto-start
  And fallback a direct mode
  And devuelvo warning indicando degradacion
  And "backend" es "direct"

Scenario: Saltar auto-start con flag
  Given --no-auto-daemon presente
  When intento semantica query
  Then no intento auto-start
  And procedo directo o error
```

### 10. Trazabilidad de pruebas
- Positivo: `TP-DAE / TC-DAE-010`
- Positivo: `TP-DAE / TC-DAE-011`
- Negativo: `TP-DAE / TC-DAE-012`
