# RF-WKS-005 - Aplicar gate de gobernanza al inicio de toda tarea

## 1. Execution Sheet

| Campo | Valor |
|---|---|
| ID | RF-WKS-005 |
| Titulo | Aplicar gate de gobernanza al inicio de toda tarea |
| Actores | Desarrollador, Skill, Agente, CLI/Core |
| Prioridad | alta |
| Severidad | alta |
| FL origen | FL-BOOT-01 |

## 2. Detailed Preconditions

| Condicion | Tipo | Estado requerido |
|---|---|---|
| Workspace resoluble | funcional | obligatorio |
| `00_gobierno_documental.md` presente | funcional | obligatorio |
| Proyeccion ejecutable sincronizada | tecnica | obligatorio |

## 3. Process Steps (Happy Path)

1. Toda tarea consulta el estado de gobernanza del workspace antes de continuar.
2. `workspace status` expone perfil, sync, index sync y estado bloqueado.
3. Si la gobernanza es valida, el workflow normal puede seguir.
4. Si la gobernanza es invalida, el repo entra en `blocked mode`.
5. En `blocked mode` solo quedan permitidos diagnostico y reparacion de gobernanza.

## 4. Typed Errors

| Codigo | Causa | Trigger | Respuesta esperada |
|---|---|---|---|
| `WKS_GOV_BLOCKED` | gobernanza invalida | doc faltante, YAML invalido, proyeccion stale, indice stale | devolver estado bloqueado y pasos de reparacion |
| `WKS_GOV_UNCLEAR` | perfil o cadenas contradictorias | schema semivalido pero ambiguo | bloquear y listar contradicciones |

## 5. Special Cases and Variants

- El gate corre al inicio de toda tarea spec-driven, no solo de tareas documentales.
- `workspace status` y `nav governance` siguen disponibles aun en `blocked mode`.
- El policy layer (`AGENTS.md`, `CLAUDE.md`, skills) debe respetar este gate aunque el usuario no lo mencione.

## 6. Data Model Impact

- `GovernanceStatus`
- `WorkspaceRegistration`
- `QueryEnvelope`
