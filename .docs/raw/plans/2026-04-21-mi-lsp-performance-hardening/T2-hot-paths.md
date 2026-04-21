# T2 - Bounded hot paths

## Scope

- Add daemon request backpressure for daemon-served heavy operations.
- Keep cheap/direct operations outside daemon auto-start.
- Make `nav.workspace-map` direct by default.
- Replace full-file reads in context slices, LSP document open, and admin log tail with bounded or cached reads.

## Acceptance

- Saturation returns `daemon/backpressure_busy`.
- `nav.workspace-map` does not auto-start daemon by default.
- Context/log/LSP paths have explicit bounds or LRU caching.
