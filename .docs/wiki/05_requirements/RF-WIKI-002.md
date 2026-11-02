---
wiki_contract: WIKI-CONTRACT-v1
id: RF-WIKI-002
kind: requirement
status: active
title: RF-WIKI-002 - Inventariar documentacion disponible en todos los workspaces
links:
  implements:
  - FL-WIKI-01
  code:
  - path: internal/service/app.go
    role: behavior
---

## Propósito

RF-WIKI-002 - Inventariar documentacion disponible en todos los workspaces

### 1. Hoja de ejecución
- **Campo**: ID; **Valor**: RF-WIKI-002
- **Campo**: Título; **Valor**: Inventariar documentacion disponible en todos los workspaces
- **Campo**: Actores; **Valor**: Usuario, Skill, Agente, CLI/Core
- **Campo**: Prioridad; **Valor**: media
- **Campo**: Severidad; **Valor**: media
- **Campo**: FL origen; **Valor**: FL-WIKI-01

## Comportamiento ordenado

### 2. Precondiciones detalladas
- **Condición**: Al menos un workspace registrado; **Tipo**: funcional; **Estado requerido**: obligatorio
- **Condición**: `~/.mi-lsp/registry.toml` accesible; **Tipo**: operativa; **Estado requerido**: obligatorio
- **Condición**: Flag `--all-workspaces` presente; **Tipo**: técnica; **Estado requerido**: obligatorio

### 4. Secuencia principal
1. La CLI recibe `nav wiki inventory --all-workspaces [--with-layer-counts]`.
2. El core itera sobre todos los workspaces registrados en `~/.mi-lsp/registry.toml`.
3. Para cada workspace, carga su metadatos: `alias`, `root`, `wiki_root`, `governance_blocked`, `docs_ready`, `doc_count`, `last_indexed_at`.
4. Si `--with-layer-counts` esta presente, cuenta documentos por capa (01-09) desde `.mi-lsp/index.db` con semaphore=4.
5. Agrupa items por workspace en el envelope respetando orden de registro.
6. Sin flag, payload nominal es ~2-3KB (50 workspaces); con flag, ~5KB.
7. La CLI devuelve resultado en formato solicitado.

### 7. Casos especiales y variantes
- Si un workspace no tiene `.docs/wiki/`, incluir en resultado con `docs_ready=false` y `doc_count=0`.
- Si `governance_blocked=true`, marcar explicitamente pero incluir en resultado.
- Ignorar `last_indexed_at=null` (workspace nunca indexado); reportar con timestamp especial "never".
- Si `--with-layer-counts` y el conteo falla para un workspace, omitir ese objeto `layers` pero continuar con otros.
- Payload nominales: default ~2-3KB para 50 workspaces; con flag ~5KB.

### 11. Decisiones cerradas
- Supuestos prohibidos:
  - no asumir que todos los workspaces tienen index.db
  - no omitir workspaces con docs_ready=false
  - no hacer conteos por capa sin flag explícito
- Decisiones cerradas:
  - `nav wiki inventory` es comando nuevo bajo subgrupo wiki
  - Default: metadatos basicos sin conteos
  - Con flag: agregar conteos por capa
- TODO explicit = 0
- Fuera de alcance:
  - indexación asincona (solo metadatos existentes)
  - sincronización de índices entre workspaces

## Superficies

CLI `mi-lsp`, con operaciones y selectores detallados en el comportamiento del requisito.

## Entradas/salidas tipadas

### 3. Entradas
- **Campo**: `--all-workspaces`; **Tipo**: bool; **Req.**: si; **Origen**: CLI; **Validacion**: flag presente; **RN**: RF-WIKI-002
- **Campo**: `--with-layer-counts`; **Tipo**: bool; **Req.**: no; **Origen**: CLI; **Validacion**: opt-in para conteos por capa; **RN**: RF-WIKI-002
- **Campo**: `format`; **Tipo**: enum; **Req.**: no; **Origen**: CLI; **Validacion**: `compact`, `json`, `text`, `toon` o `yaml`; default `compact`; **RN**: RF-WIKI-002

### 5. Salidas
- **Campo**: `ok`; **Tipo**: bool; **Destino**: usuario/skill; **Efecto observable**: estado de la operación
- **Campo**: `items`; **Tipo**: lista; **Destino**: usuario/skill; **Efecto observable**: array con inventario per workspace
- **Campo**: `alias`; **Tipo**: string; **Destino**: usuario/skill; **Efecto observable**: identificador del workspace
- **Campo**: `root`; **Tipo**: path; **Destino**: usuario/skill; **Efecto observable**: raiz absoluta del workspace
- **Campo**: `wiki_root`; **Tipo**: path; **Destino**: usuario/skill; **Efecto observable**: raiz de documentacion (normalmente `.docs/wiki`)
- **Campo**: `governance_blocked`; **Tipo**: bool; **Destino**: usuario/skill; **Efecto observable**: si el workspace esta bloqueado por governance
- **Campo**: `docs_ready`; **Tipo**: bool; **Destino**: usuario/skill; **Efecto observable**: si la documentacion existe y puede indexarse
- **Campo**: `doc_count`; **Tipo**: número; **Destino**: usuario/skill; **Efecto observable**: total de documentos cuando docs_ready=true
- **Campo**: `last_indexed_at`; **Tipo**: timestamp; **Destino**: usuario/skill; **Efecto observable**: cuando se hizo la ultima indexación
- **Campo**: `layers` (con flag); **Tipo**: objeto; **Destino**: usuario/skill; **Efecto observable**: conteo por capa: `{RS: 0, FL: 2, RF: 15, TP: 10, TECH: 8, DB: 5, CT: 3}`
- **Campo**: `warnings`; **Tipo**: lista; **Destino**: usuario/skill; **Efecto observable**: workspaces con metadatos faltantes

### 8. Impacto en el modelo de datos
- `QueryEnvelope` con array de `InventoryItem`: alias, root, wiki_root, governance_blocked, docs_ready, doc_count, last_indexed_at
- Con flag: agregar campo `layers: {RS: int, FL: int, RF: int, TP: int, TECH: int, DB: int, CT: int}`

## Errores

### 6. Errores tipados
- **Código**: `WIKI_REGISTRY_EMPTY`; **Causa**: ningún workspace registrado; **Trigger**: registry.toml vacio o inexistente; **Respuesta esperada**: abortar con error explícito
- **Código**: `WIKI_INDEX_NOT_FOUND`; **Causa**: workspace sin .mi-lsp/index.db; **Trigger**: docs_ready=false; **Respuesta esperada**: incluir en items con doc_count=0, sin error
- **Código**: `WIKI_LAYER_COUNT_TIMEOUT`; **Causa**: layer count toma > 30s; **Trigger**: workspace muy grande; **Respuesta esperada**: warning en `warnings`, omitir conteos pero continuar

## Criterios de aceptación (numerados)

1. Listar inventario de todos los workspaces sin conteos: Dado al menos dos workspaces registrados Cuando ejecuto "mi-lsp nav wiki inventory --all-workspaces --format toon" Entonces la respuesta contiene items para todos los workspaces Y cada item tiene: alias, root, wiki_root, governance_blocked, docs_ready, doc_count, last_indexed_at Y el payload es menor que 3KB
2. Incluir conteos por capa cuando se solicita: Dado un workspace con documentacion completa Cuando ejecuto "nav wiki inventory --all-workspaces --with-layer-counts --format toon" Entonces cada item contiene campo "layers" con conteos por capa Y layers.RF es mayor que cero si el workspace tiene RF docs
3. Manejar workspaces sin documentacion: Dado un workspace sin .docs/wiki/ Cuando ejecuto "nav wiki inventory --all-workspaces" Entonces aparece en items con docs_ready=false Y doc_count=0 Y no error se genera

### 9. Escenarios de aceptación (Gherkin)
```gherkin
Scenario: Listar inventario de todos los workspaces sin conteos
  Given al menos dos workspaces registrados
  When ejecuto "mi-lsp nav wiki inventory --all-workspaces --format toon"
  Then la respuesta contiene items para todos los workspaces
  And cada item tiene: alias, root, wiki_root, governance_blocked, docs_ready, doc_count, last_indexed_at
  And el payload es menor que 3KB

Scenario: Incluir conteos por capa cuando se solicita
  Given un workspace con documentacion completa
  When ejecuto "nav wiki inventory --all-workspaces --with-layer-counts --format toon"
  Then cada item contiene campo "layers" con conteos por capa
  And layers.RF es mayor que cero si el workspace tiene RF docs

Scenario: Manejar workspaces sin documentacion
  Given un workspace sin .docs/wiki/
  When ejecuto "nav wiki inventory --all-workspaces"
  Then aparece en items con docs_ready=false
  And doc_count=0
  And no error se genera
```

### 10. Trazabilidad de pruebas
- Positivo: `TP-WIKI / TC-WIKI-005`
- Positivo: `TP-WIKI / TC-WIKI-006`
- Negativo: `TP-WIKI / TC-WIKI-007`

```yaml
harness_protocol: SDD-HARNESS-v1
id: RF-WIKI-002
kind: requirement
audience: llm-first
imports:
- .docs/wiki/04_flows/FL-WIKI-01.md
exports:
- RF-WIKI-002
- RF-WIKI-002 - Inventariar documentacion disponible en todos los workspaces
agent_must_read:
- .docs/wiki/05_requirements/RF-WIKI-002.md
- .docs/wiki/04_flows/FL-WIKI-01.md
- .docs/wiki/06_tests/TP-WIKI.md
agent_may_edit:
- .docs/wiki/05_requirements/RF-WIKI-002.md
agent_must_not_edit:
- .docs/wiki/00_governance.md
- AGENTS.md
- internal/service/app.go
verify:
- 'Para los casos de este requisito en .docs/wiki/06_tests/TP-WIKI.md, ejecutar el
  comando publicado: go test ./internal/service ./internal/cli ./internal/docgraph
  ./internal/indexer -count=1 -run ''TestClassifyWikiMapHubDefaultsRemainLocked|TestGroupWikiMapDocsUsesDefaultOrCustomDeclarationOrder|TestTrimWikiMapHubs|TestWalkWikiMapDocs|TestNavWikiMapUsesDirectExecution|TestKnowledgeWikiRoots|TestDocumentGraph''
  (la evidencia de ejecución debe registrarse por separado).'
stop_if:
- 'Detenerse y escalar si el cambio contradice el límite documentado: - Supuestos
  prohibidos:'
evidence:
- .docs/wiki/05_requirements/RF-WIKI-002.md
- .docs/wiki/06_tests/TP-WIKI.md
```
