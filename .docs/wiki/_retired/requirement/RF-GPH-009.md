---
id: RF-GPH-009
title: Federar grafos entre workspaces por cross-RID
status: planned
flows:
  - FL-GPH-02
tests:
  - .docs/wiki/06_pruebas/TP-GPH.md
---

```yaml
harness_protocol: SDD-HARNESS-v1
id: "RF-GPH-009"
kind: "support-doc"
audience: "llm-first"
imports:
  - '[[00_gobierno_documental]]'
  - '[[FL-GPH-02]]'
  - '[[RF-GPH-001]]'
  - '[[RF-GPH-002]]'
  - '[[RF-GPH-005]]'
exports:
  - 'RF-GPH-009'
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/03_FL/FL-GPH-02.md
  - .docs/wiki/04_RF/RF-GPH-001.md
  - .docs/wiki/04_RF/RF-GPH-002.md
  - .docs/wiki/04_RF/RF-GPH-005.md
  - .docs/wiki/04_RF/RF-GPH-009.md
agent_may_edit:
  - .docs/wiki/04_RF/RF-GPH-009.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
evidence:
  - .docs/wiki/04_RF/RF-GPH-009.md
```

# RF-GPH-009 - Federar grafos entre workspaces por cross-RID

## 1. Resultado requerido

Consultar relaciones entre workspaces registrados sin convertir el daemon ni un store externo en autoridad. Cada `index.db` conserva su generation local; una `GlobalGraphSnapshot` derivativa fija el par `(workspace_identity, generation_id)` de cada miembro y resuelve cross-links por cross-RID/provenance.

## 2. Construccion del snapshot global

1. Enumerar solo workspaces registrados y permitidos por el caller.
2. Abrir read-only el manifest/sello de su generation activa.
3. Registrar workspace/repository identity, generation, schema, freshness y availability.
4. Resolver links externos declarados por package/module/contract identity; nombres simples o texto quedan ambiguous.
5. Sellar members y cross-edges en un snapshot derivado con digest determinista.
6. Ejecutar queries bounded usando fan-out por store; no cargar todos los grafos en RAM.

El snapshot global puede cachearse en estado local global, pero se invalida si cambia cualquier member generation, registry scope o adapter version. Nunca escribe en los `index.db` miembros.

## 3. Fuentes de cross-edge

- project/package dependency resuelta por compiler/LSP/package manager;
- contract/route/event identity explicita y versionada;
- repository/module references declaradas;
- import externo Graphify solo bajo provenance `external:graphify` y status advisory;
- remote snapshot solo mediante pack firmado/configurado explicitamente, nunca red implicita.

Un cross-edge conserva source/target workspace, member generations y evidencia. Si el target no esta registrado/disponible, se emite unresolved externo; no se crea nodo fantasma activo.

## 4. Query y aislamiento

Las operaciones RF-GPH-005/006 aceptan `--all-workspaces` o scope explicito. El envelope lista member generations, unavailable members, omissions y snapshot digest. Budgets globales se reparten deterministicamente; un workspace lento/ausente no habilita scan ilimitado de otros.

Gobernanza, permisos y document authority se evalúan por workspace. Un documento de un repo no gobierna otro salvo enlace canonico explicito reconocido por ambos perfiles.

## 5. Invariantes

- Repo-local SQLite sigue siendo autoridad; global es una vista derivativa y reproducible.
- Ninguna query global muta registry, member stores o wiki.
- Cross-RID iguales con identity fields incompatibles producen conflicto y bloquean ese link.
- Un member stale/blocked se declara y no se usa como evidencia vigente.
- Direct mode puede construir el mismo snapshot desde registry; daemon solo cachea/supervisa.
- No hay servicio remoto, NetworkX completo, MCP o red requerida.

## 6. Errores tipados

| Codigo | Causa | Respuesta |
|---|---|---|
| `GPH_GLOBAL_SCOPE_EMPTY` | ningun workspace permitido | items vacios + hint |
| `GPH_GLOBAL_MEMBER_UNAVAILABLE` | index/generation no accesible | omission por member |
| `GPH_GLOBAL_SCHEMA_INCOMPATIBLE` | schema fuera de ventana | excluir member + warning |
| `GPH_GLOBAL_CROSS_RID_CONFLICT` | RID igual, identidad distinta | bloquear cross-edge |
| `GPH_GLOBAL_TARGET_UNRESOLVED` | dependencia sin workspace target | unresolved externo |
| `GPH_GLOBAL_BUDGET_EXCEEDED` | fanout/result/token ceiling | truncation determinista |
| `GPH_GLOBAL_AUTHORITY_CONFLICT` | docs intentan gobernar otro scope | drift/block, no override |

## 7. Aceptacion y trazabilidad

- Fixtures de dos workspaces con dependency positiva, negativa, target ausente y cross-RID conflictivo.
- Mismos members/generations producen snapshot digest y ordering identicos.
- Query global bounded, direct/daemon parity y cero writes a member stores.
- Cross-RID y authority checks en Windows/Linux/macOS soportados.
- `TP-GPH / TP-GPH-006 / TC-GPH-039..042`.
