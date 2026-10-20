---
wiki_contract: WIKI-CONTRACT-v1
id: CT-NAV-EVIDENCE
kind: contract
status: active
title: CT-NAV-EVIDENCE
links:
  supports:
  - RF-QRY-019
---

## Consumidor

CLI, agentes y consumidores locales de las operaciones descritas; no se amplía la autoridad del consumidor.

## Operaciones

Se conservan requests, envelopes, campos, límites y semántica detallados a continuación.

### Invocacion

```powershell
mi-lsp nav evidence inventory "AE token budget evidence lifecycle" --workspace <alias> --format toon
mi-lsp nav evidence inventory "qa conversacional" --workspace <alias> --full --format json
```

### Semántica

`nav evidence inventory` produce un inventario compacto para elegir camino de lectura. La ejecución es directa, no requiere daemon, pasa por governance gate y no lee contenido raw salvo metadata acotada de artifacts summary-first.

### Envelope

```json
{
  "ok": true,
  "workspace": "mi-lsp",
  "backend": "evidence.inventory",
  "mode": "preview",
  "items": [
    {
      "query": "qa conversacional",
      "mode": "preview",
      "recommended_read_path": "manifest_verdict",
      "context_loading_profile": "CL1_EXACT",
      "evidence_loading_profile": "EL1_MANIFEST_VERDICT",
      "canonical": {
        "anchor_doc": {"path": ".docs/wiki/05_requirements/RF-QRY-019.md", "doc_id": "RF-QRY-019"},
        "authoritative": true
      },
      "evidence_roots": [
        {
          "root": ".docs/auditoria/qa-conversacional/telegram/CQA-EXAMPLE/run-2026-05-01T00-00Z",
          "artifact_type": "cqa_bundle",
          "verdict": "PASS",
          "summary_first": ["manifest.yaml", "verdict.md", "issues.yaml"],
          "heavy_artifacts": {
            "turns": {"files": 24, "bytes": 123456, "content_embedded": false, "omitted_raw": true},
            "logs": {"files": 3, "bytes": 654321, "content_embedded": false, "omitted_raw": true},
            "screenshots": {"files": 4, "bytes": 987654, "content_embedded": false, "omitted_raw": true}
          },
          "authority": "evidence_not_canon",
          "next_queries": [
            "mi-lsp nav multi-read .docs/auditoria/.../manifest.yaml:1-120 .docs/auditoria/.../verdict.md:1-120 --workspace mi-lsp --format toon"
          ]
        }
      ]
    }
  ],
  "truncated": false,
  "stats": {"files": 31, "tokens_est": 900}
}
```

### No exposicion raw

El contrato prohibe emitir cuerpos de prompts, transcripciones, logs, OCR de screenshots, secretos, emails o PHI. Los artifacts raw solo exponen conteos, bytes, timestamp y `omitted_raw=true`.

## Errores

Se mantienen las razones, warnings, rechazos y fallbacks explícitos de cada operación.

### Errores y límites

- Falta `<query>`: error de validación `query is required`.
- Governance bloqueada: envelope de governance blocked, sin scan normal.
- Scan excede presupuesto: `ok=true`, `truncated=true`, warning y continuation hacia `--full`.
- Roots ausentes: `ok=true`, `evidence_roots=[]`, warning explicito.

## Compatibilidad

La migración cambia el contrato documental, no el protocolo runtime ni sus versiones. Los esquemas legacy que la herramienta consume continúan como compatibilidad de entrada, no como formato de autoría de esta wiki.

### Reglas de compatibilidad

- El comando es aditivo y no cambia `nav wiki inventory`.
- `backend=evidence.inventory` y `mode=preview|full` son estables.
- `stats.tokens_est` lo agrega el renderer igual que en otros envelopes.
- `truncated=true` requiere `continuation.next.op=nav.evidence.inventory`.
- `authority=evidence_not_canon` significa que el artifact orienta lectura o cierre, pero no reemplaza wiki canonica.
