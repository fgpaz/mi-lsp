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

## Consumidor
Soporta [[RF-ALLHARNESS-001]]. Fuente: pedido explícito de Gabriel `home/ideas/mi-lsp-para-todos-los-harness.md`, 2026-10-20; persona:herramientas-pocas-enganchables-20261020. Tipo de fuente: decisión del operador; no evidencia runtime.

## Operaciones

### Extensión opcional model_alias/v0 en provider v2

```json
{"name":"diagnostics","model_alias":{"namespace":"lsp","name":"diagnostics"},"effect":"read"}
```

C165/H3 implementa `model_alias` genéricamente por operación; ausencia conserva comportamiento anterior. Este documento especifica metadata del provider, no acredita que una CLI129 vieja ejecute el alias. Namespace/name son identificadores JS seguros; nombres peligrosos se omiten. El nombre canónico de operación, CLI aprobada, mapping, permisos y schema no cambian. Ejemplo `multi-read` → `lsp.read`: payload host sigue `comando: multi-read`.

Las invocaciones v2 por operación usan `invocation.input_mapping.fields` existente (argument/option/repeat), no otro mini-lenguaje.

## Errores
El seam H3 inspeccionado rechaza argumentos con guion inicial y no demuestra soporte de delimitador final en fixed_args. Por eso el provider no añade `--`: patrones ordinarios conservan el mapping vigente; nombres con guion usan CLI directa `find-files -- '-draft'`. Necesidad genérica de delimitación pendiente de C165, sin implementación paralela por surface. Nombres peligrosos se omiten.

## Compatibilidad
Los aliases se añaden sin sustituir métodos existentes. `fs.find` extiende el objeto helper fs; no reemplaza fs.read/glob. Colisiones no sustituyen built-ins. Un alias no autoriza otra CLI ni evita perfil/PEP. El catálogo completo conserva los nombres originales. No se agrega otro schema ni excepciones de dominio.

## Workspace explícito en Code Mode

El consumidor propaga el workspace real resuelto (alias o raíz física) por el campo `workspace` ya publicado. El cwd del subproceso provider puede ser temporal y el `last_workspace` global no identifica el proyecto del llamador. Se conserva el schema opcional y los defaults de la CLI local directa: no se añade auto-resolver, registro/indexación, cambio de scopes ni hooks. Scope ausente/inválido o diagnostics parcial/no disponible no equivale a cero errores. Según la evidencia runtime aportada por el padre, Pi nativo devolvió TS2322 con `workspace` explícito; esta aclaración conserva los permisos y efectos de solo lectura.

## Distribución compatible para el backend TypeScript

`internal/worker/tsserver.go` (`findTsserverPath`/`CanUseTsserver`) requiere una distribución con `node_modules/typescript/lib/tsserver.js`. La oficial `typescript@7.0.2` es solo compilador (`bin/tsc`, sin servidor JS). La clásica `typescript@6.0.3` contiene `bin/tsc`, `bin/tsserver` y `lib/tsserver.js`; instalada normalmente, permitió obtener TS2322 real según la evidencia del padre. Ejemplo compatible: `npm install -g typescript@6.0.3`. Una distribución local compatible también sirve. Este requisito aplica solo al uso del backend TypeScript, no impone TypeScript ni una instalación global a cada host o proyecto.

[[TECH-ALLHARNESS-001]] · [[TP-ALLHARNESS-001]]
