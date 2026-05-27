# T4 - Service Dispatch and Output

Route `edit-plan-v2` through the existing `nav.edit-plan` operation.

Required behavior:

- same CLI flags and output envelope
- `backend=edit-plan`
- `mode=dry_run|applied`
- AST result evidence reports language, symbol, operation, and formatted file path
- unsupported C#/TS/Python AST operations return `language_not_supported` with a suggestion to use v1 textual editing or a future backend
- v1 output shape remains compatible

