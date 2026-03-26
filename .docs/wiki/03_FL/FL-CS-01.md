# FL-CS-01

## 1. Goal

Resolver una consulta semantica profunda C# usando un worker Roslyn enrutable al repo hijo y entrypoint correctos.

## 2. Scope in/out

- In: `find_refs`, `get_context`, `get_overview`, `get_deps`, selectors `--repo`, `--entrypoint`, `--solution`, `--project`, runtime pool por entrypoint.
- Out: fanout semantico automatico sobre todos los repos hijos y edicion semantica.

## 3. Main sequence

```mermaid
sequenceDiagram
    participant C as Core
    participant D as Daemon
    participant W as Worker .NET
    participant R as Roslyn

    C->>C: resuelve repo/entrypoint
    C->>C: arma slice legible si la query es get_context
    C->>D: semantic query + target resuelto
    D->>W: request length-prefixed
    W->>R: load/use entrypoint
    R-->>W: refs/context/deps derivados
    W-->>D: response compacta
    D-->>C: envelope backend=roslyn + overlay semantico
```

## 4. Alternative/error path

| Caso | Resultado |
|---|---|
| Worker no instalado | error accionable `mi-lsp worker install` |
| Query ambigua en workspace `container` | `backend=router`, candidatos y `next_hint` |
| `--repo` o `--entrypoint` explicito | bypass de heuristica y routing directo |
| Daemon ausente | el core puede ejecutar el mismo flujo en modo directo |
| Primer candidato Roslyn falla por bootstrap | retry unico con el siguiente candidato `bundle -> installed -> dev-local`; si no recupera, error accionable |
| Roslyn no responde en `get_context` | el core conserva `slice_text` y degrada a `catalog` o `text` con warning |

## 5. Data touchpoints

- request/response stdio
- workspace Roslyn cargado por `entrypoint_path`
- estados: `worker hot`, `worker cold`, `ambiguous`
- slice textual local construido por el core

## 6. Candidate RF references

- RF-CS-001 consulta semantica C# via Roslyn con routing por repo/entrypoint
