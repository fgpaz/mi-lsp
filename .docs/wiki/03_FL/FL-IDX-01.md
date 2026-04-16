# FL-IDX-01

## 1. Goal

Construir o refrescar el indice repo-local del workspace de manera incremental, determinista y con ownership por repo, incluyendo el corpus documental necesario para `nav ask`.

## 2. Scope in/out

- In: scan de archivos, respeto de ignores por defaults + `.gitignore` + `.milspignore` + `project.toml` honrando orden y re-includes negados, escritura en SQLite, ownership por `repo_id`, actualizacion por `content_hash`, indexacion de docs `.md`, warnings de ruido evidente.
- Out: persistencia semantica completa de refs y jerarquias C#.

## 3. Main sequence

```mermaid
sequenceDiagram
    participant U as Usuario/Skill
    participant C as Core
    participant W as Walker
    participant DB as SQLite

    U->>C: index [workspace]
    C->>W: enumera archivos validos
    W-->>C: lista + ownership por repo
    C->>C: extrae catalogo TS/C# liviano
    C->>C: indexa docs y read-model del repo
    C->>DB: replace files/symbols/repos/entrypoints/meta
    C->>DB: replace doc_records/doc_edges/doc_mentions
    DB-->>C: ok
    C-->>U: stats del indice + warnings
```

## 4. Alternative/error path

| Caso | Resultado |
|---|---|
| Archivo ignorado | se omite silenciosamente |
| Archivo fuera de repos detectados | warning y asignacion al root del workspace |
| Indice contaminado por ruido | warning con sugerencia concreta para `.milspignore` |
| Cambio en `.docs/wiki`, `README*`, `docs/` o `read-model.toml` | el incremental degrada a full re-index |
| `index.db` previo sin docs canonicos aunque la wiki existe | `index` no responde `no changes detected`: degrada a full re-index para autocurar `doc_records` |
| `--clean` activo | se recompone el indice desde cero |

## 5. Data touchpoints

- `.mi-lsp/index.db`
- tablas `workspace_repos`, `workspace_entrypoints`, `symbols`, `files`, `doc_records`, `doc_edges`, `doc_mentions`, `workspace_meta`
- estados: sin indice, indice actualizado, indice con warnings de ruido

## 6. Candidate RF references

- RF-IDX-001 indexacion inicial y refresh del catalogo repo-local con ownership por repo y docs wiki-aware
- RF-IDX-002 incremental git-aware con fallback a full re-index cuando cambian docs o perfil de lectura
