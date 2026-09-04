# Runtime Drift

Use this reference when source, docs, daemon, and installed binary appear out of sync.

## Fast sanity check

```powershell
where.exe mi-lsp
mi-lsp nav --help
mi-lsp daemon status --format toon
mi-lsp worker status --format compact
```

## What to suspect first

- If docs mention a command that `mi-lsp nav --help` does not show, suspect a stale installed binary first.
- If `worker status` does not expose `cli_path` and `protocol_version`, suspect a stale installed binary first.
- If a daemon-backed command behaves older than the current source tree, suspect a stale daemon and restart it.
- If `daemon status` lacks `daemon_process` or `watchers`, suspect a stale installed binary or stale daemon.
- If `daemon status` warns about missing executable metadata, `executable_sha256` mismatch, or stale executable guidance, rebuild/install the CLI and restart the daemon before trusting daemon-backed results.
- If watcher/memory pressure is suspected, run `mi-lsp daemon perf-smoke --callers 16 --watch-mode off --format toon` after updating the binary.
- If `nav.find`, `nav.search`, or `nav.intent` are slow or inconsistent, suspect wrong PATH, stale `.mi-lsp/index.db`, or a stale binary before blaming daemon health.
- `nav.intent` may return a planner preview with graph/wiki/evidence, omissions, fallbacks, and exact expansion commands; preserve available `change`, `affected`, `callers`, `callees`, `tests`, `contracts`, and `wiki` sections. Preview, timeout, silence, `DONE`, or `PASS` without fresh evidence is not PASS and must not trigger a silent tool fallback.
- `nav.ask` and summary-first `nav.workspace-map` should stay direct and should not auto-start the daemon.
- If a direct query in a container workspace returns `backend=router`, suspect missing scope before suspecting runtime drift and rerun with `--repo`.

## Skill catalog domains and freshness

Keep the physical skill audit and the runtime catalog as separate domains and
label each observation by its stage:

- `370` is the **pre-cutover physical audit observation**: 288 top-level
  packages, including the 16 retired wrappers, plus 5 hidden `.system`
  packages, 7 `generico-setup/assets` templates, and 70 category packages.
- `354` is the **post-cutover physical audit observation**, after deleting
  those wrappers: 272 top-level packages, plus 5 hidden `.system` packages,
  7 `generico-setup/assets` templates, and 70 category packages.
- `356` is the **dated 2026-08-26 runtime snapshot** at
  `/home/tesla/.mi-lsp/skills/catalog.json`; it is not a physical filesystem
  count.

These are different stages, domains, and timestamps, so the observations are
not contemporaneous. The difference between `354` and `356` is not an
arithmetic residual and does not prove package loss or corruption: scan
filtering, snapshot timing, and normalized-ID deduplication semantics differ.

The runtime path is `ScanSkillsRoot` followed by `BuildCatalog`. The scan walks
the configured skills root, does not descend into hidden directories (except
the root itself), skips credential-looking paths with warnings, and records
parse failures as warnings while omitting those results. `BuildCatalog` merges
seed classification and keys scanned records by normalized ID, so duplicate
IDs collapse to one catalog record rather than becoming independent entries.

Keep these categories distinct when interpreting a future same-snapshot audit:

- **Hidden:** the five `.system/**/SKILL.md` packages are outside the runtime
  scan because of the hidden-directory rule.
- **Credential-looking:** paths matching the scan's protected-name/path rules
  are skipped and warned; this is a safety exclusion, not evidence of a
  missing package.
- **Parse-failure:** a file can be physically present while its scan result is
  omitted with a parse warning.
- **Duplicate-ID:** multiple successful scans can map to one normalized catalog
  ID; this is a catalog-key collision, not an additional physical package.
- **Template/asset scope:** the seven current
  `generico-setup/assets/**/SKILL.md` templates belong to the physical audit
  domain. Their absence from the dated catalog supports temporal drift, not a
  `ScanSkillsRoot` asset filter.

Do not compute `354` versus `356`, `370` versus `356`, or any other
arithmetic residual from these sources. A coherent reconciliation requires a
deliberately captured same-snapshot physical audit and runtime index; this
reference only labels the domains and preserves the existing filters. Do not
refresh or mutate the generated catalog or runtime state while diagnosing
drift.

## Wiki↔code bridge freshness

For `wiki_code_context`, freshness is reported independently for `docs_manifest`, `bindings`, `catalog`, `graph`, and `authority`. Per-domain freshness is result semantics, not a binary or daemon drift verdict. A stale graph preserves exact `direct_code` and `tests`, omits `supporting_code`, and emits a typed `graph_stale` omission for that omission. Absence is not proof and does not authorize a fallback.

## Fallback discipline

Keep `mi-lsp` first. An external fallback is permitted only when the visible reason is one of `unsupported_operation`, `unavailable_binary`, `invalid_workspace`, or `explicit_incomplete`. A timeout, silence, `DONE`, or `PASS` without fresh evidence must remain visible as incomplete evidence; it is never a silent fallback trigger. If the runtime returns labeled catalog/text/heuristic evidence, preserve that label and do not present it as semantic certainty. Use 180/300-second soft/hard watchdogs, at most two same-context recoveries with a smaller packet and no unchanged retry, at most six practical lanes with exclusive `allowed_paths`, fail-closed joins, and fresh verification. Redact prompts, transcripts, secrets, PII, PHI, argv, and raw patterns; do not invent model/provider metadata.

## After rebuild or reinstall

```powershell
mi-lsp daemon restart
mi-lsp daemon status --format toon
mi-lsp worker status --format compact
mi-lsp workspace status <alias> --format compact
```

## Path and install guidance

- Prefer the canonical installed binary on `PATH`
- For this repo's local release flow, treat `dist/<rid>/mi-lsp(.exe)` plus the installed copy under the chosen install dir as canonical
- Do not assume the repo-root `mi-lsp.exe` is the active binary unless `Get-Command mi-lsp` proves it
- Use `cli_path` from `mi-lsp worker status --format compact` to confirm which executable answered the probe
- Use `executable_sha256` from `mi-lsp daemon status --format toon` to confirm the daemon is running the same build content as the invoking CLI; path differences alone can be benign for `go run`
- Compare `protocol_version` in that output with the current source/docs when you suspect binary drift

## Semantic backend reminders

- `backend=pyright-langserver` with zero items means the Python backend answered but found no result
- `unsupported backend: pyright` means the running binary or daemon does not expose that backend
- `unsupported backend: gopls` means the running binary or daemon does not expose the Go optional backend
- `gopls is unavailable` with catalog/text fallback means the running binary is current enough, but `gopls` is not installed on the machine
- If Roslyn bootstrap fails, the user-facing remediation should point to `mi-lsp worker install`

## Graph observation / index failures

- `invalid graph observation: ... GPH_OBS_NODE_INVALID ... node violates canonical batch or matrix` with `stage=selector_validation` on `mi-lsp index` is a **provider/worker** failure, not a consumer-wiki problem. Prefer refreshing CLI **and** Roslyn worker (`mi-lsp worker install` or a release bundle that includes `workers/<rid>/`), then re-run `mi-lsp index --workspace <alias> --no-daemon`.
- Empty `display_name` / bare DocCommentId identities (`T:`, `M:`, …) from incomplete/error types were a known producer bug fixed in **v0.7.1**; older workers can still fail seal on large multi-csproj solutions.
- If index succeeds but warnings say `graph observation produced no stageable complete batch` / `graph omitted partial Roslyn observation`, the **catalog** is still usable (`index_ready` can be true) while the semantic **graph** stays stale. Fix compile errors or observe a complete project before expecting graph nav surfaces to be fresh.
- Do not "fix" this class of failure by deleting `.mi-lsp/` alone or by editing consumer product/wiki code.
