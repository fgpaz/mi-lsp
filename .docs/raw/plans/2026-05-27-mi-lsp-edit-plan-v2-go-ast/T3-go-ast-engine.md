# T3 - Go AST Engine

Implement a Go-only AST rewrite engine using `go/parser`, `go/ast`, `go/token`, and `go/format`.

Operations:

- `replace_go_function`
- `replace_go_function_body`
- `insert_go_function_after`
- `ensure_go_import`
- `remove_go_import`

Required behavior:

- symbol lookup must fail on absent or ambiguous symbols
- target language must be Go
- replacement snippets must parse before diff/apply
- rewritten output must be formatted in memory
- no writes in dry-run

