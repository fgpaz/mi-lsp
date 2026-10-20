---
wiki_contract: WIKI-CONTRACT-v1
id: TP-ALLHARNESS-001
kind: test
status: draft
source_type: operator_decision
links:
  verifies: [RF-ALLHARNESS-001]
---

# TP-ALLHARNESS-001 — mi-lsp para todos los harness

## Alcance
Verifica [[RF-ALLHARNESS-001]] criterios 1–6.

Fuente: pedido explícito de Gabriel `home/ideas/mi-lsp-para-todos-los-harness.md`, 2026-10-20; persona:herramientas-pocas-enganchables-20261020. Tipo de fuente: decisión del operador; no evidencia runtime.

Estado: plan de aceptación, **sin evidencia de ejecución en BUILDING**.

## Casos

- FINAL_VERIFY: invocar `lsp.diagnostics` y `fs.find` en execute de pi y Claude Code en cada host objetivo. Comprobar target real, respuesta y errores; listar herramientas/plugin instalado no demuestra ejecución.
- Después de editar fixture con error conocido: diagnóstico debe encontrarlo o reportar backend/degradación explícita, nunca falso limpio.
- Encontrar archivo ignorado/no ignorado y path acotado con find; comprobar límite y JSON completo.
- Comparar CLI/help completo con subconjunto modelo y llamadas legacy. Alias fs.find debe convivir con fs.read/glob; permisos siguen aplicando al target.
- Tabla antes/después por modelo (minimax/longcat), tarea y host: tokens, aciertos, errores capturados antes del test, latencia y fallos. Datos ausentes: NOT_MEASURED, nunca porcentajes inventados.
- Skill única cargada y usada en pi/Claude Code/Codex/PiG; selección por usuario y recarga de proceso documentadas.
- TS: observar backend2322 → corregir fixture → 0 sin stale. El padre acredita el smoke manual execute por host/harness; solo jefe-harness-web acredita el enganche tras-edit real al integrar su adaptador. No confundir con H1Yoga binding39 de C165 ni declarar alias PASS desde CLI129.

Evidencia final sanitizada a cargo del padre; [[TECH-ALLHARNESS-001]].
