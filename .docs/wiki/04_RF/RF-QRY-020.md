---
id: RF-QRY-020
title: Sugerir un comando `nav` equivalente a Read/Grep/Glob con `nav suggest`
implements:
  - internal/cli/suggest.go
  - internal/cli/nav.go
tests:
  - internal/cli/suggest_test.go
---

# RF-QRY-020 - Sugerir un comando `nav` equivalente a Read/Grep/Glob con `nav suggest`

```yaml
harness_protocol: SDD-HARNESS-v1
id: "RF-QRY-020"
kind: "support-doc"
audience: "llm-first"
imports:
  - '[[00_gobierno_documental]]'
  - '[[FL-QRY-01]]'
  - '[[RF-QRY-020]]'
exports:
  - 'RF-QRY-020'
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/03_FL/FL-QRY-01.md
  - .docs/wiki/04_RF/RF-QRY-020.md
agent_may_edit:
  - .docs/wiki/04_RF/RF-QRY-020.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
evidence:
  - .docs/wiki/04_RF/RF-QRY-020.md
```

## 1. Execution Sheet

| Campo | Valor |
|---|---|
| ID | RF-QRY-020 |
| Titulo | Sugerir un comando `nav` equivalente a Read/Grep/Glob con `nav suggest` |
| Actores | Skill, Agente, Puerta de host (hooks), CLI/Core |
| Prioridad | media |
| Severidad | baja |
| FL origen | FL-QRY-01 |

## 2. Detailed Preconditions

| Condicion | Tipo | Estado requerido |
|---|---|---|
| `--tool` es uno de `Read`, `Grep`, `Glob` (case-insensitive) | funcional | obligatorio para devolver un item |
| `--args` es JSON valido o esta vacio/`null` | tecnica | obligatorio |
| Workspace resoluble | funcional | no requerido |
| Daemon corriendo | tecnica | no requerido |

## 3. Process Steps (Happy Path)

1. Una skill, agente o hook de host ejecuta `mi-lsp nav suggest --tool <Read|Grep|Glob> --args <json>`.
2. La CLI normaliza `--tool` a su forma canonica; cualquier otro valor (incluida una tool desconocida) devuelve `items=[]` con `ok=true` y sale con codigo 0.
3. La CLI parsea `--args` preservando backslashes literales de paths Windows (incluido `\r` dentro de un path) antes de decodificar JSON.
4. Segun la tool: `Read` mapea a `nav multi-read <path[:start-end]>`; `Grep` mapea a `nav search <pattern>` (o `nav search --regex <pattern>` si el JSON marca el patron como regex); `Glob` mapea a `nav find <token>` solo cuando el patron es un token de simbolo (`[A-Za-z_][A-Za-z0-9_]*`), no un glob de path ni un nombre calificado como `Foo.Bar`.
5. La CLI emite un unico item `{tool, command, argv, reason}` con `command`/`argv` separados de `reason`, o `items=[]` cuando no hay equivalente.
6. La operacion corre directo, sin iniciar el daemon.

## 4. Outputs

| Campo | Tipo | Destino | Efecto observable |
|---|---|---|---|
| `items[0].tool` | string | usuario/skill | tool de origen normalizada (`Read`, `Grep`, `Glob`) |
| `items[0].command` | string | usuario/skill | comando `nav` sugerido, legible |
| `items[0].argv` | lista de strings | usuario/skill | vector de argv listo para exec sin shell |
| `items[0].reason` | string | usuario/skill | motivo separado del comando, nunca concatenado |
| `ok` | bool | usuario/skill | siempre `true`; ausencia de equivalente no es un fallo |
| `backend` | string | usuario/skill | siempre `suggest` |

## 5. Typed Errors

| Codigo | Causa | Trigger | Respuesta esperada |
|---|---|---|---|
| (sin codigo tipado propio) | `--args` no es JSON valido | JSON malformado tras preservar backslashes | error explicito `invalid --args JSON: ...`; se clasifica como `explicit_incomplete` por el mapeo de fallback compartido |

## 6. Special Cases and Variants

- Tool desconocida (por ejemplo `Write`, `Bash`) devuelve `items=[]` sin error; solo `Read`, `Grep` y `Glob` producen un item.
- `Glob` con un patron de path (`**/*.go`) o un nombre calificado (`Foo.Bar`) no sugiere nada; solo un token de simbolo simple produce `nav find`.
- `Read` con `offset` sin `limit` sugiere un rango de una sola linea (`file:N-N`); con ambos, sugiere `file:start-end`.
- `Grep` agrega `--regex` al argv solo cuando el JSON marca explicitamente el patron como regex (`regex`, `regexp`, `is_regex`, `isRegex`, `use_regex`, `useRegex`).
- Un path Windows dentro de `--args` (incluido un backslash seguido de `r`, `p`, etc.) se conserva literal; no se interpreta como escape de control.
- `nav suggest` no reemplaza a `nav intent`, no abre un router externo y no es un fallback: es una sugerencia acotada de un comando que la CLI ya expone.
- Las puertas de host (hooks de Claude Code, Grok) invocan este comando por argv, nunca por shell, y usan `items[0].argv` para el exec del hijo.

## 7. Data Model Impact

- `navSuggestItem` interno (`tool`, `command`, `argv`, `reason`); no persiste estado ni agrega un tipo de envelope nuevo.

## 8. Expanded Acceptance Criteria (Gherkin)

```gherkin
Scenario: Mapear Read a nav multi-read
  Given un llamado "Read" con file_path "C:\repos\mi-lsp\internal\cli\suggest.go" y offset 4
  When ejecuto "mi-lsp nav suggest --tool Read --args <json>"
  Then "items[0].command" es "nav multi-read"
  And "items[0].argv" preserva el path Windows literal
  And "items[0].reason" es un campo separado de "command"

Scenario: Mapear Grep con patron regex explicito
  Given un llamado "Grep" con pattern "foo.*bar" y regex=true
  When ejecuto "mi-lsp nav suggest --tool Grep --args <json>"
  Then "items[0].argv" incluye "--regex"

Scenario: No sugerir para un Glob de path
  Given un llamado "Glob" con pattern "**/*.go"
  When ejecuto "mi-lsp nav suggest --tool Glob --args <json>"
  Then "items" es una lista vacia
  And "ok" es "true"

Scenario: Rechazar --args invalido
  Given un valor de "--args" que no es JSON valido
  When ejecuto "mi-lsp nav suggest --tool Read --args <json invalido>"
  Then la operacion falla con un error explicito que menciona "--args"
```

## 9. Test Traceability

- Positivo: `TP-QRY / TC-QRY-153`
- Positivo: `TP-QRY / TC-QRY-154`
- Negativo: `TP-QRY / TC-QRY-155`

## 10. No Ambiguities Left

- Supuestos prohibidos:
  - no asumir que una tool sin equivalente es un error
  - no asumir que `command` y `reason` pueden concatenarse en un string de shell
- Decisiones cerradas:
  - solo `Read`, `Grep`, `Glob` producen un item; el resto devuelve `items=[]`
  - `Glob` solo sugiere para tokens de simbolo, nunca para globs de path
  - los backslashes de paths Windows en `--args` se preservan literales
- TODO explicit = 0
- Fuera de alcance:
  - reemplazar `nav intent` o abrir un router externo
  - reiniciar la racha Read/Grep/Glob de una herramienta no-mi-lsp (eso vive en la puerta de host, no aqui)
- Dependencias externas explicitas:
  - ninguna; mapeo local puro sin red, shell ni daemon
