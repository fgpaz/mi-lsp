---
wiki_contract: WIKI-CONTRACT-v1
id: RF-QRY-009
kind: requirement
status: active
title: RF-QRY-009 - Buscar y encontrar simbolos a traves de todos los workspaces
links:
  implements:
  - FL-QRY-01
  code:
  - path: internal/service/app.go
    role: behavior
---

## Propósito

RF-QRY-009 - Buscar y encontrar símbolos a traves de todos los workspaces

### 1. Hoja de ejecución
- **Campo**: ID; **Valor**: RF-QRY-009
- **Campo**: Título; **Valor**: Buscar y encontrar símbolos a traves de todos los workspaces registrados en paralelo
- **Campo**: Actores; **Valor**: Skill, Agente, CLI
- **Campo**: Prioridad; **Valor**: media
- **Campo**: Severidad; **Valor**: media
- **Campo**: FL origen; **Valor**: FL-QRY-01

## Comportamiento ordenado

### 2. Precondiciones detalladas
- **Condición**: Al menos un workspace registrado; **Tipo**: funcional; **Estado requerido**: obligatorio
- **Condición**: Catálogo actualizado en workspaces; **Tipo**: funcional; **Estado requerido**: preferido
- **Condición**: Flag --all-workspaces explícito; **Tipo**: funcional; **Estado requerido**: obligatorio

### 4. Secuencia principal
1. La CLI recibe pattern y `--all-workspaces`.
2. Valida que flag este presente.
3. Dispatch paralelo a cada workspace.
4. Cada worker busca en su catalogo/indice.
5. Colecta resultados con campo `workspace` por item.
6. Devuelve QueryEnvelope con items cross-workspace.

### 7. Casos especiales y variantes
- Sin `--all-workspaces`, buscar solo en workspace actual (se redirige a RF-QRY-002).
- Resultados ordenados por workspace y relevancia dentro de cada workspace.
- Timeout por workspace (default 10s) para evitar bloqueos.

### 11. Decisiones cerradas
- Supuestos prohibidos:
  - no buscar cross-workspace sin flag explícito
  - no asumir que todos los workspaces estan ok
- Decisiones cerradas:
  - flag obligatorio para evitar sorpresas
  - graceful degrade por workspace
- TODO explicit = 0
- Fuera de alcance:
  - busqueda federada remota
  - relevancia global optimizada
- Dependencias externas explícitas:
  - catálogos locales de cada workspace

## Superficies

CLI `mi-lsp`, con operaciones y selectores detallados en el comportamiento del requisito.

## Entradas/salidas tipadas

### 3. Entradas
- **Campo**: `pattern`; **Tipo**: string; **Req.**: si; **Origen**: CLI; **Validacion**: patron textual o regex; **RN**: RF-QRY-009
- **Campo**: `--all-workspaces`; **Tipo**: booleano; **Req.**: si; **Origen**: CLI; **Validacion**: debe estar presente para cross-workspace; **RN**: RF-QRY-009
- **Campo**: `--regex`; **Tipo**: booleano; **Req.**: no; **Origen**: CLI; **Validacion**: si true, interpreta pattern como regex; **RN**: RF-QRY-009
- **Campo**: `--language`; **Tipo**: enum; **Req.**: no; **Origen**: CLI; **Validacion**: filter por lenguaje; vacio = todos; **RN**: RF-QRY-009

### 5. Salidas
- **Campo**: `ok`; **Tipo**: bool; **Destino**: usuario/skill; **Efecto observable**: true si al menos un workspace tiene matches
- **Campo**: `items`; **Tipo**: array; **Destino**: usuario/skill; **Efecto observable**: resultados con workspace field en cada item
- **Campo**: `workspace_stats`; **Tipo**: objeto; **Destino**: usuario/skill; **Efecto observable**: conteos por workspace
- **Campo**: `truncated`; **Tipo**: bool; **Destino**: usuario/skill; **Efecto observable**: si hay más resultados disponibles
- **Campo**: `warnings`; **Tipo**: lista; **Destino**: usuario/skill; **Efecto observable**: workspaces no disponibles o busqueda fallida

### 8. Impacto en el modelo de datos
- `QueryEnvelope`
- Cada item en `items` incluye `workspace` field adicional

## Errores

### 6. Errores tipados
- **Código**: `QRY_NO_WORKSPACES`; **Causa**: no hay workspaces registrados; **Trigger**: registry vacio; **Respuesta esperada**: error explícito con instruccion
- **Código**: `QRY_FLAG_REQUIRED`; **Causa**: --all-workspaces no presente; **Trigger**: cross-workspace sin flag; **Respuesta esperada**: error amistoso indicando flag requerido
- **Código**: `QRY_INVALID_REGEX`; **Causa**: pattern regex inválido; **Trigger**: regex syntax error; **Respuesta esperada**: error tipado con sugerencia

## Criterios de aceptación (numerados)

1. Buscar símbolo en todos los workspaces: Dado 3+ workspaces registrados con catálogos diferentes Cuando ejecuto "mi-lsp nav search Repository --all-workspaces" Entonces devuelvo matches de todos los workspaces Y cada item incluye workspace field Y resultado ordenado por workspace
2. Requerir flag explícito para cross-workspace: Dado consulta sin --all-workspaces Cuando intento buscar en todos Entonces error amistoso indicando que flag es requerido
3. Degrade si algunos workspaces fallan: Dado 3 workspaces, 1 con catálogo corrupted Cuando ejecuto la busqueda Entonces devuelvo resultados de los 2 workspaces ok Y warning para el workspace fallido Y ok=true si al menos uno tiene matches

### 9. Escenarios de aceptación (Gherkin)
```gherkin
Scenario: Buscar simbolo en todos los workspaces
  Given 3+ workspaces registrados con catalogos diferentes
  When ejecuto "mi-lsp nav search Repository --all-workspaces"
  Then devuelvo matches de todos los workspaces
  And cada item incluye workspace field
  And resultado ordenado por workspace

Scenario: Requerir flag explicito para cross-workspace
  Given consulta sin --all-workspaces
  When intento buscar en todos
  Then error amistoso indicando que flag es requerido

Scenario: Degrade si algunos workspaces fallan
  Given 3 workspaces, 1 con catalogo corrupted
  When ejecuto la busqueda
  Then devuelvo resultados de los 2 workspaces ok
  And warning para el workspace fallido
  And ok=true si al menos uno tiene matches
```

### 10. Trazabilidad de pruebas
- Positivo: `TP-QRY / TC-QRY-029`
- Positivo: `TP-QRY / TC-QRY-030`
- Negativo: `TP-QRY / TC-QRY-031`

```yaml
harness_protocol: SDD-HARNESS-v1
id: RF-QRY-009
kind: requirement
audience: llm-first
imports:
- .docs/wiki/04_flows/FL-QRY-01.md
exports:
- RF-QRY-009
- RF-QRY-009 - Buscar y encontrar simbolos a traves de todos los workspaces
agent_must_read:
- .docs/wiki/05_requirements/RF-QRY-009.md
- .docs/wiki/04_flows/FL-QRY-01.md
- .docs/wiki/06_tests/TP-QRY.md
agent_may_edit:
- .docs/wiki/05_requirements/RF-QRY-009.md
agent_must_not_edit:
- .docs/wiki/00_governance.md
- AGENTS.md
- internal/service/app.go
verify:
- 'Para los casos de este requisito en .docs/wiki/06_tests/TP-QRY.md, ejecutar el
  comando publicado: go test ./internal/model ./internal/service ./internal/cli ./internal/indexer
  (la evidencia de ejecución debe registrarse por separado).'
stop_if:
- 'Detenerse y escalar si el cambio contradice el límite documentado: - Supuestos
  prohibidos:'
evidence:
- .docs/wiki/05_requirements/RF-QRY-009.md
- .docs/wiki/06_tests/TP-QRY.md
```
