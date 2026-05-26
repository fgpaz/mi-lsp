# AE release evidence - mi-lsp agent-first hygiene

Date: 2026-05-26

Command requested by plan:

```powershell
pwsh ./scripts/release/ae-release-binaries.ps1 -SkipBuild -SkipLocalInstall -SkipWslInstall -SkipMirror
```

Result: `pwsh` was not available in PATH in this shell. Direct script invocation was used instead.

First dry invocation:

```powershell
.\scripts\release\ae-release-binaries.ps1 -SkipBuild -SkipLocalInstall -SkipWslInstall -SkipMirror
```

Result: failed because the isolated worktree did not already contain `dist/<rid>` artifacts.

Release gate executed with build enabled and installs/mirror/publish skipped:

```powershell
.\scripts\release\ae-release-binaries.ps1 -SkipLocalInstall -SkipWslInstall -SkipMirror
```

Result: PASS.

Built RIDs and checksums:

- `win-arm64`: `dist/win-arm64/mi-lsp.exe`, sha256 `a4d43815542e803e4a1329c02a0bd3f90516076cd237097ad3d83ba7d877cdce`
- `win-x64`: `dist/win-x64/mi-lsp.exe`, sha256 `686e0311eb16e0d647d8b6f19772aff3da5ef3ce84d36d0586b0802e33550517`
- `linux-arm64`: `dist/linux-arm64/mi-lsp`, sha256 `548faafc886099a380c5b741379a505c4c76e75d5e622f3579b925d65069669e`
- `linux-x64`: `dist/linux-x64/mi-lsp`, sha256 `fd15cb2e45f75622a63eb650a32755a2fecf5787d578bc907c1305e6ca63c092`

Skipped by design:

- Local install: `SkipLocalInstall`
- WSL install: `SkipWslInstall`
- Mirror: `SkipMirror`
- Publish: `Publish` switch not set

Waiver:

No binary was installed or published from this branch. Source changes affect CLI behavior, so the build/provenance gate was run; distribution is intentionally deferred until PR/release flow.
