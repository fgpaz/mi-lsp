---
wiki_contract: WIKI-CONTRACT-v1
id: RF-DAE-004
kind: requirement
status: active
title: RF-DAE-004 - Observar archivos del workspace y re-indexar en background con debounce
links:
  implements:
  - FL-DAE-01
  code:
  - path: internal/daemon/file_watcher.go
    role: behavior
  - path: internal/daemon/lifecycle.go
    role: behavior
---

## Propósito

RF-DAE-004 - Observar archivos del workspace y re-indexar en background con debounce

### 1. Hoja de ejecución
- **Campo**: ID; **Valor**: RF-DAE-004
- **Campo**: Título; **Valor**: Observar archivos del workspace y re-indexar en background con debounce
- **Campo**: Actores; **Valor**: Daemon, File Watcher
- **Campo**: Prioridad; **Valor**: media
- **Campo**: Severidad; **Valor**: media
- **Campo**: FL origen; **Valor**: FL-DAE-01

## Comportamiento ordenado

### 2. Precondiciones detalladas
- **Condición**: Daemon corriendo; **Tipo**: operativa; **Estado requerido**: obligatorio
- **Condición**: `index.db` existente; **Tipo**: funcional; **Estado requerido**: obligatorio
- **Condición**: Workspace directory observable; **Tipo**: técnica; **Estado requerido**: obligatorio

### 4. Secuencia principal
1. El daemon decide si observar según `watch_mode`: `off` no crea watchers, `lazy` activa por `workspace_root` al usar/warmear un workspace, `eager` observa workspaces registrados al boot.
2. El watcher observa cambios de archivos en el workspace (.cs, .ts, .tsx, .py).
3. Emite evento de cambio (add/mod/del).
4. Aplica debounce de 500ms para evitar procesamiento frenético.
5. Extrae símbolos del archivo modificado.
6. Actualiza `index.db` con nuevos símbolos / deletea si se removio archivo.
7. Operación no-blocking en background.

### 7. Casos especiales y variantes
- Debounce coalesces multiples cambios en intervalo de 500ms.
- Archivos en `node_modules`, `.git`, etc. se ignoran por default.
- Extracción no-blocking, no interrumpe consultas activas.
- Si db write falla, retry hasta 3 veces con backoff.
- Aliases multiples del mismo root se deduplican por `workspace_root` canonico.
- Si se supera `max_watched_roots`, se expulsa el watcher menos recientemente usado.

### 11. Decisiones cerradas
- Supuestos prohibidos:
  - no bloquear consultas por file watcher
  - no fallar globalmente si extracción de un archivo falla
- Decisiones cerradas:
  - debounce 500ms por defecto
  - `watch_mode=lazy` por defecto
  - watchers deduplicados por root canónico y acotados por LRU
  - ignored patterns (node_modules, .git, etc.)
  - non-fatal extraction failures
- TODO explicit = 0
- Fuera de alcance:
  - multi-repo watching
  - network FS support
- Dependencias externas explícitas:
  - OS file watcher (inotify, FSEvents, etc.)
  - index.db local

## Superficies

CLI `mi-lsp`, con operaciones y selectores detallados en el comportamiento del requisito.

## Entradas/salidas tipadas

### 3. Entradas
- **Campo**: `file_change_event`; **Tipo**: evento; **Req.**: si; **Origen**: OS/watcher; **Validacion**: path, op (add/mod/del); **RN**: RF-DAE-004
- **Campo**: `debounce_interval`; **Tipo**: duracion; **Req.**: no; **Origen**: config; **Validacion**: ms; default 500ms; **RN**: RF-DAE-004
- **Campo**: `ignored_patterns`; **Tipo**: array; **Req.**: no; **Origen**: config; **Validacion**: glob patterns (e.g., node_modules); **RN**: RF-DAE-004
- **Campo**: `watch_mode`; **Tipo**: enum; **Req.**: no; **Origen**: CLI/env; **Validacion**: `off|lazy|eager`; default `lazy`; **RN**: RF-DAE-004
- **Campo**: `max_watched_roots`; **Tipo**: entero; **Req.**: no; **Origen**: CLI/env; **Validacion**: >0; default operativo acotado; **RN**: RF-DAE-004

### 5. Salidas
- **Campo**: `ok`; **Tipo**: bool; **Destino**: daemon log; **Efecto observable**: estado de actualizacion
- **Campo**: `indexed_symbols`; **Tipo**: lista; **Destino**: daemon state; **Efecto observable**: símbolos extraidos del archivo
- **Campo**: `duration_ms`; **Tipo**: entero; **Destino**: daemon log; **Efecto observable**: tiempo de extracción y update
- **Campo**: `warnings`; **Tipo**: lista; **Destino**: daemon log; **Efecto observable**: parse failures o db write issues
- **Campo**: `watchers`; **Tipo**: objeto; **Destino**: `daemon status` / `/api/status`; **Efecto observable**: modo, roots, dirs y eventos pendientes

### 8. Impacto en el modelo de datos
- `FileRecord` (path, indexed_at, deleted, symbols)
- `index.db` schema

## Errores

### 6. Errores tipados
- **Código**: `IDX_EXTRACT_FAILED`; **Causa**: fallo al extraer símbolos; **Trigger**: syntax error en archivo; **Respuesta esperada**: log warning, no-fatal
- **Código**: `IDX_DB_WRITE_FAILED`; **Causa**: error escribiendo index.db; **Trigger**: lock o I/O; **Respuesta esperada**: retry con backoff exponencial
- **Código**: `IDX_IGNORED_FILE`; **Causa**: archivo en ignored patterns; **Trigger**: path en node_modules; **Respuesta esperada**: skip silencioso

## Criterios de aceptación (numerados)

1. Re-indexar archivo modificado con debounce: Dado archivo .cs modificado en workspace Cuando watcher emite cambio Entonces debounce 500ms Y re-indexo archivo Y actualizo index.db con nuevos símbolos Y no bloqueo consultas activas
2. Ignorar archivos en patterns: Dado archivo en node_modules o .git Cuando watcher emite cambio Entonces ignore silenciosamente Y no procese el archivo
3. Continuar si extracción falla: Dado syntax error en archivo modificado Cuando intento extraer símbolos Entonces log warning Y no-fatal Y continuo observando cambios
4. Activar watcher lazy por workspace canónico: Dado daemon iniciado con watch_mode lazy Cuando uso una query semántica o warm sobre un workspace Entonces activa un watcher para su workspace_root canónico Y aliases duplicados del mismo root no agregan otro watcher
5. Limitar roots observados: Dado max_watched_roots alcanzado Cuando activo watcher para otro workspace_root Entonces expulso el watcher LRU Y `daemon status` refleja roots/dirs/eventos pendientes

### 9. Escenarios de aceptación (Gherkin)
```gherkin
Scenario: Re-indexar archivo modificado con debounce
  Given archivo .cs modificado en workspace
  When watcher emite cambio
  Then debounce 500ms
  And re-indexo archivo
  And actualizo index.db con nuevos simbolos
  And no bloqueo consultas activas

Scenario: Ignorar archivos en patterns
  Given archivo en node_modules o .git
  When watcher emite cambio
  Then ignore silenciosamente
  And no procese el archivo

Scenario: Continuar si extraccion falla
  Given syntax error en archivo modificado
  When intento extraer simbolos
  Then log warning
  And no-fatal
  And continuo observando cambios

Scenario: Activar watcher lazy por workspace canonico
  Given daemon iniciado con watch_mode lazy
  When uso una query semantica o warm sobre un workspace
  Then activa un watcher para su workspace_root canonico
  And aliases duplicados del mismo root no agregan otro watcher

Scenario: Limitar roots observados
  Given max_watched_roots alcanzado
  When activo watcher para otro workspace_root
  Then expulso el watcher LRU
  And `daemon status` refleja roots/dirs/eventos pendientes
```

### 10. Trazabilidad de pruebas
- Positivo: `TP-DAE / TC-DAE-013`
- Positivo: `TP-DAE / TC-DAE-014`
- Positivo: `TP-DAE / TC-DAE-015`
- Positivo: `TP-DAE / TC-DAE-017`
- Positivo: `TP-DAE / TC-DAE-018`
