---
wiki_contract: WIKI-CONTRACT-v1
id: RF-ALLHARNESS-001
kind: requirement
status: draft
source_type: operator_decision
links:
  implements: [FL-ALLHARNESS-001]
---

# RF-ALLHARNESS-001 — mi-lsp para todos los harness

## Propósito
Fuente: pedido explícito de Gabriel `home/ideas/mi-lsp-para-todos-los-harness.md`, 2026-10-20; persona:herramientas-pocas-enganchables-20261020. Tipo de fuente: decisión del operador; no evidencia runtime.

Realiza [[FL-ALLHARNESS-001]], satisface [[RS-ALLHARNESS-001]].

## Comportamiento ordenado

1. Default modelo: intent, read, search, refs, diagnostics y find. Descripción breve y presupuesto explícito; no catálogo nav completo en la skill inicial.
2. Un solo provider-manifest v2 define schema y argv. mi-mcp consume extensión opcional `model_alias`, genérica y sin lógica mi-lsp.
3. `milsp.*`, comandos CLI y APIs avanzadas se preservan; `nav --help` y `provider-manifest` permiten descubrimiento completo.
4. Después de editar, la skill guía diagnostics sin hook nuevo. Backend ausente o parcial jamás significa cero errores.
5. La configuración de usuario y permisos se resuelve en el host, no en el repo público; aliases no agregan grants.
6. Un mismo contenido de skill se distribuye mediante mecanismos existentes a los cuatro harness; no InstallAll ni escritura de caches durante BUILD.

## Superficies
Las superficies por defecto del modelo son intent, read, search, refs, diagnostics y find; la skill inicial no expone el catálogo nav completo.

## Entradas/salidas tipadas
Un solo provider-manifest v2 define schema y argv. mi-mcp consume la extensión opcional `model_alias`, genérica y sin lógica mi-lsp; `milsp.*`, comandos CLI y APIs avanzadas se preservan.

## Errores
Después de editar, la skill guía diagnostics sin hook nuevo; un backend ausente o parcial jamás significa cero errores.

## Criterios de aceptación
1. La configuración de usuario y permisos se resuelve en el host, no en el repo público; aliases no agregan grants.
2. Un mismo contenido de skill se distribuye mediante mecanismos existentes a los cuatro harness; no InstallAll ni escritura de caches durante BUILD.

Verifica [[TP-ALLHARNESS-001]]. Contrato [[CT-ALLHARNESS-001]].
