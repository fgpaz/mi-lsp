---
id: RF-QRY-017
title: Seleccionar impacto conservador de cambios con nav affected
implements:
  - internal/cli/nav.go
  - internal/service/app.go
  - internal/service/affected.go
  - internal/service/diff_context.go
  - internal/store/queries.go
tests:
  - internal/cli/nav_test.go
  - internal/service/affected_test.go
---

```yaml
harness_protocol: SDD-HARNESS-v1
id: "RF-QRY-017"
kind: "support-doc"
audience: "llm-first"
imports:
  - '[[00_gobierno_documental]]'
  - '[[RF-QRY-017]]'
exports:
  - 'RF-QRY-017'
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/04_RF/RF-QRY-017.md
agent_may_edit:
  - .docs/wiki/04_RF/RF-QRY-017.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
evidence:
  - .docs/wiki/04_RF/RF-QRY-017.md
```

# RF-QRY-017 - Seleccionar impacto conservador de cambios con nav affected

## Descripcion

Exponer `mi-lsp nav affected` como selector conservador de impacto para agentes. La superficie toma paths explicitos, stdin o git diff, y devuelve items estables de codigo, pruebas y documentacion canonica que conviene revisar despues de un cambio.

El resultado no promete precision de grafo completo. Mientras no exista `symbol_edges`, la salida debe declarar heuristicas con `warnings`, `reason` y `confidence`.

## Actor principal

Skill / Agente / CLI

## FL origen

FL-QRY-01

## Estado

implemented

## TP asociado

TP-QRY

## Entradas

- `paths...`: paths relativos al workspace.
- `--from-git-diff`: agrega paths desde git diff.
- `--changed-ref <ref>`: ref base del diff; `HEAD` usa working tree, staged y untracked.
- `--stdin`: lee paths desde stdin como JSON array, objeto `{paths:[]}`, lista por lineas o lista separada por comas.
- `--include-tests`: agrega comandos de prueba sugeridos.
- `--include-docs`: agrega docs canonicos probablemente afectados.
- `--quiet`: conserva items estables pero omite hints no esenciales.
- `--test-command`: sobreescribe el comando de prueba inferido.

## Salida

Cada item debe exponer:

- `kind`: `code`, `test` o `doc`.
- `path`: path estable relativo al workspace o paquete/directorio para comandos de test.
- `reason`: causa observable de seleccion.
- `confidence`: numero entre 0 y 1 que expresa confianza heuristica.
- `suggested_command`: comando opcional para actuar sobre el item.
- `evidence`: lista corta de fuentes (`git_diff`, `stdin`, `explicit_path`, `symbol:*`, `change_type:*`, `trigger_path:*`).

El envelope debe usar `backend=git+catalog+heuristic` y conservar `warnings` cuando use heuristicas.

## Reglas

- No reportar `.mi-lsp/**`, `.git/**`, `.docs/raw/**` ni `.docs/auditoria/**` como impacto funcional.
- No afirmar transitividad ni cobertura completa hasta que exista un grafo persistido de edges.
- Usar catalogo SQLite cuando este disponible para adjuntar evidencia de simbolos, pero no fallar si el catalogo no esta listo.
- Para Go, `--include-tests` infiere `go test ./<dir>`.
- Para C#, `--include-tests` infiere `dotnet test worker-dotnet/MiLsp.Worker.sln`.
- Para TypeScript, `--include-tests` infiere `npm test -- <dir>`.
- `--test-command` prevalece sobre cualquier inferencia.
- `--include-docs` debe mapear familias canonicas de path: CLI a `09/CT`, store a `08/DB`, daemon a `07/TECH`, worker a `CT-DAEMON-WORKER`, service query a RF/TP/CT.
- En ausencia de cambios, devolver `ok=true`, `items=[]` y warning explicito.

## Data model

- `AffectedItem`
- `QueryEnvelope`
- `SymbolRecord`
- `FileRecord`

## Codigos de error

- `QRY_AFFECTED_GIT_UNAVAILABLE`
- `QRY_AFFECTED_WORKSPACE_NOT_FOUND`

## Trazabilidad de tests

- Positivo: `TP-QRY / TC-QRY-110`
- Positivo: `TP-QRY / TC-QRY-111`
- Positivo: `TP-QRY / TC-QRY-112`
- Positivo: `TP-QRY / TC-QRY-113`
- Negativo: `TP-QRY / TC-QRY-114`
- Negativo: `TP-QRY / TC-QRY-115`
- Positivo: `TP-QRY / TC-QRY-116`

## Fuera de alcance

- Persistir `symbol_edges`.
- Calcular callees/callers transitivos.
- Elegir tests por cobertura real.
- Crear comandos `nav callers`, `nav callees`, `nav impact` o `nav path`.
