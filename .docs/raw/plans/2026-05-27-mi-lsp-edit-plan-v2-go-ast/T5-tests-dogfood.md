# T5 - Tests and Dogfood

Add focused unit, CLI, and integration-style fixture tests.

Required tests:

- Go AST function replacement, body replacement, function insertion, import ensure/remove
- unsupported language rejection for C#, TypeScript, Python
- v1 textual compatibility for C#/TS/Python fixtures
- dry-run no-write
- apply writes expected file only
- dirty git blocks apply
- hash mismatch and unsafe paths still reject

