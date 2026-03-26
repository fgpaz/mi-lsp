# RF-QRY-004 - Leer multiples rangos de archivo en una sola invocacion

## 1. Execution Sheet

| Campo | Valor |
|---|---|
| ID | RF-QRY-004 |
| Titulo | Leer multiples rangos de archivo en una sola invocacion |
| Actores | Skill, Agente, CLI |
| Prioridad | alta |
| Severidad | media |
| FL origen | FL-QRY-01 |

## 2. Detailed Preconditions

| Condicion | Tipo | Estado requerido |
|---|---|---|
| Workspace resoluble | funcional | obligatorio |
| Archivos existentes en workspace | funcional | obligatorio |
| Backend semantico disponible o fallback texto | tecnica | obligatorio |

## 3. Inputs

| Campo | Tipo | Req. | Origen | Validacion | RN |
|---|---|---|---|---|---|
| `ranges` | array | si | stdin/CLI | N rangos `file:startLine-endLine` | RF-QRY-004 |
| `format` | enum | no | CLI | `compact`, `json` o `text`; default `compact` | RF-QRY-004 |
| `token_budget` | entero | no | CLI | mayor que cero | RF-QRY-004 |

## 4. Process Steps (Happy Path)

1. La CLI recibe N rangos en formato `file:startLine-endLine`.
2. El core valida que cada path esté dentro del workspace.
3. Para cada rango, lee las lineas especificadas del archivo.
4. Retorna un envelope con `items` array conteniendo cada rango y su contenido.
5. Aplica truncacion segun `token_budget` y devuelve resultado.

## 5. Outputs

| Campo | Tipo | Destino | Efecto observable |
|---|---|---|---|
| `ok` | bool | usuario/skill | estado de la operacion |
| `items` | lista | usuario/skill | array con file, lines, content para cada rango |
| `truncated` | bool | usuario/skill | explicita recorte si hay mas contenido |
| `warnings` | lista | usuario/skill | paths fuera del workspace o archivos no encontrados |

## 6. Typed Errors

| Codigo | Causa | Trigger | Respuesta esperada |
|---|---|---|---|
| `QRY_PATH_TRAVERSAL` | path intenta salir del workspace | `../../../etc/passwd` | rechazar con error explicito |
| `QRY_FILE_NOT_FOUND` | archivo no existe en workspace | rango valido pero archivo inexistente | warning en `warnings`, continuar con otros rangos |
| `QRY_STDIN_TOO_LARGE` | stdin excede 10MB | payload > 10MB | abortar con error explicito |

## 7. Special Cases and Variants

- Si un rango excede el total de lineas del archivo, leer hasta EOF.
- Lineas de salida incluyen numeros de linea para referencia.
- Si stdin viene vacio, asumir entrada interactiva o fallback a CLI.

## 8. Data Model Impact

- `QueryEnvelope`
- `FileContentSlice` (line_start, line_end, content)

## 9. Expanded Acceptance Criteria (Gherkin)

```gherkin
Scenario: Leer multiples rangos en una sola invocacion
  Given N archivos validos en el workspace
  When ejecuto "mi-lsp nav multi-read 'file1:10-20' 'file2:5-15'"
  Then la respuesta incluye ambos rangos con contenido y numeros de linea
  And "truncated" refleja si se alcanco el presupuesto

Scenario: Detectar path traversal
  Given una solicitud con path "../../../etc/passwd"
  When ejecuto la consulta
  Then rechazo con "QRY_PATH_TRAVERSAL"
  And no se lee ningun archivo fuera del workspace

Scenario: Continuar con aviso en archivo no encontrado
  Given un rango valido pero archivo inexistente
  When ejecuto la consulta con otros rangos validos
  Then la respuesta incluye warning para el archivo faltante
  And los otros rangos se procesan normalmente
```

## 10. Test Traceability

- Positivo: `TP-QRY / TC-QRY-014`
- Positivo: `TP-QRY / TC-QRY-015`
- Negativo: `TP-QRY / TC-QRY-016`

## 11. No Ambiguities Left

- Supuestos prohibidos:
  - no asumir que todos los archivos existen
  - no modificar archivos durante lectura
- Decisiones cerradas:
  - lectura paralela segura dentro del workspace
  - warning en lugar de abortar si algun archivo falta
- TODO explicit = 0
- Fuera de alcance:
  - escritura de archivos
  - lectura de archivos binarios
- Dependencias externas explicitas:
  - filesystem local del workspace
