# Contributing

## Setup
Development uses the Docker dev environment: `make docker-dev` opens a shell with
Go and golangci-lint, and the source is bind-mounted. Run the checks below inside it.

A code map is in [docs/development.md](docs/development.md).

## Before opening a PR
```sh
make fmt vet test lint examples
```
Update `docs/manifest.md` and `docs/manifest.schema.json` when manifest keys
change (tests check they match the structs) and add a line to `CHANGELOG.md`.

## Commits
Use [Conventional Commits](https://www.conventionalcommits.org/):
`<type>(<optional scope>): <summary>`, imperative and lowercase, no trailing period.

- Types: `feat`, `fix`, `docs`, `test`, `refactor`, `perf`, `build`, `ci`, `chore`.
- Scope is optional, usually a package or area: `fix(pack): ...`, `feat(export): ...`.
- Breaking changes: add `!` (`feat(manifest)!: ...`) and a `BREAKING CHANGE:` footer.
- Release notes are generated from commit messages and skip `docs:` and `test:`, so use
  those types only for changes that need no release note.
- One logical change per commit.

## Releasing
See [docs/releasing.md](docs/releasing.md).
