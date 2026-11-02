---
wiki_contract: WIKI-CONTRACT-v1
id: CT-NAV-EDIT-PLAN
kind: contract
status: active
title: CT-NAV-EDIT-PLAN
links:
  supports:
  - RF-QRY-018
---

```yaml
harness_protocol: SDD-HARNESS-v1
id: CT-NAV-EDIT-PLAN
kind: cli-contract
audience: dual
imports: [.docs/wiki/07_technical.md]
exports: [CT-NAV-EDIT-PLAN]
agent_must_read: [.docs/wiki/07_technical/contracts/CT-NAV-EDIT-PLAN.md, internal/service/edit_plan.go]
agent_may_edit: [.docs/wiki/07_technical/contracts/CT-NAV-EDIT-PLAN.md]
agent_must_not_edit: [internal/service/edit_plan.go, .docs/wiki/06_tests/]
verify: ["Leer .docs/wiki/07_technical/contracts/CT-NAV-EDIT-PLAN.md y .docs/wiki/07_technical.md; cotejar las afirmaciones con la fuente publicada internal/service/edit_plan.go y sus límites. Registrar únicamente comandos y resultados efectivamente ejecutados; esta especificación no afirma una ejecución."]
stop_if: [Apply se habilita sin ambos flags, el lenguaje carece de backend AST real o target/evidencia/scope no valida.]
evidence: [.docs/wiki/07_technical/contracts/CT-NAV-EDIT-PLAN.md, .docs/wiki/07_technical.md, internal/service/edit_plan.go]
```

## Consumidor

CLI, agentes y consumidores locales de las operaciones descritas; no se amplía la autoridad del consumidor.

## Operaciones

Se conservan requests, envelopes, campos, límites y semántica detallados a continuación.

### Invocacion

```powershell
mi-lsp nav edit-plan --stdin --workspace <alias> --format toon
mi-lsp nav edit-plan --packet <file> --workspace <alias> --format toon
mi-lsp nav edit-plan --stdin --strict --include-content --workspace <alias> --format json
mi-lsp nav edit-plan --packet <file> --apply --experimental-apply --workspace <alias> --format toon
```

### Semántica

`nav edit-plan` convierte un packet `edit-plan-v1` o `edit-plan-v2` en un diff determinista. Dry-run es el comportamiento default y no escribe archivos. Apply es experimental y solo se habilita con doble opt-in: `--apply --experimental-apply`.

La ejecución no depende del daemon. Resuelve el workspace como query directa, valida paths contra el root, bloquea rutas sensibles y devuelve un envelope estable para que agentes puedan revisar el diff antes de tocar archivos.

`edit-plan-v1` conserva operaciones textuales. `edit-plan-v2` agrega contrato multi-lenguaje y operaciones AST language-aware; en esta version solo Go tiene backend AST real. C#, TypeScript y Python se reconocen por contrato pero deben devolver `language_not_supported` para operaciones AST, con hint hacia `edit-plan-v1` textual o un backend futuro.

### Packet schema v1

```json
{
  "version": "edit-plan-v1",
  "intent": "short human intent",
  "base_ref": "optional git ref",
  "targets": [
    {
      "id": "target-main",
      "path": "internal/service/example.go",
      "range": {"start_line": 1, "end_line": 40},
      "expected_hash": "sha256:<hex>",
      "symbol": {"name": "Example", "kind": "func"}
    }
  ],
  "operations": [
    {
      "id": "op-1",
      "kind": "replace_literal",
      "target_id": "target-main",
      "find": "old",
      "replace": "new",
      "max_replacements": 1
    }
  ],
  "constraints": {
    "require_clean_match": true,
    "require_evidence": true,
    "deny_paths": [".docs/generated/**"],
    "max_file_bytes": 1000000,
    "max_diff_chars": 200000
  }
}
```

### Packet schema v2

```json
{
  "version": "edit-plan-v2",
  "intent": "short human intent",
  "base_ref": "optional git ref",
  "targets": [
    {
      "id": "target-go",
      "path": "internal/service/example.go",
      "language": "go",
      "expected_hash": "sha256:<hex>",
      "symbol": {
        "name": "Example",
        "kind": "function",
        "receiver": "",
        "signature": "func Example(ctx context.Context) error"
      }
    }
  ],
  "operations": [
    {
      "id": "op-1",
      "kind": "replace_go_function_body",
      "target_id": "target-go",
      "content": "return nil\n"
    }
  ],
  "constraints": {
    "require_clean_match": true,
    "require_evidence": true,
    "deny_paths": [".docs/generated/**"],
    "max_file_bytes": 1000000,
    "max_diff_chars": 200000
  }
}
```

`targets[].language` acepta `go`, `csharp`, `typescript` y `python`; puede omitirse cuando la extension permite inferirlo (`.go`, `.cs`, `.ts/.tsx/.js/.jsx`, `.py`). `targets[].symbol` identifica el nodo estructural; `receiver` desambigua metodos Go y `signature` puede restringir el match cuando sea necesario.

### Operaciones v1 textuales

- **Kind**: `replace_literal`; **Campos**: `find`, `replace`, `max_replacements?`; **Regla**: Reemplaza literal dentro del target range; si `max_replacements` existe, el match count no puede excederlo
- **Kind**: `replace_regex_limited`; **Campos**: `find`, `replace`, `max_replacements`; **Regla**: Reemplaza regex no multilinea; `max_replacements` es obligatorio
- **Kind**: `insert_before`; **Campos**: `content`; **Regla**: Inserta antes del target range
- **Kind**: `insert_after`; **Campos**: `content`; **Regla**: Inserta después del target range
- **Kind**: `delete_range`; **Campos**: -; **Regla**: Borra el target range
- **Kind**: `replace_range`; **Campos**: `content`; **Regla**: Reemplaza el target range completo

### Operaciones v2 AST

- **Kind**: `replace_go_function`; **Lenguaje**: Go; **Campos**: `symbol.name`, `content`, `symbol.receiver?`, `symbol.signature?`; **Regla**: Reemplaza la declaracion completa de la funcion/metodo y formatea el archivo con `go/format` interno
- **Kind**: `replace_go_function_body`; **Lenguaje**: Go; **Campos**: `symbol.name`, `content`, `symbol.receiver?`, `symbol.signature?`; **Regla**: Reemplaza solo el cuerpo entre llaves y conserva la firma
- **Kind**: `insert_go_function_after`; **Lenguaje**: Go; **Campos**: `symbol.name`, `content`, `symbol.receiver?`, `symbol.signature?`; **Regla**: Inserta una funcion Go completa después de la funcion/metodo target
- **Kind**: `ensure_go_import`; **Lenguaje**: Go; **Campos**: `import_path` o `content`, `import_alias?`; **Regla**: Agrega import si falta; es idempotente
- **Kind**: `remove_go_import`; **Lenguaje**: Go; **Campos**: `import_path` o `content`; **Regla**: Remueve import si existe; es idempotente

Las operaciones AST v2 sobre `csharp`, `typescript` o `python` fallan con error tipado `language_not_supported`. No deben degradar a regex ni aplicar heuristicas textuales implicitas.

### Envelope

```json
{
  "ok": true,
  "workspace": "mi-lsp",
  "backend": "edit-plan",
  "mode": "dry_run",
  "items": [
    {
      "patch_packet": {"version": "edit-plan-v1"},
      "diff": "diff --git a/path b/path\n...",
      "files_changed": 1,
      "operations": [
        {"id": "op-1", "kind": "replace_literal", "target_id": "target-main", "path": "path", "status": "ok", "replacements": 1}
      ],
      "evidence": [
        {"kind": "target_hash", "path": "path", "value": "sha256:<hex>"},
        {"kind": "target_range", "path": "path", "value": "1-40"}
      ],
      "guardrails": [
        {"code": "dry_run_default", "status": "active"},
        {"code": "go_ast_only", "status": "active"}
      ],
      "apply_status": {
        "requested": false,
        "applied": false,
        "rollback": false,
        "message": "dry-run only; no files were written"
      }
    }
  ],
  "truncated": false
}
```

### Apply guardrails

- `--apply` sin `--experimental-apply` debe fallar antes de escribir.
- El workspace git debe estar limpio antes de cualquier escritura.
- Todos los targets deben tener `expected_hash` y el hash debe revalidarse justo antes de escribir.
- `.git/**`, `.mi-lsp/**`, `.docs/wiki/_mi-lsp/read-model.toml`, binarios y `constraints.deny_paths` quedan bloqueados.
- No se permite path traversal ni symlink que escape del workspace.
- No se permiten operaciones solapadas sobre el mismo target range.
- El comando no stagea, commitea, formatea, renombra, cambia chmod ni borra directorios.
- Si una escritura falla, el runtime debe restaurar los bytes anteriores de archivos ya tocados cuando sea posible.
- Para `edit-plan-v2`, los mismos guardrails aplican antes y después de la transformacion AST: hashes esperados, paths seguros, diff generado en la misma ejecución y git limpio para escritura.

### `nav prepare` — semantic-preparation-evidence/v1

`nav prepare <task>` is an additive, read-only preparation surface. It resolves the canonical workspace root, computes task/governance/index-generation digests, invokes route and pack services in-process, and returns timing evidence. It never enables `edit-plan --apply` and never infers allowed paths from an implicit git diff.

Input may include `affected_paths` or an explicit edit-plan JSON (`plan`, `packet`, or `edit_plan`). Without either, `allowed_paths` is empty and the response includes a warning. Direct and daemon envelopes use the same item schema; only transport routing metadata may differ. Daemon cache keys include canonical root, task digest, governance digest, index generation, and plan digest when present.

- **Marcador estable**: CT-NAV-EDIT-PLAN.semantic-preparation-v1
- **description**: Read-only semantic preparation evidence
- **command_signature**: mi-lsp nav prepare <task> [--affected path...] [--plan file]
- **result_schema**: semantic-preparation-evidence/v1
- **mutation_authority**: false
- **allowed_paths_source**: explicit affected_paths or supplied edit-plan targets only
- **failure_taxonomy**:
  - workspace_resolution_failed
  - workspace_root_unavailable
  - task_required
  - governance_unavailable
  - governance_blocked
  - plan_invalid
  - plan_path_invalid
  - affected_path_invalid
- **cache_invalidation**:
  - canonical_root
  - task_digest
  - governance_digest
  - active_sqlite_generation_snapshot
  - plan_digest
  - affected_paths_digest
- **latency_evidence**: total_ms and stats.ms remain measured on failure; deterministic sampling uses N>=5 observations and reports p50/p95 without wall-clock thresholds

## Errores

Se mantienen las razones, warnings, rechazos y fallbacks explícitos de cada operación.

## Compatibilidad

La migración cambia el contrato documental, no el protocolo runtime ni sus versiones. Los esquemas legacy que la herramienta consume continúan como compatibilidad de entrada, no como formato de autoría de esta wiki.

### Compatibilidad

`edit-plan-v2` es aditivo y no rompe `edit-plan-v1`. Los clientes que ignoran `guardrails`, `evidence` o `apply_status` siguen pudiendo consumir `diff` y `files_changed`. Los packet `edit-plan-v1` textuales siguen siendo la ruta recomendada para C#, TypeScript y Python hasta que existan backends AST especificos.
