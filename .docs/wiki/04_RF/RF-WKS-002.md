---
id: RF-WKS-002
title: Indexar automaticamente al registrar un workspace nuevo
implements:
  - internal/service/workspace_ops.go
  - internal/cli/workspace.go
  - internal/workspace/registry.go
tests:
  - internal/service/app_test.go
  - internal/service/workspace_resolution_test.go
---

```yaml
harness_protocol: SDD-HARNESS-v1
id: "RF-WKS-002"
kind: "support-doc"
audience: "llm-first"
imports:
  - '[[00_gobierno_documental]]'
  - '[[RF-WKS-002]]'
exports:
  - 'RF-WKS-002'
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/04_RF/RF-WKS-002.md
agent_may_edit:
  - .docs/wiki/04_RF/RF-WKS-002.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
evidence:
  - .docs/wiki/04_RF/RF-WKS-002.md
```

# RF-WKS-002 - Indexar automaticamente al registrar un workspace nuevo

## 1. Execution Sheet

| Campo | Valor |
|---|---|
| ID | RF-WKS-002 |
| Titulo | Indexar automaticamente al registrar un workspace nuevo |
| Actores | Desarrollador, CLI, Skill |
| Prioridad | alta |
| Severidad | media |
| FL origen | FL-BOOT-01 |

## 2. Detailed Preconditions

| Condicion | Tipo | Estado requerido |
|---|---|---|
| Path del workspace valido | funcional | obligatorio |
| Directorio escribible | operativa | obligatorio |
| `.mi-lsp/project.toml` creado | funcional | obligatorio |

## 3. Inputs

| Campo | Tipo | Req. | Origen | Validacion | RN |
|---|---|---|---|---|---|
| `workspace_path` | string | si | CLI | path absoluto o relativo | RF-WKS-002 |
| `--alias` | string | no | CLI | alias custom; default derived from path | RF-WKS-002 |
| `--no-index` | booleano | no | CLI | si true, salta indexacion automatica | RF-WKS-002 |

## 4. Process Steps (Happy Path)

1. La CLI recibe comando `workspace add <path>`.
2. Valida layout y crea `project.toml`.
3. Registra workspace en `~/.mi-lsp/registry.toml`.
4. Por defecto (sin `--no-index`), inicia indexacion automatica.
5. Devuelve respuesta con registro + stats de indexacion.
6. Si indexacion falla, devuelve warning pero registro exitoso.

## 5. Outputs

| Campo | Tipo | Destino | Efecto observable |
|---|---|---|---|
| `ok` | bool | usuario/skill | true si registro exitoso |
| `registration` | objeto | usuario/skill | WorkspaceRegistration con alias, path, languages |
| `index_stats` | objeto | usuario/skill | files_indexed, symbols_indexed, duration |
| `warnings` | lista | usuario/skill | index failure non-fatal, o --no-index applied |

## 6. Typed Errors

| Codigo | Causa | Trigger | Respuesta esperada |
|---|---|---|---|
| `WKS_INVALID_PATH` | path no existe o no es directorio | path invalido | rechazar sin modificar registry |
| `WKS_ALREADY_REGISTERED` | mismo alias ya registrado con root incompatible | alias conflicto | rechazar o sugerir otro alias |
| `WKS_DUPLICATE_ROOT_ALIAS` | multiples aliases apuntan al mismo root | alias distinto para root existente | permitir; `workspace list` preserva aliases y `workspace list --group-by-root`/`workspace doctor` diagnostican |
| `WKS_INDEX_FAILED` | indexacion falla | IO error en index.db | registrar con warning, ok=true |

## 7. Special Cases and Variants

- `--no-index` salta indexacion completamente.
- Si indexacion falla, el registro sigue siendo exitoso (warning no-fatal).
- Si indexacion excede timeout (default 30s), log warning y continua.
- `workspace doctor` enriquece cada hallazgo con `health` (`ok|attention|action_required`) y `next_actions` ordenadas para reparar alias duplicados, roots stale, worktrees ambiguos, shadowing de binario o drift de revision entre el binario activo y binarios visibles sin mutar `registry.toml`.

## 8. Data Model Impact

- `WorkspaceRegistration` (alias, path, languages, registered_at)
- `registry.toml` file

## 9. Expanded Acceptance Criteria (Gherkin)

```gherkin
Scenario: Registrar e indexar workspace automaticamente
  Given path valido de workspace
  When ejecuto "mi-lsp workspace add /path/to/workspace"
  Then registro el workspace en registry
  And indexo automaticamente sin necesidad de comando adicional
  And devuelvo stats de indexacion en respuesta

Scenario: Saltar indexacion con --no-index
  Given --no-index flag presente
  When ejecuto workspace add
  Then registro el workspace
  And no inicio indexacion
  And devuelvo registro sin stats de indexacion

Scenario: Continuar si indexacion falla
  Given indexacion falla por IO error
  When ejecuto workspace add
  Then registro exitoso
  And warning de index failure
  And ok=true (registro prevalece)
```

## 10. Test Traceability

- Positivo: `TP-WKS / TC-WKS-006`
- Positivo: `TP-WKS / TC-WKS-007`
- Positivo: `TP-WKS / TC-WKS-017`
- Positivo: `TP-WKS / TC-WKS-018`
- Positivo: `TP-WKS / TC-WKS-019`
- Positivo: `TP-WKS / TC-WKS-026`
- Negativo: `TP-WKS / TC-WKS-008`

## 11. Implementation Evidence

- `internal/workspace/registry.go`: group-by-root read model over aliases without registry mutation.
- `internal/service/workspace_ops.go`: `workspace list --group-by-root` and `workspace doctor` service behavior.
- `internal/cli/workspace.go`: CLI flags and doctor command surface.
- `internal/service/app_test.go`: grouped list and non-mutating doctor tests.

## 12. No Ambiguities Left

- Supuestos prohibidos:
  - no fallar globalmente si indexacion falla
  - no obligar --no-index para casos normales
- Decisiones cerradas:
  - auto-index por defecto en add
  - --no-index solo salta index, no registration
  - index failures no-fatal
- TODO explicit = 0
- Fuera de alcance:
  - async indexacion separada
  - auto-update de index en background (vease RF-DAE-004)
- Dependencias externas explicitas:
  - filesystem local
  - registry.toml persistencia
