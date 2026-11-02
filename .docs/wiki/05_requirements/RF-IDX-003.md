---
wiki_contract: WIKI-CONTRACT-v1
id: RF-IDX-003
kind: requirement
status: active
title: Proyectar y sincronizar el read-model desde el gobierno documental
links:
  implements: [FL-IDX-01]
  code:
    - path: internal/docgraph/wiki_contract_governance.go
      role: behavior
    - path: internal/docgraph/wiki_contract_projection.go
      role: behavior
    - path: internal/docgraph/governance.go
      role: compatibility
    - path: internal/indexer/incremental.go
      role: behavior
---

## Propósito

Proyectar un contrato de lectura/edición ejecutable y versionado desde la autoridad documental. Actores: mantenedor de wiki, skill y CLI/core. Prioridad y severidad altas. El TOML nunca redefine la autoridad de governance.

## Comportamiento ordenado

1. Resolver la fuente real de gobierno y leer `00_governance.md` como machine-YAML WIKI-CONTRACT-v1.
2. Leer los traits cerrados y sus reasons, ui_roots solo bajo has_ui y las excepciones explícitas de capa.
3. Derivar las capas y superficies por kind; no pedir hierarchy, profile ni overlays a esta wiki.
4. Materializar `<wiki>/_mi-lsp/read-model.toml`, con `projection_source=wiki-contract-v1/traits` y `agent_surface` por kind.
5. Informar drift y, cuando el destino sea seguro y escribible y se haya solicitado sincronización, escribir la proyección. Drift y fallo de escritura v1 son advisory; no se los transforma en un gate legacy.
6. Informar freshness del índice. Cambiar gobierno/proyección requiere reindexación posterior autorizada antes de confiar en el índice documental; BUILD no la ejecuta.

El read-model de un canon se interpreta respecto de su raíz propietaria. La precedencia de proyección local no cambia. No se escribe una raíz ajena read-only. Una fuente ausente o ambigua no se inventa.

## Superficies

`mi-lsp nav governance`, `workspace status` y la proyección consumida por navegación documental. La escritura de sincronización no convierte una consulta read-only en una autorización para mutar otro canon.

## Entradas/salidas tipadas

- Entrada v1: `wiki_contract`, `version`, `traits`, `layer_exceptions`.
- Salida: `DocsReadProfile` y `DocsGovernanceProfile`; `agent_surface` contiene `kind`, `read_globs`, `edit_globs`.
- La representación interna conserva campos históricos como profile/effective_base por compatibilidad; no son parámetros de autoría v1.
- La proyección queda versionada en este repo. Con raíces declaradas se preservan los límites de RF-WKS-008.

## Errores

La ausencia o invalidez de la fuente y las rutas inseguras se explican mediante el diagnóstico correspondiente. En v1 una proyección stale o no escribible produce warning y no se inventa éxito de sincronización. `index_sync=stale` sigue siendo evidencia de freshness insuficiente.

### Compatibilidad de entrada legacy conservada

Para repos aún no migrados, la herramienta conserva el schema común estricto y la compilación de `ordered_wiki`, `spec_backend`, `spec_full` y `custom` a base + overlays; custom debe extender un perfil canónico. Ese comportamiento no gobierna esta wiki v1.

Los errores legacy originales siguen siendo: `IDX_GOV_DOC_MISSING` (falta gobierno: bloquear y explicar reparación), `IDX_GOV_YAML_INVALID` (parse/schema: bloquear y listar issues), `IDX_GOV_PROJECTION_WRITE_FAILED` (permisos/ruta: error accionable). El gate legacy no permite continuar con superficies docs-first sobre un índice stale. Se conserva esa semántica como compatibilidad, no se copia su metadata SDD en headers v1.

## Criterios de aceptación (numerados)

1. La fuente v1 se lee desde el gobierno real como machine-YAML; la fuente no se sintetiza desde menciones.
2. Traits y kind determinan capas/superficies; has_ui agrega DESIGN.md, PRODUCT.md y ui_roots seguros; stores_data agrega DM/DB; exposes_api agrega CT; sold_product ausente no agrega negocio.
3. La proyección lleva el marcador wiki-contract-v1/traits y superficies read/edit por kind, sin declarar perfiles legacy en governance.
4. El TOML es derivado y versionado; el diagnóstico no afirma sincronización si la escritura falla.
5. La sincronización solo escribe un destino seguro del workspace de código, nunca un canon ajeno read-only.
6. Los cambios de fuente/proyección dejan visible la necesidad de reindexación; drift de proyección v1 permanece advisory y no acredita freshness.

```yaml
harness_protocol: SDD-HARNESS-v1
id: RF-IDX-003
kind: requirement
audience: llm-first
imports:
- .docs/wiki/04_flows/FL-IDX-01.md
exports:
- RF-IDX-003
- Proyectar y sincronizar el read-model desde el gobierno documental
agent_must_read:
- .docs/wiki/05_requirements/RF-IDX-003.md
- .docs/wiki/04_flows/FL-IDX-01.md
- .docs/wiki/06_tests/TP-IDX.md
agent_may_edit:
- .docs/wiki/05_requirements/RF-IDX-003.md
agent_must_not_edit:
- .docs/wiki/00_governance.md
- AGENTS.md
- internal/docgraph/wiki_contract_governance.go
- internal/docgraph/wiki_contract_projection.go
- internal/docgraph/governance.go
- internal/indexer/incremental.go
verify:
- Validar el contrato y sus referencias con `mi-lsp nav wiki validate-harness --workspace
  <alias>`; contrastar los casos de .docs/wiki/06_tests/TP-IDX.md sin inferir que
  fueron ejecutados.
stop_if:
- Detenerse si la propuesta requiere alterar criterios, decisiones o comportamiento
  más allá de RF-IDX-003.
evidence:
- .docs/wiki/05_requirements/RF-IDX-003.md
- .docs/wiki/06_tests/TP-IDX.md
```
