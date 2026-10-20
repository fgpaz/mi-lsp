# RF-QRY-010 - Responder preguntas docs-first guiadas por wiki y relacionarlas con evidencia de codigo

```yaml
harness_protocol: SDD-HARNESS-v1
id: "RF-QRY-010"
kind: "support-doc"
audience: "llm-first"
imports:
  - '[[00_gobierno_documental]]'
  - '[[RF-QRY-010]]'
exports:
  - 'RF-QRY-010'
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/04_RF/RF-QRY-010.md
agent_may_edit:
  - .docs/wiki/04_RF/RF-QRY-010.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
evidence:
  - .docs/wiki/04_RF/RF-QRY-010.md
```

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
3. El core normaliza de forma conservadora las preguntas que piden "anclas" y mezclan meta-terminos SDD (`RS`, `RF`, `FL`, `CT`, `TECH`, `DB`, `TP`) para tratarlos como formato/capa esperada, no como dominio de ranking.
4. El core rankea documentos canonicos por familia e intensidad de match usando el scorer owner-aware compartido por `nav route`, `nav ask`, `nav pack` y `nav.intent`.
5. El core elige un documento primario y evidencia documental de soporte.
6. El core deriva evidencia de codigo desde menciones explicitas o fallback textual, filtrando antes de emitir snippets los paths operacionales `.mi-lsp/**`, artefactos documentales `.docs/**` y archivos binarios/sidecars (`.db`, `.sqlite`, `.exe`, `.dll`, etc.).
7. Devuelve un envelope con `summary`, `primary_doc`, `doc_evidence`, `code_evidence`, `why` y `next_queries`.
8. Cuando la respuesta cae a fallback textual, queda con evidencia fina, normaliza meta-terminos de anclas o deja un next step muy fuerte, el envelope puede agregar `coach` query-level con `trigger`, `message`, `confidence` y `actions`.
9. Puede agregar `continuation` para dejar una siguiente busqueda estructurada (`expand_preview`, `low_evidence`, `follow_doc`) y `memory_pointer` para reentrada wiki-aware cuando existe snapshot repo-local util.
10. En AXI preview efectivo, conserva el mismo contrato explainable pero puede condensar `doc_evidence`/`code_evidence` y delegar la expansion a `--full`; ese recorte puede anunciarse tambien via `coach.trigger=preview_trimmed`.

## 4. Typed Errors

| Codigo | Causa | Trigger | Respuesta esperada |
|---|---|---|---|
| `QRY_ASK_QUESTION_REQUIRED` | falta pregunta | argumento vacio | abortar con error explicito |
| `QRY_ASK_WORKSPACE_NOT_FOUND` | workspace invalido | alias/path no resoluble | abortar con `ok=false` |
| `QRY_ASK_DOC_INDEX_UNAVAILABLE` | store repo-local no accesible | `index.db` no abrible | abortar con error explicito |

## 5. Special Cases and Variants

- Si no hay documentos indexados **y existe wiki canonica**, `nav ask` usa el fallback Tier 1 del route core para resolver un anchor canonico desde governance/read-model. No cae a README.md (ver RF-QRY-015).
- El scorer owner-aware aplica FTS, overlap lexico, `doc_id`, stem/path, penalizacion a `generic/README` y a artefactos de soporte en `.docs/raw/` cuando ya existe un candidato canonico positivo, y `owner_hints` opcionales proyectados desde `00_gobierno_documental.md`.
- Las preguntas de inventario de anclas que mezclan meta-terminos SDD (`RS`, `RF`, `FL`, `CT`, `TECH`, `DB`, `TP`) los tratan como intencion de salida/capa. La query de ranking conserva los terminos de dominio y puede emitir `coach.trigger=anchor_drift` con `continuation.next.op=nav.pack` para que el agente confirme el pack canonico.
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
- `code_evidence` nunca debe usar estado operacional repo-local ni artefactos documentales como prueba de codigo. Si una mencion documental o fallback textual apunta a `.mi-lsp/index.db`, sidecars SQLite, binarios, `.docs/raw/**`, `.docs/auditoria/**`, matrices/indices wiki o cualquier `.docs/**`, esa evidencia se descarta y se continua con la siguiente fuente segura.

## 6. Data Model Impact

- `DocRecord`
- `DocEdge`
- `DocMention`
- `DocsReadProfile`
- `AskResult`
- `QueryEnvelope`
- `QueryOptions`

## 7. Expanded Acceptance Criteria (Gherkin)

```gherkin
Scenario: Filtrar evidencia operacional y binaria antes de nav ask
  Given un documento canonico menciona ".mi-lsp/index.db", un sidecar ".sqlite" y un archivo fuente real
  When ejecuto "nav ask" o su constructor de evidencia
  Then "code_evidence" incluye el archivo fuente real
  And "code_evidence" no incluye ".mi-lsp/**"
  And "code_evidence" no incluye extensiones binarias como ".db" o ".sqlite"
  And "code_evidence" no incluye artefactos ".docs/**"; estos solo pueden aparecer como "doc_evidence"
```

```gherkin
Scenario: Normalizar meta-terminos SDD en preguntas de anclas
  Given una pregunta "Que anclas RS RF FL CT TECH aplican a validar datos de entrada de un workflow con servicios configurados"
  When ejecuto "nav ask"
  Then el ranking usa los terminos de dominio "validar datos entrada workflow servicios configurados"
  And no usa "RS RF FL CT TECH anclas" como dominio de ranking
  And si el guard detecta riesgo de drift emite "coach.trigger=anchor_drift"
  And la continuacion apunta a "nav.pack" para confirmar el reading pack canonico
```

## 8. Test Traceability

- Positivo: `TP-QRY / TC-QRY-032`
- Positivo: `TP-QRY / TC-QRY-039`
- Positivo: `TP-QRY / TC-QRY-107`
- Negativo: `TP-QRY / TC-QRY-034`
