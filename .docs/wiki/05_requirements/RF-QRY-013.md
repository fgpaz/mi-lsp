---
wiki_contract: WIKI-CONTRACT-v1
id: RF-QRY-013
kind: requirement
status: active
title: RF-QRY-013 - Diagnosticar gobernanza y perfil efectivo con nav governance
links:
  implements:
  - FL-QRY-01
  code:
  - path: internal/service/app.go
    role: behavior
---

## Propósito

RF-QRY-013 - Diagnosticar gobernanza y perfil efectivo con nav governance

### 1. Hoja de ejecución
- **Campo**: ID; **Valor**: RF-QRY-013
- **Campo**: Título; **Valor**: Diagnosticar gobernanza y perfil efectivo con nav governance
- **Campo**: Actores; **Valor**: Usuario, Skill, Agente, CLI/Core
- **Campo**: Prioridad; **Valor**: alta
- **Campo**: Severidad; **Valor**: alta
- **Campo**: FL origen; **Valor**: FL-QRY-01

## Comportamiento ordenado

### 2. Precondiciones detalladas
- **Condición**: Workspace resoluble; **Tipo**: funcional; **Estado requerido**: obligatorio
- **Condición**: `.docs/wiki/00_governance.md` accesible; **Tipo**: funcional; **Estado requerido**: obligatorio

### 3. Secuencia principal
1. La CLI recibe `mi-lsp nav governance`.
2. El core inspecciona `00_governance.md`, extrae el bloque YAML y valida el schema.
3. Resuelve el perfil efectivo y compila `base + overlays`.
4. Verifica sincronización con `.docs/wiki/_mi-lsp/read-model.toml`.
5. Informa si el índice del workspace quedo stale respecto de las fuentes de gobernanza e incluye timestamps comparados (`index.db`, `00`, `read-model`) para explicar la causa.
6. Devuelve pasos accionables de reparación o siguientes pasos si la gobernanza es valida.

### 5. Casos especiales y variantes
- `nav governance` es la superficie primaria de diagnóstico cuando el repo esta bloqueado.
- La respuesta debe servir tanto a personas como a skills.
- El comando puede auto-sincronizar la proyección, pero no debe ocultar que hace falta reindex si corresponde.
- El bloque `index_sync_details` es aditivo y debe explicar `reason`, `index_path` y cada path comparado sin requerir que el agente lea el filesystem por separado.

## Superficies

CLI `mi-lsp`, con operaciones y selectores detallados en el comportamiento del requisito.

## Entradas/salidas tipadas

### 6. Impacto en el modelo de datos
- `GovernanceStatus`
- `DocsReadProfile`
- `QueryEnvelope`

## Errores

### 4. Errores tipados
- **Código**: `QRY_GOV_WORKSPACE_NOT_FOUND`; **Causa**: workspace inválido; **Trigger**: alias/path no resoluble; **Respuesta esperada**: abortar con error explícito
- **Código**: `QRY_GOV_INVALID_SCHEMA`; **Causa**: YAML inválido; **Trigger**: parse/validacion falla; **Respuesta esperada**: devolver estado bloqueado e issues
- **Código**: `QRY_GOV_INDEX_STALE`; **Causa**: índice stale; **Trigger**: `00` o `read-model` más nuevos que `index.db`; **Respuesta esperada**: devolver warning bloqueante con timestamps y pedir `mi-lsp index --workspace <alias>`

## Criterios de aceptación (numerados)

1. La CLI recibe `mi-lsp nav governance`.
2. El core inspecciona `00_governance.md`, extrae el bloque YAML y valida el schema.
3. Resuelve el perfil efectivo y compila `base + overlays`.
4. Verifica sincronización con `.docs/wiki/_mi-lsp/read-model.toml`.
5. Informa si el índice del workspace quedo stale respecto de las fuentes de gobernanza e incluye timestamps comparados (`index.db`, `00`, `read-model`) para explicar la causa.
6. Devuelve pasos accionables de reparación o siguientes pasos si la gobernanza es valida.

```yaml
harness_protocol: SDD-HARNESS-v1
id: RF-QRY-013
kind: requirement
audience: llm-first
imports:
- .docs/wiki/04_flows/FL-QRY-01.md
exports:
- RF-QRY-013
- RF-QRY-013 - Diagnosticar gobernanza y perfil efectivo con nav governance
agent_must_read:
- .docs/wiki/05_requirements/RF-QRY-013.md
- .docs/wiki/04_flows/FL-QRY-01.md
- .docs/wiki/06_tests/TP-QRY.md
- .docs/wiki/06_tests/TP-QRY-PREPARATION.md
agent_may_edit:
- .docs/wiki/05_requirements/RF-QRY-013.md
agent_must_not_edit:
- .docs/wiki/00_governance.md
- AGENTS.md
- internal/service/app.go
verify:
- 'Para los casos de este requisito en .docs/wiki/06_tests/TP-QRY.md, ejecutar el
  comando publicado: go test ./internal/model ./internal/service ./internal/cli ./internal/indexer
  (la evidencia de ejecución debe registrarse por separado).'
stop_if:
- Detenerse si la propuesta requiere alterar criterios, decisiones o comportamiento
  más allá de RF-QRY-013.
evidence:
- .docs/wiki/05_requirements/RF-QRY-013.md
- .docs/wiki/06_tests/TP-QRY.md
- .docs/wiki/06_tests/TP-QRY-PREPARATION.md
```
