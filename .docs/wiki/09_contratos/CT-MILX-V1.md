---
doc_id: CT-MILX-V1
title: Protocolo local de extensiones MILX-v1
layer: CT
family: MILX
status: accepted-design
---

```yaml
harness_protocol: SDD-HARNESS-v1
source_protocol: SDD-WIKI-SOURCE-v1
id: "CT-MILX-V1"
doc_id: "CT-MILX-V1"
kind: "contract"
audience: "llm-first"
imports:
  - '[[00_gobierno_documental]]'
  - '[[09_contratos_tecnicos]]'
  - '[[FL-GPH-03]]'
  - '[[RF-GPH-010]]'
  - '[[RF-GPH-011]]'
  - '[[TECH-GRAPH-NATIVE]]'
  - '[[DB-SYMBOL-EDGE-GRAPH]]'
exports:
  - 'CT-MILX-V1'
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/03_FL/FL-GPH-03.md
  - .docs/wiki/04_RF/RF-GPH-010.md
  - .docs/wiki/04_RF/RF-GPH-011.md
  - .docs/wiki/09_contratos/CT-MILX-V1.md
agent_may_edit:
  - .docs/wiki/09_contratos_tecnicos.md
  - .docs/wiki/09_contratos/CT-MILX-V1.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --paths .docs/wiki/09_contratos/CT-MILX-V1.md --format toon
  - mi-lsp nav wiki validate-source --workspace mi-lsp --paths .docs/wiki/09_contratos/CT-MILX-V1.md --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
  - capability_escape=true
  - MCP_or_network_required=true
evidence:
  - .docs/wiki/09_contratos/CT-MILX-V1.md
  - .docs/wiki/06_pruebas/TP-GPH.md
```

Volver a [09_contratos_tecnicos.md](../09_contratos_tecnicos.md).

## Boundary

MILX-v1 es un protocolo local stdio entre el core y un proceso hijo aislado. No es MCP, no publica tools, HTTP, SSE ni JSON-RPC remoto. Solo consume un snapshot/pack read-only y produce resultados derivativos serializados.

## Manifest

```toon
doc_id: CT-MILX-V1
block_id: CT-MILX-V1.manifest
kind: manifest-contract
source_of_truth: this
verify:
  - mi-lsp nav wiki validate-source --workspace mi-lsp --format toon
evidence:
  - .docs/wiki/09_contratos/CT-MILX-V1.md
  - .docs/wiki/06_pruebas/TP-GPH.md
schema: milx-manifest/v1
required:
  - extension_id
  - extension_version
  - executable_sha256
  - protocol_min
  - protocol_max
  - operations
  - input_schemas
  - output_schemas
  - capabilities
  - deterministic
optional:
  - resource_hints
  - pack_families
validation:
  extension_id: lowercase-ascii-segments
  executable_sha256: 64-lowercase-hex
  protocol_range: must-include-1
  unknown_capability: reject
  digest_mismatch: reject
```

## Framing y envelope

```toon
doc_id: CT-MILX-V1
block_id: CT-MILX-V1.framing
kind: wire-contract
source_of_truth: this
verify:
  - mi-lsp nav wiki validate-source --workspace mi-lsp --format toon
evidence:
  - .docs/wiki/09_contratos/CT-MILX-V1.md
  - .docs/wiki/06_pruebas/TP-GPH.md
transport: child-process-stdio
frame:
  prefix: uint32-big-endian-payload-length
  payload: canonical-JSON-UTF8
  max-bytes: 1048576
  partial-frame: protocol-error
  trailing-bytes: next-frame-only
envelope:
  schema: milx-envelope/v1
  required: [request_id, operation, protocol_version, payload]
  request_id: caller-generated-opaque-string-max-128
  protocol_version: 1
response:
  required: [request_id, operation, status, payload]
  status: [ok, rejected, canceled, timeout, failed]
limits:
  max-in-flight-per-process: 1
  max-total-output-bytes: 16777216
  default-request-timeout-ms: 60000
  max-request-timeout-ms: 300000
  shutdown-grace-ms: 2000
```

Canonical JSON ordena object keys, usa UTF-8 sin BOM, enteros base 10 y no admite NaN/Infinity. Payloads que superan el frame no se fragmentan implicitamente: el contrato de operacion debe usar pages/packs bounded.

## Operaciones

```toon
doc_id: CT-MILX-V1
block_id: CT-MILX-V1.operations
kind: operation-contract
source_of_truth: this
verify:
  - mi-lsp nav wiki validate-source --workspace mi-lsp --format toon
evidence:
  - .docs/wiki/09_contratos/CT-MILX-V1.md
  - .docs/wiki/06_pruebas/TP-GPH.md
operations:
  describe:
    input: empty
    output: [extension_id, extension_version, executable_sha256, protocol_range, operations, capabilities, schemas]
  prepare:
    input: [generation_id, graph_schema_version, pack_schema, pack_digest, budgets]
    output: [prepared_id, accepted_capabilities, effective_budgets]
  execute:
    input: [prepared_id, operation_name, parameters, parameters_digest]
    output: [result_schema, result, result_digest, provenance, omissions]
  cancel:
    input: [target_request_id]
    output: [cancel_status]
  health:
    input: empty
    output: [status, protocol_version, extension_id]
  shutdown:
    input: empty
    output: [cleanup_status]
state_machine:
  - spawned-to-described
  - described-to-prepared
  - prepared-to-executing
  - executing-to-prepared
  - any-to-shutdown
invalid_transition: GPH_MILX_STATE_INVALID
```

## Capabilities y pack access

```toon
doc_id: CT-MILX-V1
block_id: CT-MILX-V1.capabilities
kind: security-contract
source_of_truth: this
verify:
  - mi-lsp nav wiki validate-source --workspace mi-lsp --format toon
evidence:
  - .docs/wiki/09_contratos/CT-MILX-V1.md
  - .docs/wiki/06_pruebas/TP-GPH.md
allowlist-v1:
  - graph.read.nodes
  - graph.read.edges
  - graph.read.evidence
  - documents.read.pack
  - analysis.emit
  - visual.emit
  - import.emit-advisory
forbidden-v1:
  - graph.write
  - wiki.write
  - network
  - process.spawn
  - secrets.read
  - MCP
pack:
  access: read-only-bounded
  required: [schema, generation_id, selection, provenance, digest, omissions]
  database-write-handle: forbidden
  arbitrary-host-path: forbidden
result:
  authority: derived-only
  primary-graph-mutation: forbidden
  wiki-authority: forbidden
```

El host puede materializar un pack serializado o un handle read-only controlado. Nunca entrega credenciales, writable DB, registry mutable ni environment completa.

## Lifecycle y sandbox

```toon
doc_id: CT-MILX-V1
block_id: CT-MILX-V1.lifecycle
kind: runtime-contract
source_of_truth: this
verify:
  - mi-lsp nav wiki validate-source --workspace mi-lsp --format toon
evidence:
  - .docs/wiki/09_contratos/CT-MILX-V1.md
  - .docs/wiki/06_pruebas/TP-GPH.md
spawn:
  shell: false
  cwd: per-execution-temporary-directory
  environment: explicit-allowlist-without-secrets
  process-tree-containment: required-per-RID
  network: denied-or-verified-absent
prepare:
  validate-before-data-access: [manifest, executable-digest, protocol, capabilities, budgets]
cancel:
  cooperative-first: true
  force-after-grace: extension-process-tree-only
cleanup:
  always-run: true
  verify: [process-exited, handles-closed, temp-resources-disposed]
telemetry:
  allowed: [extension-id-version, generation, operation, status, duration, output-size, reason-code, cleanup-status]
  forbidden: [raw-pack, raw-result, stderr-dump, secrets, source-content]
```

## Error contract

```toon
doc_id: CT-MILX-V1
block_id: CT-MILX-V1.errors
kind: error-contract
source_of_truth: this
verify:
  - mi-lsp nav wiki validate-source --workspace mi-lsp --format toon
evidence:
  - .docs/wiki/09_contratos/CT-MILX-V1.md
  - .docs/wiki/06_pruebas/TP-GPH.md
errors:
  GPH_MILX_PROTOCOL_UNSUPPORTED: reject-before-spawn-or-prepare
  GPH_MILX_MANIFEST_INVALID: reject
  GPH_MILX_EXECUTABLE_DIGEST_MISMATCH: reject
  GPH_MILX_CAPABILITY_DENIED: reject-before-execute
  GPH_MILX_STATE_INVALID: reject-operation
  GPH_MILX_FRAME_INVALID: discard-process-output
  GPH_MILX_OUTPUT_INVALID: discard-result
  GPH_MILX_TIMEOUT: cancel-then-terminate
  GPH_MILX_PROCESS_FAILED: isolate-and-preserve-core
  GPH_MILX_CLEANUP_FAILED: security-gate-fail
  GPH_MILX_NETWORK_FORBIDDEN: terminate-and-security-gate-fail
  GPH_MILX_MCP_FORBIDDEN: terminate-and-security-gate-fail
response_error_fields: [code, stage, retryable, hint, sanitized_summary]
raw_error_or_payload: forbidden
```

## Persistencia derivativa

```toon
doc_id: CT-MILX-V1
block_id: CT-MILX-V1.analysis-cache
kind: cache-contract
source_of_truth: DB-SYMBOL-EDGE-GRAPH
verify:
  - mi-lsp nav wiki validate-source --workspace mi-lsp --format toon
evidence:
  - .docs/wiki/09_contratos/CT-MILX-V1.md
  - .docs/wiki/06_pruebas/TP-GPH.md
cache_key:
  - generation_id
  - extension_id
  - extension_version
  - executable_sha256
  - operation
  - parameters_digest
  - authority_profile_digest
  - output_schema
cache_value:
  - result_digest
  - bounded_result_json
  - provenance
  - omissions
  - status
invalidation: any-key-component-change
cache-authority: none
```

Graphify Import solo emite analysis/import advisory bajo `external:graphify`. Stores, Algorithms, Visual, Documents, Languages y Remote Snapshot usan el mismo host y no obtienen privilegios nuevos por familia.

## Compatibilidad, seguridad y pruebas

- Minor additions require optional fields and unchanged framing; incompatible wire/schema behavior requires a new protocol version.
- A failed extension never changes CLI/core availability or active generation.
- `TC-GPH-043..048` validate framing, manifest, capability denial, timeout/crash/cancel, malformed/oversize, cleanup, no-write, no-network and no-MCP per RID.
- Release-visible changes require `[[AE-RELEASE-DISTRIBUTION]]`, binary provenance and cross-RID evidence.

## Sync

RF owners: `[[RF-GPH-010]]`, `[[RF-GPH-011]]`. Runtime: `[[TECH-GRAPH-NATIVE]]`. Cache/store: `[[DB-SYMBOL-EDGE-GRAPH]]`. Tests: `[[TP-GPH]]`.
