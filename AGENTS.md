# AGENTS.md

`cubby` is a Go CLI (module in `src/`) that turns a YAML manifest into slotted
divider-panel designs for boardgame boxes. Code map: `docs/development.md`.
Manifest reference: `docs/manifest.md`.

## Work in the Docker dev environment

Run Go tooling in the `dev` container, not on the host. From the repo root:

```sh
export LOCAL_UID=$(id -u) LOCAL_GID=$(id -g)
docker compose run --rm dev go build ./...          # workdir is /workspace/src
docker compose run --rm dev go vet ./...
docker compose run --rm dev gofmt -l .
docker compose run --rm dev go test ./...
docker compose run --rm dev golangci-lint run ./...
docker compose run --rm dev go run ./cmd/cubby build ../examples/chess-checkers.yaml
docker compose run --rm dev make -C .. examples      # Makefile targets run from the repo root
```

`make docker-test` runs the race tests and `make docker-dev` opens a shell.
After editing the `Dockerfile`, run `docker compose build dev`. On
`/cache ... permission denied`: `docker compose build --no-cache dev && docker compose down -v`.

## Before finishing

`gofmt -l .` prints nothing, and `go vet`, `go test ./...` and `make examples` pass.

## Conventions

- Manifest changes: update `src/internal/manifest/schema.go`,
  `docs/manifest.md` and `docs/manifest.schema.json` together (doc tests enforce it).
- New export formats are registered in `exporters()` in `src/internal/cli/build.go`.
- The Go, golangci-lint and GoReleaser versions are pinned in several files;
  `sh scripts/check-versions.sh` checks they agree.
- Add a `CHANGELOG.md` entry for user-visible changes.
- Local-only manifests go in `private/` (gitignored).
- Do not commit generated output (`out/`) or hand-edit demo artifacts in `examples/` (use `make demos`).

## Commits

Use [Conventional Commits](https://www.conventionalcommits.org/): `<type>(<optional scope>): <summary>`,
imperative and lowercase, no trailing period.

- Types: `feat`, `fix`, `docs`, `test`, `refactor`, `perf`, `build`, `ci`, `chore`.
- Scope is optional, usually a package or area: `fix(pack): ...`, `feat(export): ...`.
- Breaking changes: add `!` (`feat(manifest)!: ...`) and a `BREAKING CHANGE:` footer.
- Release notes are generated from commit messages and skip `docs:` and `test:`, so use
  those types only for changes that need no release note.
- One logical change per commit.
