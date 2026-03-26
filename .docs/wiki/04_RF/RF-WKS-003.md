# RF-WKS-003 - Inicializar el workspace actual y dejarlo listo para `nav ask`

## 1. Execution Sheet

| Campo | Valor |
|---|---|
| ID | RF-WKS-003 |
| Titulo | Inicializar el workspace actual y dejarlo listo para `nav ask` |
| Actores | Desarrollador, Skill, CLI/Core |
| Prioridad | alta |
| Severidad | media |
| FL origen | FL-BOOT-01 |

## 2. Detailed Preconditions

| Condicion | Tipo | Estado requerido |
|---|---|---|
| Path objetivo existe o se usa el `cwd` | tecnica | obligatorio |
| Layout compatible detectable | funcional | obligatorio |
| `~/.mi-lsp/registry.toml` es escribible | operativa | obligatorio |

## 3. Process Steps (Happy Path)

1. La CLI recibe `mi-lsp init [path] [--name alias]`.
2. Si el path falta, usa `.` como workspace objetivo.
3. Reusa la deteccion de layout y persistencia de `workspace add`.
4. Registra el workspace y lo deja como `last_workspace`.
5. Por defecto indexa automaticamente.
6. Devuelve un resultado resumido con `next_steps` centrados en `nav ask`.

## 4. Typed Errors

| Codigo | Causa | Trigger | Respuesta esperada |
|---|---|---|---|
| `WKS_INIT_UNSUPPORTED_LAYOUT` | root incompatible | no se detectan marcadores soportados | abortar sin side effects |
| `WKS_INIT_REGISTRY_FAILED` | fallo de persistencia | no se puede escribir `registry.toml` | abortar con error explicito |

## 5. Special Cases and Variants

- `init` es una puerta corta; no redefine el contrato de `workspace add`.
- `--no-index` conserva semantica equivalente a `workspace add --no-index`.
- Si la indexacion falla, el registro sigue siendo exitoso con warning accionable.

## 6. Data Model Impact

- `WorkspaceRegistration`
- `ProjectConfig`
- `QueryEnvelope`
