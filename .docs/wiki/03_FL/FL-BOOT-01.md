# FL-BOOT-01

## 1. Goal

Registrar o inicializar un workspace `single` o `container` y dejar lista su topologia repo-local para indexacion, `nav ask` y consultas posteriores.

## 2. Scope in/out

- In: deteccion de root, alias opcional, clasificacion `single|container`, deteccion de repos hijos y `entrypoints`, creacion de `.mi-lsp/`, persistencia de `project.toml`, alta en registry global minimo, `init` como happy path corto.
- Out: descarga automatica de worker y setup remoto.

## 3. Main sequence

```mermaid
sequenceDiagram
    participant U as Usuario/Skill
    participant CLI as CLI
    participant C as Core
    participant R as Registry

    U->>CLI: workspace add|init <path>
    CLI->>C: detect workspace layout
    C->>C: clasifica single|container
    C->>C: detecta repos y entrypoints validos
    C->>C: crea .mi-lsp/project.toml
    C->>R: registra alias, root y kind
    C->>C: indexa por defecto salvo --no-index
    C-->>CLI: estado listo + next_steps
```

## 4. Alternative/error path

| Caso | Resultado |
|---|---|
| Path invalido | error explicito sin side effects |
| No se detecta stack compatible | warning + rechazo |
| Layout ambiguo | se persiste la topologia minima y se exponen defaults claros |
| Paths auxiliares (`.worktrees/`, ignores) | se omiten del bootstrap |
| Entrypoints bajo `.docs/` o `template(s)` | permanecen visibles en topologia, pero no deben quedar como default semantico si existe una opcion real del repo |
| Indexacion falla | registro exitoso con warning no fatal |

## 5. Data touchpoints

- Repo-local: `.mi-lsp/project.toml`
- Repo-local opcional: `.docs/wiki/_mi-lsp/read-model.toml`
- Global: `~/.mi-lsp/registry.toml`
- Estados: `detected`, `registered`, `single|container`

## 6. Candidate RF references

- RF-WKS-001 registrar workspace por path y alias, incluyendo topologia `single|container`
- RF-WKS-002 indexar automaticamente al registrar un workspace nuevo
- RF-WKS-003 inicializar el workspace actual y dejarlo listo para `nav ask`
