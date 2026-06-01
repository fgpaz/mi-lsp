# AE-RELEASE-DISTRIBUTION

```yaml
harness_protocol: SDD-HARNESS-v1
source_protocol: SDD-WIKI-SOURCE-v1
doc_id: "AE-RELEASE-DISTRIBUTION"
id: "AE-RELEASE-DISTRIBUTION"
kind: "support-doc"
audience: "llm-first"
imports:
  - '[[00_gobierno_documental]]'
  - '[[TECH-DEPENDENCY-HARDENING]]'
  - '[[09_contratos_tecnicos]]'
  - '[[AE-EVIDENCE-POLICY]]'
exports:
  - 'AE-RELEASE-DISTRIBUTION'
agent_must_read:
  - .docs/wiki/ae/AE-RELEASE-DISTRIBUTION.md
  - .docs/wiki/07_tech/TECH-DEPENDENCY-HARDENING.md
  - .docs/wiki/09_contratos_tecnicos.md
agent_may_edit:
  - .docs/wiki/ae/AE-RELEASE-DISTRIBUTION.md
  - scripts/release/ae-release-binaries.ps1
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - pwsh ./scripts/release/ae-release-binaries.ps1 -SkipBuild -SkipLocalInstall -SkipWslInstall -SkipMirror
  - mi-lsp nav governance --workspace mi-lsp --format toon
stop_if:
  - governance_blocked=true
  - release-visible work lacks provenance evidence
evidence:
  - .docs/wiki/ae/AE-RELEASE-DISTRIBUTION.md
  - scripts/release/ae-release-binaries.ps1
```

## Release Distribution Gate

```toon
doc_id: AE-RELEASE-DISTRIBUTION
block_id: AE-RELEASE-DISTRIBUTION.gate
kind: policy
source_of_truth: this
gate_id: ae_release_distribution
applies_when:
  - CLI behavior can drift between source and installed binary
  - worker bootstrap or bundled worker layout changes
  - release, install, version, doctor, daemon, or cross-OS behavior changes
  - an agent claims final readiness after modifying binary-producing code
required_targets:
  local_current_machine:
    windows_arm64: C:/Users/fgpaz/bin/mi-lsp.exe
    wsl_linux:
      detection: "wsl sh -lc 'whoami; printf %s \"$HOME\"; uname -m'"
      install_paths:
        - "$HOME/.local/bin/mi-lsp"
        - "$HOME/bin/mi-lsp"
  release_assets:
    - win-arm64
    - win-x64
    - linux-arm64
    - linux-x64
  public_install_scripts:
    - scripts/install/install.ps1
    - scripts/install/install.sh
    - scripts/install/install-agent.ps1
    - scripts/install/install-agent.sh
  optional_skill_mirror:
    roots:
      - C:/Users/fgpaz/.agents/skills/mi-lsp
      - C:/repos/buho/assets/skills/mi-lsp
    files:
      - bin/mi-lsp-win-x64.exe
      - bin/mi-lsp-linux-x64
  public_install_contract:
    latest_release: GitHub releases/latest
    checksum_asset: mi-lsp_<version>_checksums.txt
    archive_layout: mi-lsp(.exe) plus workers/<rid> inside the release archive
    agent_install: npx skills add fgpaz/mi-lsp --skill mi-lsp -g -a codex -a claude-code -y
    no_silent_auto_update: true
default_command: scripts/release/ae-release-binaries.ps1
publish_command:
  shape: "pwsh ./scripts/release/ae-release-binaries.ps1 -Clean -Publish -Tag <vX.Y.Z>"
  effect: "build all RIDs, refresh local installs, verify provenance, and push the release tag that triggers GitHub release upload"
stop_if:
  - current worktree is dirty and Publish is requested
  - tag does not point at HEAD when Publish is requested
  - any required RID artifact is missing
  - public install script references an asset name not produced by GoReleaser
  - public install script extracts before checksum verification
  - install-agent bypasses npx with an ungoverned folder-copy fallback
  - local ARM64 install was skipped without waiver on this workstation
  - WSL install was skipped without waiver when WSL is available
  - local executable remains locked after daemon stop and copy retries
verify:
  - powershell -File ./scripts/release/ae-release-binaries.ps1 -SkipBuild -SkipLocalInstall -SkipWslInstall -SkipMirror
  - mi-lsp nav wiki validate-source --workspace mi-lsp --format toon
evidence:
  - .docs/wiki/ae/AE-RELEASE-DISTRIBUTION.md
  - scripts/release/ae-release-binaries.ps1
```

## Operational Notes

`scripts/release/ae-release-binaries.ps1` is the maintained entrypoint for this gate. By default it builds all four RIDs and refreshes the current host install. On Windows it also refreshes the active WSL install when WSL is present and the matching Linux RID was built.

`scripts/install/install.ps1` and `scripts/install/install.sh` are the public CLI install/update entrypoints. They consume GitHub `releases/latest`, map the host to one of the four published RIDs, verify the release checksum before extraction, preserve the bundled `workers/<rid>/` layout, and run `version` plus `worker status` probes.

`scripts/install/install-agent.ps1` and `scripts/install/install-agent.sh` compose the CLI installer with `npx skills add`; they do not copy skill folders directly. A weekly release check from the skill may notify about newer releases, but binary update remains explicit user action.

The local install path must stop an existing `mi-lsp daemon` before replacing the executable and worker bundle, then retry copy/removal briefly to absorb Windows file-lock lag.
WSL install defaults are detected from the active distro user and `$HOME`; pass `-WslInstallPaths` only when the target distro uses non-standard paths.

Publishing is explicit. The script only pushes a tag when `-Publish -Tag <tag>` is passed, the worktree is clean, and the tag points at `HEAD`. The GitHub release workflow remains the public upload mechanism for release assets.
