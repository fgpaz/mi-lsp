# Recipes

Use this reference when the task is goal-shaped instead of command-shaped.

## Service audit

```powershell
mi-lsp nav service <service-path> --workspace <alias> --format compact
mi-lsp nav context <file> <line> --workspace <alias> --format compact
mi-lsp nav search "IConsumer<|PublishAsync<" --workspace <alias> --format compact
mi-lsp nav overview <service-path> --workspace <alias> --format compact
```

Use this before claiming a service is incomplete.

## Completeness check for `.NET` minimal APIs

```powershell
mi-lsp nav service src/backend/<service> --workspace <alias> --format compact
mi-lsp nav context src/backend/<service>/Program.cs <line> --workspace <alias> --format compact
mi-lsp nav search "Map(Get|Post|Put|Delete|Patch)" --workspace <alias> --format compact
```

Do not infer "not implemented" only because a guessed command or handler class is absent.

## Workspace orientation

```powershell
mi-lsp nav ask "how is this workspace organized?" --workspace <alias>
mi-lsp nav workspace-map --workspace <alias> --axi --format compact
mi-lsp nav related <important-symbol> --workspace <alias> --format compact
```

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
