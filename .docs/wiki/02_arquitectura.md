# 2. Arquitectura

## Project Decision Priority

1. Confiabilidad
2. Performance
3. Memoria
4. Portabilidad
5. DX

## Vista del sistema

```mermaid
flowchart LR
    U[CLI mi-lsp] -->|daemon activo| D[Daemon global Go]
    U -->|sin daemon o bypass| C[Core Go directo]
    U --> I[Shortcut init]
    I --> C
    D --> C
    D --> UI[Governance UI loopback]
    C --> S[SQLite repo-local]
    C --> G[Registry global minimo]
    C --> X[Discovery TS/JS + ripgrep]
    C --> DG[Docgraph + read-model]
    C --> SV[Service exploration profile]
    D --> P[Runtime pool por workspace/backend/entrypoint]
    P --> R[Worker Roslyn]
    P --> T[tsserver opcional]
    P --> PY[Pyright opcional]
    D --> DS[~/.mi-lsp/daemon state + db]
    D --> DL[{repoRoot}/.mi-lsp/daemon.log]
```

## Modelo canonico

- `Workspace single`: un repo con un root semantico obvio. Ejemplo: `gastos`.
- `Workspace container`: carpeta padre con muchos repos independientes. Ejemplo: `interbancarizacion_coelsa` sin depender de una `.sln` agregadora.
- El `registry.toml` global sigue siendo liviano: alias, root, languages y `kind`.
- La topologia detallada vive en `<repo>/.mi-lsp/project.toml` con `repo[]`, `entrypoint[]`, `default_repo` y `default_entrypoint`.
- El contexto de query incluye `caller_cwd`; si `--workspace` se omite, la resolucion efectiva sigue `selector explicito > workspace registrado cuyo root contiene caller_cwd > last_workspace`.
- El indice repo-local persiste ownership por repo (`repo_id`, `repo`) para archivos y simbolos.
- El mismo indice repo-local persiste `DocRecord`, `DocEdge` y `DocMention` para `nav wiki`, `nav ask`, `nav pack`, `nav route` y `nav trace`.
- `workspace_meta` tambien puede persistir un snapshot repo-local de reentrada (`memory_snapshot_*`) construido en `index` a partir de la wiki canonica y del handoff reciente.
- El runtime pool del daemon se keyed por `(workspace_root, backend_type, entrypoint_id)`.
- `nav wiki search`, `nav ask`, `nav pack` y `nav route` comparten un contexto request-local de docs/profile/ranking para evitar recomputacion dentro de una misma request.
- El envelope de query puede agregar un bloque opcional `coach` query-level para guidance explicito cuando hay rerun/refinement claro.
- El mismo envelope puede agregar `continuation` y `memory_pointer` con costo bajo para orientar al harness en la siguiente busqueda o reentrada.
- El doc router comparte un scorer owner-aware para `nav wiki search`, `nav route`, `nav ask` y `nav pack`, y `nav.intent` clasifica primero `mode=docs|code` antes de elegir entre docs canonicos o BM25 de simbolos.
- `nav ask` rankea docs primero y usa el codigo como evidencia; `nav pack` usa la misma base para construir reading packs global -> especifico; `nav service` agrega evidencia scoped a un path usando catalogo y busqueda textual.

## Responsabilidades por modulo

| Modulo | Responsabilidad |
|---|---|
| CLI | Parseo de comandos, flags globales, selectors semanticos y shortcut `init` |
| Daemon global Go | Routing, health, telemetry, governance UI y sharing entre clientes |
| Governance UI | Consola workspace-first con visibilidad de `kind`, repos y entrypoints |
| Core Go | Discovery de workspace, indexacion repo-local, routing semantico, truncacion, `nav wiki`, `nav ask`, `nav pack` y service exploration |
| Docgraph/read-model | Clasificar preguntas/tareas, priorizar documentos canonicos y conectar docs con codigo |
| Service exploration profile | Agregar evidencia observable por path: endpoints, consumers, publishers, entidades e infraestructura |
| Runtime pool | Mantener un runtime vivo por entrypoint semantico con LRU |
| Worker .NET | Semantica profunda C# con Roslyn |
| Pyright worker | Semantica profunda Python via `pyright-langserver` (LSP generico) |
| TS discovery | Discovery TS/Next basico y busqueda textual; incluye fallback nativo en Go (`searchPatternGo`) cuando `rg` no esta disponible, respetando `.milspignore` y filtrando binarios |
| SQLite repo-local | Catalogo liviano, ownership por repo y grafo documental |

## Reglas de routing

1. `find/search/overview/symbols/intent` operan sobre el catalogo del workspace completo; en workspaces `container`, `find/search/intent` pueden acotar a un repo con `--repo` sin pasar por el routing semantico.
2. `service` combina catalogo repo-local y busqueda textual scoped al path pedido.
3. `refs/context/deps` resuelven un `semantic entrypoint` antes de tocar Roslyn.
4. `nav wiki search` consulta `doc_records` con filtros documentales; `ask` primero consulta `doc_records/doc_edges/doc_mentions`; si no hay corpus fuerte, degrada a texto.
5. Cuando `workspace` llega vacio, el core intenta resolverlo contra el `caller_cwd` real del proceso invocador; si varios aliases comparten root, prioriza `project.name`, luego basename del root, luego `last_workspace` solo como desempate de ese mismo root.
6. `find/search/intent` aceptan `--repo` con resolucion smart: exacto primero, luego prefix/fuzzy deterministico cuando el match es unico, y error accionable con candidatos concretos cuando no lo es.
5. Orden de resolucion semantica:
   - `--entrypoint`
   - `--solution` / `--project`
   - `--repo`
   - ownership por `--file`
   - match unico por catalogo
   - default del workspace si es `single`
7. Si la consulta es ambigua en un workspace `container`, el sistema falla con `backend=router`, candidatos concretos y `next_hint`.
8. Si `workspace` se omitio y no hubo match por `caller_cwd`, el sistema puede caer al `last_workspace`, pero debe dejar warning visible con el alias elegido.
9. `workspace list` usa summary-first desde registry + `project.toml`; la reconstruccion pesada de topologia queda para `workspace status`.
10. `nav.workspace-map` hace scan profundo solo bajo expansion explicita (`--full` / AXI full); el modo base devuelve un mapa corto y rapido.
11. No hay fanout semantico automatico sobre todos los repos hijos en v1.3.

## Implicancias operativas

- `mi-lsp init` es el happy path corto para dejar un workspace listo para uso manual o por agentes.
- `gastos` valido el modelo `single`: el detector prioriza `backend/Gastos.sln` y evita `.worktrees/`.
- `interbancarizacion_coelsa` valido el modelo `container`: discovery global en la carpeta padre y semantica correcta al rerun con `--repo`.
- `nav ask` reduce round-trips de onboarding cuando existe una wiki canonica util.
- `nav pack` reduce tokens al devolver el orden de lectura canonico y solo materializar slices bajo `--full`.
- `nav wiki search` reduce fallos de agentes al hacer explicita la escalera RF/FL/TP/CT/TECH/DB antes de pasar a `ask`.
- El governance panel debe exponer repo y entrypoint de cada runtime para distinguir warm state real.

## Insumos para FL

- `FL-BOOT-01`: alta de `single|container`, deteccion de repos hijos y entrypoints, y shortcut `init`.
- `FL-IDX-01`: indexacion de codigo + docs con ownership por repo y sugerencias de `.milspignore` cuando hay ruido.
- `FL-QRY-01`: queries compactas, `nav wiki` para exploracion documental, `nav ask` docs-first, `nav pack` para reading packs y `nav service` evidence-first.
- `FL-CS-01`: routing semantico por repo/entrypoint con error accionable ante ambiguedad.
- `FL-DAE-01`: runtimes y telemetria por entrypoint, visibles en la governance UI.
