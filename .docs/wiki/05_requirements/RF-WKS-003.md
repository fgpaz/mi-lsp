---
wiki_contract: WIKI-CONTRACT-v1
id: RF-WKS-003
kind: requirement
status: active
title: RF-WKS-003 - Inicializar el workspace actual y dejarlo listo para `nav ask`
links:
  implements:
  - FL-BOOT-01
  code:
  - path: internal/service/app.go
    role: behavior
---

## Propósito

RF-WKS-003 - Inicializar el workspace actual y dejarlo listo para `nav ask`

### 1. Hoja de ejecución
- **Campo**: ID; **Valor**: RF-WKS-003
- **Campo**: Título; **Valor**: Inicializar el workspace actual y dejarlo listo para `nav ask`
- **Campo**: Actores; **Valor**: Desarrollador, Skill, CLI/Core
- **Campo**: Prioridad; **Valor**: alta
- **Campo**: Severidad; **Valor**: media
- **Campo**: FL origen; **Valor**: FL-BOOT-01

## Comportamiento ordenado

### 2. Precondiciones detalladas
- **Condición**: Path objetivo existe o se usa el `cwd`; **Tipo**: técnica; **Estado requerido**: obligatorio
- **Condición**: Layout compatible detectable; **Tipo**: funcional; **Estado requerido**: obligatorio
- **Condición**: `~/.mi-lsp/registry.toml` es escribible; **Tipo**: operativa; **Estado requerido**: obligatorio

### 3. Secuencia principal
1. La CLI recibe `mi-lsp init [path] [--name alias]`.
2. Si el path falta, usa `.` como workspace objetivo.
3. Reusa la deteccion de layout y persistencia de `workspace add`.
4. Registra el workspace y lo deja como `last_workspace`.
5. Por defecto indexa automaticamente.
6. Devuelve un resultado resumido con `next_steps` centrados en `nav ask`.
7. Como `init` pertenece a la superficie AXI-default, los `next_steps` deben privilegiar reruns utiles sin repetir `--axi`; solo los caminos hacia superficies classic-default deben agregar `--axi` explicitamente.

### 5. Casos especiales y variantes
- `init` es una puerta corta; no redefine el contrato de `workspace add`.
- `--no-index` conserva semántica equivalente a `workspace add --no-index`.
- Si la indexación falla, el registro sigue siendo exitoso con warning accionable.
- `init` sigue haciendo el mismo bootstrap; el modo AXI/default solo cambia la disclosure de `next_steps`.
- `--classic` restaura la salida clasica sin desactivar el bootstrap ni el indexing.

## Superficies

CLI `mi-lsp`, con operaciones y selectores detallados en el comportamiento del requisito.

## Entradas/salidas tipadas

### 6. Impacto en el modelo de datos
- `WorkspaceRegistration`
- `ProjectConfig`
- `QueryEnvelope`
- `QueryOptions`

## Errores

### 4. Errores tipados
- **Código**: `WKS_INIT_UNSUPPORTED_LAYOUT`; **Causa**: root incompatible; **Trigger**: no se detectan marcadores soportados; **Respuesta esperada**: abortar sin side effects
- **Código**: `WKS_INIT_REGISTRY_FAILED`; **Causa**: fallo de persistencia; **Trigger**: no se puede escribir `registry.toml`; **Respuesta esperada**: abortar con error explícito

## Criterios de aceptación (numerados)

1. La CLI recibe `mi-lsp init [path] [--name alias]`.
2. Si el path falta, usa `.` como workspace objetivo.
3. Reusa la deteccion de layout y persistencia de `workspace add`.
4. Registra el workspace y lo deja como `last_workspace`.
5. Por defecto indexa automaticamente.
6. Devuelve un resultado resumido con `next_steps` centrados en `nav ask`.
7. Como `init` pertenece a la superficie AXI-default, los `next_steps` deben privilegiar reruns utiles sin repetir `--axi`; solo los caminos hacia superficies classic-default deben agregar `--axi` explicitamente.
