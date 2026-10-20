---
wiki_contract: WIKI-CONTRACT-v1
id: RS-WIKI-001
kind: outcome
status: active
title: "Recorrer la wiki y el código en una generación, respetando su idioma"
---

## Resultado observable
El consumidor recorre promesas, requisitos, pruebas y código sin abrir documentos para reconstruir relaciones. Los títulos y criterios se interpretan según el idioma declarado; los diagramas ausentes se avisan sin bloquear.

## Beneficiario
Agentes y personas que usan la CLI o el proveedor de mi-lsp desde codemode.

## Criterios de éxito
- Vecinos, caminos, impacto y trace consumen el grafo generacional wiki+código.
- La API devuelve JSON tipado, paginación por cursor y Mermaid efímero.
- validate-contract y matrix reconocen es/en; una declaración inválida no se traduce silenciosamente.

## Fuera de alcance del RS
Otro almacén de relaciones, lógica de dominio en mi-mcp, modelos de IA, gates de diagramas y sincronización de trackers. El respaldo no canónico del roadmap se rige por RS-ROADMAP-001.
