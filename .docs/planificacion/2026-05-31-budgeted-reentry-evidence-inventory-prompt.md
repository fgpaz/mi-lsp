# Prompt - mi-lsp budgeted reentry and evidence inventory

Date: 2026-05-31
Status: ready-to-run prompt
Source project: `C:\repos\buho\salud`
Target project: `C:\repos\mios\mi-lsp`

```yaml
harness_protocol: SDD-HARNESS-v1
id: "2026-05-31-budgeted-reentry-evidence-inventory-prompt"
kind: "planning-prompt"
audience: "llm-first"
imports:
  - '[[00_gobierno_documental]]'
  - '[[RF-QRY-019]]'
  - '[[TECH-EVIDENCE-INVENTORY]]'
  - '[[CT-NAV-EVIDENCE]]'
  - '[[AE-EVIDENCE-POLICY]]'
exports:
  - '2026-05-31-budgeted-reentry-evidence-inventory-prompt'
  - 'RF-QRY-019'
  - 'TECH-EVIDENCE-INVENTORY'
  - 'CT-NAV-EVIDENCE'
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/04_RF/RF-QRY-019.md
  - .docs/wiki/07_tech/TECH-EVIDENCE-INVENTORY.md
  - .docs/wiki/09_contratos/CT-NAV-EVIDENCE.md
  - .docs/wiki/ae/AE-EVIDENCE-POLICY.md
  - .docs/planificacion/2026-05-31-budgeted-reentry-evidence-inventory-prompt.md
agent_may_edit:
  - .docs/planificacion/2026-05-31-budgeted-reentry-evidence-inventory-prompt.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - go test ./internal/...
  - mi-lsp nav evidence inventory "AE evidence inventory" --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-source --workspace mi-lsp --format toon
stop_if:
  - governance_blocked=true
  - go_test_failed=true
  - harness_verdict=BLOCKED
  - wiki_source_verdict=BLOCKED
evidence:
  - .docs/auditoria/2026-05-31-budgeted-reentry-evidence-inventory/session-contract.yaml
  - .docs/auditoria/2026-05-31-budgeted-reentry-evidence-inventory/evidence-index.yaml
  - internal/service/evidence_inventory.go
  - internal/service/evidence_inventory_test.go
```

## Objective

Implement a low-token navigation improvement for agents using `mi-lsp`.

The caller problem observed in Salud:

- `mi-lsp nav route "AE token budget context evidence lifecycle" --workspace salud --format toon` returned a useful canonical anchor in preview mode with about 537 estimated tokens.
- broad `nav wiki search` on the same area returned around 4k estimated tokens and truncation.
- `mi-lsp workspace status salud --full --format toon` timed out in an agent turn.
- `.docs/auditoria/qa-conversacional` can contain thousands of files and raw evidence is too large for default agent reading.

Build a feature that helps an agent choose the cheapest safe reentry/evidence path before it burns context.

This feature should support AE callers that already carry an `ae_budget_gate`
with `persistence_mode`, `governance_depth`, `closure_profile`,
`artifact_lifecycle` and `why_not_cheaper`. `mi-lsp` does not own those policy
decisions, but it should emit enough compact evidence for the caller to select
or verify the cheapest safe gate.

## Requested feature

Add a preview-first evidence/reentry surface, name to choose in the repo design:

- preferred command shape: `mi-lsp nav evidence inventory <query> --workspace <alias> --format toon`
- acceptable alternate: `mi-lsp nav artifact inventory ...`
- acceptable split: `workspace status --reentry` plus `nav evidence inventory`

The surface should:

- return canonical wiki anchors first, not raw prompts/audits;
- summarize known evidence roots by type: `manifest`, `verdict`, `issues`, `assertions`, `turns`, `logs`, `screenshots`, `raw_prompts`, `raw_plans`;
- expose size/file-count estimates for heavy evidence roots without dumping contents;
- emit `recommended_read_path`: `route`, `wiki_search`, `wiki_pack`, `multi_read`, `manifest_verdict`, `targeted_raw`, or `full_raw`;
- emit `context_loading_profile`: `CL0_NONE|CL1_EXACT|CL2_OWNER_PACK|CL3_SUBSYSTEM|CL4_FULL_RUNTIME`;
- emit `evidence_loading_profile`: `EL0_NONE|EL1_MANIFEST_VERDICT|EL2_SUMMARY_ASSERTIONS|EL3_TARGETED_RAW|EL4_FULL_RAW`;
- mark historical prompts/plans/transcripts as non-authoritative evidence, not templates;
- prefer `manifest.yaml`, `verdict.md`, `issues.yaml`, summaries and hashes before turns/logs/screenshots.

## Non-goals

- Do not delete, move or rewrite evidence.
- Do not make `workspace status --full` the default reentry.
- Do not require a daemon for preview inventory if direct mode can answer.
- Do not expose raw prompt text, raw transcript text, secrets, emails or PHI in telemetry.

## Suggested implementation anchors

Use existing wiki-first and telemetry surfaces:

- `nav route`
- `nav wiki search|pack|inventory`
- `workspace status` preview/full distinction
- `admin export --summary`
- `memory_pointer` and `continuation`
- existing ignore/noise control for `.docs/raw`, `.docs/auditoria`, `.mi-lsp`

## Acceptance

- New preview command completes quickly on large repos and returns compact TOON by default.
- Output includes `tokens_est` or equivalent size hints.
- A test fixture with `.docs/auditoria/qa-conversacional/**/manifest.yaml`, `verdict.md`, `turns/`, `logs/` and screenshots proves manifest-first guidance.
- A fixture with `.docs/raw/prompts/*.md` proves prompts are classified as historical evidence, not canonical docs.
- `go test ./internal/...` or the smallest repo-approved targeted package set passes.

## Example desired response shape

```yaml
ok: true
backend: evidence.inventory
workspace: salud
recommended_read_path: manifest_verdict
context_loading_profile: CL1_EXACT
evidence_loading_profile: EL1_MANIFEST_VERDICT
items:
  - root: .docs/auditoria/qa-conversacional/telegram/CQA-EXAMPLE/run-2026-05-01T00-00Z
    artifact_type: cqa_bundle
    verdict: PASS
    summary_first:
      - manifest.yaml
      - verdict.md
      - issues.yaml
    heavy_artifacts:
      turns: { files: 24, bytes: 123456 }
      logs: { files: 3, bytes: 654321 }
      screenshots: { files: 4, bytes: 987654 }
    authority: evidence_not_canon
    next_queries:
      - mi-lsp nav multi-read <manifest/verdict ranges> --workspace salud --format toon
```
