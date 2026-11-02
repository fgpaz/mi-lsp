---
wiki_contract: WIKI-CONTRACT-v1
id: TECH-GOVERNANCE-PROFILES
kind: technical
status: active
title: TECH-GOVERNANCE-PROFILES
links:
  supports: [RF-IDX-003, RF-WKS-005, RF-WKS-008]
  code:
  - path: internal/service/ask.go
    role: implementation
  - path: internal/service/pack.go
    role: implementation
  - path: internal/service/workspace_ops.go
    role: implementation
---

```yaml
harness_protocol: SDD-HARNESS-v1
id: TECH-GOVERNANCE-PROFILES
kind: technical-governance
audience: dual
imports: [.docs/wiki/07_technical/TECH-DOCUMENTARY-GOVERNANCE.md, .docs/wiki/07_technical/contracts/CT-GOVERNANCE-AE-KERNEL-V2.md]
exports: [TECH-GOVERNANCE-PROFILES]
agent_must_read: [.docs/wiki/07_technical/TECH-GOVERNANCE-PROFILES.md, internal/docgraph/wiki_contract_governance.go, internal/docgraph/wiki_contract_projection.go]
agent_may_edit: [.docs/wiki/07_technical/TECH-GOVERNANCE-PROFILES.md]
agent_must_not_edit: [internal/docgraph/, .docs/wiki/_mi-lsp/read-model.toml, .docs/ae/repo-policy.yaml]
verify: ["Leer .docs/wiki/07_technical/TECH-GOVERNANCE-PROFILES.md y .docs/wiki/07_technical/TECH-DOCUMENTARY-GOVERNANCE.md; cotejar las afirmaciones con la fuente publicada internal/docgraph/wiki_contract_governance.go y sus límites. Registrar únicamente comandos y resultados efectivamente ejecutados; esta especificación no afirma una ejecución."]
stop_if: [El cambio convierta perfiles legacy, projection o ae/ histórico en nueva autoridad normativa.]
evidence: [.docs/wiki/07_technical/TECH-GOVERNANCE-PROFILES.md, .docs/wiki/07_technical/TECH-DOCUMENTARY-GOVERNANCE.md, internal/docgraph/wiki_contract_governance.go]
```

## Resumen

Gobernanza por traits + kind para WIKI-CONTRACT-v1, con compatibilidad de lectura de perfiles legacy de otros repositorios. El ID estable de esta ficha no convierte los perfiles anteriores en autoridad de esta wiki.

## Mapa de componentes

- `internal/docgraph/wiki_contract_governance.go` lee machine-YAML, traits y excepciones.
- `internal/docgraph/wiki_contract_projection.go` deriva familias y superficies read/edit por kind, con marcador `wiki-contract-v1/traits`.
- `internal/docgraph/governance.go` conserva el compilador de perfiles legacy para documentos todavía no migrados.
- `internal/service/ask.go`, `pack.go` y `workspace_ops.go` consumen la resolución y sus diagnósticos.

### Modelo v1 activo

`00_governance.md` es machine-YAML, sin id/kind/status ni links. Los únicos traits posibles son has_ui, stores_data, exposes_api y sold_product. Cada trait habilitado tiene reason; ui_roots solo se declara bajo has_ui. Las excepciones requieren layer, reason, authorized_by y date. No se declara negocio sin autoridad y sold_product.

La proyección deriva del modelo inmutable; no exige perfiles, overlays, hierarchy ni cadenas manuales. `profile=product` y `effective_base=product` pueden aparecer como representación interna generada, no como decisión de gobierno. Drift o destino no escribible es advisory en v1; freshness y seguridad del canon siguen visibles por separado.

### Compatibilidad de entrada legacy: invariantes

- `00_governance.md` es la autoridad humana.
- El bloque YAML embebido es la fuente estructurada.
- `.docs/wiki/_mi-lsp/read-model.toml` es una proyección versionada, obligatoria y auto-sincronizable.
- El perfil visible se compila internamente a `base + overlays`.
- Si la gobernanza esta ambigua, inválida o stale, el workspace queda bloqueado para superficies docs-first.
- Durante la transicion a `WIKI-CONTRACT-v1`, `mi-lsp nav wiki validate-contract` revisa encabezados y `links.*` del contrato; emite solo advertencias (no bloquea ni sale con código distinto de cero). Convive con `validate-harness` hasta el corte.

### Compatibilidad de entrada legacy: perfiles visibles

- `ordered_wiki`
- `spec_backend`
- `spec_full`
- `custom`

### Compatibilidad de entrada legacy: compilación interna

- `ordered_wiki` -> base `ordered_wiki`
- `spec_backend` -> base `ordered_wiki` + overlays `spec_core`, `technical`
- `spec_full` -> base `ordered_wiki` + overlays `spec_core`, `technical`, `uxui`
- `custom` -> extiende un perfil canónico y agrega overlays dentro del schema validado

### Compatibilidad de entrada legacy: gate operativo

- `workspace status` siempre expone estado de gobernanza
- `nav governance` siempre esta permitido
- `nav ask` y `nav pack` bloquean cuando la gobernanza no es valida
- Si la proyección cambia, el workspace debe reindexarse antes de continuar con consultas docs-first

### Doctor legacy de capas de producto (D-086, solo aviso)

- `mi-lsp` lee `.docs/ae/repo-policy.yaml` (`repo.kind`, `exceptions`) con ids alineados al setup AE: `scope`, `outcome`, `architecture`, `flow`, `requirement`, `test_plan`, `technical`.
- Para `repo.kind: product`, `nav governance` y `workspace status` incluyen `wiki_layers` (schema `ae-wiki-layers-doctor/v1`) y avisos `wiki_layer_gap:<id>` cuando falta contenido real o solo hay esqueleto `<!-- ae-skeleton: pending -->`.
- `repo.kind` ausente agrega `repo_kind_undeclared`; `kernel` y `docs` no exigen capas de producto.
- Las excepciones registradas (`capa`, `motivo`, `autorizo`, `fecha`) cubren una capa sin bloquear navegación ni `nav ask`/`nav pack`.

### Validación del Kernel v2

- El canon universal se resuelve desde `<kernel_home>/canon`; no se duplica en el repositorio.
- `.docs/ae/repo-policy.yaml` declara un alias exacto en `tracker.provider` (`Linear`, `Plane`, `Azure Boards`, `Jira` o `None`) y exactamente un bloque correspondiente (`linear`, `plane`, `azure_boards`, `jira` o `none`).
- `None` solo requiere `tracker.none.mode: local-only`; no requiere ni admite campos de proveedores externos o aliases legacy.
- Un proveedor externo requiere dentro de su bloque `base_url`, `workspace`, `key_env` (solo nombre de variable de entorno) y al menos un proyecto con `key`.
- Los aliases neutrales `tracker.base_url`, `tracker.conf_file`, `tracker.key_env`, `tracker.workspace` y `tracker.projects` son inválidos porque impiden un render determinístico entre providers.
- `repo.structure_rules` debe contener al menos una regla concreta del repositorio.

### Artefactos afectados

- `.docs/ae/repo-policy.yaml`
- `.docs/wiki/00_governance.md`
- `.docs/wiki/_mi-lsp/read-model.toml`
- `internal/docgraph/*`
- `internal/service/ask.go`
- `internal/service/pack.go`
- `internal/service/workspace_ops.go`
