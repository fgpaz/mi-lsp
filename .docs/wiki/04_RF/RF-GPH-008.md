---
id: RF-GPH-008
title: Extraer evidencia compiler-first con madurez por backend
status: planned
flows:
  - FL-GPH-01
tests:
  - .docs/wiki/06_pruebas/TP-GPH.md
---

```yaml
harness_protocol: SDD-HARNESS-v1
id: "RF-GPH-008"
kind: "support-doc"
audience: "llm-first"
imports:
  - '[[00_gobierno_documental]]'
  - '[[FL-GPH-01]]'
  - '[[RF-GPH-001]]'
  - '[[RF-GPH-003]]'
  - '[[RF-GPH-004]]'
exports:
  - 'RF-GPH-008'
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/03_FL/FL-GPH-01.md
  - .docs/wiki/04_RF/RF-GPH-001.md
  - .docs/wiki/04_RF/RF-GPH-003.md
  - .docs/wiki/04_RF/RF-GPH-004.md
  - .docs/wiki/04_RF/RF-GPH-008.md
agent_may_edit:
  - .docs/wiki/04_RF/RF-GPH-008.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
evidence:
  - .docs/wiki/04_RF/RF-GPH-008.md
```

# RF-GPH-008 - Extraer evidencia compiler-first con madurez por backend

## 1. Resultado requerido

Normalizar evidencia de Roslyn, Go, tsserver y Pyright mediante un adapter contract comun sin degradar texto a semantica. Cada backend declara capabilities, version, completeness conocida y omissions. Una familia solo pasa a `stable` cuando supera los mismos oraculos de identidad, relation precision/recall, determinismo, incrementalidad y cross-RID.

## 2. Adapter contract

Entrada: workspace/repository identity, project/module, owner paths, source digests, backend config/version, generation candidate y cancellation token.

Salida `GraphObservationBatch`:

- `nodes` con identity fields y declaration evidence;
- `edge_claims` con relation, endpoints o candidatos y status;
- `evidence` con source URI/range/digest y backend provenance;
- `unresolved` y `omissions` tipados;
- `coverage` por capability/relation/owner;
- `determinism_digest`, elapsed y resource stats.

El adapter no escribe SQLite, no publica generations y no decide autoridad wiki. Graph Kernel valida y persiste.

## 3. Matriz de madurez inicial

| Backend | Stable inicial | Experimental/gated | Regla |
|---|---|---|---|
| C# / Roslyn | declarations, contains, references, calls, implements, extends con SymbolKey/documentation ID | dataflow especializado | fuente primaria compiler-backed |
| Go / parser + types/gopls | declarations, contains, imports; calls/references solo con `go/types` o gopls resuelto | runtime/dataflow | AST-only se marca `extracted`, no compiler fact |
| TypeScript / tsserver | ninguna relation stable hasta fixture y lifecycle PASS | declarations/references/calls/extends/implements | backend opcional; ausencia es omission |
| Python / Pyright | ninguna relation stable hasta fixture y lifecycle PASS | declarations/references/calls/imports | extractor lexical solo cataloga candidatos |

Agregar o promover una capability exige actualizar esta matriz, fixtures, backend version range y gate de release. TS/Python no bloquean el slice C#/Go si se reportan explicitamente como gated.

## 4. Reglas de evidencia

- C#/Roslyn conserva solution/project/assembly context y no une simbolos por nombre simple.
- Go distingue package import path, receiver y firma; build tags/config forman parte del source fingerprint.
- tsserver/Pyright se supervisan como runtimes opcionales; restart/timeout no produce relaciones textuales sustitutas.
- Un fallback lexical puede crear `file`, `document`, catalog entry o unresolved; no `calls`, `implements`, `extends`, `reads` o `writes` exactos.
- Ranges son evidencia y no NodeKey.
- Diagnostics/raw compiler logs no se persisten; solo reason codes y summaries sanitizados.

## 5. Lifecycle y cancelacion

Adapters deben ser cancelables por owner/batch, tener timeout, health y version observable. Un crash invalida el batch parcial, limpia recursos y conserva la generation activa. Reintento usa el mismo source/config fingerprint y no mezcla resultados de dos versiones.

## 6. Invariantes

- Orden de estabilizacion: C#/Roslyn, Go, TypeScript, Python.
- Misma fixture/config/backend version produce observation digest identico en 30 reruns.
- Ningun backend afirma capabilities no descritas.
- Unsupported, partial y unavailable son estados visibles y medibles.
- Imports Graphify son `external:graphify`, nunca evidencia compiler-first.
- Adapter output no requiere MCP, red implicita ni graph store externo.

## 7. Errores tipados

| Codigo | Causa | Respuesta |
|---|---|---|
| `GPH_BACKEND_UNAVAILABLE` | runtime/worker ausente | omission con setup hint |
| `GPH_BACKEND_VERSION_UNSUPPORTED` | version fuera de contrato | reject adapter |
| `GPH_BACKEND_CAPABILITY_UNSUPPORTED` | relation no declarada | omission, no fallback semantico |
| `GPH_BACKEND_PARTIAL_BATCH` | cancel/crash/timeout | descartar batch |
| `GPH_BACKEND_PROVENANCE_MISSING` | version/source/claim incompleto | reject observation |
| `GPH_BACKEND_NONDETERMINISTIC` | digest difiere | FAIL de madurez |

## 8. Aceptacion y trazabilidad

- Fixtures Roslyn y Go positivos/negativos; TS/Python gated y, cuando se promuevan, mismos casos.
- Precision/recall por relation y backend, negative violations, omissions y coverage denominators.
- Incremental versus clean, cancel/crash/restart y cross-RID en RIDs soportados.
- `TP-GPH / TP-GPH-003 / TC-GPH-015..022`.
