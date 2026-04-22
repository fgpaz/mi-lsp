# TECH-DAEMON-GOBERNANZA

Volver a [07_baseline_tecnica.md](../07_baseline_tecnica.md).

## Summary

Define el modelo canonico del daemon global, su governance UI workspace-first y la gestion compartida de runtimes calientes entre multiples clientes locales.

## Owner and scope

- Owner logico: Runtime supervision
- Scope: daemon global, runtime pool, health, governance UI, acceso concurrente local, warm seguro y logs locales
- Non-goals: auth remota, cluster multi-host, observabilidad externa, acciones destructivas desde la UI

## Runtime o subsistema

### Topologia canonica

- Un daemon por usuario/host.
- Un runtime vivo por `(workspace_root, backend_type)`.
- Pools separados por backend:
  - `roslyn`
  - `tsserver`
- Politica por defecto:
  - `max_workers = 3`
  - `idle_timeout = 30m`
  - `watch_mode = lazy`
  - `max_watched_roots = 8`
  - `max_inflight = 16`
  - eviction `LRU`

### Acceso compartido

- El daemon vive fuera de la terminal que lo lanza.
- Claude Code, Codex y subagentes deben poder conectarse al mismo daemon bajo el mismo usuario.
- No todo `nav` debe pasar por el daemon: las lecturas baratas de catalogo/texto se resuelven directo en la CLI y reservan el daemon para queries semanticas o compuestas.
- `nav.ask` tambien pertenece a ese camino directo por default; el daemon no debe convertirse en dependencia accidental de onboarding docs-first.
- Cuando el daemon atiende diagnosticos administrativos como `worker status`, debe delegar al contrato canonico del core y no reinterpretar el payload como una lista cruda de runtimes.
- Cada request debe incluir cuando esta disponible:
  - `client_name`
  - `session_id`
- `daemon start` debe:
  - chequear health primero
  - devolver metadatos de la instancia existente si ya corre
  - crear nueva instancia solo bajo lock global
- `EnsureDaemon` debe usar el mismo lock global y health recheck que `daemon start`; el estado persistido no basta si el pipe/socket no responde.
- El watcher no arranca recursivamente sobre todos los aliases por default: `lazy` activa por root canonico, `eager` es opt-in y `off` deshabilita watchers.
- Los watchers se deduplican por `workspace_root` canonico y se acotan por LRU con `max_watched_roots`.
- Requests pesadas daemon-aware se acotan con `max_inflight`; saturacion devuelve `daemon/backpressure_busy`.

### Governance UI

- Expuesta solo en `127.0.0.1:<port>`.
- Instancia unica global.
- `admin open --workspace <alias>` y `daemon open --workspace <alias>` deben abrir la misma UI enfocando el workspace por query params.
- Deep-link canonico:
  - `/?workspace=<alias>&panel=<overview|activity|logs|metrics>&window=<recent|7d|30d|90d>&backend=<type>`
- Vistas minimas:
  - estado del daemon
  - KPIs operativos
  - tabs por workspace
  - runtimes activos y memoria por backend
  - access events recientes por cliente/sesion con corte temporal explicito y drawer con `workspace_root`, `workspace_input`, `error_kind` y `error_code`
  - drawer de detalle
  - tail de logs locales
  - panel metrics: p50/p95 por operacion, error rate y truncation rate con ventana configurable
- Acciones seguras soportadas:
  - `refresh`
  - `warm workspace`
  - `open logs`
  - `copy CLI command`

## Endpoints admin locales

- `GET /`
- `GET /api/status?window=<recent|7d|30d|90d>`
- `GET /api/workspaces?window=<recent|7d|30d|90d>`
- `GET /api/workspaces/{workspace}?window=<recent|7d|30d|90d>`
- `POST /api/workspaces/{workspace}/warm`
- `GET /api/accesses?window=<recent|7d|30d|90d>`
- `GET /api/logs?tail=<n>`
- `GET /api/metrics?window=<recent|7d|30d|90d>` — computa p50/p95, error rate y truncation rate por operacion/workspace/cliente desde `access_events`; mantiene compatibilidad legacy con `days=<n>`
- `mi-lsp admin export --summary` reutiliza la misma ventana y agrega breakdowns opcionales por route/client/hint/failure-stage; el raw export prioriza CLI antes que UI para debugging operativo y es la primera superficie donde deben aparecer los campos causales nuevos.
- `GET /api/status` tambien expone `daemon_process` y `watchers`, equivalentes a `daemon status`, para validar presupuestos de agentes sin inspeccion externa.

## Dependencias e interacciones

- CLI publica
- named pipe en Windows / unix socket en Linux
- `~/.mi-lsp/daemon/state.json`
- `~/.mi-lsp/daemon/daemon.db`
- `{repoRoot}/.mi-lsp/daemon.log`
- worker Roslyn
- `tsserver` opcional

## Failure modes y notas operativas

| Riesgo | Sintoma | Mitigacion canonica |
|---|---|---|
| Doble daemon | dos `start` simultaneos | lock + health recheck |
| Auto-start concurrente | N agentes arrancan N procesos | `EnsureDaemon` comparte lock y no confia en state sin health |
| Explosion de handles por watchers | aliases duplicados o registry grande | `watch_mode=lazy`, dedupe por root canonico, cap LRU |
| Saturacion de requests | latencia/memoria crecen sin limite | backpressure `daemon/backpressure_busy` y env `MI_LSP_DAEMON_MAX_INFLIGHT` |
| Socket/pipe huerfano | connect falla pero state existe | purge + restart controlado |
| RAM excesiva | demasiados runtimes vivos | `max_workers` + LRU eviction |
| UI duplicada | cada cliente abre una vista separada | admin URL unica + deep link por query |
| Cliente antiguo | errores sutiles de protocolo | handshake con version explicita |
| Warm fallido | runtime no queda disponible | warning visible + logs locales |
| `tsserver` frio roto | cada request vuelve a intentar bootstrap y falla | cooldown corto + degradacion sticky a catalog/text |

## Related docs

- [DB-STATE-Y-TELEMETRIA.md](../08_db/DB-STATE-Y-TELEMETRIA.md)
- [CT-CLI-DAEMON-ADMIN.md](../09_contratos/CT-CLI-DAEMON-ADMIN.md)
- [CT-DAEMON-WORKER.md](../09_contratos/CT-DAEMON-WORKER.md)
