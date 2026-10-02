# Releasing

Releases are cut by pushing a tag; the `Release` workflow does the rest.

1. In `CHANGELOG.md`, set the release date on the version heading (from the second release on, move the `[Unreleased]` entries under a new `## [X.Y.Z] - date` heading) and merge to `main`.
2. Make sure CI is green on `main`.
3. Tag and push:
   ```sh
   git tag vX.Y.Z
   git push origin vX.Y.Z
   ```
   Use `vX.Y.Z-alpha1` for a prerelease (GoReleaser marks it as a prerelease and the
   Docker `latest` tag does not move).

The workflow validates the tag, re-runs CI as a gate, publishes GoReleaser
archives plus `checksums.txt` to the GitHub release, and pushes the multi-arch
image to `ghcr.io/desvelao/cubby`.

## One-time setup
- Repository settings -> Actions: allow workflows to run; the workflow requests
  its own `contents: write` and `packages: write` permissions.
- After the first image push, make the GHCR package public.
- Optionally protect `main` and require the `CI` checks.
