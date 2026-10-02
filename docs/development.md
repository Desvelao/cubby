# Development guide

A short map of the code for contributors. Setup and the pre-PR checklist are in
[CONTRIBUTING.md](../CONTRIBUTING.md); the manifest format is in
[manifest.md](manifest.md).

## Pipeline

`cubby build` (`src/internal/cli/build.go`) runs these steps:

1. `manifest.Load` / `manifest.Validate`: strict YAML decoding and validation.
2. `pack.LayoutBox`: shelf-packs components into compartments and compartments
   into the box, returning a `pack.BoxResult`.
3. `render/slotted.Backend.Render`: turns the `BoxResult` into
   `[]geometry.Panel` (outlines, notches, 3D positions). It only runs for
   non-`console` formats or when a height reduction is set.
4. `export.Exporter.Export`: writes one output format from the `BoxResult`
   and/or the panels.

## Package map (`src/internal/`)

| package | owns |
|---|---|
| `cli` | cobra commands (`build`, `validate`, `list`, `groups`, `version`), flags, exit codes, output staging |
| `manifest` | schema structs (`schema.go`), loading, validation, unit conversion, height reductions |
| `pack` | shelf packing, `expand` and `defaults.fillRemaining`, `BoxResult` |
| `geometry` | panels, outlines, notches, rectangle decomposition |
| `render` | the `Backend` interface and the `slotted` backend |
| `export` | the `Exporter` interface, shared helpers (nesting, grouping, box case) and one sub-package per format |
| `version` | build metadata injected with `-ldflags` |

Tests live next to the code. End-to-end tests are in `src/test/e2e` (they run
the CLI on `examples/` and `src/testdata`).

## Recipe: add a manifest field

1. Add the field to `manifest/schema.go` and check it in `manifest/validate.go`.
2. Use it in `pack` or `render`.
3. Update `docs/manifest.md` and `docs/manifest.schema.json`. The doc tests in
   `manifest/docs_test.go` fail if the structs, docs and schema drift apart.
4. Add or extend an example in `examples/` and a line in `CHANGELOG.md`.

## Recipe: add an export format

1. Implement `export.Exporter` in `export/<format>/`.
2. Register it in `exporters()` in `cli/build.go` (and in `binaryFormats` if
   the output is binary).
3. Add it to `EXAMPLES_FORMATS` / `EXAMPLES_SUFFIXES` in the `Makefile` and to
   the "Output formats" section of the README.
4. Add tests, including the bounding-box agreement check in
   `src/test/e2e/bbox_test.go`.

## Running and testing

Development is based on the Docker dev environment (see `AGENTS.md` for the
full command list). From the repo root:

```sh
make docker-dev        # shell in the container (workdir /workspace/src)
go test ./...
go run ./cmd/cubby build ../examples/chess-checkers.yaml
```

or one-off: `LOCAL_UID=$(id -u) LOCAL_GID=$(id -g) docker compose run --rm dev go test ./...`.

`make examples` builds every example in every format, and `make demos-check`
fails if the committed demo images are stale. In the Docker dev container, run
`docker compose build dev` after changing the `Dockerfile`; compose does not
rebuild the image for you.
