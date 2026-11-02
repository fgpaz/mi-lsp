---
wiki_contract: WIKI-CONTRACT-v1
id: RF-GPH-009
kind: requirement
status: draft
title: RF-GPH-009 - Federar grafos entre workspaces por cross-RID
links:
  implements:
  - FL-GPH-02
  code:
  - path: internal/service/app.go
    role: behavior
---

## Propósito

RF-GPH-009 - Federar grafos entre workspaces por cross-RID

## Comportamiento ordenado

### 1. Resultado requerido

Consultar relaciones entre workspaces registrados sin convertir el daemon ni un store externo en autoridad. Cada `index.db` conserva su generation local; una `GlobalGraphSnapshot` derivativa fija el par `(workspace_identity, generation_id)` de cada miembro y resuelve cross-links por cross-RID/provenance.

### 2. Construcción del snapshot global

1. Enumerar solo workspaces registrados y permitidos por el caller.
2. Abrir read-only el manifest/sello de su generation activa.
3. Registrar workspace/repository identity, generation, schema, freshness y availability.
4. Resolver links externos declarados por package/module/contract identity; nombres simples o texto quedan ambiguous.
5. Sellar members y cross-edges en un snapshot derivado con digest determinista.
6. Ejecutar queries bounded usando fan-out por store; no cargar todos los grafos en RAM.

El snapshot global puede cachearse en estado local global, pero se inválida si cambia cualquier member generation, registry scope o adapter version. Nunca escribe en los `index.db` miembros.

### 3. Fuentes de cross-edge

- project/package dependency resuelta por compiler/LSP/package manager;
- contract/route/event identity explícita y versionada;
- repository/module references declaradas;
- import externo Graphify solo bajo provenance `external:graphify` y status advisory;
- remote snapshot solo mediante pack firmado/configurado explicitamente, nunca red implicita.

Un cross-edge conserva source/target workspace, member generations y evidencia. Si el target no esta registrado/disponible, se emite unresolved externo; no se crea nodo fantasma activo.

### 4. Query y aislamiento

Las operaciones RF-GPH-005/006 aceptan `--all-workspaces` o scope explicito. El envelope lista member generations, unavailable members, omissions y snapshot digest. Budgets globales se reparten deterministicamente; un workspace lento/ausente no habilita scan ilimitado de otros.

Gobernanza, permisos y document authority se evalúan por workspace. Un documento de un repo no gobierna otro salvo enlace canónico explícito reconocido por ambos perfiles.

### 5. Invariantes

- Repo-local SQLite sigue siendo autoridad; global es una vista derivativa y reproducible.
- Ninguna query global muta registry, member stores o wiki.
- Cross-RID iguales con identity fields incompatibles producen conflicto y bloquean ese link.
- Un member stale/blocked se declara y no se usa como evidencia vigente.
- Direct mode puede construir el mismo snapshot desde registry; daemon solo cachea/supervisa.
- No hay servicio remoto, NetworkX completo, MCP o red requerida.

## Superficies

CLI `mi-lsp`, con operaciones y selectores detallados en el comportamiento del requisito.

## Entradas/salidas tipadas

Las entradas, salidas, enums y límites forman parte del contrato de comportamiento ordenado de este documento; los campos conservan sus nombres de protocolo.

## Errores

### 6. Errores tipados

- **Código**: `GPH_GLOBAL_SCOPE_EMPTY`; **Causa**: ningún workspace permitido; **Respuesta**: items vacios + hint
- **Código**: `GPH_GLOBAL_MEMBER_UNAVAILABLE`; **Causa**: index/generation no accesible; **Respuesta**: omission por member
- **Código**: `GPH_GLOBAL_SCHEMA_INCOMPATIBLE`; **Causa**: schema fuera de ventana; **Respuesta**: excluir member + warning
- **Código**: `GPH_GLOBAL_CROSS_RID_CONFLICT`; **Causa**: RID igual, identidad distinta; **Respuesta**: bloquear cross-edge
- **Código**: `GPH_GLOBAL_TARGET_UNRESOLVED`; **Causa**: dependencia sin workspace target; **Respuesta**: unresolved externo
- **Código**: `GPH_GLOBAL_BUDGET_EXCEEDED`; **Causa**: fanout/result/token ceiling; **Respuesta**: truncation determinista
- **Código**: `GPH_GLOBAL_AUTHORITY_CONFLICT`; **Causa**: docs intentan gobernar otro scope; **Respuesta**: drift/block, no override

## Criterios de aceptación (numerados)

1. Fixtures de dos workspaces con dependency positiva, negativa, target ausente y cross-RID conflictivo.
2. Mismos members/generations producen snapshot digest y ordering identicos.
3. Query global bounded, direct/daemon parity y cero writes a member stores.
4. Cross-RID y authority checks en Windows/Linux/macOS soportados.

### 7. Aceptacion y trazabilidad

- Fixtures de dos workspaces con dependency positiva, negativa, target ausente y cross-RID conflictivo.
- Mismos members/generations producen snapshot digest y ordering identicos.
- Query global bounded, direct/daemon parity y cero writes a member stores.
- Cross-RID y authority checks en Windows/Linux/macOS soportados.
- `TP-GPH / TP-GPH-006 / TC-GPH-039..042`.

```yaml
harness_protocol: SDD-HARNESS-v1
id: RF-GPH-009
kind: requirement
audience: llm-first
imports:
- .docs/wiki/04_flows/FL-GPH-02.md
exports:
- RF-GPH-009
- RF-GPH-009 - Federar grafos entre workspaces por cross-RID
agent_must_read:
- .docs/wiki/05_requirements/RF-GPH-009.md
- .docs/wiki/04_flows/FL-GPH-02.md
- .docs/wiki/06_tests/TP-GPH.md
agent_may_edit:
- .docs/wiki/05_requirements/RF-GPH-009.md
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
  más allá de RF-GPH-009.
evidence:
- .docs/wiki/05_requirements/RF-GPH-009.md
- .docs/wiki/06_tests/TP-GPH.md
```
