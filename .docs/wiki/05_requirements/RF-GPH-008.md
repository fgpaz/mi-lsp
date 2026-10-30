---
wiki_contract: WIKI-CONTRACT-v1
id: RF-GPH-008
kind: requirement
status: draft
title: RF-GPH-008 - Extraer evidencia compiler-first con madurez por backend
links:
  implements:
  - FL-GPH-01
  code:
  - path: internal/service/app.go
    role: behavior
---

## Propósito

RF-GPH-008 - Extraer evidencia compiler-first con madurez por backend

## Comportamiento ordenado

### 1. Resultado requerido

Normalizar evidencia de Roslyn, Go, tsserver y Pyright mediante un adapter contract comun sin degradar texto a semantica. Cada backend declara capabilities, version, completeness conocida y omissions. Una familia solo pasa a `stable` cuando supera los mismos oraculos de identidad, relation precision/recall, determinismo, incrementalidad y cross-RID.

### 2. Adapter contract

Entrada: workspace/repository identity, project/module, owner paths, source digests, backend config/version, generation candidate y cancellation token.

Salida `GraphObservationBatch`:

- `nodes` con identity fields y declaration evidence;
- `edge_claims` con relation, endpoints o candidatos y status;
- `evidence` con source URI/range/digest y backend provenance;
- `unresolved` y `omissions` tipados;
- `coverage` por capability/relation/owner;
- `determinism_digest`, elapsed y resource stats.

El adapter no escribe SQLite, no publica generations y no decide autoridad wiki. Graph Kernel valida y persiste.

### 3. Matriz de madurez inicial

- **Backend**: C# / Roslyn; **Stable inicial**: declarations, contains, references, calls, implements, extends con SymbolKey/documentation ID; **Experimental/gated**: dataflow especializado; **Regla**: fuente primaria compiler-backed
- **Backend**: Go / parser + types/gopls; **Stable inicial**: declarations, contains, imports; calls/references solo con `go/types` o gopls resuelto; **Experimental/gated**: runtime/dataflow; **Regla**: AST-only se marca `extracted`, no compiler fact
- **Backend**: TypeScript / tsserver; **Stable inicial**: ninguna relation stable hasta fixture y lifecycle PASS; **Experimental/gated**: declarations/references/calls/extends/implements; **Regla**: backend opcional; ausencia es omission
- **Backend**: Python / Pyright; **Stable inicial**: ninguna relation stable hasta fixture y lifecycle PASS; **Experimental/gated**: declarations/references/calls/imports; **Regla**: extractor lexical solo cataloga candidatos
- **Backend**: Kotlin / fork fwcd compiler 2.1.0; **Stable inicial**: ninguna hasta conformance; **Experimental/gated**: declarations/references exact compiler en batch experimental parcial; **Regla**: registro aditivo Kotlin; calls unsupported; ReadyForStaging estricto

Agregar o promover una capability exige actualizar esta matriz, fixtures, backend version range y gate de release. TS/Python no bloquean el slice C#/Go si se reportan explicitamente como gated.

### 4. Reglas de evidencia

- C#/Roslyn conserva solution/project/assembly context y no une símbolos por nombre simple.
- Go distingue package import path, receiver y firma; build tags/config forman parte del source fingerprint.
- tsserver/Pyright se supervisan como runtimes opcionales; restart/timeout no produce relaciones textuales sustitutas.
- Un fallback lexical puede crear `file`, `document`, catalog entry o unresolved; no `calls`, `implements`, `extends`, `reads` o `writes` exactos.
- Ranges son evidencia y no NodeKey.
- Diagnostics/raw compiler logs no se persisten; solo reason codes y summaries sanitizados.

### 5. Lifecycle y cancelacion

Adapters deben ser cancelables por owner/batch, tener timeout, health y version observable. Un crash inválida el batch parcial, limpia recursos y conserva la generation activa. Reintento usa el mismo source/config fingerprint y no mezcla resultados de dos versiones.

### 6. Invariantes

- Orden de estabilizacion: C#/Roslyn, Go, TypeScript, Python.
- Misma fixture/config/backend version produce observation digest identico en 30 reruns.
- Ningún backend afirma capabilities no descritas.
- Unsupported, partial y unavailable son estados visibles y medibles.
- Imports Graphify son `external:graphify`, nunca evidencia compiler-first.
- Adapter output no requiere MCP, red implicita ni graph store externo.

### Kotlin: exportación fuente v2 y admisión aditiva sin promoción

[[TECH-KOTLIN-BACKEND]] fija el pin stock y el fork honesto `1.3.13+mi-lsp.obs2`, método `kotlin/miLsp/compilerObservations/v2`, capability `miLspCompilerObservationsV2`. `worker-kotlin/` aporta fuente/patch/build explícito/licencias. El descriptor main JVM resuelto es una afirmación externa conservadora; se usan JARs/fuentes/JDK/target exactos y fingerprints, no backup ni PSI/text como hechos. Referencias raw solo desde `REFERENCE_TARGET`; calls siguen unsupported sin resolved-call export. Módulos, firmas/sobrecargas/receivers y evidence quedan separados.

[[ADR-KOTLIN-GRAPH-ADMISSION-V1]] documenta la decisión delegada de coord-milsp: registrar exactamente backend `kotlin-language-server`, lenguaje `kotlin`, resolución `kotlin/binding-context`, kinds type/function/method/property y references con endpoints locales representables. Sustituye solo el guardrail local K2, no Store/DDL/NodeKey-v1 ni los gates de promoción. El adapter entrega batch canónico sin sello. Completeness de dominio sale de la cobertura real de declarations/references y del recibo de configuración certificable; lo no representable se cuenta y queda partial. Calls unsupported permanece visible y fuera del batch. ValidateCanonical→rebase→seal→ReadyForStaging no se relaja.

Gate de promoción, conservador, en `KotlinPromotionAllowed`: no consulta env, permiso ni etiqueta. Esta revisión devuelve false aunque la evidencia venga llena (`promotion_not_granted_this_revision`). Oráculos que faltan antes de publicar, exactos: `oracle_identity_fixture`, `oracle_relation_precision_recall`, `oracle_determinism_30_reruns`, `oracle_incremental_vs_clean`, `oracle_cross_rid`, `oracle_conformance_final_verify`. Tras recheck1 consumido permanecen **NOTRUN** (cadena cortada en `graph_observe`); pin init `ebccdf78…` en `52964ee` es concordancia estática Go/Kotlin, no oráculo cumplido. Contadores FV/audit/recheck del objetivo: **1/1/1 agotados**. Owner coord-milsp, 2026-10-17. Unix experimental, Windows unavailable hasta Job Objects; ningún PASS ni admisión stageable.

## Superficies

CLI `mi-lsp`, con operaciones y selectores detallados en el comportamiento del requisito.

## Entradas/salidas tipadas

Las entradas, salidas, enums y límites forman parte del contrato de comportamiento ordenado de este documento; los campos conservan sus nombres de protocolo.

## Errores

### 7. Errores tipados

- **Código**: `GPH_BACKEND_UNAVAILABLE`; **Causa**: runtime/worker ausente; **Respuesta**: omission con setup hint
- **Código**: `GPH_BACKEND_VERSION_UNSUPPORTED`; **Causa**: version fuera de contrato; **Respuesta**: reject adapter
- **Código**: `GPH_BACKEND_CAPABILITY_UNSUPPORTED`; **Causa**: relation no declarada; **Respuesta**: omission, no fallback semántico
- **Código**: `GPH_BACKEND_PARTIAL_BATCH`; **Causa**: cancel/crash/timeout; **Respuesta**: descartar batch
- **Código**: `GPH_BACKEND_PROVENANCE_MISSING`; **Causa**: version/source/claim incompleto; **Respuesta**: reject observation
- **Código**: `GPH_BACKEND_NONDETERMINISTIC`; **Causa**: digest difiere; **Respuesta**: FAIL de madurez

## Criterios de aceptación (numerados)

1. Fixtures Roslyn y Go positivos/negativos; TS/Python gated y, cuando se promuevan, mismos casos.
2. Precision/recall por relation y backend, negative violations, omissions y coverage denominators.
3. Incremental versus clean, cancel/crash/restart y cross-RID en RIDs soportados.

### 8. Aceptacion y trazabilidad

- Fixtures Roslyn y Go positivos/negativos; TS/Python gated y, cuando se promuevan, mismos casos.
- Precision/recall por relation y backend, negative violations, omissions y coverage denominators.
- Incremental versus clean, cancel/crash/restart y cross-RID en RIDs soportados.
- `TP-GPH / TP-GPH-003 / TC-GPH-015..022`.

```yaml
harness_protocol: SDD-HARNESS-v1
id: RF-GPH-008
kind: requirement
audience: llm-first
imports:
- .docs/wiki/04_flows/FL-GPH-01.md
exports:
- RF-GPH-008
- RF-GPH-008 - Extraer evidencia compiler-first con madurez por backend
agent_must_read:
- .docs/wiki/05_requirements/RF-GPH-008.md
- .docs/wiki/04_flows/FL-GPH-01.md
- .docs/wiki/06_tests/TP-GPH.md
agent_may_edit:
- .docs/wiki/05_requirements/RF-GPH-008.md
agent_must_not_edit:
- .docs/wiki/00_governance.md
- AGENTS.md
- internal/service/app.go
verify:
- 'Para los casos de este requisito en .docs/wiki/06_tests/TP-GPH.md, ejecutar el
  comando publicado: go test ./internal/model/... ./internal/indexer/... ./internal/service/...
  ./internal/daemon/... (la evidencia de ejecución debe registrarse por separado).'
stop_if:
- Detenerse si la propuesta requiere alterar criterios, decisiones o comportamiento
  más allá de RF-GPH-008.
evidence:
- .docs/wiki/05_requirements/RF-GPH-008.md
- .docs/wiki/06_tests/TP-GPH.md
```
