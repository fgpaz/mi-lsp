# CT-NAV-EDIT-PLAN

```yaml
harness_protocol: SDD-HARNESS-v1
id: "CT-NAV-EDIT-PLAN"
kind: "wiki-doc"
audience: "llm-first"
imports:
  - '[[RF-QRY-018]]'
exports:
  - 'CT-NAV-EDIT-PLAN'
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/04_RF/RF-QRY-018.md
  - .docs/wiki/09_contratos/CT-NAV-EDIT-PLAN.md
agent_may_edit:
  - .docs/wiki/09_contratos/CT-NAV-EDIT-PLAN.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
evidence:
  - .docs/wiki/09_contratos/CT-NAV-EDIT-PLAN.md
```

## Invocacion

```powershell
mi-lsp nav edit-plan --stdin --workspace <alias> --format toon
mi-lsp nav edit-plan --packet <file> --workspace <alias> --format toon
mi-lsp nav edit-plan --stdin --strict --include-content --workspace <alias> --format json
mi-lsp nav edit-plan --packet <file> --apply --experimental-apply --workspace <alias> --format toon
```

## Semantica

`nav edit-plan` convierte un packet `edit-plan-v1` o `edit-plan-v2` en un diff determinista. Dry-run es el comportamiento default y no escribe archivos. Apply es experimental y solo se habilita con doble opt-in: `--apply --experimental-apply`.

La ejecucion no depende del daemon. Resuelve el workspace como query directa, valida paths contra el root, bloquea rutas sensibles y devuelve un envelope estable para que agentes puedan revisar el diff antes de tocar archivos.

`edit-plan-v1` conserva operaciones textuales. `edit-plan-v2` agrega contrato multi-lenguaje y operaciones AST language-aware; en esta version solo Go tiene backend AST real. C#, TypeScript y Python se reconocen por contrato pero deben devolver `language_not_supported` para operaciones AST, con hint hacia `edit-plan-v1` textual o un backend futuro.

## Packet schema v1

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

## Packet schema v2

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

## Operaciones v1 textuales

| Kind | Campos | Regla |
|---|---|---|
| `replace_literal` | `find`, `replace`, `max_replacements?` | Reemplaza literal dentro del target range; si `max_replacements` existe, el match count no puede excederlo |
| `replace_regex_limited` | `find`, `replace`, `max_replacements` | Reemplaza regex no multilinea; `max_replacements` es obligatorio |
| `insert_before` | `content` | Inserta antes del target range |
| `insert_after` | `content` | Inserta despues del target range |
| `delete_range` | - | Borra el target range |
| `replace_range` | `content` | Reemplaza el target range completo |

## Operaciones v2 AST

| Kind | Lenguaje | Campos | Regla |
|---|---|---|---|
| `replace_go_function` | Go | `symbol.name`, `content`, `symbol.receiver?`, `symbol.signature?` | Reemplaza la declaracion completa de la funcion/metodo y formatea el archivo con `go/format` interno |
| `replace_go_function_body` | Go | `symbol.name`, `content`, `symbol.receiver?`, `symbol.signature?` | Reemplaza solo el cuerpo entre llaves y conserva la firma |
| `insert_go_function_after` | Go | `symbol.name`, `content`, `symbol.receiver?`, `symbol.signature?` | Inserta una funcion Go completa despues de la funcion/metodo target |
| `ensure_go_import` | Go | `import_path` o `content`, `import_alias?` | Agrega import si falta; es idempotente |
| `remove_go_import` | Go | `import_path` o `content` | Remueve import si existe; es idempotente |

Las operaciones AST v2 sobre `csharp`, `typescript` o `python` fallan con error tipado `language_not_supported`. No deben degradar a regex ni aplicar heuristicas textuales implicitas.

## Envelope

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

## Apply guardrails

- `--apply` sin `--experimental-apply` debe fallar antes de escribir.
- El workspace git debe estar limpio antes de cualquier escritura.
- Todos los targets deben tener `expected_hash` y el hash debe revalidarse justo antes de escribir.
- `.git/**`, `.mi-lsp/**`, `.docs/wiki/_mi-lsp/read-model.toml`, binarios y `constraints.deny_paths` quedan bloqueados.
- No se permite path traversal ni symlink que escape del workspace.
- No se permiten operaciones solapadas sobre el mismo target range.
- El comando no stagea, commitea, formatea, renombra, cambia chmod ni borra directorios.
- Si una escritura falla, el runtime debe restaurar los bytes anteriores de archivos ya tocados cuando sea posible.
- Para `edit-plan-v2`, los mismos guardrails aplican antes y despues de la transformacion AST: hashes esperados, paths seguros, diff generado en la misma ejecucion y git limpio para escritura.

## Compatibilidad

`edit-plan-v2` es aditivo y no rompe `edit-plan-v1`. Los clientes que ignoran `guardrails`, `evidence` o `apply_status` siguen pudiendo consumir `diff` y `files_changed`. Los packet `edit-plan-v1` textuales siguen siendo la ruta recomendada para C#, TypeScript y Python hasta que existan backends AST especificos.
