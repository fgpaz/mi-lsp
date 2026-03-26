# RF-DAE-001 - Iniciar, consultar y detener el daemon global idempotente

## 1. Execution Sheet

| Campo | Valor |
|---|---|
| ID | RF-DAE-001 |
| Titulo | Iniciar, consultar y detener el daemon global idempotente |
| Actores | Desarrollador, Agente, CLI, Daemon |
| Prioridad | alta |
| Severidad | alta |
| FL origen | FL-DAE-01 |

## 2. Detailed Preconditions

| Condicion | Tipo | Estado requerido |
|---|---|---|
| Binario `mi-lsp` disponible | tecnica | obligatorio |
| Home del usuario escribible | operativa | obligatorio |
| Pipe/socket local creable | tecnica | obligatorio |

## 3. Inputs

| Campo | Tipo | Req. | Origen | Validacion | RN |
|---|---|---|---|---|---|
| `command` | enum | si | CLI | `start`, `status`, `stop` | RF-DAE-001 |
| `idle_timeout` | duracion | no | CLI/config | si se informa, debe ser positiva | RF-DAE-001 |
| `max_workers` | entero | no | CLI/config | si se informa, debe ser positivo | RF-DAE-001 |

## 4. Process Steps (Happy Path)

1. La CLI recibe `daemon start`, `daemon status` o `daemon stop`.
2. Para `start`, la CLI hace health check y lock global.
3. Si ya existe un daemon saludable, devuelve su metadata sin crear otro.
4. Si no existe, crea la nueva instancia, persiste `state.json` y responde saludable.
5. Para `status`, lee el estado actual y valida salud del endpoint.
6. Para `stop`, termina la instancia actual y limpia el estado vivo.

## 5. Outputs

| Campo | Tipo | Destino | Efecto observable |
|---|---|---|---|
| `pid` | numero | usuario/skill | proceso daemon activo |
| `admin_url` | string | usuario/skill | URL loopback de gobernanza |
| `status` | string | usuario/skill | `up` o `down` |

## 6. Typed Errors

| Codigo | Causa | Trigger | Respuesta esperada |
|---|---|---|---|
| `DAE_LOCK_FAILED` | no se puede obtener lock | contencion o permisos | abortar con error explicito |
| `DAE_BOOT_FAILED` | el daemon no alcanza estado saludable | fallo al iniciar | abortar y no dejar `state.json` inconsistente |
| `DAE_STOP_FAILED` | no se puede detener la instancia | proceso no responde | error explicito y estado conservado |

## 7. Special Cases and Variants

- `daemon start` es idempotente: si ya hay una instancia saludable, la reutiliza.
- `daemon status` puede devolver `down` sin considerarse error fatal.
- La CLI debe seguir operando aunque el daemon no exista.

## 8. Data Model Impact

- `DaemonState`
- `DaemonRun`

## 9. Expanded Acceptance Criteria (Gherkin)

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

## 10. Test Traceability

- Positivo: `TP-DAE / TC-DAE-001`
- Positivo: `TP-DAE / TC-DAE-002`
- Negativo: `TP-DAE / TC-DAE-003`

## 11. No Ambiguities Left

- Supuestos prohibidos:
  - no asumir un daemon por workspace
  - no asumir que el daemon vive atado a la terminal lanzadora
- Decisiones cerradas:
  - existe un daemon global por usuario/host
  - `start` es idempotente
- TODO explicit = 0
- Fuera de alcance:
  - multi-host o auth remota
- Dependencias externas explicitas:
  - filesystem del home del usuario y pipe/socket local
