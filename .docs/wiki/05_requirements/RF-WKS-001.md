---
wiki_contract: WIKI-CONTRACT-v1
id: RF-WKS-001
kind: requirement
status: active
title: RF-WKS-001 - Registrar workspace por path y alias
links:
  implements:
  - FL-BOOT-01
  code:
  - path: internal/service/app.go
    role: behavior
---

## Propósito

RF-WKS-001 - Registrar workspace por path y alias

### 1. Hoja de ejecución
- **Campo**: ID; **Valor**: RF-WKS-001
- **Campo**: Título; **Valor**: Registrar workspace por path y alias
- **Campo**: Actores; **Valor**: Desarrollador, Skill, CLI/Core
- **Campo**: Prioridad; **Valor**: alta
- **Campo**: Severidad; **Valor**: alta
- **Campo**: FL origen; **Valor**: FL-BOOT-01

## Comportamiento ordenado

### 2. Precondiciones detalladas
- **Condición**: Path objetivo existe; **Tipo**: técnica; **Estado requerido**: obligatorio
- **Condición**: Path objetivo es accesible; **Tipo**: técnica; **Estado requerido**: obligatorio
- **Condición**: El root o alguno de sus hijos contiene `.sln`, `.csproj`, `go.mod`, `go.work`, `Cargo.toml`, `package.json`, `tsconfig.json`, `pyproject.toml`, `setup.py`, `setup.cfg` o `requirements.txt`; **Tipo**: funcional; **Estado requerido**: obligatorio
- **Condición**: `~/.mi-lsp/registry.toml` es escribible; **Tipo**: operativa; **Estado requerido**: obligatorio

### 4. Secuencia principal
1. La CLI recibe `workspace add <path> [--name alias]`.
2. El core normaliza el path y valida accesibilidad.
3. El detector clasifica el layout como `single` o `container`.
4. El detector obtiene `repo[]`, `entrypoint[]`, `default_repo` y `default_entrypoint` respetando ignores. `Cargo.toml` identifica Rust y se conserva como entrypoint de proyecto; un root con `[workspace]` mantiene sus crates miembros dentro del mismo repo. Para un repo Go, la topología conserva la ruta exacta del entrypoint relativa al workspace (por ejemplo, `runtime/go.mod`), y el selector que consume el observador puede ser una ruta de módulo relativa al repo configurada explícitamente (`go.mod`) o un ID generado de `WorkspaceEntrypoint`; el ID se resuelve con el repo y entrypoint exactos, se rebasa su ruta workspace-relative al repo seleccionado y se revalida como `go.mod` seguro y regular. IDs desconocidos/malformados y rutas inseguras fallan cerrado; resolver un ID no convierte contenido raíz no relacionado en un entrypoint.
5. El core crea o actualiza `<repo>/.mi-lsp/project.toml`.
6. El registry global hace upsert del alias, root, languages y `kind`.
7. La CLI devuelve confirmacion con topología resumida.

### 7. Casos especiales y variantes
- `single`: un repo con un root semántico obvio.
- `container`: carpeta padre con muchos repos hijos, sin requerir `.sln` agregadora.
- Un `Cargo.toml` raíz selecciona `single` aunque `src/` o crates miembros tengan subdirectorios con fuentes Rust; si declara `[workspace]`, sus manifiestos miembros quedan como entrypoints del mismo repo.
- Paths auxiliares como `.worktrees/` nunca deben ser elegidos como default.
- Entrypoints ubicados bajo `.docs/` o `template(s)` pueden seguir visibles en la topología, pero no deben quedar como `default_entrypoint` si existe una alternativa real del repo.
- Para Go, un módulo anidado se representa con una ruta exacta relativa al workspace (por ejemplo, `runtime/go.mod`), mientras la observación recibe el selector repo-local explícito (`go.mod`) configurado para ese repo. `DefaultEntrypoint` también puede ser un ID de `WorkspaceEntrypoint`: se resuelve con coincidencia exacta de repo y entrypoint, se rebasa su ruta workspace-relative al root del repo seleccionado y se revalida que el resultado sea un `go.mod` repo-local, seguro y regular antes de observar. Los IDs desconocidos o malformados, paths inexistentes/no regulares, symlinks finales o selecciones inseguras fallan cerrado y quedan como omisión/diagnóstico (`""`), sin activar el fallback. `go.work` no es un selector soportado por la observación Go y se rechaza de forma fail-closed.
- Solo si el selector Go repo-local está vacío se conserva el fallback determinista al `go.mod` del root. Un root que solo contiene `go.work` no selecciona módulo ni intenta extracción de grafo; no se hace descubrimiento recursivo arbitrario para sustituir ese fallback, y el contenido raíz no relacionado conserva su ownership de catálogo.
- Si el alias ya existe, el comportamiento es `upsert`.
- `workspace doctor` puede derivar estado `health` y `next_actions` desde el registro existente para diagnóstico humano/agente; no modifica `WorkspaceRegistration`, no elige alias por el usuario y no borra worktrees.
#### Reparación manual mínima del nombre local

La reparación puntual no es un alta ni un rename del registro. El procedimiento único está en [[CT-CLI-DAEMON-ADMIN]], bloque `CT-CLI-DAEMON-ADMIN.project-name-manual-v1`.

- **Marcador estable**: RF-WKS-001.project-name-manual-v1
- **operation**: Edición manual por el owner de name dentro de [project]
- **procedure_ref**: CT-CLI-DAEMON-ADMIN.project-name-manual-v1
- **preconditions**:
  - Owner confirma raíz física absoluta y target concreto; main y worktree no son intercambiables
  - CLI/revisión y contrato aplicables confirmados; alias deseado ya registrado para exactamente esa raíz
  - Sin colisión de alias, writers concurrentes ni edición de WIP ajeno
- **effect**:
  - **field**: <raíz>/.mi-lsp/project.toml: [project].name
  - **same_root_alias_precedence**: project.name coincidente; basename coincidente; last_workspace de esa raíz; orden lexicográfico
  - **single_alias**: Se selecciona el único alias de la raíz, independientemente de project.name
- **Invariantes**:
  - Nombre mostrado y raíz física son identidades distintas; un selector explícito puede normalizar su etiqueta al alias canónico de la misma raíz
  - Cambiar name no registra aliases, no mueve raíz/main/índice y no genera graph
  - No modifica registry, defaults, topología, comentarios ni otros settings
  - No repara catálogo missing, generaciones ni valores null
- **selector_boundary**:
  - **unregistered_worktree**: Ruta absoluta explícita en resolución readonly según contrato v0.12.5; conserva la raíz sin registrar ni indexar
  - **omitted_or_cwd**: Conserva resolución anterior CWD/Git y fallback last_workspace; no extrapolar la garantía de ruta explícita
- **missing_precondition**: No editar; declarar gap específico y owner, sin inventar comando de rename/config ni receta alternativa

## Superficies

- Edición manual por el owner de name dentro de [project]

## Entradas/salidas tipadas

### 3. Entradas
- **Campo**: `path`; **Tipo**: path absoluto o relativo; **Req.**: si; **Origen**: CLI; **Validacion**: debe existir y ser directorio
- **Campo**: `alias`; **Tipo**: string; **Req.**: no; **Origen**: CLI; **Validacion**: si falta, usar nombre del root

### 5. Salidas
- **Campo**: `workspace`; **Tipo**: string; **Destino**: usuario/skill; **Efecto observable**: alias registrado
- **Campo**: `kind`; **Tipo**: string; **Destino**: usuario/skill; **Efecto observable**: `single` o `container`
- **Campo**: `repo_count`; **Tipo**: número; **Destino**: usuario/skill; **Efecto observable**: cantidad de repos detectados
- **Campo**: `entrypoint_count`; **Tipo**: número; **Destino**: usuario/skill; **Efecto observable**: cantidad de entrypoints semánticos
- **Campo**: `project.toml`; **Tipo**: archivo; **Destino**: repo local; **Efecto observable**: topología creada o actualizada

### 8. Impacto en el modelo de datos
- `WorkspaceRegistration`
- `ProjectConfig`
- `WorkspaceRepo`
- `WorkspaceEntrypoint`

## Errores

### 6. Errores tipados
- **Código**: `WKS_PATH_NOT_FOUND`; **Causa**: path inexistente; **Trigger**: el directorio no existe; **Respuesta esperada**: abortar sin side effects
- **Código**: `WKS_UNSUPPORTED_LAYOUT`; **Causa**: root incompatible; **Trigger**: no se detectan marcadores soportados en root o hijos; **Respuesta esperada**: abortar con mensaje explícito
- **Código**: `WKS_REGISTRY_WRITE_FAILED`; **Causa**: fallo de persistencia; **Trigger**: no se puede escribir `registry.toml`; **Respuesta esperada**: abortar con error y sin registro parcial

## Criterios de aceptación (numerados)

1. La CLI recibe `workspace add <path> [--name alias]`.
2. El core normaliza el path y valida accesibilidad.
3. El detector clasifica el layout como `single` o `container`.
4. El detector obtiene `repo[]`, `entrypoint[]`, `default_repo` y `default_entrypoint` respetando ignores. `Cargo.toml` identifica Rust y se conserva como entrypoint de proyecto; un root con `[workspace]` mantiene sus crates miembros dentro del mismo repo. Para un repo Go, la topología conserva la ruta exacta del entrypoint relativa al workspace (por ejemplo, `runtime/go.mod`), y el selector que consume el observador puede ser una ruta de módulo relativa al repo configurada explícitamente (`go.mod`) o un ID generado de `WorkspaceEntrypoint`; el ID se resuelve con el repo y entrypoint exactos, se rebasa su ruta workspace-relative al repo seleccionado y se revalida como `go.mod` seguro y regular. IDs desconocidos/malformados y rutas inseguras fallan cerrado; resolver un ID no convierte contenido raíz no relacionado en un entrypoint.
5. El core crea o actualiza `<repo>/.mi-lsp/project.toml`.
6. El registry global hace upsert del alias, root, languages y `kind`.
7. La CLI devuelve confirmacion con topología resumida.
