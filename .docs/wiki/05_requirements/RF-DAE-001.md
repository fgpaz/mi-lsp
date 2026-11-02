---
wiki_contract: WIKI-CONTRACT-v1
id: RF-DAE-001
kind: requirement
status: active
title: RF-DAE-001 - Iniciar, consultar y detener el daemon global idempotente
links:
  implements:
  - FL-DAE-01
  code:
  - path: internal/service/app.go
    role: behavior
---

## Propósito

RF-DAE-001 - Iniciar, consultar y detener el daemon global idempotente

### 1. Hoja de ejecución
- **Campo**: ID; **Valor**: RF-DAE-001
- **Campo**: Título; **Valor**: Iniciar, consultar y detener el daemon global idempotente
- **Campo**: Actores; **Valor**: Desarrollador, Agente, CLI, Daemon
- **Campo**: Prioridad; **Valor**: alta
- **Campo**: Severidad; **Valor**: alta
- **Campo**: FL origen; **Valor**: FL-DAE-01

## Comportamiento ordenado

### 2. Precondiciones detalladas
- **Condición**: Binario `mi-lsp` disponible; **Tipo**: técnica; **Estado requerido**: obligatorio
- **Condición**: Home del usuario escribible; **Tipo**: operativa; **Estado requerido**: obligatorio
- **Condición**: Pipe/socket local creable; **Tipo**: técnica; **Estado requerido**: obligatorio

### 4. Secuencia principal
1. La CLI recibe `daemon start`, `daemon status` o `daemon stop`.
2. Para `start`, la CLI hace health check y lock global.
3. Si ya existe un daemon saludable, devuelve su metadata sin crear otro.
4. Si no existe, crea la nueva instancia, persiste `state.json` y responde saludable.
5. Para `status`, lee el estado actual y valida salud del endpoint.
6. Para `stop`, termina la instancia actual y limpia el estado vivo.

### 7. Casos especiales y variantes
- `daemon start` es idempotente: si ya hay una instancia saludable, la reutiliza.
- `daemon status` puede devolver `down` sin considerarse error fatal.
- `admin_token` se conserva para autenticación interna, pero nunca se imprime en claro: las salidas CLI que incluyen estado muestran `[REDACTADO:token]`.
- La CLI debe seguir operando aunque el daemon no exista.

### 11. Decisiones cerradas
- Supuestos prohibidos:
  - no asumir un daemon por workspace
  - no asumir que el daemon vive atado a la terminal lanzadora
- Decisiones cerradas:
  - existe un daemon global por usuario/host
  - `start` es idempotente
- TODO explicit = 0
- Fuera de alcance:
  - multi-host o auth remota
- Dependencias externas explícitas:
  - filesystem del home del usuario y pipe/socket local

## Superficies

CLI `mi-lsp`, con operaciones y selectores detallados en el comportamiento del requisito.

## Entradas/salidas tipadas

### 3. Entradas
- **Campo**: `command`; **Tipo**: enum; **Req.**: si; **Origen**: CLI; **Validacion**: `start`, `status`, `stop`; **RN**: RF-DAE-001
- **Campo**: `idle_timeout`; **Tipo**: duracion; **Req.**: no; **Origen**: CLI/config; **Validacion**: si se informa, debe ser positiva; **RN**: RF-DAE-001
- **Campo**: `max_workers`; **Tipo**: entero; **Req.**: no; **Origen**: CLI/config; **Validacion**: si se informa, debe ser positivo; sin valor (0) rige `MI_LSP_DAEMON_MAX_WORKERS` o el default 8; **RN**: RF-DAE-001

### 5. Salidas
- **Campo**: `pid`; **Tipo**: número; **Destino**: usuario/skill; **Efecto observable**: proceso daemon activo
- **Campo**: `admin_url`; **Tipo**: string; **Destino**: usuario/skill; **Efecto observable**: URL loopback de gobernanza
- **Campo**: `status`; **Tipo**: string; **Destino**: usuario/skill; **Efecto observable**: `up` o `down`

### 8. Impacto en el modelo de datos
- `DaemonState`
- `DaemonRun`

## Errores

### 6. Errores tipados
- **Código**: `DAE_LOCK_FAILED`; **Causa**: no se puede obtener lock; **Trigger**: contencion o permisos; **Respuesta esperada**: abortar con error explícito
- **Código**: `DAE_BOOT_FAILED`; **Causa**: el daemon no alcanza estado saludable; **Trigger**: fallo al iniciar; **Respuesta esperada**: abortar y no dejar `state.json` inconsistente
- **Código**: `DAE_STOP_FAILED`; **Causa**: no se puede detener la instancia; **Trigger**: proceso no responde; **Respuesta esperada**: error explícito y estado conservado

## Criterios de aceptación (numerados)

1. Iniciar daemon global por primera vez: Dado que no existe un daemon saludable Cuando ejecuto "mi-lsp daemon start" Entonces queda una instancia viva con "pid" y "admin_url" Y existe "~/.mi-lsp/daemon/state.json"
2. Reusar daemon vivo en segundo start: Dado que ya existe un daemon saludable Cuando ejecuto "mi-lsp daemon start" Entonces la respuesta devuelve el mismo "pid" Y no se crea una segunda instancia
3. Informar estado down sin romper la CLI: Dado que no existe daemon vivo Cuando ejecuto "mi-lsp daemon status" Entonces la respuesta informa "down" Y el resto de comandos de la CLI pueden seguir usando fallback directo

### 9. Escenarios de aceptación (Gherkin)
```gherkin
Scenario: Iniciar daemon global por primera vez
  Given que no existe un daemon saludable
  When ejecuto "mi-lsp daemon start"
  Then queda una instancia viva con "pid" y "admin_url"
  And existe "~/.mi-lsp/daemon/state.json"

Scenario: Reusar daemon vivo en segundo start
  Given que ya existe un daemon saludable
  When ejecuto "mi-lsp daemon start"
  Then la respuesta devuelve el mismo "pid"
  And no se crea una segunda instancia

Scenario: Informar estado down sin romper la CLI
  Given que no existe daemon vivo
  When ejecuto "mi-lsp daemon status"
  Then la respuesta informa "down"
  And el resto de comandos de la CLI pueden seguir usando fallback directo
```

### 10. Trazabilidad de pruebas
- Positivo: `TP-DAE / TC-DAE-001`
- Positivo: `TP-DAE / TC-DAE-002`
- Negativo: `TP-DAE / TC-DAE-003`

```yaml
harness_protocol: SDD-HARNESS-v1
id: RF-DAE-001
kind: requirement
audience: llm-first
imports:
- .docs/wiki/04_flows/FL-DAE-01.md
exports:
- RF-DAE-001
- RF-DAE-001 - Iniciar, consultar y detener el daemon global idempotente
agent_must_read:
- .docs/wiki/05_requirements/RF-DAE-001.md
- .docs/wiki/04_flows/FL-DAE-01.md
- .docs/wiki/06_tests/TP-DAE.md
agent_may_edit:
- .docs/wiki/05_requirements/RF-DAE-001.md
agent_must_not_edit:
- .docs/wiki/00_governance.md
- AGENTS.md
- internal/service/app.go
verify:
- Validar el contrato y sus referencias con `mi-lsp nav wiki validate-harness --workspace
  <alias>`; contrastar los casos de .docs/wiki/06_tests/TP-DAE.md sin inferir que
  fueron ejecutados.
stop_if:
- 'Detenerse y escalar si el cambio contradice el límite documentado: - Supuestos
  prohibidos:'
evidence:
- .docs/wiki/05_requirements/RF-DAE-001.md
- .docs/wiki/06_tests/TP-DAE.md
```
