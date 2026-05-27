# T2 - Model and Contract v2

Extend model structs without breaking v1 JSON.

Required behavior:

- `EditPlanVersionV1 = "edit-plan-v1"`
- `EditPlanVersionV2 = "edit-plan-v2"`
- `targets[].language` accepts `go`, `csharp`, `typescript`, `python`, or can be inferred from extension.
- `targets[].symbol.receiver` supports method disambiguation.
- v1 packets continue through the existing textual engine.
- v2 packets route to structural engine dispatch.

