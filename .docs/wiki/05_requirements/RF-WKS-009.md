---
wiki_contract: WIKI-CONTRACT-v1
id: RF-WKS-009
kind: requirement
status: active
title: RF-WKS-009 - Resolver workspace y ejecutable en uso con `workspace which`
links:
  implements:
  - FL-BOOT-01
  code:
  - path: internal/cli/workspace.go
    role: behavior
  - path: internal/cli/workspace_which.go
    role: behavior
---

## Propósito

RF-WKS-009 - Resolver workspace y ejecutable en uso con `workspace which`

### 1. Hoja de ejecución
- **Campo**: ID; **Valor**: RF-WKS-009
- **Campo**: Título; **Valor**: Resolver workspace y ejecutable en uso con `workspace which`
- **Campo**: Actores; **Valor**: Usuario, Skill, Agente, CLI/Core, Puerta MCP (retirada 2026-10-15)
- **Campo**: Prioridad; **Valor**: media
- **Campo**: Severidad; **Valor**: media
- **Campo**: FL origen; **Valor**: FL-BOOT-01

## Comportamiento ordenado

### 2. Precondiciones detalladas
- **Condición**: Registry global legible (`~/.mi-lsp/registry.toml`); **Tipo**: técnica; **Estado requerido**: obligatorio
- **Condición**: Workspace explícito o cwd resoluble contra un root registrado, o `last_workspace` disponible; **Tipo**: funcional; **Estado requerido**: uno de los tres
- **Condición**: Daemon corriendo; **Tipo**: técnica; **Estado requerido**: no requerido
- **Condición**: Índice construido; **Tipo**: técnica; **Estado requerido**: no requerido

### 3. Secuencia principal
1. El usuario, skill o agente ejecuta `mi-lsp workspace which [--workspace <alias>] [--cwd <path>]`.
2. La CLI aplica la misma precedencia que el resto de la superficie: `--workspace` explícito, luego el root registrado que contiene `--cwd` o el cwd del proceso, luego `last_workspace`.
3. La resolución lee unicamente el registry en memoria; no abre índice, no dispara daemon ni red, y no muta `registry.toml`.
4. La CLI resuelve la ruta del ejecutable en uso con `os.Executable()` y la deja tal cual, preservando backslashes de Windows.
5. La CLI emite el envelope estable con `backend=registry` y un único item `{name, root, executable, source}`.

### 6. Casos especiales y variantes
- `workspace which` es de solo lectura: no indexa, no abre el daemon, no usa red y no modifica `registry.toml`.
- La precedencia es identica al resto de la CLI: `--workspace` explícito > root registrado que contiene `--cwd`/cwd del proceso > `last_workspace`.
- Un path Windows con backslash simple (por ejemplo `C:\repos\mios\mis-plugins-cc`) se conserva literal en la salida JSON; no pasa por `strconv.Unquote` ni por un ensamblado `fmt` que interprete `\r`/`\p` como escapes.
- Cuando varios aliases comparten el mismo root, se aplica el mismo desempate que usa el resto del registry (`last_workspace` primero, luego orden alfabetico case-insensitive del alias).
- El diagnóstico amplio de salud del registry sigue siendo `workspace doctor`; `which` no reemplaza ese reporte.
- [RETIRADO 2026-10-15] La puerta `mi-lsp mcp` no reinterpreta este comando: expone la misma resolución de solo lectura via `nav_*`/`workspace which` como uno más de los comandos que ya delega.

### 10. Decisiones cerradas
- Supuestos prohibidos:
  - no asumir que `which` puede iniciar el daemon o abrir el índice
  - no asumir que `which` puede mutar `registry.toml`
- Decisiones cerradas:
  - solo lectura sobre el registry en memoria
  - misma precedencia de resolución que el resto de la CLI
  - path Windows literal, sin reinterpretar backslashes
- TODO explicit = 0
- Fuera de alcance:
  - diagnóstico amplio de salud (`workspace doctor`)
  - resolución cross-workspace explícita (`--allow-cross-workspace` es de otras superficies)
- Dependencias externas explícitas:
  - solo `~/.mi-lsp/registry.toml` y `os.Executable()`

## Superficies

CLI `mi-lsp`, con operaciones y selectores detallados en el comportamiento del requisito.

## Entradas/salidas tipadas

### 4. Salidas
- **Campo**: `items[0].name`; **Tipo**: string; **Destino**: usuario/skill; **Efecto observable**: alias del workspace resuelto
- **Campo**: `items[0].root`; **Tipo**: string; **Destino**: usuario/skill; **Efecto observable**: root registrado, literal (Windows conserva `\`)
- **Campo**: `items[0].executable`; **Tipo**: string; **Destino**: usuario/skill; **Efecto observable**: ruta del binario `mi-lsp` en uso
- **Campo**: `items[0].source`; **Tipo**: string; **Destino**: usuario/skill; **Efecto observable**: `explicit`, `caller_cwd` o `last_workspace`
- **Campo**: `workspace`; **Tipo**: string; **Destino**: usuario/skill; **Efecto observable**: alias resuelto a nivel de envelope
- **Campo**: `backend`; **Tipo**: string; **Destino**: usuario/skill; **Efecto observable**: siempre `registry`

### 7. Impacto en el modelo de datos
- Reutiliza `model.RegistryFile` / `model.WorkspaceRegistration` existentes.
- No agrega un tipo nuevo de persistencia; el resultado vive solo en el envelope de respuesta.

## Errores

### 5. Errores tipados
- **Código**: (sin código tipado propio); **Causa**: workspace explícito no registrado; **Trigger**: `--workspace <selector>` no matchea ningún alias ni root; **Respuesta esperada**: error explícito `workspace "<selector>" is not registered`; se clasifica como `invalid_workspace` por el mapeo de fallback compartido
- **Código**: `WKS_SELECTOR_NOT_FOUND` vía `WorkspaceResolutionError` en resolución omitida del registry; **Causa**: sin selector y sin default; **Trigger**: no hay `--workspace`, el cwd no cae dentro de ningún root registrado y `last_workspace` está vacío o ausente; **Respuesta esperada**: error tipado cuyo texto incluye `workspace not found in registry and path does not exist` y la acción `pass --workspace <alias> or configure an explicit default workspace`; se clasifica como `invalid_workspace`. `workspace which` conserva el error explícito `no workspace specified and no default workspace configured`

## Criterios de aceptación (numerados)

1. Resolver por workspace explícito: Dado un workspace registrado con alias "gastos" Cuando ejecuto "mi-lsp workspace which --workspace gastos --format json" Entonces la respuesta incluye "backend=registry" Y "items[0].source" es "explicit" Y el registry no fue modificado
2. Resolver por caller_cwd sin --workspace: Dado el proceso corre dentro del root de un workspace registrado Cuando ejecuto "mi-lsp workspace which" Entonces "items[0].source" es "caller_cwd"
3. Conservar un path Windows literal: Dado un workspace registrado con root "C:\repos\mios\mis-plugins-cc" Cuando ejecuto "mi-lsp workspace which --workspace mis-plugins-cc --format json" Entonces "items[0].root" es exactamente "C:\repos\mios\mis-plugins-cc" sin escapes adicionales
4. Rechazar un workspace explícito no registrado: Dado ningún workspace registrado con alias "missing-workspace" Cuando ejecuto "mi-lsp workspace which --workspace missing-workspace" Entonces la operación falla con un error explícito que menciona el selector

### 8. Escenarios de aceptación (Gherkin)
```gherkin
Scenario: Resolver por workspace explicito
  Given un workspace registrado con alias "gastos"
  When ejecuto "mi-lsp workspace which --workspace gastos --format json"
  Then la respuesta incluye "backend=registry"
  And "items[0].source" es "explicit"
  And el registry no fue modificado

Scenario: Resolver por caller_cwd sin --workspace
  Given el proceso corre dentro del root de un workspace registrado
  When ejecuto "mi-lsp workspace which"
  Then "items[0].source" es "caller_cwd"

Scenario: Conservar un path Windows literal
  Given un workspace registrado con root "C:\repos\mios\mis-plugins-cc"
  When ejecuto "mi-lsp workspace which --workspace mis-plugins-cc --format json"
  Then "items[0].root" es exactamente "C:\repos\mios\mis-plugins-cc" sin escapes adicionales

Scenario: Rechazar un workspace explicito no registrado
  Given ningun workspace registrado con alias "missing-workspace"
  When ejecuto "mi-lsp workspace which --workspace missing-workspace"
  Then la operacion falla con un error explicito que menciona el selector
```

### 9. Trazabilidad de pruebas
- Positivo: `TP-WKS / TC-WKS-044`
- Positivo: `TP-WKS / TC-WKS-045`
- Positivo: `TP-WKS / TC-WKS-046`
- Negativo: `TP-WKS / TC-WKS-047` (documentado en código; ver Special Cases del cierre de wiki, sin test Go nombrado que cubra ambas ramas de error)

```yaml
harness_protocol: SDD-HARNESS-v1
id: RF-WKS-009
kind: requirement
audience: llm-first
imports:
- .docs/wiki/04_flows/FL-BOOT-01.md
exports:
- RF-WKS-009
- RF-WKS-009 - Resolver workspace y ejecutable en uso con `workspace which`
agent_must_read:
- .docs/wiki/05_requirements/RF-WKS-009.md
- .docs/wiki/04_flows/FL-BOOT-01.md
- .docs/wiki/06_tests/TP-WKS.md
agent_may_edit:
- .docs/wiki/05_requirements/RF-WKS-009.md
agent_must_not_edit:
- .docs/wiki/00_governance.md
- AGENTS.md
- internal/cli/workspace.go
- internal/cli/workspace_which.go
verify:
- 'Para los casos de este requisito en .docs/wiki/06_tests/TP-WKS.md, ejecutar el
  comando publicado: go test -count=1 ./internal/service -run ''^TestExecuteAutoRegisterSkipsIndexWithoutCommits$''
  (la evidencia de ejecución debe registrarse por separado).'
stop_if:
- 'Detenerse y escalar si el cambio contradice el límite documentado: - Supuestos
  prohibidos:'
evidence:
- .docs/wiki/05_requirements/RF-WKS-009.md
- .docs/wiki/06_tests/TP-WKS.md
```
