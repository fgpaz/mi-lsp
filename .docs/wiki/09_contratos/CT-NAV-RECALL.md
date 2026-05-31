# CT-NAV-RECALL

```yaml
harness_protocol: SDD-HARNESS-v1
id: "CT-NAV-RECALL"
kind: "support-doc"
audience: "llm-first"
imports:
  - '[[00_gobierno_documental]]'
  - '[[CT-NAV-RECALL]]'
exports:
  - 'CT-NAV-RECALL'
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/09_contratos/CT-NAV-RECALL.md
agent_may_edit:
  - .docs/wiki/09_contratos/CT-NAV-RECALL.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
evidence:
  - .docs/wiki/09_contratos/CT-NAV-RECALL.md
```

## Boundary

Usuario/agente -> CLI publica `mi-lsp nav recall`

## Forma de invocacion

```text
mi-lsp nav recall <query> [--workspace <alias>] [--max-items 10] [--token-budget 2000] [--format compact|json|text|toon] [--map]
```

La CLI acepta una consulta libre (query semantica) y produce un envelope `backend=recall` o `backend=recall+lexical` (fallback).

## Payload logico

- `query`: string requerido
- `workspace`: alias o path resoluble
- `max_items`: limite de resultados (default 10)
- `token_budget`: presupuesto de tokens aproximado (default 2000)
- `format`: salida estructura (default toon en AXI, compact classico)
- `map`: boolean, agregar mini-mapa de ubicacion documento si valor true

Cuando `workspace` se omite, el runtime resuelve primero el workspace registrado cuyo root contiene el `caller_cwd` real del invocador. Solo si no hay match puede caer a `last_workspace`, y ese caso debe quedar visible en `warnings`.

## Respuesta

Cada item de `backend=recall` o `backend=recall+lexical` contiene:
- `archivo`: ruta relativa al workspace
- `heading`: titulo o numero de seccion del chunk
- `score`: float [0, 1] de similitud de coseno normalizada
- `snippet`: fragmento de contexto de 2-3 lineas
- `start_line`: numero de linea en el archivo original

El envelope puede contener ademas:
- `coach.trigger`: cuando el backend cae a fallback automatico
- `coach.message`: guidance sobre la degradacion
- `continuation.reason`: cuando hay siguiente paso recomendado
- `hint`: cuando embeddings no estan configurados o API falla sin fallback lexical util

## Gating y prerequisitos

- **Ungated**: no requiere `governance_blocked=false` ni `docs_index_ready=true`
- Hot path directo: no auto-inicia daemon
- Si embeddings no estan listos, cae inmediatamente a FTS/ripgrep
- Si embeddings estan configurados pero API falla por transient, reintenta exponencial breve (3x) antes de fallback
- Si API agota timeout o falla permanentemente, transicion a lexical con `coach.trigger=backend_runtime_fallback`

## Backends de busqueda

- `recall`: vector similarity puro cuando embeddings estan listos
- `recall+lexical`: cosine + FTS/ripgrep cuando fallback activo
- Degradacion automatica: usuario no necesita cambiar comando

## Semantica observable

- `score` de embeddings siempre [0, 1] post-normalizacion
- Score de FTS (fallback lexical) tambien normalizado [0, 1] para uniformidad
- Ranking determinista: dentro de backend, por score descendente
- Top-k = `min(max_items, presupuesto_token / tokens_por_item)`
- Cuando `token_budget` agota, truncar con `truncated=true` y `next_hint` accionable

## Warnings esperables

- `embeddings_unconfigured` — `[embeddings]` no esta seteado en `project.toml`; operacion cae a lexical
- `embeddings_unavailable` — API fallo pero fallback lexical disponible
- `api_timeout` — timeout en embedding API; reintentando fallback
- `search_fallback` — documentado porque FTS/ripgrep se usa por defecto offline
- `workspace omitted; multiple registry aliases share root ...`
- `workspace omitted; no registered workspace matched caller cwd ...; falling back to last_workspace=...`

## Errores

- query vacia -> error explicito
- workspace no resoluble -> error explicito
- `index.db` no accesible -> error explicito
- API key invalida o endpoint malformado -> warning + fallback lexical (no error duro)

## Relacion con otros comandos

`nav recall` es complementario a:
- `nav ask`: respuesta docs-first con reasoning; `nav recall` es pure vector similarity
- `nav search`: busqueda textual; `nav recall` es semantica enriquecida cuando embeddings listos
- `nav wiki search`: busqueda dentro de docs gobernados; `nav recall` idem pero vectorial
- `nav route`: routing de tarea canonico; `nav recall` es busqueda libre sin ancla RF

## Envelope structure

```json
{
  "ok": true,
  "backend": "recall",
  "workspace": "alias",
  "items": [
    {
      "archivo": ".docs/wiki/07_baseline_tecnica.md",
      "heading": "## Decisiones e invariantes",
      "score": 0.87,
      "snippet": "Existe un unico daemon por usuario/host; ...",
      "start_line": 92
    }
  ],
  "warnings": [],
  "coach": {
    "trigger": null
  },
  "stats": {"items": 3, "backend_time_ms": 145},
  "truncated": false
}
```

Si embeddings no estan configurados:

```json
{
  "ok": true,
  "backend": "recall+lexical",
  "workspace": "alias",
  "items": [...],
  "warnings": ["embeddings_unconfigured"],
  "coach": {
    "trigger": "embeddings_unconfigured",
    "message": "Semantic recall disabled; using lexical search. Configure [embeddings] in .mi-lsp/project.toml to enable vectors.",
    "confidence": "low",
    "actions": [
      {
        "kind": "configure",
        "label": "Set up embeddings",
        "command": "# Edit .mi-lsp/project.toml [embeddings]"
      }
    ]
  },
  "hint": "configure embeddings for semantic search",
  "stats": {"items": 5, "backend_time_ms": 234},
  "truncated": false
}
```

## Profile-driven selection

- `embeddings.profile = "knowledge-wiki"`: ranking docs-first, penaliza generic README
- `embeddings.profile = "spec-driven"`: penaliza hits textuales puros, prioriza docs gobernados RF/CT
- Selection influencia en token budget y scoring

## Operaciones adicionales

`mi-lsp workspace status` debe exponer en su envelope:
- `embeddings_enabled: true|false`
- `embeddings_profile`: "knowledge-wiki" o "spec-driven" (o `null` si unconfigured)
- `embeddings_model`: nombre del modelo actual si enabled
- Si embeddings esta en estado `unconfigured` o `offline`, incluir hint accionable
