# Recipes

Use this reference when the task is goal-shaped instead of command-shaped.

## Canonical wiki / traceability discovery

```powershell
mi-lsp nav route "how does login work?" --workspace <alias> --format compact
mi-lsp nav wiki search "RF-AUTH login" --workspace <alias> --layer RF,TP,CT --format compact
mi-lsp nav wiki pack "how does login work?" --workspace <alias> --format compact
mi-lsp nav wiki trace RF-AUTH-001 --workspace <alias> --format compact
```

Use this when the task is about canonical docs, requirements, tests, contracts, or traceability.
If AXI preview is trimmed, rerun the same wiki command with `--full` before broadening the search.
If a later `nav search` returns prompts, audits, `.docs/raw`, or other support artifacts, treat that as non-canonical evidence and keep the wiki lane as the source of truth.

## Intent-first graph and change recipes

Start with the goal in `nav intent`; it is mandatory for every supported goal-shaped request. Routing is automatic and has no routing opt-out. `explain-change` is an intent/operation of `nav intent`, not a required literal alias in the request. The planner returns a bounded preview with the information currently available, graph/wiki/evidence, explicit candidates, fallbacks, and omissions. Preserve the seven sections `change`, `affected`, `callers`, `callees`, `tests`, `contracts`, and `wiki`; use the exact `expansions[].command` as the second query and read its reason. Follow the emitted command rather than translating it by hand.

```powershell
# Explain a working-tree or explicitly supplied change.
mi-lsp nav intent "explain the change and its impact" --workspace <alias> --format toon
mi-lsp nav explain-change --path internal/service/intent.go --workspace <alias> --format toon

# Impact, callers, and callees.
mi-lsp nav intent "what is affected by this change?" --workspace <alias> --format toon
mi-lsp nav affected internal/service/intent.go --include-tests --include-docs --workspace <alias> --format toon
mi-lsp nav callers MySymbol --workspace <alias> --format toon
mi-lsp nav callees MySymbol --workspace <alias> --format toon

# A bounded path, exact edge explanation, and neighborhood.
mi-lsp nav path FromSymbol ToSymbol --workspace <alias> --format toon
mi-lsp nav explain <edge-cross-rid> --workspace <alias> --format toon
mi-lsp nav neighbors MySymbol --workspace <alias> --format toon
```

Natural-language “neighborhood” routes to the exposed `nav neighbors` command. Use `nav related MySymbol --depth definition,callers,implementors,tests` only when that specific four-lane summary is wanted. For every preview, preserve available evidence and read `expansions[].reason`; use the exact `expansions[].command` for the next bounded read. A timeout, silence, `DONE`, or `PASS` without fresh evidence is incomplete evidence, not PASS and not a silent fallback.

Only these external fallback reasons are allowed: `unsupported_operation`, `unavailable_binary`, `invalid_workspace`, and `explicit_incomplete`. If none is visible, stay with `mi-lsp` and report the limitation. Use 180/300-second soft/hard watchdogs, at most two same-context recoveries with a smaller packet and no unchanged retry, at most six practical lanes with exclusive `allowed_paths`, fail-closed joins, fresh verification, and redacted evidence without prompts, transcripts, secrets, PII, PHI, argv, or raw patterns. Do not invent model/provider metadata.

## Preparation and guarded editing

```powershell
mi-lsp nav prepare "review the routing change" --affected internal/service/intent.go --workspace <alias> --format toon
mi-lsp nav edit-plan --packet .mi-lsp/plan.json --workspace <alias> --format toon
```

`nav prepare` is read-only semantic preparation. `nav edit-plan` is a separate deterministic patch packet surface: dry-run by default; writing requires `--apply --experimental-apply`, a clean Git workspace, safe paths, and matching hashes. Neither command is a generic fallback for the other.

## Wiki pack, trace, and validator scopes

Use the actual wiki subcommands and scopes exposed by the runtime:

```powershell
mi-lsp nav wiki pack "understand the routing contract" --workspace <alias> --format toon
mi-lsp nav wiki pack "indexing docs" --workspace <alias> --rf RF-IDX-001 --format toon
mi-lsp nav wiki trace RF-QRY-003 --workspace <alias> --format toon
mi-lsp nav wiki trace --all --summary --workspace <alias> --format toon
mi-lsp nav wiki validate-harness --workspace <alias> --ids CT-NAV-INTENT --format toon
mi-lsp nav wiki validate-harness --workspace <alias> --paths .docs/wiki/09_contratos/CT-NAV-INTENT.md --format toon
mi-lsp nav wiki validate-source --workspace <alias> --ids CT-NAV-INTENT --format toon
mi-lsp nav wiki validate-source --workspace <alias> --paths .docs/wiki/09_contratos/CT-NAV-INTENT.md --format toon
```

`nav wiki search` accepts `--layer RS,RF,FL,TP,CT,TECH,DB`. `validate-harness` validates SDD-HARNESS-v1 contracts and `validate-source` validates SDD-WIKI-SOURCE-v1 source blocks; both accept comma-separated `--ids` or `--paths`. Use the two named wiki validator subcommands; do not infer a combined validator surface.

## Service audit

```powershell
mi-lsp nav service <service-path> --workspace <alias> --format compact
mi-lsp nav context <file>:<line> --workspace <alias> --format compact
mi-lsp nav search "IConsumer<|PublishAsync<" --workspace <alias> --format compact
mi-lsp nav overview <service-path> --workspace <alias> --format compact
```

Use this before claiming a service is incomplete.
For Go packages, `nav service` reports `profile=go-package` and uses Go-aware evidence (`net/http`, router-style calls, Cobra, worker signals) instead of .NET-only endpoint/event patterns.

## Completeness check for `.NET` minimal APIs

```powershell
mi-lsp nav service src/backend/<service> --workspace <alias> --format compact
mi-lsp nav context src/backend/<service>/Program.cs:<line> --workspace <alias> --format compact
mi-lsp nav search "Map(Get|Post|Put|Delete|Patch)" --workspace <alias> --format compact
```

Do not infer "not implemented" only because a guessed command or handler class is absent.

## Workspace orientation

```powershell
mi-lsp nav governance --workspace <alias> --format compact
mi-lsp nav ask "how is this workspace organized?" --workspace <alias>
mi-lsp nav workspace-map --workspace <alias> --axi --format compact
mi-lsp nav related <important-symbol> --workspace <alias> --format compact
```

If governance is blocked, stop normal exploration and repair the governance document/projection first.

## PR review / impact analysis

```powershell
mi-lsp nav diff-context HEAD~1 --workspace <alias> --format compact
mi-lsp nav diff-context main --include-content --workspace <alias> --format compact
```

## Batch exploration

```powershell
mi-lsp nav search "PublishAsync" --include-content --workspace <alias> --format compact
mi-lsp nav multi-read src/Service.cs:1-100 src/Controller.cs:50-150 src/Model.cs:1-80 --workspace <alias> --format compact
```

If that still implies too many separate calls, switch to `nav batch`.

## Portable preparation recipe

```powershell
mi-lsp prepare create --workspace <alias> --output <validated-evidence-root>
mi-lsp prepare verify --workspace <alias> --input <packet>
mi-lsp prepare refresh --workspace <alias> --input <packet> --output <validated-evidence-root>
```

Keep seed receipts and catalogs isolated with explicit roots. Treat typed drift as repair guidance only: preparation evidence never authorizes writes, protected mutations, or promotions.
