# TP-GPH

```yaml
harness_protocol: SDD-HARNESS-v1
id: "TP-GPH"
kind: "support-doc"
audience: "llm-first"
imports:
  - '[[00_gobierno_documental]]'
  - '[[FL-GPH-01]]'
  - '[[FL-GPH-02]]'
  - '[[FL-GPH-03]]'
  - '[[RF-GPH-001]]'
  - '[[RF-GPH-002]]'
  - '[[RF-GPH-003]]'
  - '[[RF-GPH-004]]'
  - '[[RF-GPH-005]]'
  - '[[RF-GPH-006]]'
  - '[[RF-GPH-007]]'
  - '[[RF-GPH-008]]'
  - '[[RF-GPH-009]]'
  - '[[RF-GPH-010]]'
  - '[[RF-GPH-011]]'
exports:
  - 'TP-GPH'
  - 'TP-GPH-001'
  - 'TP-GPH-002'
  - 'TP-GPH-003'
  - 'TP-GPH-004'
  - 'TP-GPH-005'
  - 'TP-GPH-006'
  - 'TP-GPH-007'
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/03_FL/FL-GPH-01.md
  - .docs/wiki/03_FL/FL-GPH-02.md
  - .docs/wiki/03_FL/FL-GPH-03.md
  - .docs/wiki/04_RF/RF-GPH-001.md
  - .docs/wiki/04_RF/RF-GPH-002.md
  - .docs/wiki/04_RF/RF-GPH-003.md
  - .docs/wiki/04_RF/RF-GPH-004.md
  - .docs/wiki/04_RF/RF-GPH-005.md
  - .docs/wiki/04_RF/RF-GPH-006.md
  - .docs/wiki/04_RF/RF-GPH-007.md
  - .docs/wiki/04_RF/RF-GPH-008.md
  - .docs/wiki/04_RF/RF-GPH-009.md
  - .docs/wiki/04_RF/RF-GPH-010.md
  - .docs/wiki/04_RF/RF-GPH-011.md
  - .docs/wiki/06_pruebas/TP-GPH.md
agent_may_edit:
  - .docs/wiki/06_pruebas/TP-GPH.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
evidence:
  - .docs/wiki/06_pruebas/TP-GPH.md
```

## Contrato comun de prueba

- Fixtures versionadas: C#, Go, TypeScript, Python, wiki, mixed, cross-workspace, extensions y relations (`positive`, `negative`, `ambiguous`, `unresolved`, `not-comparable`).
- Oraculos: JSON canonico/goldens, hashes de fixture, invariantes SQLite, source digests, cross-RID y comparacion incremental-clean. Un LLM o screenshot nunca es el unico juez.
- Toda medicion competitiva fija Graphify `9bf14a4931658152969586ace39eb965c010f0d1`, baseline mi-lsp `a251ab1f8db4e96f029926fbef275b078a20a111`, fixture digest, comando, entorno y raw samples.
- Benchmarks relevantes ejecutan 30 repeticiones por variante con el mismo protocolo cold/warm/incremental. Metrica unavailable es `BLOCKED/NOT_COMPARABLE`, no PASS.
- Evidencia durable es sanitizada: no secretos, raw compiler logs ni payloads arbitrarios. Cada fila incluye generation/cross-RID o explica por que no aplica.
- Gates fail-closed: governance, hash, comparator, baseline, cross-RID, determinismo, no-MCP/no-network, rollback y negative violations.

## TP-GPH-001 - Identidad, NodeKey y cross-RID

**Cobertura:** RF-GPH-001 y modelo semantico.

| Caso | Tipo | Descripcion / oraculo |
|---|---|---|
| TC-GPH-001 | positivo | golden vectors validan payload length-prefixed, SHA-256 BLOB(32), hex externo y cross-RID |
| TC-GPH-002 | positivo | relocation de root, CRLF/LF y cambio solo de rango conservan NodeKey; cambio semantico lo cambia |
| TC-GPH-003 | positivo | 30 reruns y RIDs soportados producen identidad/cross-RID byte-identical |
| TC-GPH-004 | negativo | repository identity/campo requerido ausente, path absoluto/traversal o backend inestable queda unresolved |
| TC-GPH-005 | negativo | colision simulada del digest con tupla distinta invalida la generation y no rehasha con sal |
| TC-GPH-006 | negativo | Unicode no NFC, version desconocida, case/path conflictivo o simbolo anonimo sin anchor no se publica |

## TP-GPH-002 - Generation, publicacion, migracion y recovery

**Cobertura:** RF-GPH-002.

| Caso | Tipo | Descripcion / oraculo |
|---|---|---|
| TC-GPH-007 | positivo | staging completo permanece invisible y pointer swap publica una unica generation sellada |
| TC-GPH-008 | positivo | readers concurrentes observan snapshot viejo o nuevo completo, nunca mezcla |
| TC-GPH-009 | negativo | crash/cancel en cada ventana invalida staging y restaura/conserva pointer anterior |
| TC-GPH-010 | positivo | migracion additive mantiene tablas legacy y dual-read/write transaccional durante compatibilidad |
| TC-GPH-011 | positivo | rollback/downgrade ejercitado vuelve a snapshot/schema compatible sin perdida del backup metadata |
| TC-GPH-012 | negativo | pointer conflict reintenta explicitamente; publish del mismo ID/payload es idempotente |
| TC-GPH-013 | negativo | mismo generation ID con digest/counts distintos bloquea como corrupcion |
| TC-GPH-014 | negativo | query-time migration o repair implicito falla el test de solo lectura |

## TP-GPH-003 - Edges, adapters e incrementalidad

**Cobertura:** RF-GPH-003, RF-GPH-004 y RF-GPH-008.

| Caso | Tipo | Descripcion / oraculo |
|---|---|---|
| TC-GPH-015 | positivo | Roslyn emite declarations/contains/references/calls/implements/extends exactos con provenance |
| TC-GPH-016 | positivo | Go emite declarations/contains/imports y solo promueve calls/references cuando types/gopls resuelve |
| TC-GPH-017 | negativo | tsserver ausente/experimental produce omission; texto no crea edge semantica |
| TC-GPH-018 | negativo | Pyright ausente/experimental y extractor lexical producen candidatos/unresolved, no compiler facts |
| TC-GPH-019 | negativo | ambiguous, stale o endpoint missing produce GraphUnresolved; validacion confirma cero dangling edges |
| TC-GPH-020 | positivo | create/change/delete/rename reemplaza solo owners afectados y conserva snapshot equivalente al clean |
| TC-GPH-021 | positivo | cambio de superficie recalcula fanout; uncertainty/config/backend drift fuerza full rebuild explicitamente |
| TC-GPH-022 | negativo | cancel/crash/partial batch/provenance faltante descarta staging; stale_rate final es 0.0 |

## TP-GPH-004 - Navegacion bounded y determinista

**Cobertura:** RF-GPH-005.

| Caso | Tipo | Descripcion / oraculo |
|---|---|---|
| TC-GPH-023 | positivo | neighbors respeta direction/relation/depth y ordering canonico |
| TC-GPH-024 | positivo | callers usa calls entrante y callees calls saliente sin mezclar references |
| TC-GPH-025 | positivo | path elige shortest path y resuelve empates lexicograficamente |
| TC-GPH-026 | positivo | explain devuelve claim, endpoints, evidence y marca inferred sin elevarlo |
| TC-GPH-027 | positivo | graph stats cuenta status/backend/unresolved; validate detecta schema/pointer/endpoint/cross-RID |
| TC-GPH-028 | negativo | selector ausente/ambiguo y generation invalid/retired devuelven error/candidatos accionables |
| TC-GPH-029 | negativo | depth/result/token ceiling y cursor stale producen reject o truncation determinista |
| TC-GPH-030 | positivo | direct/daemon son semanticamente iguales y hash de tablas confirma cero writes/migrations |

## TP-GPH-005 - Impacto y autoridad wiki-codigo

**Cobertura:** RF-GPH-006 y RF-GPH-007.

| Caso | Tipo | Descripcion / oraculo |
|---|---|---|
| TC-GPH-031 | positivo | changed path resuelve seeds y direct impact con evidence path exacto |
| TC-GPH-032 | positivo | transitive mode usa solo allowlist/direccion/costo y separa inferred |
| TC-GPH-033 | positivo | tests/docs se agregan por edges/anchors; convenciones quedan heuristic |
| TC-GPH-034 | negativo | relation textual, ambiguous o unresolved no infla positivos ni viola fixture negative |
| TC-GPH-035 | comparativo | precision/recall/F1/negative violations no regresan contra affected previo ni Graphify comparable |
| TC-GPH-036 | positivo | docs canonicos siguen primeros y primary doc no cambia al enriquecer con codigo |
| TC-GPH-037 | negativo | governance/projection stale bloquea; canon-code conflict produce drift, nunca override |
| TC-GPH-038 | positivo | context pack conserva cadena obligatoria y reduce tokens solo con coverage/omissions explicitos |

## TP-GPH-006 - Federacion, MILX-v1 y packs

**Cobertura:** RF-GPH-009, RF-GPH-010 y seguridad de RF-GPH-011.

| Caso | Tipo | Descripcion / oraculo |
|---|---|---|
| TC-GPH-039 | positivo | dos member generations producen snapshot global/cross-edge determinista por cross-RID |
| TC-GPH-040 | negativo | member/target ausente, schema incompatible o cross-RID conflictivo queda omission/unresolved |
| TC-GPH-041 | positivo | query global direct/daemon mantiene budgets y no escribe member stores |
| TC-GPH-042 | negativo | documento de un workspace no gobierna otro sin enlace canonico bilateral |
| TC-GPH-043 | positivo | framing length-prefixed, describe/prepare/execute/cancel/health/shutdown y manifest golden |
| TC-GPH-044 | negativo | capability desconocida, graph/wiki write, secret, network o MCP se deniega antes de ejecutar |
| TC-GPH-045 | negativo | timeout/crash/cancel termina solo process tree de extension y cleanup queda probado |
| TC-GPH-046 | negativo | frame malformed/oversize/schema invalido se descarta sin mutar graph/core |
| TC-GPH-047 | positivo | Graphify import queda `external:graphify` advisory y no reemplaza NodeKey/evidence/wiki |
| TC-GPH-048 | positivo | pack deterministic y cache key/invalidation incluyen generation, version, digest, params y authority |

## TP-GPH-007 - Victory Lab, performance y cierre del slice

**Cobertura:** RF-GPH-011 y gates transversales RF-GPH-001..010.

| Caso | Tipo | Descripcion / oraculo |
|---|---|---|
| TC-GPH-049 | positivo | manifest/revision/fixture hashes se verifican; CRLF/LF equivalen sin ocultar mutacion semantica |
| TC-GPH-050 | comparativo | 30 repeticiones por current, Graphify y baseline previo con mismo fixture/protocolo |
| TC-GPH-051 | comparativo | correctness, precision, recall, F1 y negative violations reportan numerador/denominador |
| TC-GPH-052 | comparativo | token count canonico mide objetivo <=0.70x sin omitir autoridad/evidence requerida |
| TC-GPH-053 | comparativo | warm p95 mide objetivo <=0.80x y hot-path guard rail baseline x1.10 +25 ms |
| TC-GPH-054 | comparativo | peak RSS mide objetivo <=0.50x sin materializar grafo completo |
| TC-GPH-055 | comparativo | incremental mide objetivo <=0.70x y stale_rate=0.0 contra clean oracle |
| TC-GPH-056 | positivo | 30 outputs canonicos, NodeKeys, cross-RIDs y digests son identicos por variante/RID |
| TC-GPH-057 | negativo | security scan/runtime prueba no-MCP, no-network, no secret leak, no query write y rollback |
| TC-GPH-058 | negativo | metrica/comparator/raw sample unavailable, threshold fallido o claim global desde subset bloquea PASS |

## Comandos de verificacion minimos

```text
python -m unittest discover -s scripts/bench/victory_lab -p "test_*.py"
python scripts/bench/victory_lab/runner.py --manifest benchmarks/victory-lab/v1/manifest.json --repetitions 30 --output <evidence-dir>
python scripts/bench/victory_lab/report.py --input <evidence-dir> --output <report.json>
gofmt -l cmd internal
go vet ./...
go test ./...
mi-lsp nav governance --workspace mi-lsp --format toon
mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
```

PASS final requiere los siete TP, comparadores fijados, raw evidence, cross-RID, rollback, seguridad y autoridad wiki. Un caso `planned` no cuenta como ejecutado hasta enlazar test automatizado o evidencia manual determinista.
