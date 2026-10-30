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
  - path: internal/daemon/server.go
    role: behavior
  - path: internal/daemon/instance_owner.go
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
4. Si el health check falla, no presume que el dueño murió: consulta el estado del mismo directorio de daemon. Un PID vivo o cuya muerte no puede confirmarse impide otro arranque antes de fork, listener y apertura de `daemon.db`; devuelve diagnóstico de dueño/endpoint, sin matar procesos ni cambiar permisos. Si no hay dueño o está demostrablemente muerto y sin `--no-auto-daemon`, intenta iniciar daemon en background bajo exclusión.
5. Espera hasta `daemon_start_timeout` (default 3s) a que daemon quede saludable.
6. Enruta la consulta al daemon.
7. Si timeout o fallo, fallback a direct mode y devuelve warning.

### 7. Casos especiales y variantes
- `--no-auto-daemon` salta auto-start completamente, intenta direct o falla.
- Timeout de 3s es configurable pero recomendado para UX.
- Auto-start no bloquea interactivamente; procede en background.
- La exclusión de arranque no sustituye la exclusión durante toda la vida del servidor: dos constructores nuevos del mismo directorio de daemon, incluso con endpoints diferentes, no pueden abrir juntos `daemon.db`. La reserva se conserva hasta cerrar la DB y se libera también si falla el constructor.
- Un error de acceso al proceso, un endpoint inaccesible o un estado ilegible no acreditan muerte. El rechazo conserva el estado ajeno y no anuncia una nueva instancia saludable. Si la operación permite fallback directo, lo informa como degradación, no como daemon iniciado.
- La mitigación no detiene daemons antiguos ya duplicados ni resuelve por sí sola CPU, idle o WAL. Los binarios antiguos no cooperan con la nueva reserva: la recarga de consumidores/servicio pertenece al supervisor.

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
4. Dueño vivo o indeterminado inaccesible: no se crea otro proceso daemon, listener ni DB; se conserva el estado y se devuelve diagnóstico de dueño, sin señalizar procesos ni ampliar permisos.
5. Dos constructores del mismo almacenamiento: sólo uno mantiene la reserva de vida; el otro termina de forma acotada, aunque use otro endpoint. Tras cierre de DB o fallo del constructor propio, la reserva propia se libera.

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

```yaml
harness_protocol: SDD-HARNESS-v1
id: RF-DAE-003
kind: requirement
audience: llm-first
imports:
- .docs/wiki/04_flows/FL-DAE-01.md
exports:
- RF-DAE-003
- RF-DAE-003 - Auto-iniciar daemon en la primera consulta semantica
agent_must_read:
- .docs/wiki/05_requirements/RF-DAE-003.md
- .docs/wiki/04_flows/FL-DAE-01.md
- .docs/wiki/06_tests/TP-DAE.md
agent_may_edit:
- .docs/wiki/05_requirements/RF-DAE-003.md
agent_must_not_edit:
- .docs/wiki/00_governance.md
- AGENTS.md
- internal/service/app.go
- internal/daemon/server.go
- internal/daemon/instance_owner.go
verify:
- Validar el contrato y sus referencias con `mi-lsp nav wiki validate-harness --workspace
  <alias>`; contrastar los casos de .docs/wiki/06_tests/TP-DAE.md sin inferir que
  fueron ejecutados.
stop_if:
- 'Detenerse y escalar si el cambio contradice el límite documentado: - Supuestos
  prohibidos:'
evidence:
- .docs/wiki/05_requirements/RF-DAE-003.md
- .docs/wiki/06_tests/TP-DAE.md
```
