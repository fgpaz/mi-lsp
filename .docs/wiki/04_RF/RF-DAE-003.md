# RF-DAE-003 - Auto-iniciar daemon en la primera consulta semantica

## 1. Execution Sheet

| Campo | Valor |
|---|---|
| ID | RF-DAE-003 |
| Titulo | Auto-iniciar daemon en la primera consulta semantica si no esta corriendo |
| Actores | CLI, Daemon, Skill, Agente |
| Prioridad | alta |
| Severidad | media |
| FL origen | FL-DAE-01 |

## 2. Detailed Preconditions

| Condicion | Tipo | Estado requerido |
|---|---|---|
| Binario daemon disponible | tecnica | obligatorio |
| Consulta semantica valida | funcional | obligatorio |
| Home del usuario escribible | operativa | obligatorio |

## 3. Inputs

| Campo | Tipo | Req. | Origen | Validacion | RN |
|---|---|---|---|---|---|
| `query_type` | enum | si | CLI | `refs`, `context`, `deps`, `related` (operaciones semanticas) | RF-DAE-003 |
| `--no-auto-daemon` | booleano | no | CLI | si true, salta auto-start | RF-DAE-003 |
| `daemon_start_timeout` | duracion | no | config | segundos; default 3s | RF-DAE-003 |

## 4. Process Steps (Happy Path)

1. La CLI recibe una consulta semantica (refs, context, etc).
2. Verifica si daemon esta vivo (health check).
3. Si daemon vivo, enruta la consulta al daemon.
4. Si no vivo y sin `--no-auto-daemon`, intenta iniciar daemon en background.
5. Espera hasta `daemon_start_timeout` (default 3s) a que daemon quede saludable.
6. Enruta la consulta al daemon.
7. Si timeout o fallo, fallback a direct mode y devuelve warning.

## 5. Outputs

| Campo | Tipo | Destino | Efecto observable |
|---|---|---|---|
| `ok` | bool | usuario/skill | true si consulta completa |
| `result` | objeto | usuario/skill | resultado de la consulta |
| `backend` | string | usuario/skill | `daemon` o `direct` (fallback) |
| `warnings` | lista | usuario/skill | si fallback a direct |

## 6. Typed Errors

| Codigo | Causa | Trigger | Respuesta esperada |
|---|---|---|---|
| `DAE_AUTO_START_TIMEOUT` | daemon no se levanta en tiempo | timeout > 3s | fallback a direct mode con warning |
| `DAE_AUTO_START_FAILED` | fallo al iniciar daemon | error durante boot | fallback a direct mode con warning |
| `QRY_DIRECT_MODE_FAILED` | fallback directo tambien falla | error sin daemon | abortar con error explicito |

## 7. Special Cases and Variants

- `--no-auto-daemon` salta auto-start completamente, intenta direct o falla.
- Timeout de 3s es configurable pero recomendado para UX.
- Auto-start no bloquea interactivamente; procede en background.

## 8. Data Model Impact

- `DaemonState`
- `RuntimeSnapshot`

## 9. Expanded Acceptance Criteria (Gherkin)

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

## 10. Test Traceability

- Positivo: `TP-DAE / TC-DAE-010`
- Positivo: `TP-DAE / TC-DAE-011`
- Negativo: `TP-DAE / TC-DAE-012`

## 11. No Ambiguities Left

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
- Dependencias externas explicitas:
  - daemon binario local
  - home directory escribible
