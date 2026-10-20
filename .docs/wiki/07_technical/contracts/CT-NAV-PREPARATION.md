---
wiki_contract: WIKI-CONTRACT-v1
id: CT-NAV-PREPARATION
kind: contract
status: active
title: CT-NAV-PREPARATION
links:
  supports:
  - RF-QRY-019
---

## Consumidor

CLI, agentes y consumidores locales de las operaciones descritas; no se amplía la autoridad del consumidor.

## Operaciones

Se conservan requests, envelopes, campos, límites y semántica detallados a continuación.

### [ct-nav-preparation-harness]

- **Marcador estable**: ct-nav-preparation-harness
- **id**: CT-NAV-PREPARATION
- **Comprobación prevista**:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-source --workspace mi-lsp --ids CT-NAV-PREPARATION --format toon
- **Condiciones de rechazo**:
  - governance_blocked=true
  - harness_verdict=BLOCKED
  - authorization_claim=true
  - evidence_escape=true
- **Evidencia referenciada**:
  - .docs/wiki/05_requirements/RF-QRY-019.md
  - .docs/wiki/06_tests/TP-QRY-PREPARATION.md
  - .docs/auditoria/mi-lsp-portable-preparation-v1/session-contract.yaml
  - .docs/auditoria/mi-lsp-portable-preparation-v1/current-handoff.md

### [ct-nav-preparation-schema]

- **Marcador estable**: ct-nav-preparation-schema
- **schema**: mi-lsp-preparation/v1
- **commands**:
  - prepare create
  - prepare verify
  - prepare refresh
- **intents**:
  - read_only
  - source_mutation
  - artifact_create
  - artifact_promote
  - evidence_write
  - temp_internal
- **seed_receipt**: optional_metadata_and_digests_only
- **canonical_digest**: excludes_own_digest
- **fields**:
  - schema
  - workspace_identity
  - task_digest
  - governance_digest
  - index_digest
  - plan_digest
  - intent
  - targets
  - output_scope
  - created_at
  - expires_at
  - evidence
  - digest
- **Comprobación prevista**:
  - validate-harness
  - validate-source
- **Evidencia referenciada**:
  - .docs/auditoria/mi-lsp-portable-preparation-v1/session-contract.yaml
  - .docs/auditoria/mi-lsp-portable-preparation-v1/current-handoff.md

### [ct-nav-preparation-security]

- **Marcador estable**: ct-nav-preparation-security
- **workspace**: explicit_workspace_makes_cwd_independent
- **output**: explicit_validated_output_under_workspace_or_declared_evidence_root
- **path_rules**:
  - targets_relative
  - reject_absolute
  - reject_traversal
  - reject_symlink
  - reject_junction
  - reject_evidence_escape
- **authority**: mi-lsp_emits_serializes_verifies_refreshes_evidence_only
- **forbidden**:
  - authorize
  - promote
  - protected_write
  - broker
- **ttl**: default_15m; configurable_max_24h
- **Comprobación prevista**:
  - validate-harness
  - validate-source
- **Evidencia referenciada**:
  - .docs/auditoria/mi-lsp-portable-preparation-v1/session-contract.yaml
  - .docs/auditoria/mi-lsp-portable-preparation-v1/current-handoff.md

### [ct-nav-preparation-results]

- **Marcador estable**: ct-nav-preparation-results
- **codes**:
  - PREPARATION_READY
  - PREPARATION_MISSING
  - TRANSFER_REQUIRED
  - PACKET_EXPIRED
  - WORKSPACE_MISMATCH
  - TASK_DIGEST_MISMATCH
  - GOVERNANCE_DRIFT
  - INDEX_DRIFT
  - PLAN_DRIFT
  - PATH_SCOPE_MISMATCH
  - PACKET_TAMPERED
- **result_fields**:
  - code
  - repairable
  - recommended_action
- **legacy_packets**: accepted_as_legacy
- **Comprobación prevista**:
  - validate-harness
  - validate-source
- **Evidencia referenciada**:
  - .docs/auditoria/mi-lsp-portable-preparation-v1/session-contract.yaml
  - .docs/auditoria/mi-lsp-portable-preparation-v1/current-handoff.md
La versión `mi-lsp-preparation/v1` es un sobre portable y determinista. `create` emite, `verify` valida y `refresh` vuelve a emitir evidencia; ninguna operación concede autoridad de escritura o promoción. Los fallos deben declarar si son reparables y una acción recomendada. La compatibilidad legacy conserva lectura, pero no amplía permisos.

### Cierre v0.7.0 — implementación y evidencia

- **block_id**: `closure-v070-evidence`
- **kind**: executed-acceptance-map
- **source_of_truth**: this document
- **verify**: `mi-lsp nav wiki validate-source --workspace <alias> --ids CT-NAV-PREPARATION --format toon`
- **evidence**: `.docs/auditoria/mi-lsp-portable-preparation-v1/traceability-closure.yaml`
- **status**: `implemented_and_verified`
- **automation**: automated
- **result**: `17/17 PASS`
- **tp11_windows_junction**: `PASS`
- **implementation**: `internal/cli/prepare.go`, `internal/service/prepare.go`, `internal/service/preparation_packet.go`, `internal/model/preparation.go`, `internal/skills/preparation_receipt.go`
- **tests**: `internal/service/preparation_packet_matrix_test.go`, `internal/service/preparation_packet_test.go`, `internal/service/prepare_test.go`, `internal/skills/preparation_receipt_test.go`
- **evidence_root**: `.docs/auditoria/mi-lsp-portable-preparation-v1/`
- **review**: `541bd2bf`
- **head**: `a449abf`
- **authority**: evidence never authorizes writes or promotions

## Errores

Se mantienen las razones, warnings, rechazos y fallbacks explícitos de cada operación.

## Compatibilidad

La migración cambia el contrato documental, no el protocolo runtime ni sus versiones. Los esquemas legacy que la herramienta consume continúan como compatibilidad de entrada, no como formato de autoría de esta wiki.
