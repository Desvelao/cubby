# Changelog

All notable changes to this project are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project uses
[Semantic Versioning](https://semver.org/).

## [0.1.0-alpha1] - 2026-10-02

Initial release. This is an alpha: the manifest format and CLI may change before 0.1.0.

- `cubby` CLI (`validate`, `list`, `groups`, `build`, `version`): reads a YAML
  manifest, packs components into compartments and generates slotted divider
  panels.
- Export formats: `console`, `csv`, `svg`, `dxf`, `stl`, `step` (experimental),
  `iso-svg` / `iso-png` (plus exploded variants) and an `assembly` PDF.
- Layout options: `padding`, `margin`, `fullWalls`, `jointType`, `dividers`,
  `expand`, `removable`, `floor`, `fillRemaining` and height reductions;
  `--box-case` draws the game box in the outputs.
- Prebuilt binaries for Linux, macOS and Windows (amd64 and arm64) and a
  multi-arch Docker image on GHCR.
