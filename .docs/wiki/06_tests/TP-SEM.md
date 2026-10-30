---
wiki_contract: WIKI-CONTRACT-v1
id: TP-SEM
kind: test
status: active
title: TP-SEM
links:
  verifies:
  - RF-SEM-001
  - RF-SEM-002
  - RF-SEM-003
  - RF-SEM-004
---

## Alcance

RF-SEM-001, RF-SEM-002, RF-SEM-003, RF-SEM-004. Se conserva el inventario de escenarios, límites, disposiciones y evidencia histórica, sin reejecución.

## Casos

### Cobertura objetivo

- RF-SEM-001
- RF-SEM-002
- RF-SEM-003
- RF-SEM-004

### Escenarios
- **Caso**: TC-SEM-001; **Tipo**: positivo; **RF**: RF-SEM-001; **Descripción**: acepta config `[embeddings]` completa con `provider`, `base_url`, `model`, `dim`, `api_key_env`, `profile`, `batch_size`, `timeout_ms`, `encoding_format` y `user_agent`
  - RF-SEM-001 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: acepta config `[embeddings]` completa con `provider`, `base_url`, `model`, `dim`, `api_key_env`, `profile`, `batch_size`, `timeout_ms`, `encoding_format` y `user_agent`. No acredita una ejecución de esta migración.
- **Caso**: TC-SEM-002; **Tipo**: positivo; **RF**: RF-SEM-001; **Descripción**: envia payload OpenAI-compatible con `encoding_format = "float"`, `Accept`, `User-Agent` y valida dimensión estrictamente contra `dim`
  - RF-SEM-001 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: envia payload OpenAI-compatible con `encoding_format = "float"`, `Accept`, `User-Agent` y valida dimensión estrictamente contra `dim`. No acredita una ejecución de esta migración.
- **Caso**: TC-SEM-003; **Tipo**: negativo; **RF**: RF-SEM-001; **Descripción**: si key/provider/config falla, no imprime secretos y recomienda `nav wiki search` sin fallback local oculto
  - RF-SEM-001 — Criterio 3. Disposición: manual_evidence_pending. Evidencia esperada: si key/provider/config falla, no imprime secretos y recomienda `nav wiki search` sin fallback local oculto. No acredita una ejecución de esta migración.
- **Caso**: TC-SEM-004; **Tipo**: positivo; **RF**: RF-SEM-002; **Descripción**: reindexa cuando cambia metadata-prefix, texto enriquecido, content hash, modelo o dimensión
  - RF-SEM-002 — Criterio 2. Disposición: manual_evidence_pending. Evidencia esperada: reindexa cuando cambia metadata-prefix, texto enriquecido, content hash, modelo o dimensión. No acredita una ejecución de esta migración.
- **Caso**: TC-SEM-005; **Tipo**: positivo; **RF**: RF-SEM-002; **Descripción**: persiste `wiki_chunk_embeddings` con `doc_path`, `chunk_id`, rango de líneas, heading, snippet, `embedding_model`, `embedding_dim` y BLOB float32
  - RF-SEM-002 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: persiste `wiki_chunk_embeddings` con `doc_path`, `chunk_id`, rango de líneas, heading, snippet, `embedding_model`, `embedding_dim` y BLOB float32. No acredita una ejecución de esta migración.
- **Caso**: TC-SEM-006; **Tipo**: negativo; **RF**: RF-SEM-002; **Descripción**: rechaza reutilizar vectores stale cuando el hash, modelo, dimensión o BLOB faltante no coincide
  - RF-SEM-002 — Criterio 2. Disposición: manual_evidence_pending. Evidencia esperada: rechaza reutilizar vectores stale cuando el hash, modelo, dimensión o BLOB faltante no coincide. No acredita una ejecución de esta migración.
- **Caso**: TC-SEM-007; **Tipo**: positivo; **RF**: RF-SEM-003; **Descripción**: `nav recall --intent formula` devuelve `RecallResult.intent` y prioriza reglas, contratos y definiciones
  - RF-SEM-003 — Criterio 4. Disposición: manual_evidence_pending. Evidencia esperada: `nav recall --intent formula` devuelve `RecallResult.intent` y prioriza reglas, contratos y definiciones. No acredita una ejecución de esta migración.
- **Caso**: TC-SEM-008; **Tipo**: positivo; **RF**: RF-SEM-003; **Descripción**: `nav recall --intent route` descubre candidatos de ruta, pero no convierte material route-only en fuente final
  - RF-SEM-003 — Criterio 4. Disposición: manual_evidence_pending. Evidencia esperada: `nav recall --intent route` descubre candidatos de ruta, pero no convierte material route-only en fuente final. No acredita una ejecución de esta migración.
- **Caso**: TC-SEM-009; **Tipo**: negativo; **RF**: RF-SEM-003; **Descripción**: con embeddings no configurados o provider fallido, el envelope entrega hint accionable hacia `nav wiki search` y no expone API keys
  - RF-SEM-003 — Criterio 2. Disposición: manual_evidence_pending. Evidencia esperada: con embeddings no configurados o provider fallido, el envelope entrega hint accionable hacia `nav wiki search` y no expone API keys. No acredita una ejecución de esta migración.
- **Caso**: TC-SEM-010; **Tipo**: positivo; **RF**: RF-SEM-004; **Descripción**: con `[recall.rerank_extension] enabled=true`, `nav recall` envia candidatos acotados por stdin JSON versionado a un comando local
  - RF-SEM-004 — Criterio 3. Disposición: manual_evidence_pending. Evidencia esperada: con `[recall.rerank_extension] enabled=true`, `nav recall` envia candidatos acotados por stdin JSON versionado a un comando local. No acredita una ejecución de esta migración.
- **Caso**: TC-SEM-011; **Tipo**: positivo; **RF**: RF-SEM-004; **Descripción**: una respuesta valida del hook reordena candidatos antes del corte final y marca `why=external_rerank`
  - RF-SEM-004 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: una respuesta valida del hook reordena candidatos antes del corte final y marca `why=external_rerank`. No acredita una ejecución de esta migración.
- **Caso**: TC-SEM-012; **Tipo**: negativo; **RF**: RF-SEM-004; **Descripción**: comando faltante, timeout, salida no JSON, exit no cero o version inválida preservan el orden semántico original
  - RF-SEM-004 — Criterio 2. Disposición: manual_evidence_pending. Evidencia esperada: comando faltante, timeout, salida no JSON, exit no cero o version inválida preservan el orden semántico original. No acredita una ejecución de esta migración.
- **Caso**: TC-SEM-013; **Tipo**: negativo; **RF**: RF-SEM-004; **Descripción**: índices duplicados o fuera de rango se rechazan y preservan el orden semántico original
  - RF-SEM-004 — Criterio 2. Disposición: manual_evidence_pending. Evidencia esperada: índices duplicados o fuera de rango se rechazan y preservan el orden semántico original. No acredita una ejecución de esta migración.
- **Caso**: TC-SEM-014; **Tipo**: negativo; **RF**: RF-SEM-004; **Descripción**: warnings/logs no exponen query, snippets, stdin/stdout completos, provider responses, tokens ni secretos
  - RF-SEM-004 — Criterio 2. Disposición: manual_evidence_pending. Evidencia esperada: warnings/logs no exponen query, snippets, stdin/stdout completos, provider responses, tokens ni secretos. No acredita una ejecución de esta migración.

```yaml
harness_protocol: SDD-HARNESS-v1
id: TP-SEM
kind: test-plan
audience: llm-first
imports:
  - .docs/wiki/00_governance.md
  - .docs/wiki/05_requirements/RF-SEM-001.md
exports:
  - TP-SEM
agent_must_read:
  - .docs/wiki/06_tests/TP-SEM.md
agent_may_edit:
  - .docs/wiki/06_tests/TP-SEM.md
agent_must_not_edit:
  - .docs/wiki/05_requirements/
  - internal/
verify:
  - .docs/wiki/06_tests/TP-SEM.md#Casos
stop_if:
  - harness_verdict=BLOCKED
  - manual_evidence_pending_is_reported_as_pass
  - test_oracle_or_acceptance_would_change
evidence:
  - .docs/wiki/06_tests/TP-SEM.md
  - .docs/wiki/05_requirements/RF-SEM-001.md
  - .docs/wiki/05_requirements/RF-SEM-002.md
  - .docs/wiki/05_requirements/RF-SEM-003.md
  - .docs/wiki/05_requirements/RF-SEM-004.md
```
