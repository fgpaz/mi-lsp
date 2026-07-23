# 5. Modelo de datos

```yaml
harness_protocol: SDD-HARNESS-v1
id: "05_modelo_datos"
kind: "support-doc"
audience: "dual"
imports:
  - '[[00_gobierno_documental]]'
  - '.docs/wiki/05_modelo_datos.md'
exports:
  - '05_modelo_datos'
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/05_modelo_datos.md
agent_may_edit:
  - .docs/wiki/05_modelo_datos.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
evidence:
  - .docs/wiki/05_modelo_datos.md
```

## Proposito

`mi-lsp` modela estado operativo local, no un dominio de negocio tradicional.
La base vigente distingue workspaces `single` de workspaces `container`, persiste ownership por repo/entrypoint y sostiene un grafo documental repo-local. La evolucion graph-native agrega identidad durable, generations inmutables, adjacency con evidencia y extensiones derivativas, sin reemplazar la autoridad wiki ni requerir servicios externos.

## Entidades canonicas

| Entidad | Tipo | Owner | Persistencia | Descripcion |
|---|---|---|---|---|
| WorkspaceRegistration | Operativa | Core runtime | `~/.mi-lsp/registry.toml` | Alias, root, languages, `kind` y compatibilidad legacy |
| ProjectConfig | Operativa | Workspace owner | `<repo>/.mi-lsp/project.toml` | Nombre local, ignores, `repos`, `entrypoints`, defaults, `[embeddings]` (`provider`, `base_url`, `model`, `dim`, `api_key_env`, `profile`, `batch_size`, `timeout_ms`, `encoding_format`, `user_agent`) y `[recall.rerank_extension]` (`enabled`, `command`, `args`, `timeout_ms`, `candidate_count`, `top_n`, `max_snippet_chars`); alias semantico de `ProjectFile` en codigo Go |
| WorkspaceRepo | Operativa derivada | Core runtime | `<repo>/.mi-lsp/project.toml` | Repo hijo reconocido dentro de un workspace `container` |
| WorkspaceEntrypoint | Operativa derivada | Core runtime | `<repo>/.mi-lsp/project.toml` | `.sln` o `.csproj` semanticamente enrutable |
| SymbolRecord | Derivada | Indexer | `<repo>/.mi-lsp/index.db` | Declaracion liviana con `repo_id` y `repo` |
| FileRecord | Derivada | Indexer | `<repo>/.mi-lsp/index.db` | Metadata de archivo indexado con ownership por repo |
| DocRecord | Derivada | Doc indexer | `<repo>/.mi-lsp/index.db` | Documento indexado con `path`, `doc_id`, `layer`, `family` y texto de ranking |
| DocEdge | Derivada | Doc indexer | `<repo>/.mi-lsp/index.db` | Relacion explicita documento -> documento por doc ID o link markdown |
| DocMention | Derivada | Doc indexer | `<repo>/.mi-lsp/index.db` | Menciones explicitas desde docs hacia paths, simbolos o comandos |
| DocSourceBlock | Derivada | Doc indexer | `<repo>/.mi-lsp/index.db` | Bloque `toon` normativo de un artefacto `SDD-WIKI-SOURCE-v1`, con `block_id`, `doc_id`, lineas y hash |
| DocSourceRecord | Derivada | Doc indexer | `<repo>/.mi-lsp/index.db` | Record referenciable dentro de un bloque fuente, con `record_id`, `record_type`, lineas y hash |
| WikiChunkEmbedding | Derivada | Doc indexer | `<repo>/.mi-lsp/index.db` | Chunk wiki enriquecido con metadata, hash de contenido/prefix, BLOB float32, `embedding_model`, `embedding_dim`, heading, snippet y rango de lineas |
| GovernanceSource | Operativa local | Maintainer de wiki | `<repo>/.docs/wiki/00_gobierno_documental.md` | Bloque YAML fuente que define perfil, jerarquia, cadenas y reglas de bloqueo |
| GovernanceStatus | Derivada | CLI/Core | Respuesta en memoria | Estado efectivo de gobernanza: perfil, sync, bloqueos, overlays y pasos de reparacion |
| DocsReadProfile | Operativa local | Maintainer de wiki | `<repo>/.docs/wiki/_mi-lsp/read-model.toml` | Perfil opcional que clasifica familias, paths y fallback documental |
| DocsOwnerHint | Operativa local | Maintainer de wiki | `<repo>/.docs/wiki/00_gobierno_documental.md` -> `read-model.toml` | Hint opcional repo-especifico para ownership documental de capabilities nuevas |
| DocsGovernanceProfile | Operativa derivada | CLI/Core | `<repo>/.docs/wiki/_mi-lsp/read-model.toml` | Proyeccion ejecutable de la gobernanza humana: perfil efectivo, base, overlays y cadenas |
| WorkspaceMeta | Derivada | Indexer | `<repo>/.mi-lsp/index.db` | Totales, defaults, schema y punteros activos del indice |
| GraphGeneration | Derivada inmutable | Graph Kernel | `<repo>/.mi-lsp/index.db` | Snapshot sellado por source/config/backend digests, con estado staged/active/retired/invalid |
| GraphNode | Derivada | Graph Kernel | `<repo>/.mi-lsp/index.db` | Nodo tipado con `node_id` local, `NodeKey` BLOB(32), identity fields, owner, generation, provenance y cross-RID |
| GraphEdge | Derivada | Graph Kernel | `<repo>/.mi-lsp/index.db` | Relacion dirigida y tipada entre NodeKeys existentes en la misma generation |
| GraphEvidence | Derivada | Backend/Graph Kernel | `<repo>/.mi-lsp/index.db` | Observacion versionada con source URI/range/digest, backend, claim, status, generation y cross-RID |
| GraphUnresolved | Derivada | Backend/Validator | `<repo>/.mi-lsp/index.db` | Identidad, edge o target no publicable con reason code, candidatos bounded y recovery hint |
| GraphMigration | Operativa derivada | SQLite Publisher | `<repo>/.mi-lsp/index.db` | Ventana de schema, preflight, checksums, dual-read/write y rollback metadata |
| GraphAnalysis | Derivada descartable | Core/MILX Host | `<repo>/.mi-lsp/index.db` o cache local | Resultado de algoritmo/pack keyeado por generation, extension y parametros; nunca autoridad primaria |
| GlobalGraphSnapshot | Derivada descartable | Core/Daemon opcional | Estado global local | Vista sellada de member workspaces/generations y cross-edges; no escribe stores miembros |
| MILXManifest | Operativa local | Extension owner/Core | Archivo de extension validado | ID/version/digest, protocol, schemas, capabilities y resource hints |
| MILXExecution | Historica local acotada | MILX Host | Telemetria/cache local | request, generation, status, budgets, output digest y cleanup; sin payload arbitrario |
| ContextPack | Derivada | Context Optimizer | Respuesta/cache | Seleccion bounded de autoridad/evidencia con costo de tokens, coverage, omissions y digest |
| DaemonState | Operativa | Runtime supervision | `~/.mi-lsp/daemon/state.json` | PID, endpoint, admin URL y version/protocolo |
| DaemonRun | Historica local | Runtime supervision | `~/.mi-lsp/daemon/daemon.db` | Una corrida del daemon global |
| RuntimeSnapshot | Derivada | Runtime supervision | `~/.mi-lsp/daemon/daemon.db` | Estado de un runtime por `(workspace_root, backend, entrypoint)` |
| AccessEvent | Historica local | Runtime supervision | `~/.mi-lsp/daemon/daemon.db` | Acceso ejecutado con cliente, sesion, repo y entrypoint |
| QueryEnvelope | Derivada | CLI/Core | Respuesta en memoria | Envelope estable que ve el usuario o skill; mapea a `Envelope` en `internal/model/types.go` |
| AskResult | Derivada | CLI/Core | Respuesta en memoria | Resultado de `nav ask` con `summary`, `primary_doc`, evidencias, `why` y `next_queries` |
| RecallResult | Derivada | CLI/Core | Respuesta en memoria | Resultado de `nav recall` con `query`, `intent`, `archivo`, `heading`, `score`, `snippet`, rango de lineas y `why` |
| PackResult | Derivada | CLI/Core | Respuesta en memoria | Resultado de `nav pack` con familia, modo, documento primario, reading pack y siguientes pasos |
| PackDoc | Derivada | CLI/Core | Respuesta en memoria | Documento seleccionado dentro del reading pack con stage, targets y slice opcional |
| PackTarget | Derivada | CLI/Core | Respuesta en memoria | Heading/linea sugerida para orientar lectura compacta del documento |
| ServiceSurfaceSummary | Derivada | Core/service exploration | Respuesta en memoria | Resumen evidence-first de un path de servicio |
| MultiReadItem | Derivada | CLI/Core | Respuesta en memoria | Contenido de un rango de archivo leido en batch |
| BatchResult | Derivada | CLI/Core | Respuesta en memoria | Resultado de una operacion individual dentro de un nav batch |
| SymbolNeighborhood | Derivada | Core/service | Respuesta en memoria | Vecindario de un simbolo: definicion, callers, implementors, tests |
| WorkspaceMapEntry | Derivada | Core/service | Respuesta en memoria | Mapa de repos, servicios, endpoints, consumers, publishers y dependencias |
| DiffContextResult | Derivada | Core/service | Respuesta en memoria | Simbolos cambiados en un diff git con analisis de impacto |

## Relaciones y ownership

- Un `WorkspaceRegistration` referencia un workspace `single` o `container`.
- Un `ProjectConfig` puede contener muchos `WorkspaceRepo` y muchos `WorkspaceEntrypoint`.
- Un `ProjectConfig` puede declarar `[embeddings]`; `api_key_env` nombra una variable de entorno, no guarda secretos.
- Un `ProjectConfig` puede declarar `[recall.rerank_extension]` como hook local externo; no guarda payloads, respuestas de proveedor ni secretos.
- Cada `FileRecord` y `SymbolRecord` pertenece a un `repo_id`.
- Cada `DocRecord` puede tener muchos `DocEdge`, `DocMention`, `DocSourceBlock` y `DocSourceRecord`.
- Un `WikiChunkEmbedding` pertenece a un chunk documental y se invalida cuando cambia metadata-prefix, texto enriquecido, content hash, modelo o dimension.
- Un `GovernanceSource` manda sobre el `DocsReadProfile`; la proyeccion ejecutable no redefine la autoridad humana.
- Un `DocsOwnerHint` vive en `GovernanceSource` y se proyecta al `DocsReadProfile`; no redefine la gobernanza, solo refina ranking documental repo-especifico.
- Un `DocsReadProfile` gobierna como se interpreta la wiki del repo, pero no reemplaza el corpus indexado.
- Un `RuntimeSnapshot` pertenece a una combinacion `daemon_run_id + runtime_key`, donde `runtime_key` incluye `workspace_root` y `entrypoint_id`.
- Un `AccessEvent` puede guardar `workspace` visible, identidad canonica del workspace, `repo` y `entrypoint_id` para explicar routing y ambiguedad.
- Un `AskResult` se deriva de `DocRecord/DocEdge/DocMention` y, de forma secundaria, de `SymbolRecord/FileRecord`.
- Un `PackResult` se deriva de `DocRecord/DocEdge` y del `DocsReadProfile`; las slices se materializan on-demand desde archivos del workspace.
- Un `ServiceSurfaceSummary` se deriva de `SymbolRecord`, `FileRecord` y evidencia textual scoped al path pedido.
- Un `GraphGeneration` contiene muchos `GraphNode`, `GraphEdge`, `GraphEvidence` y `GraphUnresolved`; solo una generation sellada puede ser activa por workspace/schema.
- `GraphEdge` referencia dos `GraphNode` de la misma generation y una o mas `GraphEvidence`; unresolved nunca se materializa como endpoint fantasma.
- `GraphMigration` gobierna compatibilidad entre schema legacy y graph-native; no modifica el significado de las entidades.
- `GraphAnalysis` y `ContextPack` se derivan de generation(s), authority/profile digest, pack/extension version y parameters digest; invalidar cualquiera invalida el cache.
- `GlobalGraphSnapshot` referencia member generations inmutables por `(workspace_identity, generation_id)` y resuelve cross-edges por cross-RID sin adquirir ownership de sus datos.
- `MILXExecution` consume un pack/snapshot read-only y puede producir `GraphAnalysis`; no crea o modifica `GraphNode`, `GraphEdge`, wiki ni generation activa.

## Identidad graph-native

`NodeKey v1` es SHA-256 de la serializacion binaria versionada y length-prefixed de `{repository_identity, backend_type, language, project_or_module, owner_path, symbol_kind, semantic_identity}`. Strings usan UTF-8/NFC y paths repo-relative con `/`; root absoluto, timestamps y declaration ranges no participan. `node_id INTEGER` es solo surrogate SQLite. Missing fields, normalizacion no determinista o una colision hash con tupla distinta producen `GraphUnresolved` y bloquean la publicacion.

Los cross-RIDs de nodo/edge/evidence son representaciones versionadas derivadas de sus identidades canonicas, no del RID del binario ni del host. El mismo fixture/config/backend version debe producirlos byte-identical en los RIDs soportados.

## Estados operativos

### Workspace

- `detected`: el root fue identificado como compatible
- `registered`: existe alias en `registry.toml`
- `indexed`: existe `.mi-lsp/index.db` valido
- `container`: el workspace agrupa muchos repos hijos y requiere routing semantico
- `docs_profiled`: existe `read-model.toml` propio o se usa el default embebido
- `governance_blocked`: la gobernanza esta invalida o el indice quedo stale respecto de `00`/`read-model`

### Runtime

- `cold`: no existe runtime activo para el entrypoint pedido
- `active`: runtime vivo en el pool del daemon
- `evicted`: runtime removido por LRU o idle timeout
- `ambiguous`: no se pudo resolver repo/entrypoint de forma unica

### GraphGeneration

- `staged`: completa en escritura, aun invisible para readers
- `active`: snapshot unico seleccionado por el puntero del workspace
- `retired`: snapshot validado anterior, retenido para rollback
- `invalid`: staging rechazado o incompleto; nunca consultable

### Backend / claim

- `exact`: compiler/LSP resolvio identidad y relacion
- `extracted`: parser estructural versionado observo la forma
- `inferred`: regla derivativa explicita, nunca compiler fact
- `ambiguous` / `unresolved` / `unsupported`: no se publica claim positivo

## Invariantes

- `registry.toml` no contiene topologia detallada del container.
- `project.toml` es la fuente local para `repo[]`, `entrypoint[]`, `default_repo` y `default_entrypoint`.
- `SymbolRecord`, `FileRecord` y `DocRecord` son reconstruibles y nunca persisten ASTs ni refs profundas.
- `RuntimeSnapshot` y `AccessEvent` deben ser suficientes para explicar por que un acceso fue warm, cold o ambiguo.
- `QueryEnvelope` siempre incluye `backend`, `warnings`, `stats` y `truncated`; si hay ambiguedad, el `backend` canonico es `router`.
- `QueryEnvelope` puede agregar `mode` cuando la superficie publica distingue variantes estables (`nav.intent docs|code`).
- `AskResult` nunca debe invertir prioridad: la wiki rankea primero y el codigo actua como evidencia o verificacion.
- `PackResult` debe preservar el orden canonico global -> especifico y no degradar silenciosamente a docs genericos cuando la wiki canonica existe pero el indice documental esta vacio.
- `ServiceSurfaceSummary` no persiste score de completitud ni conclusion final de auditoria.
- SQLite repo-local es la autoridad de adjacency; queries no materializan el grafo completo en RAM, no persisten closures transitivos y no migran schema.
- Readers fijan una sola `GraphGeneration`; observan el snapshot anterior o el nuevo completo, nunca una mezcla.
- Todo `GraphEdge` activo tiene endpoints, evidence, generation, provenance y cross-RID validos; ambiguous/unresolved no crea dangling edge.
- Publicacion, dual-read/write y rollback son transaccionales; crash/cancel conserva o restaura el puntero validado anterior.
- Wiki sigue siendo autoridad: graph/code puede verificar o detectar drift, no sobreescribir canon.
- MILX/packs son aislados y derivativos, sin graph/wiki write, MCP, red o secretos en v1.
- Toda claim de victoria conserva raw samples, ambos comparadores, 30 repeticiones y metricas unavailable explicitas.

## Datos tocados por RF

| RF | Entidades principales |
|---|---|
| RF-WKS-001 | WorkspaceRegistration, ProjectConfig, WorkspaceRepo, WorkspaceEntrypoint |
| RF-WKS-002 | WorkspaceRegistration, ProjectConfig, SymbolRecord, FileRecord |
| RF-WKS-003 | WorkspaceRegistration, ProjectConfig, QueryEnvelope |
| RF-WKS-005 | GovernanceSource, GovernanceStatus, WorkspaceRegistration, QueryEnvelope |
| RF-IDX-001 | SymbolRecord, FileRecord, DocRecord, DocEdge, DocMention, DocSourceBlock, DocSourceRecord, WorkspaceMeta |
| RF-IDX-002 | SymbolRecord, FileRecord, DocRecord, DocEdge, DocMention, DocSourceBlock, DocSourceRecord, WorkspaceMeta |
| RF-IDX-003 | GovernanceSource, DocsGovernanceProfile, DocsReadProfile, WorkspaceMeta |
| RF-GPH-001 | GraphNode, GraphUnresolved |
| RF-GPH-002 | GraphGeneration, GraphMigration, WorkspaceMeta |
| RF-GPH-003 | GraphNode, GraphEdge, GraphEvidence, GraphUnresolved |
| RF-GPH-004 | GraphGeneration, GraphNode, GraphEdge, GraphEvidence, GraphUnresolved |
| RF-GPH-005 | GraphGeneration, GraphNode, GraphEdge, GraphEvidence, QueryEnvelope |
| RF-GPH-006 | GraphGeneration, GraphEdge, GraphEvidence, GraphUnresolved, DiffContextResult |
| RF-GPH-007 | DocRecord, DocEdge, DocMention, GraphNode, GraphEdge, ContextPack |
| RF-GPH-008 | GraphNode, GraphEdge, GraphEvidence, GraphUnresolved |
| RF-GPH-009 | GlobalGraphSnapshot, GraphGeneration, GraphEdge |
| RF-GPH-010 | MILXManifest, MILXExecution, GraphAnalysis |
| RF-GPH-011 | ContextPack, GraphAnalysis, MILXManifest |
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
| RF-QRY-011 | SymbolRecord, DocRecord, DocsOwnerHint, QueryEnvelope |
| RF-QRY-012 | PackResult, PackDoc, PackTarget, DocRecord, DocEdge, DocsReadProfile, QueryEnvelope |
| RF-QRY-013 | GovernanceStatus, DocsReadProfile, QueryEnvelope |
| RF-QRY-016 | WikiSearchResult, HarnessValidationResult, WikiSourceValidationResult, DocRecord, DocSourceBlock, DocSourceRecord, QueryEnvelope |
| RF-CS-001 | QueryEnvelope, RuntimeSnapshot, WorkspaceEntrypoint |
| RF-DAE-001 | DaemonState |
| RF-DAE-002 | RuntimeSnapshot, AccessEvent, DaemonState |
| RF-DAE-003 | DaemonState |
| RF-DAE-004 | SymbolRecord, FileRecord |
