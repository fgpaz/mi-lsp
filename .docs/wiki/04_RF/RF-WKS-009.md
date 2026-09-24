---
id: RF-WKS-009
title: Resolver workspace y ejecutable en uso con `workspace which`
implements:
  - internal/cli/workspace_which.go
  - internal/cli/workspace.go
tests:
  - internal/cli/workspace_which_test.go
---

# RF-WKS-009 - Resolver workspace y ejecutable en uso con `workspace which`

```yaml
harness_protocol: SDD-HARNESS-v1
id: "RF-WKS-009"
kind: "support-doc"
audience: "llm-first"
imports:
  - '[[00_gobierno_documental]]'
  - '[[FL-BOOT-01]]'
  - '[[RF-WKS-009]]'
exports:
  - 'RF-WKS-009'
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/03_FL/FL-BOOT-01.md
  - .docs/wiki/04_RF/RF-WKS-009.md
agent_may_edit:
  - .docs/wiki/04_RF/RF-WKS-009.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
evidence:
  - .docs/wiki/04_RF/RF-WKS-009.md
```

## 1. Execution Sheet

| Campo | Valor |
|---|---|
| ID | RF-WKS-009 |
| Titulo | Resolver workspace y ejecutable en uso con `workspace which` |
| Actores | Usuario, Skill, Agente, CLI/Core, Puerta MCP |
| Prioridad | media |
| Severidad | media |
| FL origen | FL-BOOT-01 |

## 2. Detailed Preconditions

| Condicion | Tipo | Estado requerido |
|---|---|---|
| Registry global legible (`~/.mi-lsp/registry.toml`) | tecnica | obligatorio |
| Workspace explicito o cwd resoluble contra un root registrado, o `last_workspace` disponible | funcional | uno de los tres |
| Daemon corriendo | tecnica | no requerido |
| Indice construido | tecnica | no requerido |

## 3. Process Steps (Happy Path)

1. El usuario, skill o agente ejecuta `mi-lsp workspace which [--workspace <alias>] [--cwd <path>]`.
2. La CLI aplica la misma precedencia que el resto de la superficie: `--workspace` explicito, luego el root registrado que contiene `--cwd` o el cwd del proceso, luego `last_workspace`.
3. La resolucion lee unicamente el registry en memoria; no abre indice, no dispara daemon ni red, y no muta `registry.toml`.
4. La CLI resuelve la ruta del ejecutable en uso con `os.Executable()` y la deja tal cual, preservando backslashes de Windows.
5. La CLI emite el envelope estable con `backend=registry` y un unico item `{name, root, executable, source}`.

## 4. Outputs

| Campo | Tipo | Destino | Efecto observable |
|---|---|---|---|
| `items[0].name` | string | usuario/skill | alias del workspace resuelto |
| `items[0].root` | string | usuario/skill | root registrado, literal (Windows conserva `\`) |
| `items[0].executable` | string | usuario/skill | ruta del binario `mi-lsp` en uso |
| `items[0].source` | string | usuario/skill | `explicit`, `caller_cwd` o `last_workspace` |
| `workspace` | string | usuario/skill | alias resuelto a nivel de envelope |
| `backend` | string | usuario/skill | siempre `registry` |

## 5. Typed Errors

| Codigo | Causa | Trigger | Respuesta esperada |
|---|---|---|---|
| (sin codigo tipado propio) | workspace explicito no registrado | `--workspace <selector>` no matchea ningun alias ni root | error explicito `workspace "<selector>" is not registered`; se clasifica como `invalid_workspace` por el mapeo de fallback compartido |
| (sin codigo tipado propio) | sin selector y sin default | no hay `--workspace`, el cwd no cae dentro de ningun root registrado y `last_workspace` esta vacio o invalido | error explicito `no workspace specified and no default workspace configured`; se clasifica como `invalid_workspace` |

## 6. Special Cases and Variants

- `workspace which` es de solo lectura: no indexa, no abre el daemon, no usa red y no modifica `registry.toml`.
- La precedencia es identica al resto de la CLI: `--workspace` explicito > root registrado que contiene `--cwd`/cwd del proceso > `last_workspace`.
- Un path Windows con backslash simple (por ejemplo `C:\repos\mios\mis-plugins-cc`) se conserva literal en la salida JSON; no pasa por `strconv.Unquote` ni por un ensamblado `fmt` que interprete `\r`/`\p` como escapes.
- Cuando varios aliases comparten el mismo root, se aplica el mismo desempate que usa el resto del registry (`last_workspace` primero, luego orden alfabetico case-insensitive del alias).
- El diagnostico amplio de salud del registry sigue siendo `workspace doctor`; `which` no reemplaza ese reporte.
- La puerta `mi-lsp mcp` no reinterpreta este comando: expone la misma resolucion de solo lectura via `nav_*`/`workspace which` como uno mas de los comandos que ya delega.

## 7. Data Model Impact

- Reutiliza `model.RegistryFile` / `model.WorkspaceRegistration` existentes.
- No agrega un tipo nuevo de persistencia; el resultado vive solo en el envelope de respuesta.

## 8. Expanded Acceptance Criteria (Gherkin)

```gherkin
Scenario: Resolver por workspace explicito
  Given un workspace registrado con alias "gastos"
  When ejecuto "mi-lsp workspace which --workspace gastos --format json"
  Then la respuesta incluye "backend=registry"
  And "items[0].source" es "explicit"
  And el registry no fue modificado

Scenario: Resolver por caller_cwd sin --workspace
  Given el proceso corre dentro del root de un workspace registrado
  When ejecuto "mi-lsp workspace which"
  Then "items[0].source" es "caller_cwd"

Scenario: Conservar un path Windows literal
  Given un workspace registrado con root "C:\repos\mios\mis-plugins-cc"
  When ejecuto "mi-lsp workspace which --workspace mis-plugins-cc --format json"
  Then "items[0].root" es exactamente "C:\repos\mios\mis-plugins-cc" sin escapes adicionales

Scenario: Rechazar un workspace explicito no registrado
  Given ningun workspace registrado con alias "missing-workspace"
  When ejecuto "mi-lsp workspace which --workspace missing-workspace"
  Then la operacion falla con un error explicito que menciona el selector
```

## 9. Test Traceability

- Positivo: `TP-WKS / TC-WKS-044`
- Positivo: `TP-WKS / TC-WKS-045`
- Positivo: `TP-WKS / TC-WKS-046`
- Negativo: `TP-WKS / TC-WKS-047` (documentado en codigo; ver Special Cases del cierre de wiki, sin test Go nombrado que cubra ambas ramas de error)

## 10. No Ambiguities Left

- Supuestos prohibidos:
  - no asumir que `which` puede iniciar el daemon o abrir el indice
  - no asumir que `which` puede mutar `registry.toml`
- Decisiones cerradas:
  - solo lectura sobre el registry en memoria
  - misma precedencia de resolucion que el resto de la CLI
  - path Windows literal, sin reinterpretar backslashes
- TODO explicit = 0
- Fuera de alcance:
  - diagnostico amplio de salud (`workspace doctor`)
  - resolucion cross-workspace explicita (`--allow-cross-workspace` es de otras superficies)
- Dependencias externas explicitas:
  - solo `~/.mi-lsp/registry.toml` y `os.Executable()`
