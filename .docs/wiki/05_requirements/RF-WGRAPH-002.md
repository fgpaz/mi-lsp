---
wiki_contract: WIKI-CONTRACT-v1
id: RF-WGRAPH-002
kind: requirement
status: active
title: "Comprobar secciones, criterios e ilustraciones según governance"
links:
  implements: [FL-WGRAPH-001]
  code:
    - path: internal/model/wiki_contract.go
    - path: internal/service/wiki_contract_validate.go
    - path: internal/service/contract_validate.go
    - path: internal/service/wiki_graph.go
    - path: internal/service/wiki_views.go
    - path: internal/docgraph/wiki_views.go
---

## Propósito
Leer el contrato en el idioma declarado sin ocultar declaraciones inválidas.

## Comportamiento ordenado
Se resuelve language desde la fuente efectiva de governance y se utilizan claves de sección estables con títulos es/en. El frontmatter tiene prioridad sobre fences posteriores. matrix y validate-contract comparten el parser localizado. Validate-contract selecciona documentos bajo las raíces gobernadas resueltas; un scope parcial valida destinos contra el corpus gobernado completo. El doctor informa la raíz efectiva y sus capas. Las comprobaciones no reparan ni traducen documentos.

## Superficies
nav wiki validate-contract y nav wiki matrix. checks incluye secciones, criterios, casos y findings tipados.

## Entradas/salidas tipadas
language: es|en. Ausencia: es, valid=true y language_missing_default_es. Valor inválido: valid=false, sin idioma efectivo, language_invalid. TP cita Criterio N o Criterion N según idioma; matrix conserva disposición y evidencia.

## Errores
Los avisos de idioma, secciones y diagramas son advisory. No se suprimen los checks previos de encabezados, destinos de links ni paths.

## Criterios de aceptación
1. Los títulos es/en y las referencias Criterio N/Criterion N se interpretan en validate-contract y matrix según language, sin admitir el otro idioma como cobertura.
2. Falta de language devuelve es con warning; un valor inválido no se convierte silenciosamente a es ni en criterios cubiertos.
3. Falta de C4Context/C4Container en ARCH y sequenceDiagram en FL produce findings diagram_missing con blocking=false; los checks anteriores siguen activos.
4. Un scope sin documentos es ADVISORY, nunca PASS vacuo; fuentes inválidas o esquema Setup desconocido no abren una raíz alternativa.
5. El registro de retiros admite `entries` y el YAML previo `retirements`: filas con `id`, `old_path`, `archived_path`, `reason`, `terminal` y `superseded_by` según el contrato técnico. La ausencia explícita y válida no inventa filas; filas inválidas, ambigüedad o contradicción permanecen ADVISORY.

```yaml
harness_protocol: SDD-HARNESS-v1
id: RF-WGRAPH-002
kind: requirement
audience: llm-first
imports:
- .docs/wiki/04_flows/FL-WGRAPH-001.md
exports:
- RF-WGRAPH-002
- '"Comprobar secciones, criterios e ilustraciones según governance"'
agent_must_read:
- .docs/wiki/05_requirements/RF-WGRAPH-002.md
- .docs/wiki/04_flows/FL-WGRAPH-001.md
- .docs/wiki/06_tests/TP-GPH.md
agent_may_edit:
- .docs/wiki/05_requirements/RF-WGRAPH-002.md
agent_must_not_edit:
- .docs/wiki/00_governance.md
- AGENTS.md
- internal/model/wiki_contract.go
- internal/service/wiki_contract_validate.go
- internal/service/contract_validate.go
- internal/service/wiki_graph.go
- internal/service/wiki_views.go
- internal/docgraph/wiki_views.go
verify:
- 'Para los casos de este requisito en .docs/wiki/06_tests/TP-GPH.md, ejecutar el
  comando publicado: go test ./internal/model/... ./internal/indexer/... ./internal/service/...
  ./internal/daemon/... (la evidencia de ejecución debe registrarse por separado).'
stop_if:
- Detenerse si la propuesta requiere alterar criterios, decisiones o comportamiento
  más allá de RF-WGRAPH-002.
evidence:
- .docs/wiki/05_requirements/RF-WGRAPH-002.md
- .docs/wiki/06_tests/TP-GPH.md
```
