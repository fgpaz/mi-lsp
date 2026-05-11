# TECH-WIKI-FANOUT

```yaml
harness_protocol: SDD-HARNESS-v1
id: "TECH-WIKI-FANOUT"
kind: "tech-spec"
audience: "llm-first"
imports:
  - '[[FL-WIKI-01]]'
exports:
  - '[[CT-NAV-WIKI]]'
agent_must_read:
  - .docs/wiki/03_FL/FL-WIKI-01.md
  - .docs/wiki/07_tech/TECH-WIKI-FANOUT.md
agent_may_edit:
  - .docs/wiki/07_tech/TECH-WIKI-FANOUT.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --paths .docs/wiki/07_tech/TECH-WIKI-FANOUT.md --format toon
  - mi-lsp nav wiki validate-source --workspace mi-lsp --paths .docs/wiki/07_tech/TECH-WIKI-FANOUT.md --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
evidence:
  - .docs/wiki/07_tech/TECH-WIKI-FANOUT.md
  - internal/nav/fanout_wiki.go
  - internal/service/ask.go:465-564
```

Volver a [07_baseline_tecnica.md](../07_baseline_tecnica.md).

## Proposito

Documentar la arquitectura tecnica del fan-out wiki global (`nav ask/search/find --all-workspaces`) que implementa el patrón de paralelismo acotado, manejo de fallos graceful y merge determinista sobre multiples workspaces registrados.

## Owner y scope

- Owner: Compound commands (fan-out orchestration)
- Scope: arquitectura del fan-out, semáforo, timeout por workspace, envolventes modificados, scoring y reuso de patrones
- Non-goals: contratos CLI exactos (eso es CT), test cases (eso es TP), host remoto o SSH/Tailscale (eso es Hermes wrapper), cache distribuido

## Arquitectura tecnica del fan-out wiki

```block_id: tech-wiki-fanout-architecture
type: architecture
```

El fan-out wiki implementa un patrón de paralelismo bounded sobre multiples workspaces con las siguientes caracteristicas:

1. **Inicio del fan-out**:
   - `nav ask/search/find --all-workspaces` arranca un pipeline que itera sobre el resultado de `workspace.ListWorkspaces()`.
   - El flag `--all-workspaces` fuerza el modo fan-out; ausente, el comando opera sobre un unico workspace (el resuelto por `--workspace` o `cwd`).

2. **Semaforo y concurrencia acotada**:
   - Un canal buffered `semaphore := make(chan struct{}, maxConcurrent)` controla el max de goroutines activas.
   - `maxConcurrent = 4` es el valor canonical hardcoded; heredable via env `MI_LSP_FANOUT_MAX_CONCURRENT`.
   - Cada workspace goroutine adquiere el semaforo al entrar y lo libera al terminar; no hay wait infinito.

3. **Dispatch por workspace**:
   - Para cada workspace registrado se crea una goroutine independiente que:
     - Clona el `CommandRequest` original.
     - Inyecta `subRequest.Context.Workspace = wsReg.Name`.
     - Remueve `all_workspaces` del payload para evitar recursion.
     - Ejecuta la query original (`ask`, `search` o `find`) contra ese workspace.
     - Envia resultado (envelope o error) a un canal `results`.

4. **Timeout por workspace**:
   - Cada query individual hereda el timeout del contexto padre (default 30s en `nav ask`).
   - No hay timeout separado por workspace; si uno se cuelga, el semaforo lo protege de bloquear otros.
   - El `context.Context` pasado al dispatch permite cancelacion global si es necesario.

5. **Acumulacion de resultados**:
   - Los resultados llegan desordenados por completion order al canal `results`.
   - Se itera el canal y se acumulan en una slice `scored[]` junto con metadata: `result`, `score`, `wsName`.
   - Fallos (errores de query) generan warnings acumulados; el fan-out no aborta.

## Politica de manejo de fallos

```block_id: tech-wiki-fanout-failure-policy
type: failure-modes
```

- **Workspace que falla**: La query de ese workspace retorna error, se agrega un warning al acumulador y se continua con los demas.
- **No hay aborto global**: El fan-out siempre retorna `ok=true` con stats parciales, incluso si todos los workspaces fallaron.
- **Warnings acumulados**: Cada error se transforma en un string de warning con formato `"{wsName}: <operacion> failed: <error>"`.
- **Stats parciales**: El envelope retornado expone `stats.workspaces_queried` (total) y `stats.workspaces_failed[]` (IDs de espacios que fallaron).
- **Workspace stale**: Aliases con root inexistente se ignoran silenciosamente; el warning agregado apunta a `workspace prune --stale --dry-run`.

## Envolvente modificado para fan-out

```block_id: tech-wiki-fanout-envelope-shape
type: envelope-contract
```

Cuando `--all-workspaces=true`, el envelope `model.Envelope` expone campos adicionales:

```toon
model.Envelope:
  Ok: bool (siempre true en modo fan-out, incluso con fallos parciales)
  Items: []T (items globales, mergeados y ordenados)
  Warnings: []string (warnings locales + errores de fan-out)
  Stats:
    Workspaces_queried: int (total de workspaces iterados)
    Workspaces_failed: []string (IDs de espacios que tuvieron error)
    Truncated_per_workspace: map[string]int (items truncados por workspace si maxItems fue el limitador)
  Coach: optional (generado en cliente sobre resultado ganador del merge)
  Memory_pointer: optional (reentrada wiki respecto del workspace dominante)
  Continuation: optional (sugerencia de siguiente paso)
```

## Reuso de patron desde ask.go

```block_id: tech-wiki-fanout-reuse-pattern
type: implementation-reference
```

El fan-out wiki reutiliza directamente el patron implementado en `internal/service/ask.go:465-564`:

- Semaforo a traves de canal buffered (no mutex/condvar).
- Iteracion sin deadlock usando `sync.WaitGroup`.
- Acumulacion de resultados en canal no buffered que se cierra despues del `Wait()`.
- Scoring local de cada resultado (en ask.go: `score = len(DocEvidence)*10 + len(CodeEvidence)*5 + ...`).
- Sort global por score descendente, tie-break por workspace name.
- Limite final con `if i >= maxItems { break }`.

El nuevo helper vive en `internal/nav/fanout_wiki.go` y entra en el mismo camino directo (no daemon) que las operaciones de bajo costo (`nav.find`, `nav.search`).

## Scoring y merge determinista

```block_id: tech-wiki-fanout-scoring
type: scoring-policy
```

- **Score por item**: Cada item retornado por una query individual conserva su score local nativo del FTS5/BM25 del workspace.
- **Merge global**: Una vez acumulados todos los items en memoria, se re-sortem por `(score DESC, workspace ASC, doc_id ASC)`.
- **Determinismo**: El tie-break secundario por `workspace` (orden alfabetico) y terciario por `doc_id` aseguran que dos ejecuciones identicas de la misma query retornan items en el mismo orden, incluso con scores identicos.
- **Offset de workspace**: Cuando score es igual entre dos items de workspaces distintos, el workspace "alfabeticamente menor" gana; esto favorece results consistentes en el reporte.

## Non-goals

```block_id: tech-wiki-fanout-non-goals
type: scope-boundary
```

- **Multi-maquina**: `mi-lsp` no sabe de hosts remotos, SSH o Tailscale; el fan-out solo itera workspaces registrados localmente.
- **Cache distribuido**: No hay estado distribuido entre instancias; cada workspace query es independiente.
- **Orquestacion de Hermes**: El wrapper Hermes que coordina fan-out entre multiples maquinas es un concern separado fuera de este documento.
- **Contratos CLI**: Los flags exactos, formatos de salida y handshake viven en `[[CT-NAV-WIKI]]`.
- **Test cases**: Los procedimientos QA de fan-out viven en `[[TP-WIKI]]`.

## Invariantes

- El fan-out nunca hace polling activo; usa canales y `WaitGroup` para coordinacion pasiva.
- Cada query es independiente: el fallo de una no contagia a otra.
- El timeout operativo (default 30s) se respeta globalmente; si uno tarda 30s, los demas en paralelo continuan.
- El indice repo-local se consulta por workspace; no hay fusion de indices.
- El scoring final es determinista y reproducible.

## Relacionados

- [FL-WIKI-01.md](../../03_FL/FL-WIKI-01.md) — flujo base del wiki navigator
- [CT-NAV-WIKI.md](../09_contratos/CT-NAV-WIKI.md) — contratos CLI y envelopes
- [TECH-DAEMON-GOBERNANZA.md](TECH-DAEMON-GOBERNANZA.md) — topologia del daemon (fan-out es directo, no daemon)
