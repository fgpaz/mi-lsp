# RF-QRY-001 - Emitir envelope estable y truncacion determinista

## 1. Execution Sheet

| Campo | Valor |
|---|---|
| ID | RF-QRY-001 |
| Titulo | Emitir envelope estable y truncacion determinista |
| Actores | Usuario, Skill, Agente, CLI/Core |
| Prioridad | alta |
| Severidad | alta |
| FL origen | FL-QRY-01 |

## 2. Detailed Preconditions

| Condicion | Tipo | Estado requerido |
|---|---|---|
| Workspace resoluble | funcional | obligatorio |
| Comando `nav` soportado | funcional | obligatorio |
| Presupuestos numericos validos | tecnica | obligatorio |

## 3. Inputs

| Campo | Tipo | Req. | Origen | Validacion | RN |
|---|---|---|---|---|---|
| `format` | enum | no | CLI | `compact`, `json`, `text`, `toon` o `yaml`; default `compact` | RF-QRY-001 |
| `token_budget` | entero | no | CLI | mayor que cero | RF-QRY-001 |
| `max_items` | entero | no | CLI | mayor que cero | RF-QRY-001 |
| `max_chars` | entero | no | CLI | mayor que cero | RF-QRY-001 |

## 4. Process Steps (Happy Path)

1. La CLI recibe una consulta `nav`.
2. El core ejecuta la operacion y obtiene `items` normalizados.
3. El truncador aplica `max_items`, `max_chars` y `token_budget` en orden determinista.
4. El formatter emite un unico envelope estable.
5. La CLI devuelve el resultado en el formato solicitado o normalizado.

## 5. Outputs

| Campo | Tipo | Destino | Efecto observable |
|---|---|---|---|
| `ok` | bool | usuario/skill | estado de la operacion |
| `backend` | string | usuario/skill | origen semantico o sintactico de la respuesta |
| `items` | lista | usuario/skill | resultado truncado o completo |
| `truncated` | bool | usuario/skill | explicita recorte |
| `warnings` | lista | usuario/skill | contexto de degradacion o frescura |
| `hint` | string/null | usuario/skill | diagnóstico cuando `items=[]` o daemon no disponible (omitempty) |
| `next_hint` | string/null | usuario/skill | sugerencia para pedir mas detalle |

## 6. Typed Errors

| Codigo | Causa | Trigger | Respuesta esperada |
|---|---|---|---|
| `QRY_WORKSPACE_UNRESOLVED` | workspace invalido | alias/path no resoluble | abortar con `ok=false` |
| `QRY_INVALID_BUDGET` | flags invalidos | algun presupuesto es `<= 0` | abortar con error tipado |
| `QRY_RENDER_FAILED` | fallo de serializacion | formatter no puede construir output | abortar con error explicito |

## 7. Special Cases and Variants

- Si `format` es invalido, la respuesta se normaliza a `compact`.
- Si se alcanza un limite, `truncated=true` y `next_hint` debe indicar como pedir mas precision.
- `compact` usa keys cortos y JSON sin whitespace innecesario.
- `toon` serializa el envelope en TOON (Token-Oriented Object Notation); ~20-40% menos tokens que JSON en arrays grandes.
- `yaml` serializa el envelope en YAML estándar; útil para lectura humana o parsers YAML.
- Si `items=[]`, el envelope emite `hint` con diagnóstico de causa (patron no encontrado, timeout, regex-like sin `--regex`).
- Si el daemon falla y el fallback directo responde, el envelope emite `hint: "daemon_unavailable; served from local text index"`.

## 8. Data Model Impact

- `QueryEnvelope`

## 9. Expanded Acceptance Criteria (Gherkin)

```gherkin
Scenario: Responder con envelope estable en formato compact
  Given una consulta valida y un workspace resoluble
  When ejecuto "mi-lsp nav find Repository --workspace gastos --format compact"
  Then la respuesta incluye "ok", "backend", "items", "warnings", "stats" y "truncated"

Scenario: Truncar de forma determinista
  Given una consulta valida que excede el presupuesto
  When ejecuto la misma consulta dos veces con el mismo "token_budget"
  Then ambas respuestas tienen el mismo orden y el mismo punto de corte
  And "truncated" es "true"

Scenario: Rechazar presupuestos invalidos
  Given un "token_budget" igual a cero
  When ejecuto una consulta "nav"
  Then la operacion falla con "QRY_INVALID_BUDGET"
```

## 10. Test Traceability

- Positivo: `TP-QRY / TC-QRY-001`
- Positivo: `TP-QRY / TC-QRY-002`
- Negativo: `TP-QRY / TC-QRY-003`

## 11. No Ambiguities Left

- Supuestos prohibidos:
  - no asumir formato libre por comando
  - no omitir `backend`, `warnings` o `truncated`
- Decisiones cerradas:
  - envelope unico para toda respuesta `nav`
  - truncacion determinista y visible
- TODO explicit = 0
- Fuera de alcance:
  - streaming parcial de respuestas
- Dependencias externas explicitas:
  - solo formatter/truncator locales
