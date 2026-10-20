---
wiki_contract: WIKI-CONTRACT-v1
id: TP-WIKI-001
kind: test
status: active
title: "Verificar el recorrido wiki+código con CLI real"
links:
  verifies: [RF-WGRAPH-001]
---

## Alcance
Piloto aislado con Git, módulo Go, símbolo real y wiki v1; indexado exclusivamente por la CLI nativa. No se escriben tablas SQL. Las lecturas de store para contar aristas son mode=ro. La evidencia persistente pertenece al paquete de cierre, no al canon.

## Casos
- Criterio 1 — disposición: manual; evidencia: declaraciones del fixture, vecinos por relación, inversos y conteos SQL de solo lectura con generación.
- Criterio 2 — disposición: manual; evidencia: JSON de neighbors RF, path RS→símbolo real, explain de edge y trace/ wiki trace con wiki-graph/v1 y la misma generación.
- Criterio 3 — disposición: manual; evidencia: diff real, affected/change-pack, index incremental nativo, generación antes/después y vecinos del link cambiado.
- Criterio 4 — disposición: manual; evidencia: JSON con Mermaid, hashes de wiki antes/después, cursor de página 1 consumido en página 2 y manifiesto de proveedor.
