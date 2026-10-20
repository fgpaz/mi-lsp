---
wiki_contract: WIKI-CONTRACT-v1
id: CT-ALLHARNESS-001
kind: contract
status: draft
source_type: operator_decision
links:
  supports: [RF-ALLHARNESS-001]
---

# CT-ALLHARNESS-001 — mi-lsp para todos los harness

## Extensión opcional model_alias/v0 en provider v2
Soporta [[RF-ALLHARNESS-001]]. Fuente: pedido explícito de Gabriel `home/ideas/mi-lsp-para-todos-los-harness.md`, 2026-10-20; persona:herramientas-pocas-enganchables-20261020. Tipo de fuente: decisión del operador; no evidencia runtime.

```json
{"name":"diagnostics","model_alias":{"namespace":"lsp","name":"diagnostics"},"effect":"read"}
```

C165/H3 implementa `model_alias` genéricamente por operación; ausencia conserva comportamiento anterior. Este documento especifica metadata del provider, no acredita que una CLI129 vieja ejecute el alias. Namespace/name son identificadores JS seguros; nombres peligrosos se omiten. El nombre canónico de operación, CLI aprobada, mapping, permisos y schema no cambian. Ejemplo `multi-read` → `lsp.read`: payload host sigue `comando: multi-read`.

Los aliases se añaden sin sustituir métodos existentes. `fs.find` extiende el objeto helper fs; no reemplaza fs.read/glob. Colisiones no sustituyen built-ins. Un alias no autoriza otra CLI ni evita perfil/PEP. El catálogo completo conserva los nombres originales.

Invocaciones v2 por operación usan `invocation.input_mapping.fields` existente (argument/option/repeat), no otro mini-lenguaje. El seam H3 inspeccionado rechaza argumentos con guion inicial y no demuestra soporte de delimitador final en fixed_args. Por eso el provider no añade `--`: patrones ordinarios conservan el mapping vigente; nombres con guion usan CLI directa `find-files -- '-draft'`. Necesidad genérica de delimitación pendiente de C165, sin implementación paralela por surface. No agregar otro schema ni excepciones de dominio.

[[TECH-ALLHARNESS-001]] · [[TP-ALLHARNESS-001]]
