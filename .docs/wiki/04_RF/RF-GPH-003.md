---
id: RF-GPH-003
title: Persistir aristas tipadas con evidencia y unresolved explicito
status: planned
flows:
  - FL-GPH-01
tests:
  - .docs/wiki/06_pruebas/TP-GPH.md
---

```yaml
harness_protocol: SDD-HARNESS-v1
id: "RF-GPH-003"
kind: "support-doc"
audience: "llm-first"
imports:
  - '[[00_gobierno_documental]]'
  - '[[FL-GPH-01]]'
  - '[[RF-GPH-001]]'
  - '[[RF-GPH-002]]'
  - '[[05_modelo_datos]]'
exports:
  - 'RF-GPH-003'
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/03_FL/FL-GPH-01.md
  - .docs/wiki/04_RF/RF-GPH-001.md
  - .docs/wiki/04_RF/RF-GPH-002.md
  - .docs/wiki/05_modelo_datos.md
  - .docs/wiki/04_RF/RF-GPH-003.md
agent_may_edit:
  - .docs/wiki/04_RF/RF-GPH-003.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
evidence:
  - .docs/wiki/04_RF/RF-GPH-003.md
```

# RF-GPH-003 - Persistir aristas tipadas con evidencia y unresolved explicito

## 1. Resultado requerido

Representar relaciones dirigidas sin inflar precision ni ocultar ambiguedad. Toda arista publicable debe enlazar dos `NodeKey` existentes en la misma generacion y tener al menos una evidencia versionada. Una relacion no resuelta se persiste como `GraphUnresolved`; nunca como edge dangling.

## 2. Taxonomia inicial

`contains`, `imports`, `references`, `calls`, `implements`, `extends`, `tests`, `route_to_handler`, `publishes`, `consumes`, `reads`, `writes` y `doc_mentions`.

Agregar un tipo exige version de schema, semantica de direccion, backends habilitados, evidencia minima y tests negativos. `doc_mentions` expresa una referencia documental; no convierte el documento en autoridad sobre una relacion de codigo.

## 3. Estado y confianza

| Estado | Significado | Puede presentarse como hecho del compilador |
|---|---|---|
| `exact` | compiler/LSP resolvio ambos simbolos y relacion | si, con provenance |
| `extracted` | parser estructural versionado resolvio la forma | no como compiler fact; si como extracted |
| `inferred` | regla derivativa sobre evidencia publicada | no; siempre etiquetada |
| `ambiguous` | existen destinos/candidatos multiples | no se publica edge activa |

`ambiguous`, `unsupported`, `stale_source`, `identity_missing` y `endpoint_missing` producen unresolved/omission. Un score numerico opcional no puede elevar `extracted` o `inferred` a `exact`.

## 4. Identidad de edge y evidencia

- `edge_key` = SHA-256 de serializacion `MILSP-EK/v1` con `from_node_key`, `relation`, `to_node_key` y `claim_scope` canonicos. Es dirigido; para una relacion simetrica se materializan reglas explicitas, no sorting accidental de endpoints.
- Varias observaciones del mismo claim comparten edge y se conservan como evidencias separadas.
- `GraphEvidence` contiene `source_uri` repo-relative, rango, backend, compiler/extractor version, source digest, claim observado, confidence/status, generation y cross-RID.
- `evidence_digest` excluye timestamps y usa bytes de fuente normalizados segun el backend; no normaliza semantica arbitrariamente.
- `edge_cross_rid` = `milsp:gph-edge:v1:<lowercase-hex-edge-key>`; cada evidencia tiene un cross-RID derivado de edge key + evidence digest + ordinal canonico.

## 5. Validacion

1. Resolver ambos endpoints por `NodeKey` dentro del staging actual.
2. Validar que relation y backend admitan la combinacion de kinds.
3. Validar source fingerprint y evidence digest.
4. Deduplicar por `edge_key` sin descartar evidencias distintas.
5. Ordenar evidencias por source URI, rango, backend y digest.
6. Enviar casos incompletos a `GraphUnresolved` con reason y recovery hint.

## 6. Invariantes

- Cero edges silenciosamente dangling.
- Toda edge activa tiene uno o mas `GraphEvidence` y el mismo `generation_id` que sus endpoints.
- Una edge inferred nunca se devuelve sin `status=inferred` y regla derivativa.
- Texto libre solo puede producir candidatos, `doc_mentions` o unresolved; no `calls`, `implements`, `extends`, `reads` o `writes` exactos.
- Orden y deduplicacion son deterministas entre RIDs.
- Imports Graphify u otra fuente externa viven en namespace/provenance `external:*` y no sustituyen evidencia nativa.

## 7. GraphUnresolved

Debe conservar `reason_code`, input/selector sanitizado, candidatos bounded, backend, owner path, generation, source digest, cross-RID y `recovery_hint`. No contiene payload arbitrario, secretos ni raw compiler logs. Los unresolved participan en stats y explican recall perdido.

## 8. Errores tipados

| Codigo | Causa | Resultado |
|---|---|---|
| `GPH_EDGE_ENDPOINT_MISSING` | source o target no existe | unresolved; generacion invalida si se marco exact |
| `GPH_EDGE_RELATION_UNSUPPORTED` | combinacion no registrada | omission/unresolved |
| `GPH_EDGE_EVIDENCE_MISSING` | edge sin evidencia | reject |
| `GPH_EDGE_SOURCE_STALE` | digest no coincide | unresolved; no publicar claim |
| `GPH_EDGE_AMBIGUOUS` | multiples destinos no desambiguados | unresolved con candidatos bounded |
| `GPH_EDGE_PROVENANCE_INVALID` | backend/version/claim incompleto | reject |

## 9. Aceptacion y trazabilidad

- Fixtures positivos, negativos, ambiguos, unresolved y not-comparable.
- Precision/recall por relation; `negative_violations=0` es gate independiente.
- Validacion de orden, deduplicacion, cross-RID y cero dangling edges en 30 reruns.
- `TP-GPH / TP-GPH-003 / TC-GPH-015..018`.
