# RF-WKS-001 - Registrar workspace por path y alias

## 1. Execution Sheet

| Campo | Valor |
|---|---|
| ID | RF-WKS-001 |
| Titulo | Registrar workspace por path y alias |
| Actores | Desarrollador, Skill, CLI/Core |
| Prioridad | alta |
| Severidad | alta |
| FL origen | FL-BOOT-01 |

## 2. Detailed Preconditions

| Condicion | Tipo | Estado requerido |
|---|---|---|
| Path objetivo existe | tecnica | obligatorio |
| Path objetivo es accesible | tecnica | obligatorio |
| El root o alguno de sus hijos contiene `.sln`, `.csproj`, `package.json`, `tsconfig.json`, `pyproject.toml`, `setup.py`, `setup.cfg` o `requirements.txt` | funcional | obligatorio |
| `~/.mi-lsp/registry.toml` es escribible | operativa | obligatorio |

## 3. Inputs

| Campo | Tipo | Req. | Origen | Validacion |
|---|---|---|---|---|
| `path` | path absoluto o relativo | si | CLI | debe existir y ser directorio |
| `alias` | string | no | CLI | si falta, usar nombre del root |

## 4. Process Steps (Happy Path)

1. La CLI recibe `workspace add <path> [--name alias]`.
2. El core normaliza el path y valida accesibilidad.
3. El detector clasifica el layout como `single` o `container`.
4. El detector obtiene `repo[]`, `entrypoint[]`, `default_repo` y `default_entrypoint` respetando ignores.
5. El core crea o actualiza `<repo>/.mi-lsp/project.toml`.
6. El registry global hace upsert del alias, root, languages y `kind`.
7. La CLI devuelve confirmacion con topologia resumida.

## 5. Outputs

| Campo | Tipo | Destino | Efecto observable |
|---|---|---|---|
| `workspace` | string | usuario/skill | alias registrado |
| `kind` | string | usuario/skill | `single` o `container` |
| `repo_count` | numero | usuario/skill | cantidad de repos detectados |
| `entrypoint_count` | numero | usuario/skill | cantidad de entrypoints semanticos |
| `project.toml` | archivo | repo local | topologia creada o actualizada |

## 6. Typed Errors

| Codigo | Causa | Trigger | Respuesta esperada |
|---|---|---|---|
| `WKS_PATH_NOT_FOUND` | path inexistente | el directorio no existe | abortar sin side effects |
| `WKS_UNSUPPORTED_LAYOUT` | root incompatible | no se detectan marcadores soportados en root o hijos | abortar con mensaje explicito |
| `WKS_REGISTRY_WRITE_FAILED` | fallo de persistencia | no se puede escribir `registry.toml` | abortar con error y sin registro parcial |

## 7. Special Cases and Variants

- `single`: un repo con un root semantico obvio.
- `container`: carpeta padre con muchos repos hijos, sin requerir `.sln` agregadora.
- Paths auxiliares como `.worktrees/` nunca deben ser elegidos como default.
- Entrypoints ubicados bajo `.docs/` o `template(s)` pueden seguir visibles en la topologia, pero no deben quedar como `default_entrypoint` si existe una alternativa real del repo.
- Si el alias ya existe, el comportamiento es `upsert`.

## 8. Data Model Impact

- `WorkspaceRegistration`
- `ProjectConfig`
- `WorkspaceRepo`
- `WorkspaceEntrypoint`

