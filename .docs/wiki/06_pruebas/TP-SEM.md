# TP-SEM

```yaml
harness_protocol: SDD-HARNESS-v1
id: "TP-SEM"
kind: "support-doc"
audience: "llm-first"
imports:
  - '[[00_gobierno_documental]]'
  - '[[RF-SEM-001]]'
  - '[[RF-SEM-002]]'
  - '[[RF-SEM-003]]'
exports:
  - 'TP-SEM'
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/06_pruebas/TP-SEM.md
agent_may_edit:
  - .docs/wiki/06_pruebas/TP-SEM.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
evidence:
  - .docs/wiki/06_pruebas/TP-SEM.md
```

## Cobertura objetivo

- RF-SEM-001
- RF-SEM-002
- RF-SEM-003

## Casos

| Caso | Tipo | RF | Descripcion |
|---|---|---|---|
| TC-SEM-001 | positivo | RF-SEM-001 | acepta config `[embeddings]` completa con `provider`, `base_url`, `model`, `dim`, `api_key_env`, `profile`, `batch_size`, `timeout_ms`, `encoding_format` y `user_agent` |
| TC-SEM-002 | positivo | RF-SEM-001 | envia payload OpenAI-compatible con `encoding_format = "float"`, `Accept`, `User-Agent` y valida dimension estrictamente contra `dim` |
| TC-SEM-003 | negativo | RF-SEM-001 | si Nan/key/provider/config falla, no imprime secretos y recomienda `nav wiki search` sin fallback BGE oculto |
| TC-SEM-004 | positivo | RF-SEM-002 | reindexa cuando cambia metadata-prefix, texto enriquecido, content hash, modelo o dimension |
| TC-SEM-005 | positivo | RF-SEM-002 | persiste `wiki_chunk_embeddings` con `doc_path`, `chunk_id`, rango de lineas, heading, snippet, `embedding_model`, `embedding_dim` y BLOB float32 |
| TC-SEM-006 | negativo | RF-SEM-002 | rechaza reutilizar vectores stale cuando el hash, modelo, dimension o BLOB faltante no coincide |
| TC-SEM-007 | positivo | RF-SEM-003 | `nav recall --intent formula` devuelve `RecallResult.intent` y prioriza reglas, contratos y definiciones |
| TC-SEM-008 | positivo | RF-SEM-003 | `nav recall --intent route` descubre candidatos de ruta, pero no convierte material route-only en fuente final |
| TC-SEM-009 | negativo | RF-SEM-003 | con embeddings no configurados o provider fallido, el envelope entrega hint accionable hacia `nav wiki search` y no expone API keys |
