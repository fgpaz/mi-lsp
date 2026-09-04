# RF-WKS-001 - Registrar workspace por path y alias

```yaml
harness_protocol: SDD-HARNESS-v1
id: "RF-WKS-001"
kind: "support-doc"
audience: "llm-first"
imports:
  - '[[00_gobierno_documental]]'
  - '[[RF-WKS-001]]'
exports:
  - 'RF-WKS-001'
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/04_RF/RF-WKS-001.md
agent_may_edit:
  - .docs/wiki/04_RF/RF-WKS-001.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
evidence:
  - .docs/wiki/04_RF/RF-WKS-001.md
```

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
| El root o alguno de sus hijos contiene `.sln`, `.csproj`, `go.mod`, `go.work`, `package.json`, `tsconfig.json`, `pyproject.toml`, `setup.py`, `setup.cfg` o `requirements.txt` | funcional | obligatorio |
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
4. El detector obtiene `repo[]`, `entrypoint[]`, `default_repo` y `default_entrypoint` respetando ignores. Para un repo Go, la topología conserva la ruta exacta del entrypoint relativa al workspace (por ejemplo, `runtime/go.mod`), y el selector que consume el observador puede ser una ruta de módulo relativa al repo configurada explícitamente (`go.mod`) o un ID generado de `WorkspaceEntrypoint`; el ID se resuelve con el repo y entrypoint exactos, se rebasa su ruta workspace-relative al repo seleccionado y se revalida como `go.mod` seguro y regular. IDs desconocidos/malformados y rutas inseguras fallan cerrado; resolver un ID no convierte contenido raíz no relacionado en un entrypoint.
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
- Para Go, un módulo anidado se representa con una ruta exacta relativa al workspace (por ejemplo, `runtime/go.mod`), mientras la observación recibe el selector repo-local explícito (`go.mod`) configurado para ese repo. `DefaultEntrypoint` también puede ser un ID de `WorkspaceEntrypoint`: se resuelve con coincidencia exacta de repo y entrypoint, se rebasa su ruta workspace-relative al root del repo seleccionado y se revalida que el resultado sea un `go.mod` repo-local, seguro y regular antes de observar. Los IDs desconocidos o malformados, paths inexistentes/no regulares, symlinks finales o selecciones inseguras fallan cerrado y quedan como omisión/diagnóstico (`""`), sin activar el fallback. `go.work` no es un selector soportado por la observación Go y se rechaza de forma fail-closed.
- Solo si el selector Go repo-local está vacío se conserva el fallback determinista al `go.mod` del root. Un root que solo contiene `go.work` no selecciona módulo ni intenta extracción de grafo; no se hace descubrimiento recursivo arbitrario para sustituir ese fallback, y el contenido raíz no relacionado conserva su ownership de catálogo.
- Si el alias ya existe, el comportamiento es `upsert`.
- `workspace doctor` puede derivar estado `health` y `next_actions` desde el registro existente para diagnostico humano/agente; no modifica `WorkspaceRegistration`, no elige alias por el usuario y no borra worktrees.
## 8. Data Model Impact

- `WorkspaceRegistration`
- `ProjectConfig`
- `WorkspaceRepo`
- `WorkspaceEntrypoint`
