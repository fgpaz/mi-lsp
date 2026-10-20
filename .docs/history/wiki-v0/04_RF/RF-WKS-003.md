# RF-WKS-003 - Inicializar el workspace actual y dejarlo listo para `nav ask`

```yaml
harness_protocol: SDD-HARNESS-v1
id: "RF-WKS-003"
kind: "support-doc"
audience: "llm-first"
imports:
  - '[[00_gobierno_documental]]'
  - '[[RF-WKS-003]]'
exports:
  - 'RF-WKS-003'
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/04_RF/RF-WKS-003.md
agent_may_edit:
  - .docs/wiki/04_RF/RF-WKS-003.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
evidence:
  - .docs/wiki/04_RF/RF-WKS-003.md
```

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
7. Como `init` pertenece a la superficie AXI-default, los `next_steps` deben privilegiar reruns utiles sin repetir `--axi`; solo los caminos hacia superficies classic-default deben agregar `--axi` explicitamente.

## 4. Typed Errors

| Codigo | Causa | Trigger | Respuesta esperada |
|---|---|---|---|
| `WKS_INIT_UNSUPPORTED_LAYOUT` | root incompatible | no se detectan marcadores soportados | abortar sin side effects |
| `WKS_INIT_REGISTRY_FAILED` | fallo de persistencia | no se puede escribir `registry.toml` | abortar con error explicito |

## 5. Special Cases and Variants

- `init` es una puerta corta; no redefine el contrato de `workspace add`.
- `--no-index` conserva semantica equivalente a `workspace add --no-index`.
- Si la indexacion falla, el registro sigue siendo exitoso con warning accionable.
- `init` sigue haciendo el mismo bootstrap; el modo AXI/default solo cambia la disclosure de `next_steps`.
- `--classic` restaura la salida clasica sin desactivar el bootstrap ni el indexing.

## 6. Data Model Impact

- `WorkspaceRegistration`
- `ProjectConfig`
- `QueryEnvelope`
- `QueryOptions`
