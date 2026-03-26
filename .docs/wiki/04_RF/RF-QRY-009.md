# RF-QRY-009 - Buscar y encontrar simbolos a traves de todos los workspaces

## 1. Execution Sheet

| Campo | Valor |
|---|---|
| ID | RF-QRY-009 |
| Titulo | Buscar y encontrar simbolos a traves de todos los workspaces registrados en paralelo |
| Actores | Skill, Agente, CLI |
| Prioridad | media |
| Severidad | media |
| FL origen | FL-QRY-01 |

## 2. Detailed Preconditions

| Condicion | Tipo | Estado requerido |
|---|---|---|
| Al menos un workspace registrado | funcional | obligatorio |
| Catalogo actualizado en workspaces | funcional | preferido |
| Flag --all-workspaces explicito | funcional | obligatorio |

## 3. Inputs

| Campo | Tipo | Req. | Origen | Validacion | RN |
|---|---|---|---|---|---|
| `pattern` | string | si | CLI | patron textual o regex | RF-QRY-009 |
| `--all-workspaces` | booleano | si | CLI | debe estar presente para cross-workspace | RF-QRY-009 |
| `--regex` | booleano | no | CLI | si true, interpreta pattern como regex | RF-QRY-009 |
| `--language` | enum | no | CLI | filter por lenguaje; vacio = todos | RF-QRY-009 |

## 4. Process Steps (Happy Path)

1. La CLI recibe pattern y `--all-workspaces`.
2. Valida que flag este presente.
3. Dispatch paralelo a cada workspace.
4. Cada worker busca en su catalogo/indice.
5. Colecta resultados con campo `workspace` por item.
6. Devuelve QueryEnvelope con items cross-workspace.

## 5. Outputs

| Campo | Tipo | Destino | Efecto observable |
|---|---|---|---|
| `ok` | bool | usuario/skill | true si al menos un workspace tiene matches |
| `items` | array | usuario/skill | resultados con workspace field en cada item |
| `workspace_stats` | objeto | usuario/skill | conteos por workspace |
| `truncated` | bool | usuario/skill | si hay mas resultados disponibles |
| `warnings` | lista | usuario/skill | workspaces no disponibles o busqueda fallida |

## 6. Typed Errors

| Codigo | Causa | Trigger | Respuesta esperada |
|---|---|---|---|
| `QRY_NO_WORKSPACES` | no hay workspaces registrados | registry vacio | error explicito con instruccion |
| `QRY_FLAG_REQUIRED` | --all-workspaces no presente | cross-workspace sin flag | error amistoso indicando flag requerido |
| `QRY_INVALID_REGEX` | pattern regex invalido | regex syntax error | error tipado con sugerencia |

## 7. Special Cases and Variants

- Sin `--all-workspaces`, buscar solo en workspace actual (se redirige a RF-QRY-002).
- Resultados ordenados por workspace y relevancia dentro de cada workspace.
- Timeout por workspace (default 10s) para evitar bloqueos.

## 8. Data Model Impact

- `QueryEnvelope`
- Cada item en `items` incluye `workspace` field adicional

## 9. Expanded Acceptance Criteria (Gherkin)

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

## 10. Test Traceability

- Positivo: `TP-QRY / TC-QRY-029`
- Positivo: `TP-QRY / TC-QRY-030`
- Negativo: `TP-QRY / TC-QRY-031`

## 11. No Ambiguities Left

- Supuestos prohibidos:
  - no buscar cross-workspace sin flag explicito
  - no asumir que todos los workspaces estan ok
- Decisiones cerradas:
  - flag obligatorio para evitar sorpresas
  - graceful degrade por workspace
- TODO explicit = 0
- Fuera de alcance:
  - busqueda federada remota
  - relevancia global optimizada
- Dependencias externas explicitas:
  - catalogos locales de cada workspace
