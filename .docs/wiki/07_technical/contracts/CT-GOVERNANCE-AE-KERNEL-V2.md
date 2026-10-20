---
wiki_contract: WIKI-CONTRACT-v1
id: CT-GOVERNANCE-AE-KERNEL-V2
kind: contract
status: active
title: CT-GOVERNANCE-AE-KERNEL-V2
links:
  supports:
  - RF-WKS-008
---

## Consumidor

CLI, agentes y consumidores locales de las operaciones descritas; no se amplía la autoridad del consumidor.

## Operaciones

Se conservan requests, envelopes, campos, límites y semántica detallados a continuación.

### Anclaje canónico y compatibilidad del triple SDD

Las reglas imports/exports y los slots SDD conservados debajo describen el consumidor legacy de la herramienta para fuentes no migradas. No son metadata ni obligaciones de autoría de los documentos WIKI-CONTRACT-v1 de este repo. Governance v1 no tiene links ni importa el kernel: la política repo-local y este contrato mantienen el anclaje al canon externo, sin promover ae/ a autoridad.

### Canonical binding

- **Marcador estable**: CT-GOVERNANCE-AE-KERNEL-V2.canonical-binding
- **normative**:
  - **ae_canon.mode**: kernel_v2
  - **ae_canon.source**: <kernel_home>/canon
  - **repo_policy**: .docs/ae/repo-policy.yaml
  - **required_modules**:
    - AE-KERNEL-V2.md
    - AE-PHASES.md
    - AE-HARNESS-ORCHESTRATION.md
    - AE-EVIDENCE-POLICY.md
    - AE-POLICY-PROJECTION.md
  - **states**:
    - valid
    - missing
    - mismatch
  - **imports_rule**:
    - **applies_to**:
      - SDD-HARNESS-v1
      - SDD-WIKI-SOURCE-v1
    - **required**: true
    - **root_empty_exception**:
      - **source_document**: exact governance status HumanDoc
      - **default_path**: .docs/wiki/00_governance.md
      - **allowed_when**:
        - governance_source_is_unique_and_validated
        - contract_path_matches_exact_governance_source
      - **value**:
    - **non_root_empty**: blocked
- **Comprobación prevista**:
  - inspect governance status and projection round-trip
- **Condiciones de rechazo**:
  - ae_canon.mode != kernel_v2
  - ae_canon.source != <kernel_home>/canon
  - ae_canon.repo_policy != .docs/ae/repo-policy.yaml
- **Evidencia referenciada**:
  - .docs/wiki/00_governance.md

`imports` sigue siendo un campo requerido para contratos `SDD-HARNESS-v1` y `SDD-WIKI-SOURCE-v1`. La única excepción admite `imports: []` en el documento de gobernanza que el estado de gobernanza identifica sin ambigüedad y valida (`governance_source_is_unique_and_validated`), cuando la ruta del contrato coincide exactamente con esa fuente. Todo contrato que no sea esa raíz y declare imports vacíos permanece bloqueado; la compuerta general continúa siendo fail-closed.

### Repository policy slots

- **Marcador estable**: CT-GOVERNANCE-AE-KERNEL-V2.repo-policy-slots
- **required_slots**:
  - repo.name
  - repo.description
  - repo.structure_rules[]
  - language.policy_lang
  - language.docs_lang
  - tracker.provider
  - tracker.<provider>.base_url (external providers only)
  - tracker.<provider>.workspace (external providers only)
  - tracker.<provider>.key_env (external providers only; env name, never a secret value)
  - tracker.<provider>.projects[].key (external providers only)
  - tracker.none.mode (None provider only; must be local-only)
  - secrets.vault
  - secrets.tool
  - wiki.layers_map.02
  - wiki.workspace_alias
  - wiki.paths_file
  - subagents.roster_file
  - wrappers[]
  - qa.canon_paths[]
  - last_updated
- **policy**:
  - **supported_tracker_providers**:
    - Linear
    - Plane
    - Azure Boards
    - Jira
    - None
  - **provider_aliases**: exact_case_aliases_resolve_to_one_block
  - **exactly_one_provider_block**: true
  - **none_mode**: local-only_only
  - **top_level_tracker_aliases**: forbidden
  - **external_key_env**: environment_variable_name_only
  - **no_secrets**: true
  - **no_fixture_config**: true
- **Comprobación prevista**:
  - parse YAML and validate every required slot is non-empty
- **Condiciones de rechazo**:
  - missing_required_slot
  - secret_material_present
  - fixture_config_used_as_productive_policy
- **Evidencia referenciada**:
  - .docs/ae/repo-policy.yaml

### Path and symlink safety

- **Marcador estable**: CT-GOVERNANCE-AE-KERNEL-V2.path-safety
- **rules**:
  - **path_traversal**: reject absolute paths, dot paths, and extra parent escapes
  - **governed_boundary**: source_doc is a relative path in-workspace OR inside a declared [[RF-WKS-008|canon]] root
  - **projection_output**: relative in-workspace path in the code workspace; never a foreign read-only [[RF-WKS-008|canon]] root
  - **symlink**: reject symlinked root, directory components, and files
  - **regular_file**: require regular files for canon modules and repo policy
  - **external_kernel**: resolve <kernel_home> from AE_KERNEL_HOME or local AE config without copying canon into repo
- **states**:
  - **valid**: all modules and slots pass checks
  - **missing**: a required module or slot is absent
  - **mismatch**: configuration, path, YAML, or symlink safety is invalid
- **Comprobación prevista**:
  - go test ./internal/docgraph ./internal/service
  - inspect governance status
- **Condiciones de rechazo**:
  - path_traversal_detected
  - symlink_detected
  - invalid_yaml
  - unsafe_external_source
- **Evidencia referenciada**:
  - internal/docgraph/governance.go
  - internal/service/governance.go

### Projection and closure

- **Marcador estable**: CT-GOVERNANCE-AE-KERNEL-V2.projection-closure
- **projection**: .docs/wiki/_mi-lsp/read-model.toml
- **required_slots**:
  - imports
  - exports
  - verify
  - stop_if
  - evidence
  - doc_id
  - block_id
- **closure**:
  - **completion_handoff**: finishing-a-development-branch
  - **handoff_ready**: after_setup_pass
  - **audit_required**: true
- **Comprobación prevista**:
  - projection matches the fenced YAML source
  - deterministic policy render passes against the fresh external kernel
  - audit manifest and traceability closure exist
- **Condiciones de rechazo**:
  - projection_out_of_sync
  - render_failed
  - audit_missing
  - setup_verdict != PASS
- **Evidencia referenciada**:
  - .docs/auditoria/2026-07-13-ae-kernel-v2-integration/traceability-closure.yaml
  - .docs/auditoria/2026-07-13-ae-kernel-v2-integration/session-contract.yaml
  - .docs/auditoria/2026-07-13-ae-kernel-v2-integration/traceability-closure.yaml

## Errores

Se mantienen las razones, warnings, rechazos y fallbacks explícitos de cada operación.

## Compatibilidad

La migración cambia el contrato documental, no el protocolo runtime ni sus versiones. Los esquemas legacy que la herramienta consume continúan como compatibilidad de entrada, no como formato de autoría de esta wiki.
