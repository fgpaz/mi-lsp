# CT-NAV-EVIDENCE

```yaml
harness_protocol: SDD-HARNESS-v1
id: "CT-NAV-EVIDENCE"
kind: "wiki-doc"
audience: "llm-first"
imports:
  - '[[RF-QRY-019]]'
  - '[[TECH-EVIDENCE-INVENTORY]]'
exports:
  - 'CT-NAV-EVIDENCE'
agent_must_read:
  - .docs/wiki/04_RF/RF-QRY-019.md
  - .docs/wiki/07_tech/TECH-EVIDENCE-INVENTORY.md
  - .docs/wiki/09_contratos/CT-NAV-EVIDENCE.md
agent_may_edit:
  - .docs/wiki/09_contratos/CT-NAV-EVIDENCE.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
evidence:
  - .docs/wiki/09_contratos/CT-NAV-EVIDENCE.md
```

## Invocacion

```powershell
mi-lsp nav evidence inventory "AE token budget evidence lifecycle" --workspace <alias> --format toon
mi-lsp nav evidence inventory "qa conversacional" --workspace <alias> --full --format json
```

## Semantica

`nav evidence inventory` produce un inventario compacto para elegir camino de lectura. La ejecucion es directa, no requiere daemon, pasa por governance gate y no lee contenido raw salvo metadata acotada de artifacts summary-first.

## Envelope

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
        "anchor_doc": {"path": ".docs/wiki/04_RF/RF-QRY-019.md", "doc_id": "RF-QRY-019"},
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

## Reglas de compatibilidad

- El comando es aditivo y no cambia `nav wiki inventory`.
- `backend=evidence.inventory` y `mode=preview|full` son estables.
- `stats.tokens_est` lo agrega el renderer igual que en otros envelopes.
- `truncated=true` requiere `continuation.next.op=nav.evidence.inventory`.
- `authority=evidence_not_canon` significa que el artifact orienta lectura o cierre, pero no reemplaza wiki canonica.

## Errores y limites

- Falta `<query>`: error de validacion `query is required`.
- Governance bloqueada: envelope de governance blocked, sin scan normal.
- Scan excede presupuesto: `ok=true`, `truncated=true`, warning y continuation hacia `--full`.
- Roots ausentes: `ok=true`, `evidence_roots=[]`, warning explicito.

## No exposicion raw

El contrato prohibe emitir cuerpos de prompts, transcripciones, logs, OCR de screenshots, secretos, emails o PHI. Los artifacts raw solo exponen conteos, bytes, timestamp y `omitted_raw=true`.
