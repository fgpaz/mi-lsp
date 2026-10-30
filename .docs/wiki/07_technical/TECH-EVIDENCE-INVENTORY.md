---
wiki_contract: WIKI-CONTRACT-v1
id: TECH-EVIDENCE-INVENTORY
kind: technical
status: active
title: TECH-EVIDENCE-INVENTORY
links:
  supports:
  - RF-QRY-019
  code:
  - path: internal/service/evidence_inventory.go
    role: implementation
---

```yaml
harness_protocol: SDD-HARNESS-v1
id: TECH-EVIDENCE-INVENTORY
kind: technical
audience: llm-first
imports: [.docs/wiki/07_technical/contracts/CT-NAV-EVIDENCE.md]
exports: [TECH-EVIDENCE-INVENTORY]
agent_must_read: [.docs/wiki/07_technical/TECH-EVIDENCE-INVENTORY.md, internal/service/evidence_inventory.go]
agent_may_edit: [.docs/wiki/07_technical/TECH-EVIDENCE-INVENTORY.md]
agent_must_not_edit: [internal/service/evidence_inventory.go, .docs/auditoria/, .docs/raw/]
verify: ["Leer .docs/wiki/07_technical/TECH-EVIDENCE-INVENTORY.md y .docs/wiki/07_technical/contracts/CT-NAV-EVIDENCE.md; cotejar las afirmaciones con la fuente publicada internal/service/evidence_inventory.go y sus límites. Registrar únicamente comandos y resultados efectivamente ejecutados; esta especificación no afirma una ejecución."]
stop_if: [La propuesta requiera leer o emitir contenido raw, ampliar raíces o tratar artefactos de evidencia como canon.]
evidence: [.docs/wiki/07_technical/TECH-EVIDENCE-INVENTORY.md, .docs/wiki/07_technical/contracts/CT-NAV-EVIDENCE.md, internal/service/evidence_inventory.go]
```

## Resumen

Documentar la arquitectura técnica de `nav evidence inventory`: una lectura directa, preview-first y metadata-only para que agentes decidan si deben leer canon, manifests/verdicts o evidencia raw puntual.

### Propósito

Documentar la arquitectura técnica de `nav evidence inventory`: una lectura directa, preview-first y metadata-only para que agentes decidan si deben leer canon, manifests/verdicts o evidencia raw puntual.

## Mapa de componentes

Los componentes, paths exactos, responsabilidades y límites se mantienen en el detalle técnico siguiente.

Volver a [07_technical.md](../07_technical.md).

### Arquitectura

- **Marcador estable**: TECH-EVIDENCE-INVENTORY.architecture
- **entrypoint**: internal/service/evidence_inventory.go
- **operation**: nav.evidence.inventory
- **command**: mi-lsp nav evidence inventory <query>
- **execution**:
  - **route**: direct
  - **daemon_required**: false
  - **governance_gate**: required
  - **default_mode**: preview
- **Entradas**:
  - query
  - workspace
  - full
- **Salidas**:
  - canonical_route
  - recommended_read_path
  - context_loading_profile
  - evidence_loading_profile
  - evidence_roots
  - continuation_when_truncated

El servicio primero reutiliza el route core para obtener `RouteCanonicalLane`. Luego inspecciona solo metadata de `.docs/auditoria` y `.docs/raw`, sin promover esos artifacts a canon ni leer cuerpos raw.

### Taxonomia

- **Marcador estable**: TECH-EVIDENCE-INVENTORY.taxonomy
- **summary_first**:
  - **manifest**: manifest.yaml|manifest.yml
  - **verdict**: verdict.md|verdict.yaml|verdict.yml
  - **issues**: issues.yaml|issues.yml
  - **assertions**: assertions.yaml|assertions.yml|assertions.json|assertions.md
  - **summary**: summary.md|summary.yaml|summary.yml
  - **hashes**: hashes.yaml|hashes.yml|hashes.json
- **heavy_artifacts**:
  - **turns**: raw_transcript
  - **logs**: raw_runtime_log
  - **screenshots**: binary_visual_evidence
- **raw_roots**:
  - **raw_prompts**: .docs/raw/prompts
  - **raw_plans**: .docs/raw/plans
- **authority**:
  - **canonical_wiki**: task/source authority
  - **evidence_not_canon**: operational evidence or historical handoff

### Seguridad y performance

- **Marcador estable**: TECH-EVIDENCE-INVENTORY.guardrails
- **must_not_emit**:
  - prompt_body
  - transcript_text
  - log_excerpt
  - screenshot_ocr
  - secrets
  - emails
  - phi
- **path_policy**:
  - report_workspace_relative_paths
  - scan_only_known_workspace_roots
  - do_not_follow_workspace_escape
- **scan_policy**:
  - **preview_file_limit**: 5000
  - **full_file_limit**: 50000
  - **content_reads**:
    - **allowed**:
      - verdict.md bounded status extraction
    - **forbidden**:
      - raw_prompts
      - turns
      - logs
      - screenshots
- **truncation**:
  - set truncated=true
  - emit continuation to nav.evidence.inventory full

### Sync

- RF owner: `[[RF-QRY-019]]`
- CLI contract: `[[CT-NAV-EVIDENCE]]`
- Test owner: `[[TP-QRY]]`
- AE evidence policy: `[[AE-EVIDENCE-POLICY]]`
