---
wiki_contract: WIKI-CONTRACT-v1
id: RF-WKS-002
kind: requirement
status: active
title: RF-WKS-002 - Indexar automaticamente al registrar un workspace nuevo
links:
  implements:
  - FL-BOOT-01
  code:
  - path: internal/cli/workspace.go
    role: behavior
  - path: internal/service/workspace_ops.go
    role: behavior
  - path: internal/workspace/registry.go
    role: behavior
---

## Propósito

RF-WKS-002 - Indexar automáticamente al registrar un workspace nuevo

### 1. Hoja de ejecución
- **Campo**: ID; **Valor**: RF-WKS-002
- **Campo**: Título; **Valor**: Indexar automáticamente al registrar un workspace nuevo
- **Campo**: Actores; **Valor**: Desarrollador, CLI, Skill
- **Campo**: Prioridad; **Valor**: alta
- **Campo**: Severidad; **Valor**: media
- **Campo**: FL origen; **Valor**: FL-BOOT-01

## Comportamiento ordenado

### 2. Precondiciones detalladas
- **Condición**: Path del workspace valido; **Tipo**: funcional; **Estado requerido**: obligatorio
- **Condición**: Directorio escribible; **Tipo**: operativa; **Estado requerido**: obligatorio
- **Condición**: `.mi-lsp/project.toml` creado; **Tipo**: funcional; **Estado requerido**: obligatorio

### 4. Secuencia principal
1. La CLI recibe comando `workspace add <path>`.
2. Valida layout y crea `project.toml`.
3. Registra workspace en `~/.mi-lsp/registry.toml`.
4. Por defecto (sin `--no-index`), inicia indexación automatica.
5. Devuelve respuesta con registro + stats de indexacion.
6. Si indexación falla, devuelve warning pero registro exitoso.

### 7. Casos especiales y variantes
- `--no-index` salta indexación completamente.
- Si indexación falla, el registro sigue siendo exitoso (warning no-fatal).
- Si indexación excede timeout (default 30s), log warning y continua.
- `workspace doctor` enriquece cada hallazgo con `health` (`ok|attention|action_required`) y `next_actions` ordenadas para reparar alias duplicados, roots stale, worktrees ambiguos, shadowing de binario o drift de revision entre el binario activo y binarios visibles sin mutar `registry.toml`.

### 12. Decisiones cerradas
- Supuestos prohibidos:
  - no fallar globalmente si indexación falla
  - no obligar --no-index para casos normales
- Decisiones cerradas:
  - auto-index por defecto en add
  - --no-index solo salta index, no registration
  - index failures no-fatal
- TODO explicit = 0
- Fuera de alcance:
  - async indexación separada
  - auto-update de index en background (vease RF-DAE-004)
- Dependencias externas explícitas:
  - filesystem local
  - registry.toml persistencia

## Superficies

CLI `mi-lsp`, con operaciones y selectores detallados en el comportamiento del requisito.

## Entradas/salidas tipadas

### 3. Entradas
- **Campo**: `workspace_path`; **Tipo**: string; **Req.**: si; **Origen**: CLI; **Validacion**: path absoluto o relativo; **RN**: RF-WKS-002
- **Campo**: `--alias`; **Tipo**: string; **Req.**: no; **Origen**: CLI; **Validacion**: alias custom; default derived from path; **RN**: RF-WKS-002
- **Campo**: `--no-index`; **Tipo**: booleano; **Req.**: no; **Origen**: CLI; **Validacion**: si true, salta indexación automática; **RN**: RF-WKS-002

### 5. Salidas
- **Campo**: `ok`; **Tipo**: bool; **Destino**: usuario/skill; **Efecto observable**: true si registro exitoso
- **Campo**: `registration`; **Tipo**: objeto; **Destino**: usuario/skill; **Efecto observable**: WorkspaceRegistration con alias, path, languages
- **Campo**: `index_stats`; **Tipo**: objeto; **Destino**: usuario/skill; **Efecto observable**: files_indexed, symbols_indexed, duration
- **Campo**: `warnings`; **Tipo**: lista; **Destino**: usuario/skill; **Efecto observable**: index failure non-fatal, o --no-index applied

### 8. Impacto en el modelo de datos
- `WorkspaceRegistration` (alias, path, languages, registered_at)
- `registry.toml` file

## Errores

### 6. Errores tipados
- **Código**: `WKS_INVALID_PATH`; **Causa**: path no existe o no es directorio; **Trigger**: path inválido; **Respuesta esperada**: rechazar sin modificar registry
- **Código**: `WKS_ALREADY_REGISTERED`; **Causa**: mismo alias ya registrado con root incompatible; **Trigger**: alias conflicto; **Respuesta esperada**: rechazar o sugerir otro alias
- **Código**: `WKS_DUPLICATE_ROOT_ALIAS`; **Causa**: multiples aliases apuntan al mismo root; **Trigger**: alias distinto para root existente; **Respuesta esperada**: permitir; `workspace list` preserva aliases y `workspace list --group-by-root`/`workspace doctor` diagnostican
- **Código**: `WKS_INDEX_FAILED`; **Causa**: indexación falla; **Trigger**: IO error en index.db; **Respuesta esperada**: registrar con warning, ok=true

## Criterios de aceptación (numerados)

1. Registrar e indexar workspace automáticamente: Dado path valido de workspace Cuando ejecuto "mi-lsp workspace add /path/to/workspace" Entonces registro el workspace en registry Y indexo automáticamente sin necesidad de comando adicional Y devuelvo stats de indexación en respuesta
2. Saltar indexación con --no-index: Dado --no-index flag presente Cuando ejecuto workspace add Entonces registro el workspace Y no inicio indexación Y devuelvo registro sin stats de indexación
3. Continuar si indexación falla: Dado indexación falla por IO error Cuando ejecuto workspace add Entonces registro exitoso Y warning de index failure Y ok=true (registro prevalece)

### 9. Escenarios de aceptación (Gherkin)
```gherkin
Scenario: Registrar e indexar workspace automaticamente
  Given path valido de workspace
  When ejecuto "mi-lsp workspace add /path/to/workspace"
  Then registro el workspace en registry
  And indexo automaticamente sin necesidad de comando adicional
  And devuelvo stats de indexacion en respuesta

Scenario: Saltar indexacion con --no-index
  Given --no-index flag presente
  When ejecuto workspace add
  Then registro el workspace
  And no inicio indexacion
  And devuelvo registro sin stats de indexacion

Scenario: Continuar si indexacion falla
  Given indexacion falla por IO error
  When ejecuto workspace add
  Then registro exitoso
  And warning de index failure
  And ok=true (registro prevalece)
```

### 10. Trazabilidad de pruebas
- Positivo: `TP-WKS / TC-WKS-006`
- Positivo: `TP-WKS / TC-WKS-007`
- Positivo: `TP-WKS / TC-WKS-017`
- Positivo: `TP-WKS / TC-WKS-018`
- Positivo: `TP-WKS / TC-WKS-019`
- Positivo: `TP-WKS / TC-WKS-026`
- Negativo: `TP-WKS / TC-WKS-008`

### 11. Evidencia de implementación
- `internal/workspace/registry.go`: group-by-root read model over aliases without registry mutation.
- `internal/service/workspace_ops.go`: `workspace list --group-by-root` and `workspace doctor` service behavior.
- `internal/cli/workspace.go`: CLI flags and doctor command surface.
- `internal/service/app_test.go`: grouped list and non-mutating doctor tests.
