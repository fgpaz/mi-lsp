---
id: RF-QRY-018
title: Generar y aplicar experimentalmente planes de edicion con nav edit-plan
implements:
  - internal/cli/nav.go
  - internal/model/edit_plan.go
  - internal/service/app.go
  - internal/service/edit_plan.go
  - internal/service/edit_plan_go_ast.go
tests:
  - internal/cli/nav_test.go
  - internal/service/edit_plan_test.go
---

```yaml
harness_protocol: SDD-HARNESS-v1
id: "RF-QRY-018"
kind: "support-doc"
audience: "llm-first"
imports:
  - '[[00_gobierno_documental]]'
  - '[[RF-QRY-018]]'
  - '[[CT-NAV-EDIT-PLAN]]'
exports:
  - 'RF-QRY-018'
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/04_RF/RF-QRY-018.md
  - .docs/wiki/09_contratos/CT-NAV-EDIT-PLAN.md
agent_may_edit:
  - .docs/wiki/04_RF/RF-QRY-018.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
evidence:
  - .docs/wiki/04_RF/RF-QRY-018.md
  - .docs/wiki/09_contratos/CT-NAV-EDIT-PLAN.md
```

# RF-QRY-018 - Generar y aplicar experimentalmente planes de edicion con nav edit-plan

## Descripcion

Exponer `mi-lsp nav edit-plan` como superficie agent-first para convertir un packet declarativo en un diff determinista. El comando debe ser dry-run por default y solo puede escribir archivos cuando el usuario pasa explicitamente `--apply --experimental-apply`.

La superficie acepta `edit-plan-v1` textual y `edit-plan-v2` multi-lenguaje. En esta ola `edit-plan-v2` implementa backend AST solo para Go; C#, TypeScript y Python se reconocen como lenguajes validos pero devuelven `language_not_supported` para operaciones AST, con sugerencia accionable hacia `edit-plan-v1` textual o backends futuros.

## Actor principal

Skill / Agente / CLI

## FL origen

FL-QRY-01

## Estado

implemented

## TP asociado

TP-QRY

## Entradas

- `--stdin`: lee un packet JSON `edit-plan-v1` o `edit-plan-v2` desde stdin.
- `--packet <file>`: lee un packet JSON `edit-plan-v1` o `edit-plan-v2` desde archivo.
- `--strict`: rechaza campos desconocidos y requiere hashes.
- `--include-content`: incluye evidencia de contenido del target.
- `--apply`: solicita escritura de archivos.
- `--experimental-apply`: flag companion obligatorio para cualquier `--apply`.

## Salida

El envelope debe usar `backend=edit-plan`, `mode=dry_run|applied`, y `items[0]` con:

- `patch_packet`: packet validado.
- `diff`: unified diff determinista.
- `files_changed`: cantidad de archivos con cambios planificados.
- `operations`: resultado por operacion.
- `evidence`: hashes, rangos y contenido opcional.
- `guardrails`: reglas activas y warnings.
- `apply_status`: estado de apply o dry-run.

Si el diff/evidencia supera presupuesto, debe devolver `truncated=true` y `next_hint`.

## Reglas

- Dry-run es default y no escribe bajo ningun camino.
- `--apply` falla si no viene junto con `--experimental-apply`.
- Apply requiere git limpio, hashes esperados por target, paths seguros, operaciones sin solapamiento y diff generado en la misma ejecucion.
- Apply puede escribir archivos con temp file + replace por archivo; no puede stagear, commitear, formatear, renombrar, chmod, borrar directorios ni tocar `.git/**`, `.mi-lsp/**`, binarios o `.docs/wiki/_mi-lsp/read-model.toml`.
- Si una escritura falla, debe intentar restaurar bytes previos de archivos ya tocados y devolver evidencia de rollback via error/guardrail.
- `replace_regex_limited` requiere `max_replacements` y no permite regex multilinea.
- `edit-plan-v2` agrega `targets[].language` (`go|csharp|typescript|python`, inferible por extension cuando falta), `targets[].symbol.receiver` y operaciones estructurales language-aware.
- En esta version las operaciones AST implementadas son solo Go: `replace_go_function`, `replace_go_function_body`, `insert_go_function_after`, `ensure_go_import` y `remove_go_import`.
- Para `csharp`, `typescript` y `python`, cualquier operacion AST v2 debe fallar antes de generar diff con error accionable `language_not_supported`.
- `edit-plan-v1` conserva las operaciones textuales y debe seguir funcionando sobre fixtures Go, C#, TypeScript y Python.

## Data model

- `EditPlanRequest`
- `EditPlanTarget`
- `EditPlanOperation`
- `EditPlanResult`
- `QueryEnvelope`

## Codigos de error

- `QRY_EDIT_PLAN_INVALID_PACKET`
- `QRY_EDIT_PLAN_UNSAFE_PATH`
- `QRY_EDIT_PLAN_HASH_MISMATCH`
- `QRY_EDIT_PLAN_OVERLAP`
- `QRY_EDIT_PLAN_APPLY_REQUIRES_EXPERIMENTAL`
- `QRY_EDIT_PLAN_DIRTY_GIT`
- `QRY_EDIT_PLAN_LANGUAGE_NOT_SUPPORTED`
- `QRY_EDIT_PLAN_INVALID_GO_AST`

## Trazabilidad de tests

- Positivo: `TP-QRY / TC-QRY-118`
- Positivo: `TP-QRY / TC-QRY-119`
- Positivo: `TP-QRY / TC-QRY-120`
- Negativo: `TP-QRY / TC-QRY-121`
- Negativo: `TP-QRY / TC-QRY-122`
- Negativo: `TP-QRY / TC-QRY-123`
- Negativo: `TP-QRY / TC-QRY-124`
- Negativo: `TP-QRY / TC-QRY-125`
- Positivo: `TP-QRY / TC-QRY-126`
- Positivo: `TP-QRY / TC-QRY-127`
- Positivo: `TP-QRY / TC-QRY-128`
- Positivo: `TP-QRY / TC-QRY-129`
- Positivo: `TP-QRY / TC-QRY-130`
- Negativo: `TP-QRY / TC-QRY-131`
- Negativo: `TP-QRY / TC-QRY-132`
- Negativo: `TP-QRY / TC-QRY-133`
- Positivo: `TP-QRY / TC-QRY-134`

## Fuera de alcance

- Backends AST reales para C#, TypeScript y Python.
- Ediciones AST fuera de Go.
- Staging o commit automatico.
- Formateo externo automatico.
- Creacion, renombre, chmod o borrado de archivos/directorios.
