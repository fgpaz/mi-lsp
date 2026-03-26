# RF-QRY-008 - Devolver contexto semantico de simbolos cambiados en un diff git

## 1. Execution Sheet

| Campo | Valor |
|---|---|
| ID | RF-QRY-008 |
| Titulo | Devolver contexto semantico de simbolos cambiados en un diff git |
| Actores | Skill, Agente, CLI |
| Prioridad | media |
| Severidad | media |
| FL origen | FL-QRY-01 |

## 2. Detailed Preconditions

| Condicion | Tipo | Estado requerido |
|---|---|---|
| Repositorio Git disponible | tecnica | obligatorio |
| Workspace indexado | funcional | preferido |
| Ref base resoluble (default HEAD) | tecnica | obligatorio |

## 3. Inputs

| Campo | Tipo | Req. | Origen | Validacion | RN |
|---|---|---|---|---|---|
| `git_ref` | string | no | CLI | git ref o branch; default working tree | RF-QRY-008 |
| `--include-content` | booleano | no | CLI | si true, incluye lineas modificadas | RF-QRY-008 |
| `--base-ref` | string | no | CLI | ref base para comparacion; default HEAD | RF-QRY-008 |

## 4. Process Steps (Happy Path)

1. La CLI recibe ref git (opcional).
2. El core obtiene diff entre HEAD y la ref.
3. Parsea archivos cambiados y extrae simbolos afectados.
4. Resuelve el contexto semantico de cada simbolo.
5. Si `--include-content`, incluye las lineas modificadas.
6. Devuelve DiffContextResult structurado.

## 5. Outputs

| Campo | Tipo | Destino | Efecto observable |
|---|---|---|---|
| `ok` | bool | usuario/skill | estado de la operacion |
| `diff_context` | objeto | usuario/skill | DiffContextResult con changed_files, changed_symbols, impact |
| `stats` | objeto | usuario/skill | files_changed, symbols_affected, insertions, deletions |
| `warnings` | lista | usuario/skill | git unavailable, parse failures |

## 6. Typed Errors

| Codigo | Causa | Trigger | Respuesta esperada |
|---|---|---|---|
| `QRY_GIT_UNAVAILABLE` | git no disponible | git command falla | error explicito con sugerencia |
| `QRY_NO_CHANGES_DETECTED` | no hay cambios entre refs | refs identicas | warning amistoso, devolver empty result |
| `QRY_INVALID_REF` | ref no valida | ref inexistente | error explicito |

## 7. Special Cases and Variants

- Si git no esta disponible, devolver error o empty context segun flag.
- `--include-content` por defecto false para evitar payloads grandes.
- Default a working tree si no se especifica ref.

## 8. Data Model Impact

- `DiffContextResult` (changed_files, changed_symbols, impact)
- `FileChange` (path, op, symbols_affected)
- `SymbolImpact` (symbol, impact_type, context)

## 9. Expanded Acceptance Criteria (Gherkin)

```gherkin
Scenario: Obtener contexto de cambios sin contenido
  Given cambios sin commitear en un repo git
  When ejecuto "mi-lsp nav diff-context"
  Then devuelvo archivos cambiados, simbolos afectados, e impacto
  And no incluyo lineas de codigo por defecto

Scenario: Incluir contenido modificado con flag
  Given cambios con --include-content
  When ejecuto la consulta
  Then devuelvo ademas las lineas modificadas alrededor del cambio
  And trunco si payload excede presupuesto

Scenario: Detectar cuando no hay cambios
  Given working tree limpio
  When ejecuto la consulta
  Then devuelvo warning amistoso
  And result vacio pero ok=true
```

## 10. Test Traceability

- Positivo: `TP-QRY / TC-QRY-026`
- Positivo: `TP-QRY / TC-QRY-027`
- Negativo: `TP-QRY / TC-QRY-028`

## 11. No Ambiguities Left

- Supuestos prohibidos:
  - no asumir que git siempre esta disponible
  - no obligar contenido en respuesta
- Decisiones cerradas:
  - default a working tree
  - content opcional
- TODO explicit = 0
- Fuera de alcance:
  - analisis de impacto dinamico
  - sugerencias de rollback
- Dependencias externas explicitas:
  - git binario local
