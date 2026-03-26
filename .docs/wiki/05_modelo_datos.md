# 5. Modelo de datos

## Proposito

`mi-lsp` modela estado operativo local, no un dominio de negocio tradicional.
La novedad canonica de v1.3 es distinguir workspaces `single` de workspaces `container`, persistir ownership por repo/entrypoint y sostener un grafo documental repo-local que permita responder `nav ask` sin depender de servicios externos.

## Entidades canonicas

| Entidad | Tipo | Owner | Persistencia | Descripcion |
|---|---|---|---|---|
| WorkspaceRegistration | Operativa | Core runtime | `~/.mi-lsp/registry.toml` | Alias, root, languages, `kind` y compatibilidad legacy |
| ProjectConfig | Operativa | Workspace owner | `<repo>/.mi-lsp/project.toml` | Nombre local, ignores, `repos`, `entrypoints`, defaults; alias semantico de `ProjectFile` en codigo Go |
| WorkspaceRepo | Operativa derivada | Core runtime | `<repo>/.mi-lsp/project.toml` | Repo hijo reconocido dentro de un workspace `container` |
| WorkspaceEntrypoint | Operativa derivada | Core runtime | `<repo>/.mi-lsp/project.toml` | `.sln` o `.csproj` semanticamente enrutable |
| SymbolRecord | Derivada | Indexer | `<repo>/.mi-lsp/index.db` | Declaracion liviana con `repo_id` y `repo` |
| FileRecord | Derivada | Indexer | `<repo>/.mi-lsp/index.db` | Metadata de archivo indexado con ownership por repo |
| DocRecord | Derivada | Doc indexer | `<repo>/.mi-lsp/index.db` | Documento indexado con `path`, `doc_id`, `layer`, `family` y texto de ranking |
| DocEdge | Derivada | Doc indexer | `<repo>/.mi-lsp/index.db` | Relacion explicita documento -> documento por doc ID o link markdown |
| DocMention | Derivada | Doc indexer | `<repo>/.mi-lsp/index.db` | Menciones explicitas desde docs hacia paths, simbolos o comandos |
| DocsReadProfile | Operativa local | Maintainer de wiki | `<repo>/.docs/wiki/_mi-lsp/read-model.toml` | Perfil opcional que clasifica familias, paths y fallback documental |
| WorkspaceMeta | Derivada | Indexer | `<repo>/.mi-lsp/index.db` | Totales, defaults y metadata del indice |
| DaemonState | Operativa | Runtime supervision | `~/.mi-lsp/daemon/state.json` | PID, endpoint, admin URL y version/protocolo |
| DaemonRun | Historica local | Runtime supervision | `~/.mi-lsp/daemon/daemon.db` | Una corrida del daemon global |
| RuntimeSnapshot | Derivada | Runtime supervision | `~/.mi-lsp/daemon/daemon.db` | Estado de un runtime por `(workspace_root, backend, entrypoint)` |
| AccessEvent | Historica local | Runtime supervision | `~/.mi-lsp/daemon/daemon.db` | Acceso ejecutado con cliente, sesion, repo y entrypoint |
| QueryEnvelope | Derivada | CLI/Core | Respuesta en memoria | Envelope estable que ve el usuario o skill; mapea a `Envelope` en `internal/model/types.go` |
| AskResult | Derivada | CLI/Core | Respuesta en memoria | Resultado de `nav ask` con `summary`, `primary_doc`, evidencias, `why` y `next_queries` |
| ServiceSurfaceSummary | Derivada | Core/service exploration | Respuesta en memoria | Resumen evidence-first de un path de servicio |
| MultiReadItem | Derivada | CLI/Core | Respuesta en memoria | Contenido de un rango de archivo leido en batch |
| BatchResult | Derivada | CLI/Core | Respuesta en memoria | Resultado de una operacion individual dentro de un nav batch |
| SymbolNeighborhood | Derivada | Core/service | Respuesta en memoria | Vecindario de un simbolo: definicion, callers, implementors, tests |
| WorkspaceMapEntry | Derivada | Core/service | Respuesta en memoria | Mapa de repos, servicios, endpoints, consumers, publishers y dependencias |
| DiffContextResult | Derivada | Core/service | Respuesta en memoria | Simbolos cambiados en un diff git con analisis de impacto |

## Relaciones y ownership

- Un `WorkspaceRegistration` referencia un workspace `single` o `container`.
- Un `ProjectConfig` puede contener muchos `WorkspaceRepo` y muchos `WorkspaceEntrypoint`.
- Cada `FileRecord` y `SymbolRecord` pertenece a un `repo_id`.
- Cada `DocRecord` puede tener muchos `DocEdge` y `DocMention`.
- Un `DocsReadProfile` gobierna como se interpreta la wiki del repo, pero no reemplaza el corpus indexado.
- Un `RuntimeSnapshot` pertenece a una combinacion `daemon_run_id + runtime_key`, donde `runtime_key` incluye `workspace_root` y `entrypoint_id`.
- Un `AccessEvent` puede guardar `workspace` visible, identidad canonica del workspace, `repo` y `entrypoint_id` para explicar routing y ambiguedad.
- Un `AskResult` se deriva de `DocRecord/DocEdge/DocMention` y, de forma secundaria, de `SymbolRecord/FileRecord`.
- Un `ServiceSurfaceSummary` se deriva de `SymbolRecord`, `FileRecord` y evidencia textual scoped al path pedido.

## Estados operativos

### Workspace

- `detected`: el root fue identificado como compatible
- `registered`: existe alias en `registry.toml`
- `indexed`: existe `.mi-lsp/index.db` valido
- `container`: el workspace agrupa muchos repos hijos y requiere routing semantico
- `docs_profiled`: existe `read-model.toml` propio o se usa el default embebido

### Runtime

- `cold`: no existe runtime activo para el entrypoint pedido
- `active`: runtime vivo en el pool del daemon
- `evicted`: runtime removido por LRU o idle timeout
- `ambiguous`: no se pudo resolver repo/entrypoint de forma unica

## Invariantes

- `registry.toml` no contiene topologia detallada del container.
- `project.toml` es la fuente local para `repo[]`, `entrypoint[]`, `default_repo` y `default_entrypoint`.
- `SymbolRecord`, `FileRecord` y `DocRecord` son reconstruibles y nunca persisten ASTs ni refs profundas.
- `RuntimeSnapshot` y `AccessEvent` deben ser suficientes para explicar por que un acceso fue warm, cold o ambiguo.
- `QueryEnvelope` siempre incluye `backend`, `warnings`, `stats` y `truncated`; si hay ambiguedad, el `backend` canonico es `router`.
- `AskResult` nunca debe invertir prioridad: la wiki rankea primero y el codigo actua como evidencia o verificacion.
- `ServiceSurfaceSummary` no persiste score de completitud ni conclusion final de auditoria.

## Datos tocados por RF

| RF | Entidades principales |
|---|---|
| RF-WKS-001 | WorkspaceRegistration, ProjectConfig, WorkspaceRepo, WorkspaceEntrypoint |
| RF-WKS-002 | WorkspaceRegistration, ProjectConfig, SymbolRecord, FileRecord |
| RF-WKS-003 | WorkspaceRegistration, ProjectConfig, QueryEnvelope |
| RF-IDX-001 | SymbolRecord, FileRecord, DocRecord, DocEdge, DocMention, WorkspaceMeta |
| RF-IDX-002 | SymbolRecord, FileRecord, DocRecord, DocEdge, DocMention, WorkspaceMeta |
| RF-QRY-001 | QueryEnvelope |
| RF-QRY-002 | QueryEnvelope, AccessEvent, WorkspaceEntrypoint |
| RF-QRY-003 | QueryEnvelope, ServiceSurfaceSummary, SymbolRecord, FileRecord |
| RF-QRY-004 | MultiReadItem, QueryEnvelope |
| RF-QRY-005 | BatchResult, QueryEnvelope |
| RF-QRY-006 | SymbolNeighborhood, QueryEnvelope, SymbolRecord |
| RF-QRY-007 | WorkspaceMapEntry, QueryEnvelope, SymbolRecord |
| RF-QRY-008 | DiffContextResult, QueryEnvelope, SymbolRecord |
| RF-QRY-009 | QueryEnvelope, SymbolRecord |
| RF-QRY-010 | AskResult, DocRecord, DocEdge, DocMention, DocsReadProfile, QueryEnvelope |
| RF-CS-001 | QueryEnvelope, RuntimeSnapshot, WorkspaceEntrypoint |
| RF-DAE-001 | DaemonState |
| RF-DAE-002 | RuntimeSnapshot, AccessEvent, DaemonState |
| RF-DAE-003 | DaemonState |
| RF-DAE-004 | SymbolRecord, FileRecord |
