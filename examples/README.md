# Examples

Example manifests for `cubby`. `make examples` (from the repo root) validates
and builds every `examples/*.yaml` with all exporters; `make demos`
regenerates the pre-rendered demo artifacts committed here; `make demos-check`
verifies they are not stale.

## Manifests

| Manifest | Demonstrates |
| --- | --- |
| `chess-checkers.yaml` | A combined insert for a chess and checkers set (box "Chess & Checkers Combo"). |
| `full-walls-fill-remaining.yaml` | The options `defaults.fillRemaining` (grow the last compartment to use leftover box space) and `groups[].fullWalls` (real divider panels on every side, even against the outer wall). |
| `height-reduction.yaml` | `externalHeightReduction` and `dividerHeightReduction`: defaults lower internal dividers by 30%; the "cards" tray overrides with a fixed 6 mm on its first row divider and trims its external walls by 5 mm. |
| `overfull.yaml` | Expected-failure demo: the components deliberately do not fit, so `cubby build` exits with code 3 and prints the "missing space" report. `make examples` expects this (see `EXAMPLES_EXPECT` in the Makefile). Do not "fix" it. |
| `plain-joints.yaml` | The `jointType` option: `defaults.jointType: plain` with a `groups[].jointType: notch` override for one group. |
| `row-dividers.yaml` | The `dividers` option: a tray with row dividers between shelf-packed rows versus one with `dividers: false`. |

## Committed demo artifacts

Output files are named after the manifest's `box.name`, not the file name.

| Manifest | Artifacts |
| --- | --- |
| `chess-checkers.yaml` | `chess-checkers-combo.iso-png.png`, `chess-checkers-combo.iso-png-exploded.png`, `chess-checkers-combo.iso-svg.svg` |
| `plain-joints.yaml` | `plain-joints-demo.iso-png.png`, `plain-joints-demo.iso-png-exploded.png`, `plain-joints-demo.assembly.pdf` |
| `height-reduction.yaml` | `height-reduction-demo.assembly.pdf` |
| `row-dividers.yaml` | `row-dividers-demo.assembly.pdf` |

`full-walls-fill-remaining.yaml` and `overfull.yaml` have no committed artifacts.

## Regenerating

```sh
make demos
```

This renders the artifacts into a temp dir and, only if every build succeeded
and `examples/` is writable, copies them over the committed ones (the
manifest-to-formats list is the `DEMOS` variable in the Makefile). A failed run
leaves `examples/` untouched. Rerun it after changing a manifest or the
renderers so the committed demos do not go stale.

## Checking for stale artifacts

```sh
make demos-check
```

This regenerates every demo into a temp dir and compares it with the committed
files without modifying them, failing with the list of stale or missing files
and the hint `run make demos`. PNG and SVG output was measured to be
byte-identical across runs, so those are compared byte-for-byte. The assembly
PDFs differ on every run, so for `.pdf` files (`DEMOS_NONDETERMINISTIC` in the
Makefile) only existence is checked; a stale PDF is not detected. The check is
not part of CI because PNG rendering may vary with the platform.
