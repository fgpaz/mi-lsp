---
wiki_contract: WIKI-CONTRACT-v1
id: RF-CS-001
kind: requirement
status: active
title: RF-CS-001 - Ejecutar consulta semantica C# via Roslyn worker
links:
  implements:
  - FL-CS-01
  code:
  - path: internal/service/app.go
    role: behavior
---

## Propósito

RF-CS-001 - Ejecutar consulta semántica C# via Roslyn worker

### 1. Hoja de ejecución
- **Campo**: ID; **Valor**: RF-CS-001
- **Campo**: Título; **Valor**: Ejecutar consulta semántica C# via Roslyn worker
- **Campo**: Actores; **Valor**: Usuario, Skill, Agente, Core, Worker .NET
- **Campo**: Prioridad; **Valor**: alta
- **Campo**: Severidad; **Valor**: alta
- **Campo**: FL origen; **Valor**: FL-CS-01

## Comportamiento ordenado

### 2. Precondiciones detalladas
- **Condición**: Workspace contiene C# soportado; **Tipo**: funcional; **Estado requerido**: obligatorio
- **Condición**: `dotnet` disponible o worker instalado explicitamente; **Tipo**: técnica; **Estado requerido**: obligatorio
- **Condición**: Operación pedida es semántica soportada; **Tipo**: funcional; **Estado requerido**: obligatorio

### 4. Secuencia principal
1. El core detecta que la consulta requiere backend `roslyn`.
2. El router resuelve repo y entrypoint usando selectors explícitos, ownership por archivo o match único por catalogo.
3. En `get_context`, el core construye primero un slice legible alrededor de la línea pedida.
4. El runtime manager obtiene un worker activo o inicia uno nuevo para `(workspace, backend, entrypoint)`.
5. El core envia la request al worker por `stdio` con framing length-prefixed.
6. El worker carga o reutiliza el entrypoint Roslyn correcto y devuelve metadatos semánticos compactos.
7. El core arma el envelope final con `backend=roslyn`, preserva el slice y superpone los metadatos semánticos cuando existen.

### 6. Casos especiales y variantes
- Con daemon activo, el worker puede permanecer warm según LRU e idle timeout.
- Sin daemon, el core puede ejecutar la consulta en modo directo con worker efimero.
- La resolución hot-path del worker usa orden `bundle -> installed -> dev-local` por presencia de archivos; el probe explícito queda en `worker status`.
- Si el primer candidato Roslyn falla por bootstrap/arranque, el core reintenta una sola vez con el siguiente candidato antes de devolver error accionable.
- Si Roslyn no puede enriquecer `get_context` pero el archivo existe, el core debe devolver igualmente `slice_text` y warnings accionables.
- El worker nunca retorna ASTs ni blobs completos; solo datos derivados. El slice textual lo arma el core.

## Superficies

CLI `mi-lsp`, con operaciones y selectores detallados en el comportamiento del requisito.

## Entradas/salidas tipadas

### 3. Entradas
- **Campo**: `operation`; **Tipo**: enum; **Req.**: si; **Origen**: CLI/Core; **Validacion**: `find_refs`, `get_context`, `get_overview`, `get_deps`
- **Campo**: `workspace`; **Tipo**: alias/path; **Req.**: si; **Origen**: CLI; **Validacion**: debe resolver workspace C#
- **Campo**: `repo`; **Tipo**: string; **Req.**: no; **Origen**: CLI; **Validacion**: selector explícito en `container`
- **Campo**: `entrypoint`; **Tipo**: string; **Req.**: no; **Origen**: CLI; **Validacion**: selector explícito del entrypoint
- **Campo**: `solution` / `project`; **Tipo**: path relativo; **Req.**: no; **Origen**: CLI; **Validacion**: override explícito

### 7. Impacto en el modelo de datos
- `WorkspaceEntrypoint`
- `QueryEnvelope`
- `RuntimeSnapshot`

## Errores

### 5. Errores tipados
- **Código**: `CS_WORKER_MISSING`; **Causa**: worker no disponible; **Trigger**: no hay `dotnet` ni worker instalado; **Respuesta esperada**: error accionable con `mi-lsp worker install`
- **Código**: `CS_WORKSPACE_LOAD_FAILED`; **Causa**: Roslyn no carga solucion/proyecto; **Trigger**: restore o layout inválido; **Respuesta esperada**: abortar con detalle operativo o degradar a slice+catálogo si el archivo existe
- **Código**: `CS_ROUTING_AMBIGUOUS`; **Causa**: símbolo presente en varios repos; **Trigger**: falta selector explícito; **Respuesta esperada**: `backend=router`, candidatos y `next_hint`

## Criterios de aceptación (numerados)

1. El core detecta que la consulta requiere backend `roslyn`.
2. El router resuelve repo y entrypoint usando selectors explícitos, ownership por archivo o match único por catalogo.
3. En `get_context`, el core construye primero un slice legible alrededor de la línea pedida.
4. El runtime manager obtiene un worker activo o inicia uno nuevo para `(workspace, backend, entrypoint)`.
5. El core envia la request al worker por `stdio` con framing length-prefixed.
6. El worker carga o reutiliza el entrypoint Roslyn correcto y devuelve metadatos semánticos compactos.
7. El core arma el envelope final con `backend=roslyn`, preserva el slice y superpone los metadatos semánticos cuando existen.
