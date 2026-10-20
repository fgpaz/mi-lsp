---
wiki_contract: WIKI-CONTRACT-v1
id: RF-QRY-004
kind: requirement
status: active
title: RF-QRY-004 - Leer multiples rangos de archivo en una sola invocacion
links:
  implements:
  - FL-QRY-01
  code:
  - path: internal/service/app.go
    role: behavior
---

## Propósito

RF-QRY-004 - Leer multiples rangos de archivo en una sola invocacion

### 1. Hoja de ejecución
- **Campo**: ID; **Valor**: RF-QRY-004
- **Campo**: Título; **Valor**: Leer multiples rangos de archivo en una sola invocacion
- **Campo**: Actores; **Valor**: Skill, Agente, CLI
- **Campo**: Prioridad; **Valor**: alta
- **Campo**: Severidad; **Valor**: media
- **Campo**: FL origen; **Valor**: FL-QRY-01

## Comportamiento ordenado

### 2. Precondiciones detalladas
- **Condición**: Workspace resoluble; **Tipo**: funcional; **Estado requerido**: obligatorio
- **Condición**: Archivos existentes en workspace; **Tipo**: funcional; **Estado requerido**: obligatorio
- **Condición**: Backend semántico disponible o fallback texto; **Tipo**: técnica; **Estado requerido**: obligatorio

### 4. Secuencia principal
1. La CLI recibe N rangos en formato `file:startLine-endLine`.
2. El core valida que cada path esté dentro del workspace.
3. Para cada rango, lee las líneas especificadas del archivo.
4. Retorna un envelope con `items` array conteniendo cada rango y su contenido.
5. Aplica truncación según `token_budget` y devuelve resultado.

### 7. Casos especiales y variantes
- Si un rango excede el total de líneas del archivo, leer hasta EOF.
- Líneas de salida incluyen números de línea para referencia.
- Si stdin viene vacio, asumir entrada interactiva o fallback a CLI.

### 11. Decisiones cerradas
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
- Dependencias externas explícitas:
  - filesystem local del workspace

## Superficies

CLI `mi-lsp`, con operaciones y selectores detallados en el comportamiento del requisito.

## Entradas/salidas tipadas

### 3. Entradas
- **Campo**: `ranges`; **Tipo**: array; **Req.**: si; **Origen**: stdin/CLI; **Validacion**: N rangos `file:startLine-endLine`; **RN**: RF-QRY-004
- **Campo**: `format`; **Tipo**: enum; **Req.**: no; **Origen**: CLI; **Validacion**: `compact`, `json`, `text`, `toon` o `yaml`; default `compact`; **RN**: RF-QRY-004
- **Campo**: `token_budget`; **Tipo**: entero; **Req.**: no; **Origen**: CLI; **Validacion**: mayor que cero; **RN**: RF-QRY-004

### 5. Salidas
- **Campo**: `ok`; **Tipo**: bool; **Destino**: usuario/skill; **Efecto observable**: estado de la operación
- **Campo**: `items`; **Tipo**: lista; **Destino**: usuario/skill; **Efecto observable**: array con file, lines, content para cada rango
- **Campo**: `truncated`; **Tipo**: bool; **Destino**: usuario/skill; **Efecto observable**: explícita recorte si hay más contenido
- **Campo**: `warnings`; **Tipo**: lista; **Destino**: usuario/skill; **Efecto observable**: paths fuera del workspace o archivos no encontrados
- **Campo**: `generation_id`; **Tipo**: string; **Destino**: usuario/skill; **Efecto observable**: snapshot de la generación publicada, el mismo de `q` y `nav intent` ([[RF-QRY-025]])
- **Campo**: `items[].sensibilidad`; **Tipo**: string; **Destino**: usuario/skill; **Efecto observable**: marca del frontmatter; vacía si no hay marca; mi-lsp no filtra

### 8. Impacto en el modelo de datos
- `QueryEnvelope`
- `FileContentSlice` (line_start, line_end, content)

## Errores

### 6. Errores tipados
- **Código**: `QRY_PATH_TRAVERSAL`; **Causa**: path intenta salir del workspace; **Trigger**: `../../../etc/passwd`; **Respuesta esperada**: rechazar con error explícito
- **Código**: `QRY_FILE_NOT_FOUND`; **Causa**: archivo no existe en workspace; **Trigger**: rango valido pero archivo inexistente; **Respuesta esperada**: warning en `warnings`, continuar con otros rangos
- **Código**: `QRY_STDIN_TOO_LARGE`; **Causa**: stdin excede 10MB; **Trigger**: payload > 10MB; **Respuesta esperada**: abortar con error explícito
- **Código**: `QRY_INVALID_PATH_CHARS`; **Causa**: path contiene caracteres invalidos; **Trigger**: arg contiene `\n` o `\r` (ej: shell serializa args con newlines embebidos); **Respuesta esperada**: abortar con error descriptivo `"invalid path: contains newline in ..."`

## Criterios de aceptación (numerados)

1. Leer multiples rangos en una sola invocacion: Dado N archivos válidos en el workspace Cuando ejecuto "mi-lsp nav multi-read 'file1:10-20' 'file2:5-15'" Entonces la respuesta incluye ambos rangos con contenido y números de línea Y "truncated" refleja si se alcanco el presupuesto
2. Detectar path traversal: Dado una solicitud con path "../../../etc/passwd" Cuando ejecuto la consulta Entonces rechazo con "QRY_PATH_TRAVERSAL" Y no se lee ningún archivo fuera del workspace
3. Continuar con aviso en archivo no encontrado: Dado un rango valido pero archivo inexistente Cuando ejecuto la consulta con otros rangos válidos Entonces la respuesta incluye warning para el archivo faltante Y los otros rangos se procesan normalmente

### 9. Escenarios de aceptación (Gherkin)
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

### 10. Trazabilidad de pruebas
- Positivo: `TP-QRY / TC-QRY-014`
- Positivo: `TP-QRY / TC-QRY-015`
- Negativo: `TP-QRY / TC-QRY-016`
