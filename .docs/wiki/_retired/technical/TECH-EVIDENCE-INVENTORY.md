# TECH-EVIDENCE-INVENTORY

```yaml
harness_protocol: SDD-HARNESS-v1
id: "TECH-EVIDENCE-INVENTORY"
kind: "tech-spec"
audience: "llm-first"
imports:
  - '[[RF-QRY-019]]'
  - '[[CT-NAV-EVIDENCE]]'
exports:
  - 'TECH-EVIDENCE-INVENTORY'
agent_must_read:
  - .docs/wiki/04_RF/RF-QRY-019.md
  - .docs/wiki/07_tech/TECH-EVIDENCE-INVENTORY.md
  - .docs/wiki/09_contratos/CT-NAV-EVIDENCE.md
agent_may_edit:
  - .docs/wiki/07_tech/TECH-EVIDENCE-INVENTORY.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --paths .docs/wiki/07_tech/TECH-EVIDENCE-INVENTORY.md --format toon
  - mi-lsp nav wiki validate-source --workspace mi-lsp --paths .docs/wiki/07_tech/TECH-EVIDENCE-INVENTORY.md --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
evidence:
  - .docs/wiki/07_tech/TECH-EVIDENCE-INVENTORY.md
  - internal/service/evidence_inventory.go
```

Volver a [07_baseline_tecnica.md](../07_baseline_tecnica.md).

## Proposito

Documentar la arquitectura tecnica de `nav evidence inventory`: una lectura directa, preview-first y metadata-only para que agentes decidan si deben leer canon, manifests/verdicts o evidencia raw puntual.

## Arquitectura

```toon
doc_id: TECH-EVIDENCE-INVENTORY
block_id: TECH-EVIDENCE-INVENTORY.architecture
kind: architecture
source_of_truth: this
entrypoint: internal/service/evidence_inventory.go
operation: nav.evidence.inventory
command: mi-lsp nav evidence inventory <query>
execution:
  route: direct
  daemon_required: false
  governance_gate: required
  default_mode: preview
inputs:
  - query
  - workspace
  - full
outputs:
  - canonical_route
  - recommended_read_path
  - context_loading_profile
  - evidence_loading_profile
  - evidence_roots
  - continuation_when_truncated
```

El servicio primero reutiliza el route core para obtener `RouteCanonicalLane`. Luego inspecciona solo metadata de `.docs/auditoria` y `.docs/raw`, sin promover esos artifacts a canon ni leer cuerpos raw.

## Taxonomia

```toon
doc_id: TECH-EVIDENCE-INVENTORY
block_id: TECH-EVIDENCE-INVENTORY.taxonomy
kind: data-contract
source_of_truth: this
summary_first:
  manifest: manifest.yaml|manifest.yml
  verdict: verdict.md|verdict.yaml|verdict.yml
  issues: issues.yaml|issues.yml
  assertions: assertions.yaml|assertions.yml|assertions.json|assertions.md
  summary: summary.md|summary.yaml|summary.yml
  hashes: hashes.yaml|hashes.yml|hashes.json
heavy_artifacts:
  turns: raw_transcript
  logs: raw_runtime_log
  screenshots: binary_visual_evidence
raw_roots:
  raw_prompts: .docs/raw/prompts
  raw_plans: .docs/raw/plans
authority:
  canonical_wiki: task/source authority
  evidence_not_canon: operational evidence or historical handoff
```

## Seguridad y performance

```toon
doc_id: TECH-EVIDENCE-INVENTORY
block_id: TECH-EVIDENCE-INVENTORY.guardrails
kind: guardrails
source_of_truth: this
must_not_emit:
  - prompt_body
  - transcript_text
  - log_excerpt
  - screenshot_ocr
  - secrets
  - emails
  - phi
path_policy:
  - report_workspace_relative_paths
  - scan_only_known_workspace_roots
  - do_not_follow_workspace_escape
scan_policy:
  preview_file_limit: 5000
  full_file_limit: 50000
  content_reads:
    allowed:
      - verdict.md bounded status extraction
    forbidden:
      - raw_prompts
      - turns
      - logs
      - screenshots
truncation:
  - set truncated=true
  - emit continuation to nav.evidence.inventory full
```

## Sync

- RF owner: `[[RF-QRY-019]]`
- CLI contract: `[[CT-NAV-EVIDENCE]]`
- Test owner: `[[TP-QRY]]`
- AE evidence policy: `[[AE-EVIDENCE-POLICY]]`
