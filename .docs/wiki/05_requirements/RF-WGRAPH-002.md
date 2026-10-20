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
Se resuelve language desde governance y se utilizan claves de sección estables con títulos es/en. matrix y validate-contract comparten el parser localizado. Las comprobaciones son de lectura; no reparan ni traducen documentos.

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
