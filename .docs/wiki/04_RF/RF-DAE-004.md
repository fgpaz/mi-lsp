---
id: RF-DAE-004
title: Observar archivos del workspace y re-indexar en background con debounce
implements:
  - internal/daemon/lifecycle.go
  - internal/daemon/file_watcher.go
tests:
  - internal/daemon/file_watcher_test.go
---

# RF-DAE-004 - Observar archivos del workspace y re-indexar en background con debounce

## 1. Execution Sheet

| Campo | Valor |
|---|---|
| ID | RF-DAE-004 |
| Titulo | Observar archivos del workspace y re-indexar en background con debounce |
| Actores | Daemon, File Watcher |
| Prioridad | media |
| Severidad | media |
| FL origen | FL-DAE-01 |

## 2. Detailed Preconditions

| Condicion | Tipo | Estado requerido |
|---|---|---|
| Daemon corriendo | operativa | obligatorio |
| `index.db` existente | funcional | obligatorio |
| Workspace directory observable | tecnica | obligatorio |

## 3. Inputs

| Campo | Tipo | Req. | Origen | Validacion | RN |
|---|---|---|---|---|---|
| `file_change_event` | evento | si | OS/watcher | path, op (add/mod/del) | RF-DAE-004 |
| `debounce_interval` | duracion | no | config | ms; default 500ms | RF-DAE-004 |
| `ignored_patterns` | array | no | config | glob patterns (e.g., node_modules) | RF-DAE-004 |
| `watch_mode` | enum | no | CLI/env | `off|lazy|eager`; default `lazy` | RF-DAE-004 |
| `max_watched_roots` | entero | no | CLI/env | >0; default operativo acotado | RF-DAE-004 |

## 4. Process Steps (Happy Path)

1. El daemon decide si observar segun `watch_mode`: `off` no crea watchers, `lazy` activa por `workspace_root` al usar/warmear un workspace, `eager` observa workspaces registrados al boot.
2. El watcher observa cambios de archivos en el workspace (.cs, .ts, .tsx, .py).
3. Emite evento de cambio (add/mod/del).
4. Aplica debounce de 500ms para evitar procesamiento frenético.
5. Extrae simbolos del archivo modificado.
6. Actualiza `index.db` con nuevos simbolos / deletea si se removio archivo.
7. Operacion no-blocking en background.

## 5. Outputs

| Campo | Tipo | Destino | Efecto observable |
|---|---|---|---|
| `ok` | bool | daemon log | estado de actualizacion |
| `indexed_symbols` | lista | daemon state | simbolos extraidos del archivo |
| `duration_ms` | entero | daemon log | tiempo de extraccion y update |
| `warnings` | lista | daemon log | parse failures o db write issues |
| `watchers` | objeto | `daemon status` / `/api/status` | modo, roots, dirs y eventos pendientes |

## 6. Typed Errors

| Codigo | Causa | Trigger | Respuesta esperada |
|---|---|---|---|
| `IDX_EXTRACT_FAILED` | fallo al extraer simbolos | syntax error en archivo | log warning, no-fatal |
| `IDX_DB_WRITE_FAILED` | error escribiendo index.db | lock o I/O | retry con backoff exponencial |
| `IDX_IGNORED_FILE` | archivo en ignored patterns | path en node_modules | skip silencioso |

## 7. Special Cases and Variants

- Debounce coalesces multiples cambios en intervalo de 500ms.
- Archivos en `node_modules`, `.git`, etc. se ignoran por default.
- Extraccion no-blocking, no interrumpe consultas activas.
- Si db write falla, retry hasta 3 veces con backoff.
- Aliases multiples del mismo root se deduplican por `workspace_root` canonico.
- Si se supera `max_watched_roots`, se expulsa el watcher menos recientemente usado.

## 8. Data Model Impact

- `FileRecord` (path, indexed_at, deleted, symbols)
- `index.db` schema

## 9. Expanded Acceptance Criteria (Gherkin)

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

## 10. Test Traceability

- Positivo: `TP-DAE / TC-DAE-013`
- Positivo: `TP-DAE / TC-DAE-014`
- Positivo: `TP-DAE / TC-DAE-015`
- Positivo: `TP-DAE / TC-DAE-017`
- Positivo: `TP-DAE / TC-DAE-018`

## 11. No Ambiguities Left

- Supuestos prohibidos:
  - no bloquear consultas por file watcher
  - no fallar globalmente si extraccion de un archivo falla
- Decisiones cerradas:
  - debounce 500ms por defecto
  - `watch_mode=lazy` por defecto
  - watchers deduplicados por root canonico y acotados por LRU
  - ignored patterns (node_modules, .git, etc.)
  - non-fatal extraction failures
- TODO explicit = 0
- Fuera de alcance:
  - multi-repo watching
  - network FS support
- Dependencias externas explicitas:
  - OS file watcher (inotify, FSEvents, etc.)
  - index.db local
