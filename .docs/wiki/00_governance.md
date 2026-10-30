```yaml
wiki_contract: WIKI-CONTRACT-v1
model_revision: 1.1
version: 1
language: es
traits:
  has_ui:
    reason: "Consola de gobernanza loopback del daemon para workspaces, runtimes y logs."
    ui_roots: [internal/daemon/]
  stores_data:
    reason: "Persiste catálogo, documentos, grafo y embeddings en SQLite repo-local, y estado/telemetría del daemon."
  exposes_api:
    reason: "Expone CLI, protocolo daemon/worker y endpoints admin locales consumidos por agentes y clientes."
# sold_product ausente; reason: no hay producto vendido ni autoridad business/product declarada.
# La ausencia de la clave deshabilita el trait; declarar false habilitaría una capa indebida.
layer_exceptions: []
ae_canon:
  mode: kernel_v2
  source: <kernel_home>/canon
  repo_policy: .docs/ae/repo-policy.yaml
```

```yaml
harness_protocol: SDD-HARNESS-v1
id: 00_governance
kind: governance
audience: llm-first
imports: []
exports:
  - 00_governance
agent_must_read:
  - .docs/wiki/00_governance.md
agent_may_edit:
  - .docs/wiki/00_governance.md
agent_must_not_edit:
  - .docs/ae/repo-policy.yaml
  - <kernel_home>/canon/AE-KERNEL-V2.md
verify:
  - mi-lsp nav governance --workspace <alias> --format toon
  - mi-lsp nav wiki validate-harness --workspace <alias> --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
  - canon_authority_or_policy_would_change
evidence:
  - .docs/wiki/00_governance.md
```
