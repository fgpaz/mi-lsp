---
wiki_contract: WIKI-CONTRACT-v1
id: RF-QRY-017
kind: requirement
status: active
title: RF-QRY-017 - Seleccionar impacto conservador de cambios con nav affected
links:
  implements:
  - FL-QRY-01
  code:
  - path: internal/cli/nav.go
    role: behavior
  - path: internal/service/affected.go
    role: behavior
  - path: internal/service/app.go
    role: behavior
  - path: internal/service/diff_context.go
    role: behavior
  - path: internal/store/queries.go
    role: behavior
---

## Propósito

RF-QRY-017 - Seleccionar impacto conservador de cambios con nav affected

### Descripción

Exponer `mi-lsp nav affected` como selector conservador de impacto para agentes. La superficie toma paths explícitos, stdin o git diff, y devuelve items estables de código, pruebas y documentacion canónica que conviene revisar después de un cambio.

El resultado no promete precision de grafo completo. Mientras no exista `symbol_edges`, la salida debe declarar heuristicas con `warnings`, `reason` y `confidence`.

### Actor principal

Skill / Agente / CLI

### FL origen

FL-QRY-01

## Comportamiento ordenado

### Estado

implemented

### TP asociado

TP-QRY

### Reglas

- No reportar `.mi-lsp/**`, `.git/**`, `.docs/raw/**` ni `.docs/auditoria/**` como impacto funcional.
- No afirmar transitividad ni cobertura completa hasta que exista un grafo persistido de edges.
- Usar catálogo SQLite cuando este disponible para adjuntar evidencia de símbolos, pero no fallar si el catálogo no esta listo.
- Para Go, `--include-tests` infiere `go test ./<dir>`.
- Para C#, `--include-tests` infiere `dotnet test worker-dotnet/MiLsp.Worker.sln`.
- Para TypeScript, `--include-tests` infiere `npm test -- <dir>`.
- `--test-command` prevalece sobre cualquier inferencia.
- `--include-docs` debe mapear familias canónicas de path: CLI a `09/CT`, store a `08/DB`, daemon a `07/TECH`, worker a `CT-DAEMON-WORKER`, service query a RF/TP/CT.
- En ausencia de cambios, devolver `ok=true`, `items=[]` y warning explicito.

### Fuera de alcance

- Persistir `symbol_edges`.
- Calcular callees/callers transitivos.
- Elegir tests por cobertura real.
- Crear comandos `nav callers`, `nav callees`, `nav impact` o `nav path`.

## Superficies

CLI `mi-lsp`, con operaciones y selectores detallados en el comportamiento del requisito.

## Entradas/salidas tipadas

### Entradas

- `paths...`: paths relativos al workspace.
- `--from-git-diff`: agrega paths desde git diff.
- `--changed-ref <ref>`: ref base del diff; `HEAD` usa working tree, staged y untracked.
- `--stdin`: lee paths desde stdin como JSON array, objeto `{paths:[]}`, lista por líneas o lista separada por comas.
- `--include-tests`: agrega comandos de prueba sugeridos.
- `--include-docs`: agrega docs canónicos probablemente afectados.
- `--quiet`: conserva items estables pero omite hints no esenciales.
- `--test-command`: sobreescribe el comando de prueba inferido.

Antes de leer el grafo, `nav.affected` comprueba solo los paths explícitos, en modo de solo lectura y respetando cambios ya confirmados en WAL. Con límite de 1 s, compara hashes y refresca sincrónicamente solo catálogo y `doc_records`; no ejecuta extractores de grafo/Roslyn ni genera escrituras en segundo plano. La publicación es transaccional, verifica el contexto antes del commit y omite `PRAGMA optimize` bajo deadline. Si el refresh falla o vence, se conserva la evidencia publicada y se informa la limitación. Un commit completado cuenta como refresh exitoso aunque el deadline venza inmediatamente después. Si la generation de grafo no puede respaldar el impacto por stale, la salida se limita a heurísticas con `confidence=0`, omite generation/evidencia graph-native y emite el comando de reconstrucción `mi-lsp index --workspace "<alias>"`.

### Salida

Cada item debe exponer:

- `kind`: `code`, `test` o `doc`.
- `path`: path estable relativo al workspace o paquete/directorio para comandos de test.
- `reason`: causa observable de seleccion.
- `confidence`: número entre 0 y 1 que expresa confianza heuristica.
- `suggested_command`: comando opcional para actuar sobre el item.
- `evidence`: lista corta de fuentes (`git_diff`, `stdin`, `explicit_path`, `symbol:*`, `change_type:*`, `trigger_path:*`).

El envelope debe usar `backend=git+catalog+heuristic` y conservar `warnings` cuando use heuristicas.

### Modelo de datos
- `AffectedItem`
- `QueryEnvelope`
- `SymbolRecord`
- `FileRecord`

## Errores

### Códigos de error

- `QRY_AFFECTED_GIT_UNAVAILABLE`
- `QRY_AFFECTED_WORKSPACE_NOT_FOUND`

## Criterios de aceptación (numerados)

1. No reportar `.mi-lsp/**`, `.git/**`, `.docs/raw/**` ni `.docs/auditoria/**` como impacto funcional.
2. No afirmar transitividad ni cobertura completa hasta que exista un grafo persistido de edges.
3. Usar catálogo SQLite cuando este disponible para adjuntar evidencia de símbolos, pero no fallar si el catálogo no esta listo.
4. Para Go, `--include-tests` infiere `go test ./<dir>`.
5. Para C#, `--include-tests` infiere `dotnet test worker-dotnet/MiLsp.Worker.sln`.
6. Para TypeScript, `--include-tests` infiere `npm test -- <dir>`.
7. `--test-command` prevalece sobre cualquier inferencia.
8. `--include-docs` debe mapear familias canónicas de path: CLI a `09/CT`, store a `08/DB`, daemon a `07/TECH`, worker a `CT-DAEMON-WORKER`, service query a RF/TP/CT.
9. En ausencia de cambios, devolver `ok=true`, `items=[]` y warning explicito.

### Trazabilidad de tests

- Positivo: `TP-QRY / TC-QRY-110`
- Positivo: `TP-QRY / TC-QRY-111`
- Positivo: `TP-QRY / TC-QRY-112`
- Positivo: `TP-QRY / TC-QRY-113`
- Negativo: `TP-QRY / TC-QRY-114`
- Negativo: `TP-QRY / TC-QRY-115`
- Positivo: `TP-QRY / TC-QRY-116`
