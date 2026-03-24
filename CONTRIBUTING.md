# Contributing to mi-lsp

Thanks for your interest in contributing to `mi-lsp`.

## Ground rules

- Be respectful and constructive.
- Keep pull requests focused and reviewable.
- Avoid introducing machine-specific paths, private repo names, or environment-specific assumptions in user-facing docs and examples.
- Follow the [Code of Conduct](CODE_OF_CONDUCT.md).

## Prerequisites

- Go 1.24+
- .NET 10 SDK
- PowerShell 7+ if you want to run the release helper scripts locally

## Local development

Build the CLI:

```bash
make build
```

Run the test suite:

```bash
make test
```

`make test` uses the race detector when the local Go toolchain supports it. On
platforms where `-race` is unavailable, it falls back to `go test -v ./...`.

Run formatting and vet:

```bash
make lint
```

Create release-like local bundles:

```powershell
pwsh ./scripts/release/build-dist.ps1 -Rids @('win-x64') -Clean
```

## Pull request workflow

1. Fork the repository.
2. Create a branch from `main`.
3. Make your changes.
4. Run the relevant build and test commands locally.
5. Open a pull request with a clear description of the change and how you validated it.

## Commit style

Conventional commits are preferred:

- `feat:`
- `fix:`
- `docs:`
- `refactor:`
- `test:`
- `chore:`

## Review expectations

- Maintainer review is best effort.
- Initial review target is about 7 days, depending on availability.
- Larger or riskier changes may take longer if they affect release layout, worker bootstrap, or public CLI contracts.

## Reporting bugs and feature ideas

- Use the issue templates in this repository.
- For security issues, do not open a public issue. Follow [SECURITY.md](SECURITY.md).
