# RF-QRY-010 - Responder preguntas docs-first guiadas por wiki y relacionarlas con evidencia de codigo

## 1. Execution Sheet

| Campo | Valor |
|---|---|
| ID | RF-QRY-010 |
| Titulo | Responder preguntas docs-first guiadas por wiki y relacionarlas con evidencia de codigo |
| Actores | Usuario, Skill, Agente, CLI/Core |
| Prioridad | alta |
| Severidad | alta |
| FL origen | FL-QRY-01 |

## 2. Detailed Preconditions

| Condicion | Tipo | Estado requerido |
|---|---|---|
| Workspace resoluble | funcional | obligatorio |
| Indice repo-local disponible o construible | tecnica | obligatorio |
| Corpus documental del repo accesible | funcional | obligatorio |

## 3. Process Steps (Happy Path)

1. La CLI recibe `mi-lsp nav ask <question>`.
2. El core resuelve el workspace y carga el `read-model` del proyecto o el default embebido.
3. El core rankea documentos canonicos por familia e intensidad de match usando el scorer owner-aware compartido por `nav route`, `nav ask`, `nav pack` y `nav.intent`.
4. El core elige un documento primario y evidencia documental de soporte.
5. El core deriva evidencia de codigo desde menciones explicitas o fallback textual.
6. Devuelve un envelope con `summary`, `primary_doc`, `doc_evidence`, `code_evidence`, `why` y `next_queries`.
7. Cuando la respuesta cae a fallback textual, queda con evidencia fina o deja un next step muy fuerte, el envelope puede agregar `coach` query-level con `trigger`, `message`, `confidence` y `actions`.
8. Puede agregar `continuation` para dejar una siguiente busqueda estructurada (`expand_preview`, `low_evidence`, `follow_doc`) y `memory_pointer` para reentrada wiki-aware cuando existe snapshot repo-local util.
9. En AXI preview efectivo, conserva el mismo contrato explainable pero puede condensar `doc_evidence`/`code_evidence` y delegar la expansion a `--full`; ese recorte puede anunciarse tambien via `coach.trigger=preview_trimmed`.

## 4. Typed Errors

| Codigo | Causa | Trigger | Respuesta esperada |
|---|---|---|---|
| `QRY_ASK_QUESTION_REQUIRED` | falta pregunta | argumento vacio | abortar con error explicito |
| `QRY_ASK_WORKSPACE_NOT_FOUND` | workspace invalido | alias/path no resoluble | abortar con `ok=false` |
| `QRY_ASK_DOC_INDEX_UNAVAILABLE` | store repo-local no accesible | `index.db` no abrible | abortar con error explicito |

## 5. Special Cases and Variants

- Si no hay documentos indexados **y existe wiki canonica**, `nav ask` usa el fallback Tier 1 del route core para resolver un anchor canonico desde governance/read-model. No cae a README.md (ver RF-QRY-015).
- El scorer owner-aware aplica FTS, overlap lexico, `doc_id`, stem/path, penalizacion a `generic/README` y a artefactos de soporte en `.docs/raw/` cuando ya existe un candidato canonico positivo, y `owner_hints` opcionales proyectados desde `00_gobierno_documental.md`.
- La recencia documental solo opera como `weak tie-break` y nunca rescata un doc irrelevante ni sobreescribe un match canonico fuerte.
- Si la respuesta docs-first depende de texto fallback o queda con evidencia debil, `coach.confidence` debe bajar a `low` y sugerir un rerun/refinement concreto sin reemplazar `next_queries`.
- Si la respuesta docs-first queda con evidencia debil, `continuation.reason=low_evidence` debe sugerir una siguiente busqueda estructurada sin transportar command strings raw.
- Si no hay documentos indexados **y no existe wiki canonica**, el sistema degrada a evidencia textual del workspace con warning.
- Si existe `.docs/wiki/_mi-lsp/read-model.toml`, ese archivo manda sobre el default embebido.
- El codigo no rankea por delante de la wiki; el codigo se usa como evidencia/verificacion.
- En repos sin `.docs/wiki`, el sistema cae a fallback generico sobre `README*`, `docs/` y `.docs/`.
- En workspaces `container`, si la evidencia de codigo converge en un repo hijo unico, `next_queries` debe sugerir reruns con `--repo` para mantener el scope directo.
- `nav ask` solo entra en AXI por default cuando la pregunta es claramente de onboarding/orientacion; preguntas con doc IDs, paths, simbolos o lenguaje de implementacion deben quedar clasicas salvo `--axi`.
- En superficies AXI-default, `next_queries` no deben arrastrar `--axi` de forma redundante; la expansion mas profunda vive en `next_hint` hacia `--full`.

## 6. Data Model Impact

- `DocRecord`
- `DocEdge`
- `DocMention`
- `DocsReadProfile`
- `AskResult`
- `QueryEnvelope`
- `QueryOptions`
