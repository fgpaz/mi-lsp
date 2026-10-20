---
wiki_contract: WIKI-CONTRACT-v1
id: RF-QRY-020
kind: requirement
status: active
title: RF-QRY-020 - Sugerir un comando `nav` equivalente a Read/Grep/Glob con `nav suggest`
links:
  implements:
  - FL-QRY-01
  code:
  - path: internal/cli/nav.go
    role: behavior
  - path: internal/cli/suggest.go
    role: behavior
---

## Propósito

RF-QRY-020 - Sugerir un comando `nav` equivalente a Read/Grep/Glob con `nav suggest`

### 1. Hoja de ejecución
- **Campo**: ID; **Valor**: RF-QRY-020
- **Campo**: Título; **Valor**: Sugerir un comando `nav` equivalente a Read/Grep/Glob con `nav suggest`
- **Campo**: Actores; **Valor**: Skill, Agente, Puerta de host (hooks), CLI/Core
- **Campo**: Prioridad; **Valor**: media
- **Campo**: Severidad; **Valor**: baja
- **Campo**: FL origen; **Valor**: FL-QRY-01

## Comportamiento ordenado

### 2. Precondiciones detalladas
- **Condición**: `--tool` es uno de `Read`, `Grep`, `Glob` (case-insensitive); **Tipo**: funcional; **Estado requerido**: obligatorio para devolver un item
- **Condición**: `--args` es JSON valido o esta vacio/`null`; **Tipo**: técnica; **Estado requerido**: obligatorio
- **Condición**: Workspace resoluble; **Tipo**: funcional; **Estado requerido**: no requerido
- **Condición**: Daemon corriendo; **Tipo**: técnica; **Estado requerido**: no requerido

### 3. Secuencia principal
1. Una skill, agente o hook de host ejecuta `mi-lsp nav suggest --tool <Read|Grep|Glob> --args <json>`.
2. La CLI normaliza `--tool` a su forma canónica; cualquier otro valor (incluida una tool desconocida) devuelve `items=[]` con `ok=true` y sale con código 0.
3. La CLI parsea `--args` preservando backslashes literales de paths Windows (incluido `\r` dentro de un path) antes de decodificar JSON.
4. Según la tool: `Read` mapea a `nav multi-read <path[:start-end]>`; `Grep` mapea a `nav search <pattern>` (o `nav search --regex <pattern>` si el JSON marca el patron como regex); `Glob` mapea a `nav find <token>` solo cuando el patron es un token de símbolo (`[A-Za-z_][A-Za-z0-9_]*`), no un glob de path ni un nombre calificado como `Foo.Bar`.
5. La CLI emite un único item `{tool, command, argv, reason}` con `command`/`argv` separados de `reason`, o `items=[]` cuando no hay equivalente.
6. La operación corre directo, sin iniciar el daemon.

### 6. Casos especiales y variantes
- Tool desconocida (por ejemplo `Write`, `Bash`) devuelve `items=[]` sin error; solo `Read`, `Grep` y `Glob` producen un item.
- `Glob` con un patron de path (`**/*.go`) o un nombre calificado (`Foo.Bar`) no sugiere nada; solo un token de símbolo simple produce `nav find`.
- `Read` con `offset` sin `limit` sugiere un rango de una sola línea (`file:N-N`); con ambos, sugiere `file:start-end`.
- `Grep` agrega `--regex` al argv solo cuando el JSON marca explicitamente el patron como regex (`regex`, `regexp`, `is_regex`, `isRegex`, `use_regex`, `useRegex`).
- Un path Windows dentro de `--args` (incluido un backslash seguido de `r`, `p`, etc.) se conserva literal; no se interpreta como escape de control.
- `nav suggest` no reemplaza a `nav intent`, no abre un router externo y no es un fallback: es una sugerencia acotada de un comando que la CLI ya expone.
- Las puertas de host (hooks de Claude Code, Grok) invocan este comando por argv, nunca por shell, y usan `items[0].argv` para el exec del hijo.

### 10. Decisiones cerradas
- Supuestos prohibidos:
  - no asumir que una tool sin equivalente es un error
  - no asumir que `command` y `reason` pueden concatenarse en un string de shell
- Decisiones cerradas:
  - solo `Read`, `Grep`, `Glob` producen un item; el resto devuelve `items=[]`
  - `Glob` solo sugiere para tokens de símbolo, nunca para globs de path
  - los backslashes de paths Windows en `--args` se preservan literales
- TODO explicit = 0
- Fuera de alcance:
  - reemplazar `nav intent` o abrir un router externo
  - reiniciar la racha Read/Grep/Glob de una herramienta no-mi-lsp (eso vive en la puerta de host, no aqui)
- Dependencias externas explícitas:
  - ninguna; mapeo local puro sin red, shell ni daemon

## Superficies

CLI `mi-lsp`, con operaciones y selectores detallados en el comportamiento del requisito.

## Entradas/salidas tipadas

### 4. Salidas
- **Campo**: `items[0].tool`; **Tipo**: string; **Destino**: usuario/skill; **Efecto observable**: tool de origen normalizada (`Read`, `Grep`, `Glob`)
- **Campo**: `items[0].command`; **Tipo**: string; **Destino**: usuario/skill; **Efecto observable**: comando `nav` sugerido, legible
- **Campo**: `items[0].argv`; **Tipo**: lista de strings; **Destino**: usuario/skill; **Efecto observable**: vector de argv listo para exec sin shell
- **Campo**: `items[0].reason`; **Tipo**: string; **Destino**: usuario/skill; **Efecto observable**: motivo separado del comando, nunca concatenado
- **Campo**: `ok`; **Tipo**: bool; **Destino**: usuario/skill; **Efecto observable**: siempre `true`; ausencia de equivalente no es un fallo
- **Campo**: `backend`; **Tipo**: string; **Destino**: usuario/skill; **Efecto observable**: siempre `suggest`

### 7. Impacto en el modelo de datos
- `navSuggestItem` interno (`tool`, `command`, `argv`, `reason`); no persiste estado ni agrega un tipo de envelope nuevo.

## Errores

### 5. Errores tipados
- **Código**: (sin código tipado propio); **Causa**: `--args` no es JSON valido; **Trigger**: JSON malformado tras preservar backslashes; **Respuesta esperada**: error explícito `invalid --args JSON: ...`; se clasifica como `explicit_incomplete` por el mapeo de fallback compartido

## Criterios de aceptación (numerados)

1. Mapear Read a nav multi-read: Dado un llamado "Read" con file_path "C:\repos\mi-lsp\internal\cli\suggest.go" y offset 4 Cuando ejecuto "mi-lsp nav suggest --tool Read --args <json>" Entonces "items[0].command" es "nav multi-read" Y "items[0].argv" preserva el path Windows literal Y "items[0].reason" es un campo separado de "command"
2. Mapear Grep con patron regex explícito: Dado un llamado "Grep" con pattern "foo.*bar" y regex=true Cuando ejecuto "mi-lsp nav suggest --tool Grep --args <json>" Entonces "items[0].argv" incluye "--regex"
3. No sugerir para un Glob de path: Dado un llamado "Glob" con pattern "**/*.go" Cuando ejecuto "mi-lsp nav suggest --tool Glob --args <json>" Entonces "items" es una lista vacia Y "ok" es "true"
4. Rechazar --args inválido: Dado un valor de "--args" que no es JSON valido Cuando ejecuto "mi-lsp nav suggest --tool Read --args <json inválido>" Entonces la operación falla con un error explícito que menciona "--args"

### 8. Escenarios de aceptación (Gherkin)
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

### 9. Trazabilidad de pruebas
- Positivo: `TP-QRY / TC-QRY-153`
- Positivo: `TP-QRY / TC-QRY-154`
- Negativo: `TP-QRY / TC-QRY-155`
