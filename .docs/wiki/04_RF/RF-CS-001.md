# RF-CS-001 - Ejecutar consulta semantica C# via Roslyn worker

## 1. Execution Sheet

| Campo | Valor |
|---|---|
| ID | RF-CS-001 |
| Titulo | Ejecutar consulta semantica C# via Roslyn worker |
| Actores | Usuario, Skill, Agente, Core, Worker .NET |
| Prioridad | alta |
| Severidad | alta |
| FL origen | FL-CS-01 |

## 2. Detailed Preconditions

| Condicion | Tipo | Estado requerido |
|---|---|---|
| Workspace contiene C# soportado | funcional | obligatorio |
| `dotnet` disponible o worker instalado explicitamente | tecnica | obligatorio |
| Operacion pedida es semantica soportada | funcional | obligatorio |

## 3. Inputs

| Campo | Tipo | Req. | Origen | Validacion |
|---|---|---|---|---|
| `operation` | enum | si | CLI/Core | `find_refs`, `get_context`, `get_overview`, `get_deps` |
| `workspace` | alias/path | si | CLI | debe resolver workspace C# |
| `repo` | string | no | CLI | selector explicito en `container` |
| `entrypoint` | string | no | CLI | selector explicito del entrypoint |
| `solution` / `project` | path relativo | no | CLI | override explicito |

## 4. Process Steps (Happy Path)

1. El core detecta que la consulta requiere backend `roslyn`.
2. El router resuelve repo y entrypoint usando selectors explicitos, ownership por archivo o match unico por catalogo.
3. En `get_context`, el core construye primero un slice legible alrededor de la linea pedida.
4. El runtime manager obtiene un worker activo o inicia uno nuevo para `(workspace, backend, entrypoint)`.
5. El core envia la request al worker por `stdio` con framing length-prefixed.
6. El worker carga o reutiliza el entrypoint Roslyn correcto y devuelve metadatos semanticos compactos.
7. El core arma el envelope final con `backend=roslyn`, preserva el slice y superpone los metadatos semanticos cuando existen.

## 5. Typed Errors

| Codigo | Causa | Trigger | Respuesta esperada |
|---|---|---|---|
| `CS_WORKER_MISSING` | worker no disponible | no hay `dotnet` ni worker instalado | error accionable con `mi-lsp worker install` |
| `CS_WORKSPACE_LOAD_FAILED` | Roslyn no carga solucion/proyecto | restore o layout invalido | abortar con detalle operativo o degradar a slice+catalogo si el archivo existe |
| `CS_ROUTING_AMBIGUOUS` | simbolo presente en varios repos | falta selector explicito | `backend=router`, candidatos y `next_hint` |

## 6. Special Cases and Variants

- Con daemon activo, el worker puede permanecer warm segun LRU e idle timeout.
- Sin daemon, el core puede ejecutar la consulta en modo directo con worker efimero.
- La resolucion hot-path del worker usa orden `bundle -> installed -> dev-local` por presencia de archivos; el probe explicito queda en `worker status`.
- Si el primer candidato Roslyn falla por bootstrap/arranque, el core reintenta una sola vez con el siguiente candidato antes de devolver error accionable.
- Si Roslyn no puede enriquecer `get_context` pero el archivo existe, el core debe devolver igualmente `slice_text` y warnings accionables.
- El worker nunca retorna ASTs ni blobs completos; solo datos derivados. El slice textual lo arma el core.

## 7. Data Model Impact

- `WorkspaceEntrypoint`
- `QueryEnvelope`
- `RuntimeSnapshot`

