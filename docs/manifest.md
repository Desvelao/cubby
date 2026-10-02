# Manifest reference

This is the reference for the cubby configuration file (the "manifest"): a
YAML file that describes a box, the material the panels are cut from, the
components to store and how to group them. It is written against the current
code (`src/internal/manifest/schema.go` and `validate.go`) and is kept honest
by tests: every complete example on this page is loaded and validated, and
every manifest key must be documented here and in
[`manifest.schema.json`](manifest.schema.json) (see [Editor support](#editor-support)).

Contents: [File format](#file-format) - [Full example](#full-example) -
[Field reference](#field-reference) - [Units](#units) -
[Inheritance and overrides](#inheritance-and-overrides) -
[Packing and layout semantics](#packing-and-layout-semantics) -
[Validation rules](#validation-rules) - [Exit codes](#exit-codes) -
[Common errors and fixes](#common-errors-and-fixes) -
[Where metadata appears](#where-project-metadata-appears) -
[Editor support](#editor-support)

## File format

- A manifest is a **single, non-empty YAML document**. An empty file
  (`parse manifest: manifest is empty`) or a file with more than one document
  (`parse manifest: multiple YAML documents; only one manifest per file`) is
  rejected.
- Decoding is **strict**: an unknown key anywhere is an error, so a typo such
  as `fillremaining` cannot silently do nothing (`line 2: field bogus not
  found in type manifest.Manifest`). Keys are case-sensitive and camelCase.
  Values of the wrong type are rejected too (for example
  `version: one` gives `line 1: cannot unmarshal !!str into int`, with the
  offending value quoted in the real message).
- `version: 1` is the **schema version** of the manifest format. It is
  required and must be `1`. It is *not* your design's version: use
  [`project.revision`](#project) for that.
- Standard YAML comments (`# ...`) work anywhere. YAML anchors and merge keys
  (`&base`, `<<: *base`) are handled by the YAML decoder and were verified to
  work, for example to repeat most of a component's fields.
- Syntax and type problems are reported by every command as
  `error: parse manifest: ...` with exit code 1. Rule violations found by
  `cubby validate` are reported as `path: message` lines with exit code 2
  (see [Validation rules](#validation-rules) and [Exit codes](#exit-codes)).
- All lengths are numbers in the units chosen by `box.units` (see
  [Units](#units)).
- Components are packed as rectangular bounding boxes (no circles or custom
  shapes in v1).

## Full example

A complete manifest that uses every block. Optional keys are shown with
comments; omit any you do not need.

```yaml manifest
version: 1                       # schema version; must be 1

project:                         # optional descriptive metadata (every field optional)
  name: "Chess & Checkers Insert"    # design title (box.name is the physical box)
  description: "Combined organizer for two game sets."
  revision: "1.0"                    # YOUR design revision (not the schema version)
  author: "Jane Doe"
  contact: "jane@example.com"        # free text: email, handle (@jane) or URL; email-like values must be valid
  license: "CC-BY-4.0"
  url: "https://example.com/insert"  # http or https
  tags: [chess, checkers]            # up to 20, each up to 50 characters
  created: 2026-09-30                # YYYY-MM-DD
  updated: 2026-10-05                # YYYY-MM-DD, not before created
  notes: "Cut from 3 mm foamboard."
  game: "Chess"
  publisher: "ACME"
  edition: "Classic"

box:
  name: "Chess & Checkers Combo"
  units: mm                      # mm | cm | in (default mm)
  interior:
    width: 300
    depth: 300
    height: 60

material:
  name: "3mm Foamboard"          # optional label
  thickness: 3.0                 # in box.units, must be > 0
  kerf: 0.0                      # in box.units; extra slot clearance; 0 for hand-cut, ~0.1-0.2 for laser

defaults:                        # every key optional
  padding: 1.0                   # clearance around each component's footprint
  margin: 1.5                    # clearance around a compartment vs the box wall / other compartments
  fullWalls: false               # panel on every side, even against the box wall
  jointType: notch               # notch | plain
  dividers: true                 # panels between shelf rows / side-by-side items (default ON)
  expand: false                  # stretch trays to the box edge along their row
  removable: false               # trays are self-contained open boxes
  floor: false                   # floor plate under trays
  fillRemaining: false           # grow the last compartment to the box's true edges

components:
  - id: chess-pieces-box
    name: "Chess Pieces Tray"
    width: 140
    depth: 90
    height: 40
    qty: 1
  - id: checkers-red
    name: "Red Checkers"
    width: 30
    depth: 30
    height: 8
    qty: 12
  - id: rulebook
    name: "Rulebook"
    width: 140
    depth: 90
    height: 8
    qty: 1
    allowRotate: false           # default true; ungrouped components are auto-packed

groups:
  - id: chess-compartment
    name: "Chess Pieces"
    components: [chess-pieces-box]
    padding: 2                   # optional per-group override of defaults.padding
    margin: 2                    # optional per-group override of defaults.margin
    fullWalls: true              # optional overrides of the defaults.* layout options
    jointType: plain
    dividers: false
    expand: false
    removable: false
    floor: false
```

The smallest valid manifest needs only the required keys:

```yaml manifest
version: 1
box:
  name: "Tiny Box"
  interior: {width: 100, depth: 100, height: 40}
material:
  thickness: 3
components:
  - {id: cards, width: 60, depth: 90, height: 20, qty: 1}
```

Components that are not in any group (here, `cards`) are packed automatically.

## Field reference

"Required" means `cubby validate` reports an error when the key is missing
(a missing number is read as 0). "Default" is the value used when the key is
omitted.

### Top-level keys

| field | type | required | default | notes |
|---|---|---|---|---|
| `version` | integer | yes | none | Schema version; must be `1`. Not the design revision. |
| `project` | mapping | no | none | Optional design metadata, see [`project`](#project). |
| `box` | mapping | yes | none | The container the insert must fit in, see [`box`](#box). |
| `material` | mapping | yes | none | What the panels are cut from, see [`material`](#material). |
| `defaults` | mapping | no | all defaults below | Fallback values for groups and ungrouped components. |
| `components` | list | no | empty | The kinds of pieces to store. |
| `groups` | list | no | empty | Explicit compartments ("bandages"). |

### `project`

Optional descriptive metadata about the design itself, as opposed to `box`,
which names the physical box. The whole block and every field are optional;
manifests without it are unaffected, and exporters emit only the fields that
are set. All text is checked for length and control characters (see
[Validation rules](#validation-rules)). Lengths are counted in characters.
Newlines and tabs are allowed only in `description` and `notes`.

| field | type | required | default | notes |
|---|---|---|---|---|
| `name` | string | no | none | Design title. Max 120 characters. |
| `description` | string | no | none | Free-text summary. Max 1000 characters; newlines and tabs allowed. |
| `revision` | string | no | none | Your design revision, for example `"1.2.0"`. Unrelated to `version`. Max 200; must not be blank when set. Quote it (`"1.0"`) so editors treat it as text. |
| `author` | string | no | none | Who designed the insert. Max 200. |
| `contact` | string | no | none | Free text (email, handle or URL). Max 200. A value that looks like an email address (contains `@`, has no whitespace or is `Name <addr>`, does not start with `@` and has no `://`) must parse as one (bare `user@host.tld` or `Name <user@host.tld>`) whose domain contains a dot. Handles such as `@jane` or `@jane@fosstodon.org` and URLs such as `https://medium.com/@jane` are accepted as is. |
| `license` | string | no | none | License name, ideally an SPDX identifier such as `CC-BY-4.0`. Not validated beyond length (200) and characters. |
| `url` | string | no | none | Home page. Max 200; must be an `http` or `https` URL with a host. |
| `tags` | list of strings | no | empty | Short keywords. At most 20 tags; each non-blank and at most 50 characters. |
| `created` | string | no | none | Creation date, `YYYY-MM-DD` (a real calendar date). Kept as the authored text; an unquoted YAML date is fine. |
| `updated` | string | no | none | Last-modified date, `YYYY-MM-DD`; must not be before `created`. |
| `notes` | string | no | none | Free-text build or usage notes. Max 2000 characters; newlines and tabs allowed. |
| `game` | string | no | none | The game the insert is designed for. Max 200. |
| `publisher` | string | no | none | The game's publisher. Max 200. |
| `edition` | string | no | none | Game edition or box variant. Max 200. |

### `box`

| field | type | required | default | notes |
|---|---|---|---|---|
| `name` | string | yes | none | Names the physical box. Must not be empty. Shown in the console header, SVG comment, STEP file/product name, isometric title, `groups` output and JSON `boxName`. |
| `units` | string | no | `mm` | `mm`, `cm` or `in`. See [Units](#units). |
| `interior` | mapping | yes | none | Usable inside size of the box, see below. |

### `box.interior`

Width, depth and height are in `box.units`. Width runs along X, depth along
Y (the packing plane) and height is the vertical size available to panels.

| field | type | required | default | notes |
|---|---|---|---|---|
| `width` | number | yes | none | Must be a finite number > 0. |
| `depth` | number | yes | none | Must be a finite number > 0. |
| `height` | number | yes | none | Must be a finite number > 0. |

### `material`

| field | type | required | default | notes |
|---|---|---|---|---|
| `name` | string | no | none | Label shown in console output, the SVG comment, the CSV `material` column and the isometric legend. In the CSV, a name starting with `=`, `+`, `-`, `@`, tab or carriage return is prefixed with `'` so spreadsheets do not run it as a formula. |
| `thickness` | number | yes | none | Panel thickness in `box.units`; must be > 0. |
| `kerf` | number | no | `0` | Extra slot clearance in `box.units`; must be >= 0 and less than `thickness`. Use 0 for hand-cut, about 0.1-0.2 mm for laser. The kerf is compensated in the notch (slot) widths, not by offsetting outlines. |

### `defaults`

Fallback values applied to groups (and auto-packed ungrouped components) that
do not set their own. See [Inheritance and overrides](#inheritance-and-overrides).

| field | type | required | default | notes |
|---|---|---|---|---|
| `padding` | number | no | `0` | Clearance around each component's footprint, in `box.units`; must be >= 0. |
| `margin` | number | no | `0` | Clearance around a compartment versus the box wall and other compartments, in `box.units`; must be >= 0. |
| `fullWalls` | boolean | no | `false` | Generate a real divider panel on every side of a compartment, including edges touching the outer box wall. |
| `jointType` | string | no | `notch` | `notch` or `plain`. How crossing panels meet. |
| `dividers` | boolean | no | `true` | Divider panels between shelf-packed rows and side-by-side items inside a compartment. Defaults **on**; omitted means true. |
| `expand` | boolean | no | `false` | Stretch trays along their row to the box edge. |
| `removable` | boolean | no | `false` | Make each tray a self-contained open box that can be lifted out. |
| `floor` | boolean | no | `false` | Add a floor plate under each tray. |
| `fillRemaining` | boolean | no | `false` | Grows the spatially-last compartment (bottom-most, then right-most) to reach the box's true edges, so leftover box space is enclosed within the design instead of left as an open gap. Global only (no per-group override). Which compartment ends up "last" depends on the packing order, so it can shift if you edit the manifest. |
| `externalHeightReduction` | number, `"N%"` or `{by, panels}` | no | none | Lower external panels (full-walls boundary walls `w-*`/`wv-*`, removable tray walls `rw-*`/`rv-*`). See [Height reduction](#height-reduction). |
| `dividerHeightReduction` | number, `"N%"` or `{by, panels}` | no | none | Lower divider panels (grid `h-*`/`v-*`, internal `iv-*`/`ih-*`/`ie-v-*`). See [Height reduction](#height-reduction). |

### `components[]`

One kind of physical piece to store. Dimensions are in `box.units`.

| field | type | required | default | notes |
|---|---|---|---|---|
| `id` | string | yes | none | Unique among components; not empty. Referenced by `groups[].components`. |
| `name` | string | no | none | Display name. |
| `width` | number | yes | none | Finite, > 0. |
| `depth` | number | yes | none | Finite, > 0. |
| `height` | number | yes | none | Finite, > 0. |
| `qty` | integer | yes | none | How many identical pieces; must be > 0. |
| `allowRotate` | boolean | no | `true` | Whether packing may rotate the piece 90 degrees in the plane. Omitted means true; set `false` to forbid rotation. |

### `groups[]`

An explicit, named compartment: a set of components packed together into
their own region of the box (a "bandage"). Every setting except `id`,
`name` and `components` overrides the matching `defaults` value for this group
only; omitting it inherits.

| field | type | required | default | notes |
|---|---|---|---|---|
| `id` | string | yes | none | Unique among groups; not empty; must not look like `auto-N` (reserved for generated compartments). |
| `name` | string | no | none | Display name. |
| `components` | list of strings | yes | none | Component ids; at least one. Each must exist, and a component may belong to at most one group. |
| `padding` | number | no | inherits `defaults.padding` | >= 0, in `box.units`. |
| `margin` | number | no | inherits `defaults.margin` | >= 0, in `box.units`. |
| `fullWalls` | boolean | no | inherits `defaults.fullWalls` | |
| `jointType` | string | no | inherits `defaults.jointType` | `notch` or `plain`. |
| `dividers` | boolean | no | inherits `defaults.dividers` (default on) | |
| `expand` | boolean | no | inherits `defaults.expand` | |
| `removable` | boolean | no | inherits `defaults.removable` | Cannot be mixed with non-removable groups. |
| `floor` | boolean | no | inherits `defaults.floor` | |
| `externalHeightReduction` | number, `"N%"` or `{by, panels}` | no | inherits `defaults.externalHeightReduction` | |
| `dividerHeightReduction` | number, `"N%"` or `{by, panels}` | no | inherits `defaults.dividerHeightReduction` | |

## Units

`box.units` (`mm`, `cm` or `in`; default `mm`) applies to every length you
write in the manifest: the box interior, component `width`/`depth`/`height`,
`defaults.padding`/`defaults.margin` and the per-group `padding`/`margin`,
and also `material.thickness` and `material.kerf`. Everything is converted to
millimetres once (1 cm = 10 mm, 1 in = 25.4 mm) and all outputs are always in
mm. For example with `units: in`, `thickness: 0.125` is a 3.175 mm wall.
Nothing else is affected: `qty` is a count and the `--sheet-width` and
`--nest-gap` build flags are always millimetres.

## Inheritance and overrides

- **defaults to groups.** `padding`, `margin`, `fullWalls`, `jointType`,
  `dividers`, `expand`, `removable` and `floor` are set once under `defaults`
  and can be overridden per group under `groups[]`.
- **Unset versus explicit false.** A group key that is omitted inherits the
  default; a group key set to `false` (or `0` for a number) overrides it. For
  example `defaults.fullWalls: true` with a group that sets `fullWalls: false`
  turns full walls off for that group only. `jointType` behaves the same:
  an omitted group `jointType` inherits, an explicit `notch` or `plain` wins.
- **`dividers` is the only option that defaults ON.** An omitted
  `defaults.dividers` resolves to `true`; set a literal `false` under
  `defaults` to turn it off globally, or on a group to turn it off there.
  Because it is on by default, upgrading to a version of cubby with this
  option can change the exported design (more panels) for an existing
  manifest with a multi-row compartment, even without editing it.
- **`allowRotate` defaults to `true`** per component (omitted means
  rotation is allowed); write `allowRotate: false` to forbid it.
- **`jointType` empty behaves as `notch`.**
- **Ungrouped components** use the `defaults` values (not any group's) and
  are packed automatically into whatever box space is left over after all
  groups are placed. They end up in generated compartments with the ids
  `auto-1`, `auto-2`, ... (name `Auto compartment N`, kind `auto`), which is
  why group ids of the form `auto-N` are reserved. Auto compartments use
  `defaults.padding`, `fullWalls`, `jointType`, `dividers`, `expand`,
  `removable` and `floor`; per-group settings never apply to them.
  Free space is the trailing gap of each group row plus the gap below all
  rows. These rects are tried largest area first (not by position), so
  components can spill from one rect into the next, each used rect becoming
  its own `auto-N` compartment. With `defaults.removable: true` every rect
  loses one wall thickness on each side before packing, so a component that
  fits a rect when walls are off may not fit with walls on. If a component
  cannot be placed anywhere, the reported reason is the one from the first
  (largest) rect tried. One group setting does leak through: with
  non-removable defaults, a `floor` on any group lowers the height limit of
  auto-packed components by one material thickness.
- A component may belong to **at most one** group.

## Packing and layout semantics

The packer is a deterministic grid/shelf (row-based) algorithm; see
[Design notes](../README.md#design-notes) in the README. In short:

- **Groups are compartments.** Each group's components are shelf-packed
  (left to right, wrapping to a new row) into a tight-fit compartment. The
  compartments are then shelf-packed into the box interior in the order the
  groups are declared, and ungrouped components fill the leftover space.
  Compartments are sized tightly, so side-by-side compartments in one row can
  differ in depth; the shared `v-*` divider between them is as long as the
  row's deepest compartment, and the strip beside the shallower one gets no
  wall of its own.
- **`padding` versus `margin`.** `padding` is the clearance around each
  component inside a compartment. `margin` is the clearance between a
  compartment and the box wall or neighbouring compartments (the compartment
  is inset by `margin` on every side; for removable trays the wall thickness
  is added to that inset).
- **`fillRemaining`** (`defaults.fillRemaining`, default `false`) grows the spatially-last
  compartment (bottom-most, then right-most) to the box's true edges so
  leftover space is enclosed rather than an open gap.
- **`expand`** (`defaults.expand` / `groups[].expand`, default `false`)
  stretches a tray along its row to the box edge (width only; depth never
  changes). A row's leftover width is split equally among its expanding
  trays. Packed items never move. With `dividers` on, the added width is
  walled off as its own empty cell (an `ie-v-*` panel at the old right edge);
  with `dividers: false` it is just open space inside the tray. Leftover depth
  below the last row stays open unless `fillRemaining` is also set.
- **`removable`** (default `false`) makes a tray a self-contained open box
  (`rw-*` / `rv-*` walls, plain butt joints) instead of part of the shared
  interlocking grid, so it can be lifted out. The wall thickness is added to
  the tray's footprint, and neighbors end up with a double wall between them.
  No shared `h-*`/`v-*` panels are produced, and `jointType` is ignored for
  these trays. Removable and non-removable trays cannot be mixed in one box
  (validation error).
- **`floor`** (default `false`) adds a floor plate (`fl-*`, `floor` axis)
  under a tray. Its thickness is taken out of the wall height, so the total
  stays within the box.
  In the shared-grid mode any floor raises the whole grid by one thickness:
  every grid panel, including those bordering only floorless trays, is lifted
  by one thickness and shortened to `height - thickness`, while only the
  floored trays get `fl-*` plates. Percent reductions are taken from that
  lowered base (`height - thickness`), so `10%` of a 60 mm height with a
  3 mm floor cuts 5.7 mm, not 6 mm. A `removable` tray measures from its own
  base instead (its wall thickness if it has a floor, else 0), so the same
  percentage can give a different absolute cut there; mm amounts are unaffected.
- **`fullWalls`** (default `false`) generates a real divider panel on every
  side of a compartment, including edges that touch the outer box wall, which
  otherwise get a bare, panel-less edge since the box itself is assumed to
  provide that wall. Set it under `defaults` and override per group.
- **`jointType`** (default `notch`): `notch` cuts interlocking half-lap slots
  wherever two panels cross (cubby's original behavior); `plain` leaves
  those joints uncut, for a build assembled with glue instead of slotting. A
  joint is only cut as a notch if **every** compartment touching it resolves
  to `notch`: a notch with nothing to interlock into on the other side is
  useless, so any neighboring `plain` compartment leaves the shared joint
  uncut too. Panels meeting the outer box wall always get a flush butt edge.
- **`dividers`** (default **on**) generates a divider panel between each pair
  of adjacent shelf-packed rows within a compartment, and between each pair
  of items sitting side-by-side within the same row (placed midway through
  the padding gap between them). These panels sit entirely inside the
  compartment's own content footprint, never reaching its walls or the box
  wall, so they are always a plain butt edge, unaffected by `jointType`.
- **A component that does not fit is not a validation error.** `cubby
  validate` never packs, so a manifest with a piece too big for the box still
  validates (exit 0). Whether things fit is a packing outcome, reported by
  `cubby groups` and `cubby build` (missing space per compartment, with a
  reason such as `too-wide`) with exit code 3. `examples/overfull.yaml`
  demonstrates this.

## Height reduction

`externalHeightReduction` and `dividerHeightReduction` shorten panels so
components are easier to reach. Each is set under `defaults` and may be
overridden per group (the whole value is replaced, not merged).

```yaml
defaults:
  externalHeightReduction: 5       # 5 box.units off every external panel
  dividerHeightReduction:
    by: "30%"                      # 30% of the panel height
    panels: ["iv-*", "h-2"]        # optional: only these ids (`*` globs); omit for all
```

- A number is in `box.units`; a string `"N%"` (0 to under 100) is relative to the
  panel height.
- Without `panels`, every panel of that class is affected. With it, only
  panels whose id matches; see [Referring to panels](#referring-to-panels).
  `panels` must list at least one id (an empty or null list is rejected rather
  than meaning "all"), and `by`/`panels` may each appear only once.
- Panels are trimmed from the top; slot notches keep their full-height depth so
  joints still mate. A notched panel can therefore be reduced by less than half
  its height; larger values fail the build.
- `externalHeightReduction` applies only to removable trays and to `fullWalls`
  boundary walls. On a tray that is neither, there is no external panel to
  lower and the setting is ignored; `cubby build` warns about this on stderr
  (and `--strict` fails on it, exit code 2). The check is per distinct value:
  a setting inherited from `defaults` by several trays warns only if none of
  the trays holding it is removable or `fullWalls`, so a default that works
  for some trays stays quiet for the others. A tray overriding it with its
  own value is judged on its own.
- A grid panel shared by several trays takes the largest reduction among them.
- Panel height is the box interior height; a `"N%"` reduction is N% of it, and
  the half-height notch limit uses it too. With `floor: true` the base is first
  lowered by one material thickness (the floor), then the reduction is applied.
  A reduction too large for that height fails with exit code 2, and so does a
  height that does not exceed the floor thickness (`panel height 3 must exceed
  floor thickness 3`).
- `cubby validate` and `cubby build` check reductions the same way.
  `cubby list` and `cubby groups` check the
  schema and cross-references only (they print each issue to stderr, exit 2)
  and intentionally do not render, so a reduction that fails rendering is
  accepted by them; `validate` and `build` are the commands that check it.

### Referring to panels

`panels` entries are panel ids, matched exactly or with `*` (any run of
characters, e.g. `iv-*`, `ih-card-tray-*`). Ids are generated from the layout,
so the reliable way to find them is to build once and read the `panel_id`
column:

```sh
cubby build manifest.yaml --format csv --out -
```

| Id | Panel | Class | Numbering |
|---|---|---|---|
| `h-N` | Shared grid divider running along the width, between two rows of trays | divider | `N` counts row boundaries from the front (1 = first) |
| `v-N` | Shared grid divider running along the depth, between two side-by-side trays | divider | `N` counts in layout order (row by row, left to right); shares one counter with `w-*`/`wv-*` |
| `w-N`, `wv-N` | `fullWalls` boundary wall along the width (`w`) or the depth (`wv`) | external | same counter as `v-N`, so the numbers are not contiguous per prefix |
| `iv-<tray>-<row>-<n>` | Internal divider between the `n`-th and next item of row `row` in tray `<tray>` | divider | `row` and `n` start at 1 |
| `ih-<tray>-<n>` | Internal divider between row `n` and row `n+1` of tray `<tray>` | divider | `n` starts at 1 |
| `ie-v-<tray>` | Divider closing off the extra cell of an `expand` tray | divider | one per tray |
| `rw-<tray>-front`, `rw-<tray>-back` | Front/back wall of a `removable` tray | external | |
| `rv-<tray>-left`, `rv-<tray>-right` | Left/right wall of a `removable` tray | external | |

`<tray>` is the group `id`, or `auto-N` for an ungrouped-components compartment
(see [Inheritance and overrides](#inheritance-and-overrides)). Floor plates
(`fl-*`) are never reduced.

Matching rules:

- An id only matches panels of the setting's own class. Entries are trimmed
  of surrounding whitespace on load, and `cubby validate` rejects an entry
  that can never match a panel of the setting's class: an invalid glob, or one
  whose literal text before the first `*`, `?`, `[` or `\` neither starts with
  one of the class's id prefixes (external: `w-`, `wv-`, `rw-`, `rv-`;
  divider: `h-`, `v-`, `iv-`, `ih-`, `ie-v-`) nor, for a pattern with a
  wildcard, is the start of one. So `panels: ["w-*"]` under
  `dividerHeightReduction` is an error, ids are case-sensitive (`W-1` is an
  error), while `*`, `iv-*`, `h-?` and `w*` (under `externalHeightReduction`)
  are accepted. Only the prefix is checked, so a valid-looking pattern can
  still match nothing (see the next rule).
- A pattern that matches no panel has no effect. `cubby build` prints a
  warning to stderr for each such entry (also when it only matches panels of
  the other class), and `--strict` turns it into a failure (exit code 2)
  before any output is written. An entry inherited by several trays (for
  example from `defaults`) warns only if none of them matched it. Panel ids
  are only known once the design is generated, so `cubby validate` lays out
  and renders a manifest that sets a reduction to find them: it prints the
  same warnings to stderr as `warning: ...` lines (exit 0), and
  `cubby validate --strict` fails on them with exit code 2.
- Tray-specific ids (`iv-`, `ih-`, `ie-v-`, `rw-`, `rv-`) are matched only
  against the tray whose setting holds them, but a pattern like `iv-*` in
  `defaults` covers every tray. Grid ids (`h-N`, `v-N`, `w-N`, `wv-N`) can
  change when trays are added, removed or resized, so prefer patterns over
  fixed numbers there.

## Validation rules

`cubby validate <manifest>` loads the file (strict decoding, see
[File format](#file-format)) and then checks every rule below, collecting all
problems rather than stopping at the first. Each issue is `path: message`;
paths use dotted/bracketed form such as `groups[1].components[0]`. Messages
below are the text produced by the current code (verified by running
`cubby validate`); numbers and quoted values are examples.

**Schema version and box**

| path | rule | example message |
|---|---|---|
| `version` | must equal 1 (a missing version is 0) | `unsupported manifest version 2 (expected 1)` |
| `box.units` | empty, `mm`, `cm` or `in` | `unknown units "yd" (expected mm, cm, or in)` |
| `box.name` | not empty | `must not be empty` |
| `box.interior.width` / `.depth` / `.height` | finite and > 0 | `must be > 0, got 0`, `must be a finite number, got NaN` |

**Material and defaults**

| path | rule | example message |
|---|---|---|
| `material.thickness` | finite and > 0 | `must be > 0, got 0` |
| `material.kerf` | finite, >= 0 and < `material.thickness` (slot width is thickness + kerf, so a larger kerf is almost always a units slip) | `must be >= 0, got -1`, `must be < material.thickness (3), got 3; check the units` |
| `defaults.padding`, `defaults.margin` | finite and >= 0 | `must be >= 0, got -1`, `must be a finite number, got +Inf` |
| `defaults.jointType` | empty, `notch` or `plain` | `unknown joint type "glue" (expected notch or plain)` |

**Components** (`components[i]`)

| path | rule | example message |
|---|---|---|
| `components[i].id` | not empty | `must not be empty` |
| `components[i].id` | unique | `duplicate component id "a"` |
| `components[i].qty` | > 0 | `must be > 0, got 0` |
| `components[i].width` / `.depth` / `.height` | finite and > 0 | `must be > 0, got 0`, `must be a finite number, got NaN` |

**Groups** (`groups[i]`)

| path | rule | example message |
|---|---|---|
| `groups[i].id` | not empty | `must not be empty` |
| `groups[i].id` | unique | `duplicate group id "g"` |
| `groups[i].id` | must not match the generated `auto-N` ids (`auto-` and a positive integer) | `group id "auto-1" collides with the generated auto compartment ids (auto-N)` |
| `groups[i].components` | at least one id | `must list at least one component id` |
| `groups[i].components[j]` | id must exist | `references unknown component id "nope"` |
| `groups[i].components[j]` | at most one group per component | `component "a" already belongs to group "g" (a component may belong to at most one group)` |
| `groups[i].components[j]` | no repeats within one group | `component "a" is listed more than once in group "g"` |
| `groups[i].padding`, `groups[i].margin` | when set: finite and >= 0 | `must be >= 0, got -2` |
| `groups[i].jointType` | when set: `notch` or `plain` | `unknown joint type "x" (expected notch or plain)` |
| `groups[i].removable` | resolved value (group or default) must be the same for every group; reported once, at the first group that differs; if any component is ungrouped, `defaults.removable` counts too (auto compartments use it) and is reported at `defaults.removable` | `cannot mix removable and non-removable trays in one box: groups[0] is removable=false but this group is removable=true (set removable on every tray, or none)` |

**Feasibility** (arithmetic on the box, material and margins; never looks at
components). Skipped for fields that already failed their own range check,
and when `box.units` or the interior is invalid. Values are shown in mm.

| path | rule | example message |
|---|---|---|
| `material.thickness` | must be smaller than the interior width and depth | `10 mm is not smaller than the box interior width/depth (10 x 100 mm), so the walls cannot fit` |
| `externalHeightReduction`, `dividerHeightReduction` | amount finite and >= 0, percentage in [0, 100), no empty panel ids; a notched panel's reduction must stay below half its height (checked by `build` and `validate`, which lay out and render when a reduction is set) | `percentage must be >= 0 and < 100` |
| `defaults.*HeightReduction`, `groups[i].*HeightReduction` | an absolute amount (converted to mm) must be smaller than the box interior height, minus the floor thickness when a floor is in effect for every tray using the value; skipped for percentages, `panels`-limited reductions, values no tray uses and external reductions on trays without external panels. The half-height limit for notched panels is only checked at render time | `reduction 500 mm leaves no panel height (interior height 40 mm, available panel height 40 mm)` |
| `material.thickness` | when any floor is in effect (a group resolving to `floor: true`, or ungrouped components with `defaults.floor: true`), must be smaller than the interior height | `10 mm is not smaller than the box interior height (4 mm), so a floor plate leaves no room for contents` |
| `groups[i].margin` | `2 x (margin + wall)` must be smaller than the interior width and depth; `wall` is the material thickness for removable groups and 0 otherwise | `2 x (margin 60 mm + wall 0 mm) = 120 mm is not smaller than the box interior width/depth (100 x 100 mm), leaving no usable space` |
| `defaults.margin` | the same check for groups that inherit the default margin, and for ungrouped components (auto compartments use the defaults); reported once, on `defaults.margin` | `2 x (margin 46 mm + wall 5 mm) = 102 mm is not smaller than the box interior width/depth (100 x 100 mm), leaving no usable space` |

**Project** (`project.*`; every field optional, only set values are checked)

| path | rule | example message |
|---|---|---|
| `project.name` | max 120 characters | `must be at most 120 characters, got 143` |
| `project.description` | max 1000 characters | `must be at most 1000 characters, got N` |
| `project.notes` | max 2000 characters | `must be at most 2000 characters, got N` |
| `project.revision`, `.author`, `.contact`, `.license`, `.url`, `.created`, `.updated`, `.game`, `.publisher`, `.edition` | max 200 characters | `must be at most 200 characters, got N` |
| every text field above | no control characters (newline and tab allowed only in `description` and `notes`), and not U+FFFE/U+FFFF | `must not contain control character U+0001` |
| `project.revision` | not blank when set | `must not be blank when set` |
| `project.contact` | if it looks like an email address (has `@`, no leading `@`, no `://`, no whitespace unless `Name <addr>`), must be a valid one | `looks like an email address but is not a valid email address, got "not@valid"` |
| `project.url` | valid `http`/`https` URL with a host | `must be an http or https URL, got "ftp://example.com"`, `must include a host, got "https://"` |
| `project.created`, `project.updated` | real calendar date, `YYYY-MM-DD` | `must be a date in YYYY-MM-DD format, got "2026-13-45"` |
| `project.updated` | not before `project.created` | `must not be before project.created (2026-03-01), got 2026-02-01` |
| `project.tags` | at most 20 tags | `must have at most 20 tags, got 21` |
| `project.tags[i]` | not blank; max 50 characters; no control characters | `must not be empty`, `must be at most 50 characters, got N` |

These are *parse* errors, not validation issues (exit 1 rather than 2): an
unknown key, a value of the wrong type, an empty file, several YAML documents
and an unreadable file (see [Common errors and fixes](#common-errors-and-fixes)).

## Exit codes

Every command uses the same exit code contract (from
`src/internal/cli/exitcode.go`), so cubby is safe to use in scripts and CI:

| Code | Meaning | Typical manifest cause |
|---|---|---|
| 0 | Success. | Valid manifest; everything fit. |
| 1 | Usage/runtime error (bad flags, file I/O, YAML parse error). `validate --format json` still prints `{"issues": [{"path": "", "message": ...}]}` on stdout for a parse error (not for a missing file). | Unknown key, wrong value type, empty or multi-document file, missing file. |
| 2 | Manifest validation failed. | Any issue from [Validation rules](#validation-rules), or a height reduction too large for the panels it trims (reported by `build` and `validate` alike). |
| 3 | Packing incomplete: one or more components did not fit. | `groups` / `build` on a manifest whose pieces do not fit the box. |

## Common errors and fixes

| Message (exit code) | Cause and fix |
|---|---|
| `error: parse manifest: yaml: unmarshal errors:` then `line 2: field bogus not found in type manifest.Manifest` (1) | Unknown or misspelled key. Check the name and case against the [field reference](#field-reference), and that it sits under the right block. |
| `error: parse manifest: manifest is empty` (1) | The file is empty or has only comments. |
| `error: parse manifest: multiple YAML documents; only one manifest per file` (1) | Remove the extra `---` document. |
| `line 1: cannot unmarshal !!str ... into int` (1) | A value has the wrong type, here `version: one`. Use `version: 1`. Likewise `tags` must be a list. |
| `error: read manifest: open x.yaml: no such file or directory` (1) | Wrong path. |
| `version: unsupported manifest version 2 (expected 1)` (2) | Set `version: 1`. If the key is missing you get `... version 0`. |
| `box.name: must not be empty` (2) | Add `box.name`. |
| `box.interior.width: must be > 0, got 0` (2) | The interior size is missing (a missing number reads as 0) or not positive. Add `width`, `depth` and `height`. |
| `material.thickness: must be > 0, got 0` (2) | Add `material.thickness`. |
| `components[2].id: duplicate component id "a"` (2) | Component ids must be unique. |
| `groups[1].components[1]: references unknown component id "nope"` (2) | The group lists an id that is not in `components`. Fix the spelling. |
| `groups[2].components[0]: component "a" already belongs to group "g" (...)` (2) | Put each component in only one group. |
| `groups[0].components[2]: component "a" is listed more than once in group "g"` (2) | List each component once per group; use `qty` for several instances. |
| `groups[0].id: group id "auto-1" collides with the generated auto compartment ids (auto-N)` (2) | Rename the group; `auto-N` is reserved. |
| `groups[1].removable: cannot mix removable and non-removable trays in one box: ...` (2) | Set `removable` the same way on every group (or use `defaults.removable`). |
| `material.thickness: 10 mm is not smaller than the box interior width/depth (...)` (2) | Thickness is too large for the box, or `units` is wrong (for example `thickness: 3` with `units: in`). |
| `defaults.margin: 2 x (margin 46 mm + wall 5 mm) = 102 mm is not smaller than ...` (2) | Reduce the margin; the groups would leave no usable space. |
| `project.contact: looks like an email address but is not a valid email address, got "..."` (2) | Use a real address, or write a handle (`@jane`), a URL, or text with spaces. |
| `project.created: must be a date in YYYY-MM-DD format, got "..."` (2) | Write dates as `2026-09-30`. |
| `error: 1 component(s) did not fit` from `groups`, or `error: 1 component instance(s) did not fit in the box` from `build` (3) | Not a manifest error: the pieces do not fit. Enlarge the box, reduce `padding`/`margin`, allow rotation, or split components across groups. `cubby groups` shows the missing space and a reason such as `too-wide`. |

## Where project metadata appears

The `project` block is optional and every field is optional. When present, it
surfaces in these outputs (only for the fields that are set):

| Output | What is emitted |
|---|---|
| `console` | A `Project: <name> (rev <revision>) by <author>` line and a `Description:` line (whitespace collapsed to one line). |
| `svg` | `<title>` (name), `<desc>` (description) and a Dublin Core `<metadata>` block: `dc:title`, `dc:creator` (author), `dc:description`, `dc:rights` (license), `dc:identifier` (url), one `dc:subject` per tag, `dc:date` (`updated`, else `created`) and `cubby:revision`. Values are XML-escaped. |
| `step` | `FILE_NAME` author list = `project.author`; `PRODUCT` description = `project.description`. The `FILE_NAME` name stays `<box.name> insert`. |
| `assembly` (PDF) | Info dictionary: Title (name), Author, Subject (description on one line), Keywords (tags, comma-separated) and Creator; plus a title block above the assembled preview on the first page (name, a revision/author/license line and the description, truncated after a few lines). |
| `groups --format json` | A `project` object holding the fields that are set, keyed as in the manifest; omitted when the block sets nothing. |
| `csv`, `dxf`, `stl`, `iso-svg`, `iso-png` (and their exploded variants) | Not emitted. |

Without a `project` block none of this is emitted.

## Editor support

[`manifest.schema.json`](manifest.schema.json) is a JSON Schema (draft 2020-12)
describing the manifest: every key and type, the `units` and `jointType`
enums, numeric minimums, the `project` length caps and formats, and
`additionalProperties: false` at every level (mirroring strict decoding).
Editors that use it can validate as you type and autocomplete keys.

With the YAML language server (the Red Hat YAML extension for VS Code, and
other editors that embed it), add a modeline as the first line of the
manifest. The path is relative to the manifest file:

```yaml
# yaml-language-server: $schema=../docs/manifest.schema.json
version: 1
```

Use the path that matches where the manifest lives: `../docs/manifest.schema.json`
from `examples/`, `docs/manifest.schema.json` from the repository root, or an
absolute or `https://` URL. Alternatively map the schema to a file glob in the
editor settings, for example in VS Code `settings.json`:

```json
{
  "yaml.schemas": {
    "docs/manifest.schema.json": ["examples/*.yaml", "*.cubby.yaml"]
  }
}
```

The modeline is an ordinary YAML comment, so cubby ignores it. The schema
covers structure, types and simple limits; rules that need cross-checking
(duplicate ids, unknown component references, `auto-N` collisions, mixed
`removable`, feasibility, email/URL validity, date ordering) are only
reported by `cubby validate`, so run it too. A test checks that the schema
lists exactly the keys of the Go structs and that every example uses only
allowed keys.

## Examples

See [`examples/`](../examples/) for complete manifests, including one that
deliberately does not fit (`overfull.yaml`) to demonstrate the "missing space"
report, one exercising `fillRemaining`/`fullWalls`
(`full-walls-fill-remaining.yaml`), one exercising `jointType`
(`plain-joints.yaml`), and one exercising `dividers` (`row-dividers.yaml`).
`chess-checkers.yaml` is the basic chess and checkers box.

`overfull.yaml` is an expected-failure demo: `cubby build` on it exits with
code 3 and prints the missing-space report. `make examples` expects this (see
`EXAMPLES_EXPECT` in the Makefile). It is also shipped in the release archives
alongside the other examples.
