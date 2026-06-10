# Informe de auditoría post-release v0.4.2 — mi-lsp

- doc_id: AUDIT-POST-RELEASE-V042
- fecha: 2026-06-09
- alcance: read-only (opción A bloqueada en brainstorming) — errores/performance desde v0.4.2, seguridad, mejoras de performance, reducción de tokens en harnesses
- base: tag `v0.4.2` = `cff9b30` (2026-06-03) · HEAD = `268f512` (solo docs después del tag)
- método: workflow multi-agente (6 lanes read-only + verificación adversarial por hallazgo P0/P1; 30 agentes, ~2M tokens)
- ventana de telemetría: 2026-06-03 → 2026-06-09, **19.514 access_events**, daemon run 102 (31,1h uptime, sin crashes)
- contrato: `session-contract.yaml` en esta carpeta (modo `validation_audit`)

## Resumen ejecutivo

No hay P0 (nada está roto de forma total ni hay vulnerabilidad explotable remota). Hay **4 P1**: (1) el auto-index de `workspace.add/init` hace full-scan sin deadline efectivo y bloquea 300-548s en repos grandes; (2) drift de release — el binario instalado es `+dirty` construido fuera del gate y el daemon reporta `version="dev"` hardcodeado; (3) fallas masivas de resolución de workspace (≈182 eventos en 40 workspaces en 6 días) que cortan los gates de gobernanza de los agentes; (4) las llamadas al worker Roslyn no tienen timeout per-call ni caché de fallo — cada query repite 70s de espera cuando una solution está rota. En seguridad, la postura general es buena (prepared statements, path-traversal protegido, checksums en installer, daemon por usuario) pero hay ~8 gaps de defensa en profundidad P2. En tokens, quedan **~10-15k tokens/sesión de ahorro fijo** y 20-40% por query pesada sin capturar.

## Nota de reconciliación de evidencia

Los verificadores adversariales corrieron en dos tandas; algunos refutaron hallazgos solo porque los IDs de telemetría ya habían salido de la ventana `recent_accesses` (no tenían acceso SQL). Regla aplicada: prevalece el veredicto del verificador que demostró acceso a `daemon.db`, y los registros que el orquestador presenció de primera mano en esta sesión (IDs 68444, 68445, 68447, 68454, 68458, 68459) se tratan como evidencia directa.

---

## 1. Errores y problemas de performance desde el release (telemetría real)

### AUD-01 (P1) Auto-index bloqueante sin deadline efectivo; 300-548s en repos grandes
- Evidencia telemetría (confirmada por verificador con acceso SQL): `index.start` máx **547,9s** (kraal, ID 65012), 231,6s (ID 64617), 164,4s (protocolo-proactivo, ID 68465); 14 de 103 ops >60s. `workspace.init` avg 26,6s, máx 300,5s (fnf08, ID 68447); `workspace.add` 300s (ID 68458). Warnings `auto-index failed: context deadline exceeded` (6) y contención de lock (IDs 68469/65121/65005).
- Causa raíz en código (verificada):
  - `internal/daemon/server.go:235` pasa `context.Background()` sin timeout a `app.Execute()`.
  - `internal/cli/root.go:434-435` fija 5 min para `workspace.init`; el deadline no se propaga per-stage al pipeline (`internal/indexer/indexer.go:45-73`).
  - `internal/service/workspace_ops.go:56-62` + `internal/store/index_lock.go:61-101`: lock de archivo sin timeout.
  - `internal/store/queries_incremental.go` existe (indexado incremental por git-diff) pero **no se usa en auto-index**: siempre full `IndexWorkspace`.
- Remediación propuesta: (a) auto-index asíncrono por defecto (devolver `job_id`, workspace usable con catálogo vacío); (b) usar el camino incremental cuando hay `.git` (300s → <10s en cambios chicos); (c) deadline per-stage + chequeos `ctx.Err()`; (d) timeout del lock con backoff.

### AUD-02 (P1) Drift de release: binario instalado `+dirty`, daemon `version="dev"`
- Evidencia (verificada con rebuild limpio): `C:\Users\fgpaz\bin\mi-lsp.exe` reporta `v0.4.2+dirty` `vcs_modified=true` (mtime 2026-06-05, posterior al release público del 06-04); rebuild desde el tag da `vcs_modified=false`. `dist/win-arm64/mi-lsp.exe` sí es el binario limpio del release (SHA `e945d013…`). El daemon hardcodea `Version="dev"` en `internal/daemon/server.go:63`.
- Impacto: viola el gate `AE-RELEASE-DISTRIBUTION` (provenance del artefacto instalado ≠ estado taggeado); confunde auditorías de telemetría (`version: dev` en state).
- Remediación: reinstalar desde `dist/` o re-correr `scripts/release/ae-release-binaries.ps1`; propagar la versión del build al daemon en el spawn (`internal/daemon/server.go:45`); agregar verificación CLI↔daemon de versión post-spawn. Nota: `ProtocolVersion` sí se valida (`server.go:192-194`) y la staleness por SHA256 funciona — el riesgo es de provenance, no de runtime.

### AUD-03 (P1) Fallas de resolución de workspace recurrentes que cortan los gates de agentes
- Evidencia: verificador run-1 con acceso SQL midió **182 eventos `workspace_resolution_failed` en 40 workspaces** (workspace.status 93, nav.governance 52, nav.wiki.search 10, index.start 8) — 5x lo que reportó la lane. Presenciado en vivo: ID 68459 (alias `mi-lsp` no registrado; cwd resuelve a `axi-smoke`). Contrapunto del run-2: `workspace hygiene` hoy muestra solo 2 paths stale → la mayoría son fricciones transitorias (worktrees que aparecen/desaparecen, aliases adivinados por agentes).
- Impacto: cada falla corta el gate obligatorio de gobernanza al inicio de sesiones de agentes y fuerza reintentos (tokens + latencia).
- Remediación: (a) auto-sugerencia ya existe (`rerun with --workspace X`) — convertirla en resolución automática opt-in cuando el cwd resuelve sin ambigüedad; (b) `workspace hygiene --apply-safe` periódico; (c) que los harness lean el alias desde `.mi-lsp/project.toml` en vez de adivinar.

### AUD-04 (P1) Sin timeout per-call al worker Roslyn ni caché de fallo: 70s repetidos por query
- Evidencia directa (presenciada): IDs 68444/68445, `nav.refs`, 70.091ms y 70.370ms, error `roslyn unavailable: Project name 'Shared.Contract' already exists in the 'Root' solution folder` (workspace buhosalud-wiki). El duplicado en `BuhoSalud.sln` es real (líneas 20/42/62/82/102 del .sln — defecto del repo consumidor, no de mi-lsp).
- Causa en mi-lsp: `internal/daemon/lifecycle.go:73-84` (`Manager.Call`) reenvía el contexto sin deadline propio; el fallo de carga de solution no se cachea, así que **cada** query semántica repite los ~70s de MSBuild.
- Remediación: (a) `MI_LSP_WORKER_CALL_TIMEOUT_SECONDS` (default ~30s) envolviendo `Manager.Call`; (b) cachear el fallo de solution-load por `runtime_key` con TTL y devolver error inmediato con hint; (c) detectar nombres de proyecto duplicados en el worker y reportar diagnóstico accionable.

### Otros errores observados (P2/P3)
| ID | Sev | Hallazgo | Evidencia |
|----|-----|----------|-----------|
| AUD-05 | P2 | `workspace.add` trata el alias como path relativo → `...\protocolo-proactivo\protocolo-proactivo` (error CreateFile crudo) | ID 68454 (presenciado); falta canonicalización/validación del path |
| AUD-06 | P2 | Búsquedas sin `--repo` en repos >5GB agotan el timeout de 5s — 29 incidentes (degradación elegante, pero frecuente) | hint_code=search_timeout, avg 5.527ms; IDs 68440/68443 |
| AUD-07 | P2 | 54 warnings `memory snapshot absent`: el `memory_pointer` no se publica atómicamente con el índice | ID 68461; workspaces nuevos/worktrees |
| AUD-08 | P3 | `nav.recall` (embeddings) p99 74s, 2 deadline-exceeded en kraal | IDs 65571/65572; avg 3,4s en 79 calls |
| AUD-09 | P3 | 559 truncamientos por `token_budget` (+97 max_items, +45 max_chars) — esperado por diseño preview, monitorear tasa por client_name | resumen access_events |

---

## 2. Seguridad

Postura general: **buena para un CLI local single-user**. Mitigaciones verificadas: prepared statements en todo el store (`internal/store/queries.go`), path-traversal bloqueado en multi-read (`internal/service/multi_read.go:76-88`), timeout 5s anti-ReDoS (`internal/service/config.go:23`), checksums SHA256 en installer (`scripts/install/install.ps1:127-143, 236-239`), daemon aislado por usuario, sin secretos en `registry.toml`. Sin evidencia de explotación. Gaps de defensa en profundidad:

| ID | Sev | Riesgo | Evidencia | Remediación |
|----|-----|--------|-----------|-------------|
| SEC-01 | P2 | Frame protocol sin límite de tamaño → OOM DoS local (header 0xFFFFFFFF = alloc 4GB) | `internal/worker/protocol.go:23-34` | `MaxFrameSize` (p.ej. 256MB) validado antes de alocar |
| SEC-02 | P2 | Admin HTTP sin auth ni validación Host/Origin; `POST /api/workspaces/{name}/warm` mutante accesible por CSRF/DNS-rebinding local | `internal/daemon/admin.go:55-63, 163-190` | token pre-compartido en state.json + check de Host/Origin |
| SEC-03 | P2 | Named pipe sin SDDL explícito (depende del default del OS) | `internal/daemon/server_windows.go:28` (`winio.ListenPipe(..., nil)`) | SDDL restrictivo owner+SYSTEM |
| SEC-04 | P2 | `state.json`/`start.lock` con 0o644 en Unix (otros usuarios leen admin_url/PID) | `internal/daemon/state_store.go:74, 129` | 0o600 + 0o700 en `~/.mi-lsp/daemon` |
| SEC-05 | P2 | Telemetría persiste paths absolutos + `decision_json` por query; exportes podrían filtrar comportamiento | `state_store.go:274-479`, `internal/telemetry/access_diagnostics.go:240-300` | retención agresiva, redacción en export, documentar no-compartir |
| SEC-06 | P2 | `rg` y `git` se resuelven por PATH sin pin → binary planting en máquinas compartidas | `internal/service/search.go:32-49`, `internal/indexer/incremental.go:72` | respetar `MI_LSP_RG`, agregar `MI_LSP_GIT`, considerar bundling |
| SEC-07 | P2 | MSBuildWorkspace evalúa `.targets/.props` de cualquier repo registrado (ejecución de código al abrir repos no confiables) — riesgo inherente a Roslyn, hoy sin documentar | `worker-dotnet/MiLsp.Worker/RoslynService.cs:215-233, 285` | documentar en alcance + advertencia/flag para repos no curados |
| SEC-08 | P2 | `GITHUB_TOKEN` enviado como Bearer en el installer si está en env | `scripts/install/install.ps1:53-54, 101` | warning en el script + docs |
| SEC-09 | P3 | `ProtocolVersion` vacío pasa la validación | `internal/daemon/server.go:192` | hacerlo requerido |
| SEC-10 | P3 | Admin UI sin CSP/X-Frame-Options | `internal/daemon/admin.go:82-86` | headers CSP + DENY |
| SEC-11 | P3 | Worker .NET sin firma Authenticode (es un EXE self-contained, ya cubierto por checksum del archive — endurecimiento opcional) | `MiLsp.Worker.csproj` | evaluar signing en CI |

## 3. Mejoras de performance (más allá de los P1)

| ID | Sev | Oportunidad | Evidencia | Ganancia estimada |
|----|-----|-------------|-----------|-------------------|
| PERF-01 | P2 | Usar indexado incremental git-aware en auto-index (hoy solo en `nav.diff-context`) | `queries_incremental.go` sin llamar desde `workspace_ops.go:58` | 300s → <10s en re-index de cambios chicos |
| PERF-02 | P2 | `wiki.search` carga TODOS los doc_records y re-rankea por query, sin caché | `internal/service/doc_query_context.go:54-69`, `doc_ranking.go:32-37` | el caso de 19s observado; caché TTL 5m |
| PERF-03 | P2 | FTS5 sin caché de resultados ni `PRAGMA optimize` | `internal/store/queries_docs.go:342-365` | latencia repetida en queries iguales |
| PERF-04 | P2 | SQLite sin `cache_size`/`mmap_size` (default ~2MB) | `internal/store/db.go:84-87` | menos I/O en FTS y catálogos grandes |
| PERF-05 | P2 | Telemetría: purga solo al startup, sin VACUUM ni cap por tamaño | `internal/daemon/server.go:91-95` | daemon.db acotado; menos contención |
| PERF-06 | P2 | Telemetry store con `MaxOpenConns=1` + retry reactivo (parche v0.4.2 enmascara la causa) | `internal/daemon/state_store.go:19-20, 449-628` | eliminar "database is locked" de raíz (cola async) |
| PERF-07 | P2 | Daemon a 676MB private_bytes sin política activa de reclamo (límites solo por env y reap cada 1min) | daemon status + `enforceIdleMemoryBoundsLocked` | techo de memoria predecible |
| PERF-08 | P2 | Watcher sin cap por-watcher de dirs (manager sí tiene LRU de 8 roots); 14.249 dirs ≈ 14.206 handles hoy, techo Windows ~32k | `internal/daemon/file_watcher.go:115-118`, `lifecycle.go:160,188` | margen antes del límite de handles |
| PERF-09 | P3 | Debounce per-file (500ms hardcoded) sin batching de ráfagas | `file_watcher.go:44, 165, 177-186` | menos re-index redundante |
| PERF-10 | P3 | Backpressure (max_inflight 16) solo en ops semánticas; `nav.search/find` la bypasean | `internal/daemon/server.go:355-363` | equidad bajo carga |

## 4. Reducción de tokens en harnesses (qué falta después de v0.4.2)

Ahorro fijo estimado por sesión de agente: **~10-15k tokens**; más 20-40% por query pesada.

### Lado CLI/daemon (envelopes)
| ID | Sev | Cambio | Evidencia | Ahorro |
|----|-----|--------|-----------|--------|
| TOK-01 | P2 | `daemon status`: `recent_accesses` default 20 con `decision_json` completo → bajar a 5 u opt-in `--telemetry` | medido: ~24.675 chars (~6.169 tokens) por llamada; `internal/daemon/server.go:207` | 4.500-5.500 tokens/llamada |
| TOK-02 | P2 | `decision_json` embebido por evento (~2.225 tokens en un status) → hash corto + detalle vía admin export | `internal/model/types.go:750-786` | 6.000-8.000/status |
| TOK-03 | P2 | `workspace_root`/`runtime_key` absolutos repetidos por item → emitir una vez a nivel envelope y referenciar | `types.go:758, 126` | 600-1.000/status |
| TOK-04 | P2 | `index_sync_details` (paths+timestamps por archivo) opt-in; default solo count | GovernanceStatus en `types.go`; visto en `nav governance` | 2.000-4.000/llamada en workspaces grandes |
| TOK-05 | P2 | Auto-`--compress` cuando `client_name` es harness (hoy nunca se auto-activa) y/o cuando `--format toon` | `internal/output/formatter.go:14-45, 372-508` | 15-20% en queries con símbolos (500-1.500/query) |
| TOK-06 | P3 | `ae_canon` completo repetido en `workspace.status` Y `nav.governance`; `next_steps` (5 strings largos) en cada envelope | observado en esta sesión | ~200-400/llamada |

### Lado documentación/harness (overhead fijo por sesión)
| ID | Sev | Cambio | Evidencia | Ahorro |
|----|-----|--------|-----------|--------|
| TOK-07 | P2 | Deduplicar CLAUDE.md (17,4KB ≈ 4,3k tokens) y AGENTS.md (22,6KB ≈ 5,7k tokens): ~15k chars idénticos (gateway AE, workflow, governance) → fragmento compartido + secciones específicas por harness | medición chars/4 | 3.000-5.000/sesión |
| TOK-08 | P2 | Lazy-load de los 9 docs AE obligatorios (38,6KB ≈ 9,7k tokens): manifest mínimo (fase + evidencia requerida + hash) para tareas normales; docs completos solo en `manifest_repair` o gobernanza bloqueada | `.docs/wiki/ae/*` | 6.000-8.000/tarea en ~80% de tareas |
| TOK-09 | P3 | Telemetría ya registra `stats.tokens_est` por llamada → exponer agregado por operación/client en admin export para priorizar con datos | `access_events` | habilita mejora continua |

## 5. Backlog de remediación propuesto (para próximas sesiones)

1. **Wave runtime-P1** (branch + PR): AUD-01 (auto-index async + incremental), AUD-04 (timeout per-call + caché de fallo Roslyn), AUD-05 (canonicalización de path en workspace.add). Tests de regresión sobre worktrees grandes.
2. **Wave release-hygiene**: AUD-02 — reinstalar binario limpio, propagar versión al daemon, verificación post-spawn; cerrar con evidencia `AE-RELEASE-DISTRIBUTION`.
3. **Wave tokens-v2**: TOK-01..TOK-06 (CLI) + TOK-07/TOK-08 (docs; requiere `ps-crear-agentsclaudemd` y sync de espejos de skills).
4. **Wave security-hardening**: SEC-01..SEC-06 (frame size, admin auth, SDDL, perms, redacción, pin de binarios) — bajo riesgo, alto valor defensivo.
5. **Wave perf-sqlite/watcher**: PERF-02..PERF-08.

## 6. Hallazgos refutados (no accionar)

- "Worker .dll sin firma = vulnerabilidad P1 de supply chain" → refutado: es un EXE self-contained dentro de un archive verificado por SHA256; queda como endurecimiento opcional P3 (SEC-11).
- Latencia 70s de `nav.refs` como bug *interno* de Roslyn worker → el origen es el `.sln` del consumidor (duplicado real verificado); lo accionable en mi-lsp es AUD-04 (timeout + caché de fallo).
- Varias refutaciones de la primera tanda de verificación se debieron a falta de acceso al histórico de telemetría, no a evidencia en contra (ver nota de reconciliación).
