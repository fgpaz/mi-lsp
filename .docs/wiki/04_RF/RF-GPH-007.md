---
id: RF-GPH-007
title: Integrar evidencia wiki-codigo sin invertir autoridad
status: partial
flows:
  - FL-GPH-02
tests:
  - .docs/wiki/06_pruebas/TP-GPH.md
---

```yaml
harness_protocol: SDD-HARNESS-v1
id: "RF-GPH-007"
kind: "support-doc"
audience: "llm-first"
imports:
  - '[[00_gobierno_documental]]'
  - '[[FL-GPH-02]]'
  - '[[RF-GPH-005]]'
  - '[[RF-GPH-006]]'
  - '[[RF-QRY-010]]'
  - '[[RF-QRY-012]]'
  - '[[RF-QRY-016]]'
exports:
  - 'RF-GPH-007'
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/03_FL/FL-GPH-02.md
  - .docs/wiki/04_RF/RF-GPH-005.md
  - .docs/wiki/04_RF/RF-GPH-006.md
  - .docs/wiki/04_RF/RF-QRY-010.md
  - .docs/wiki/04_RF/RF-QRY-012.md
  - .docs/wiki/04_RF/RF-QRY-016.md
  - .docs/wiki/04_RF/RF-GPH-007.md
agent_may_edit:
  - .docs/wiki/04_RF/RF-GPH-007.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-source --workspace mi-lsp --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
evidence:
  - .docs/wiki/04_RF/RF-GPH-007.md
```

# RF-GPH-007 - Integrar evidencia wiki-codigo sin invertir autoridad

## 1. Resultado requerido

Usar el grafo para mejorar `nav ask`, `route`, `pack`, `context`, `affected`, `diff-context` y `workspace-map`, preservando la jerarquia documental: `00` gobierna; alcance/arquitectura/FL/RF/TP y canon tecnico explican decisiones; codigo y runtime aportan evidencia de implementacion o drift.

## 2. Modelo de enlace

- Documentos canonicos son `GraphNode(kind=document)` con NodeKey basado en repository identity + owner path + doc ID estable.
- Wikilinks, embeds, links Markdown, doc IDs y jerarquía conservan relaciones distintas: `doc_wikilink`, `doc_embed`, `doc_markdown_link`, `doc_id` y `doc_hierarchy`.
- La resolución documental es corpus-aware: path exacto, relativo al documento, relativo a roots de conocimiento y basename único; exact-case precede case-fold.
- Anchors y alias se preservan como `doc_anchor` y `doc_alias`; un self-anchor no crea self-edge.
- Basename o doc ID duplicado queda `ambiguous_doc_target` con candidatos sorted y bounded; nunca se elige el último ni se promueve por score.
- `doc_mentions` enlaza documento a path/símbolo/comando de código cuando existe anchor explícito resoluble.
- Relaciones de código pueden verificar una promesa, pero no cambiar estado, prioridad o significado de un documento.

## 3. Orden de autoridad en respuestas

1. bloqueo/reparacion de gobernanza;
2. documento canonico primario y cadena requerida;
3. documentos de detalle permitidos por read model;
4. evidencia compiler/AST del codigo;
5. heuristicas/inferencias claramente separadas;
6. omissions, unresolved y siguientes consultas.

Si canon y codigo discrepan, el resultado es `drift_detected` con ambas evidencias y owner sugerido. El codigo no reemplaza el texto canonico y la query nunca edita wiki, projection o source.

## 4. Comportamiento fail-closed

- `governance_blocked=true`, proyeccion stale, attribution manual/invalida o source harness BLOCKED detienen la respuesta graph-enriched normal y retornan diagnostico/reparacion.
- Si el indice documental esta stale respecto de `00`/read-model, no se sirve un pack generico como equivalente.
- Si el grafo de código está ausente/stale, la respuesta docs-first sigue disponible y declara omission de evidencia de código.
- Sin `repository_identity` explícita ni exactamente un `origin`, el índice documental continúa pero omite la publicación del grafo con warning sanitizado y accionable; nunca deriva identidad del path local.
- Si una relacion wiki-code es ambiguous, se muestran candidatos bounded sin elegir uno.

## 5. Context optimizer

El selector de contexto usa budgets y produce un reading pack ordenado por autoridad, cobertura marginal y costo de tokens. Debe explicar por que incluyo/omitio cada item, deduplicar evidencia repetida y conservar siempre el documento primario, reglas de gobierno aplicables y unresolved relevantes. La optimizacion no puede descartar una precondicion canonica para mejorar tokens.

## 6. Salida

`WikiCodeContext` incluye `primary_doc`, `authority_chain`, `code_evidence`, `graph_paths`, `drift`, `omissions`, generation IDs de doc/code, provenance, token budget/used, truncated y determinism digest. Cada graph path enlaza refs de evidencia; no embebe raw logs.

## 7. Invariantes

- Cero inversiones de autoridad wiki en fixtures negativos.
- La respuesta docs-only y la enriquecida comparten la misma autoridad primaria.
- Un cambio de ranking no altera IDs o evidencia.
- Query es read-only y daemon-optional.
- Paths `.docs/raw` y `.docs/auditoria` no se promueven a canon por conectividad.
- Imports externos, incluyendo Graphify, son advisory y no autoridad.

## 8. Errores tipados

| Codigo | Causa | Respuesta |
|---|---|---|
| `GPH_WIKI_GOVERNANCE_BLOCKED` | gobierno invalido/stale | diagnostico y repair route |
| `GPH_WIKI_PRIMARY_NOT_FOUND` | no se resuelve doc primario | BLOCKED, no fallback equivalente |
| `GPH_WIKI_CODE_AMBIGUOUS` | mention con multiples targets | unresolved + candidatos |
| `GPH_WIKI_CODE_DRIFT` | canon y evidencia difieren | drift, no override |
| `GPH_WIKI_GRAPH_STALE` | generation doc/code incompatible | docs-first + omission |
| `GPH_WIKI_BUDGET_EXCEEDED` | pack supera budget minimo canonico | truncated/BLOCKED explicito |

## 9. Aceptacion y trazabilidad

- Fixtures de autoridad normal, gobernanza bloqueada, projection stale, code stale, drift y mention ambiguous.
- Oraculo determinista que falla ante cualquier inversion de autoridad.
- Mismo primary doc con y sin graph enrichment; menor token count solo si conserva cadena obligatoria.
- Precision/recall de links wiki-code y 30 repeticiones de determinismo/token budget.
- `TP-GPH / TP-GPH-005 / TC-GPH-036..038`.
