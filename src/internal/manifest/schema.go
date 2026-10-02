// Package manifest defines the YAML schema for a cubby box manifest, loads
// it strictly (unknown keys are rejected) and validates it.
//
// Top-level blocks: version (schema version, must be 1), project (optional
// descriptive metadata), box (name, units, interior size), material
// (thickness, kerf), defaults (padding, margin and the layout options
// fullWalls, jointType, dividers, expand, removable, floor,
// fillRemaining, externalHeightReduction, dividerHeightReduction), components,
// groups (each may override the layout options).
//
// doc: the user-facing reference is docs/manifest.md, with an editor JSON
// Schema in docs/manifest.schema.json. Tests in docs_test.go fail when a
// field is added or removed here without updating both.
package manifest

// Units is the linear unit a manifest's dimensions are authored in.
type Units string

const (
	UnitsMM Units = "mm"
	UnitsCM Units = "cm"
	UnitsIN Units = "in"
)

// JointType selects how two crossing divider panels meet.
type JointType string

const (
	// JointTypeNotch cuts interlocking half-lap slots into both panels so
	// they slot together (cubby's original, default behavior).
	JointTypeNotch JointType = "notch"
	// JointTypePlain leaves the joint uncut (a flat butt edge), for builds
	// assembled with glue instead of slotting.
	JointTypePlain JointType = "plain"
)

// Manifest is the root of a cubby YAML input file.
type Manifest struct {
	// Version is the manifest schema version (must be 1). It is not the
	// design revision; see Project.Revision for that.
	Version    int         `yaml:"version"`
	Project    Project     `yaml:"project"`
	Box        Box         `yaml:"box"`
	Material   Material    `yaml:"material"`
	Defaults   Defaults    `yaml:"defaults"`
	Components []Component `yaml:"components"`
	Groups     []Group     `yaml:"groups"`
}

// Project is optional descriptive metadata about the design itself (as
// opposed to Box, which names the physical box). Every field is optional and
// the whole block may be omitted; exporters emit only the fields that are set.
// Dates are ISO 8601 calendar dates (YYYY-MM-DD) kept as the authored text.
type Project struct {
	// Name is the design title (Box.Name stays the physical box's name).
	Name string `yaml:"name" json:"name,omitempty"`
	// Description is a free-text summary; newlines and tabs are allowed.
	Description string `yaml:"description" json:"description,omitempty"`
	// Revision is the design revision (for example "1.2.0"). It is unrelated
	// to Manifest.Version, the schema version.
	Revision string `yaml:"revision" json:"revision,omitempty"`
	// Author is who designed the insert.
	Author string `yaml:"author" json:"author,omitempty"`
	// Contact is free text (an email address, handle or URL); a value
	// that looks like an email address (not a handle or URL) must be valid.
	Contact string `yaml:"contact" json:"contact,omitempty"`
	// License is a free-text license name, ideally an SPDX identifier such
	// as "CC-BY-4.0". It is not validated beyond length and characters.
	License string `yaml:"license" json:"license,omitempty"`
	// URL is a home page for the design; it must be an http or https URL.
	URL string `yaml:"url" json:"url,omitempty"`
	// Tags are short free-text keywords (at most 20, each at most 50 chars).
	Tags []string `yaml:"tags" json:"tags,omitempty"`
	// Created is the creation date as YYYY-MM-DD.
	Created string `yaml:"created" json:"created,omitempty"`
	// Updated is the last-modified date as YYYY-MM-DD; not before Created.
	Updated string `yaml:"updated" json:"updated,omitempty"`
	// Notes is free-text build or usage notes; newlines and tabs are allowed.
	Notes string `yaml:"notes" json:"notes,omitempty"`
	// Game is the game the insert is designed for.
	Game string `yaml:"game" json:"game,omitempty"`
	// Publisher is the game's publisher.
	Publisher string `yaml:"publisher" json:"publisher,omitempty"`
	// Edition is the game edition or box variant.
	Edition string `yaml:"edition" json:"edition,omitempty"`
}

// IsZero reports whether no project field is set.
func (p Project) IsZero() bool {
	return p.Name == "" && p.Description == "" && p.Revision == "" && p.Author == "" &&
		p.Contact == "" && p.License == "" && p.URL == "" && len(p.Tags) == 0 &&
		p.Created == "" && p.Updated == "" && p.Notes == "" && p.Game == "" &&
		p.Publisher == "" && p.Edition == ""
}

// Box describes the container the insert must fit inside.
type Box struct {
	// Name names the physical box; it is required.
	Name string `yaml:"name"`
	// Units is the unit of every length in the manifest; empty means mm.
	Units Units `yaml:"units"`
	// Interior is the usable inside size of the box.
	Interior Dimensions `yaml:"interior"`
}

// Dimensions is a width/depth/height bounding box, in the manifest's authored units.
type Dimensions struct {
	Width  float64 `yaml:"width"`
	Depth  float64 `yaml:"depth"`
	Height float64 `yaml:"height"`
}

// Material describes what the divider panels are cut from. Thickness and Kerf
// are authored in Box.Units like every other length in the manifest; use
// Material.ToMM to get them in millimeters.
type Material struct {
	// Name is an optional label for the material.
	Name string `yaml:"name" json:"name"`
	// Thickness is the panel thickness; it must be > 0.
	Thickness float64 `yaml:"thickness" json:"thickness"`
	// Kerf is extra slot clearance; it must be >= 0 (0 for hand-cut).
	Kerf float64 `yaml:"kerf" json:"kerf"`
}

// Defaults holds fallback padding/margin/fullWalls values applied when a
// component or group doesn't specify its own.
type Defaults struct {
	// Padding is the clearance around each component's footprint.
	Padding float64 `yaml:"padding"`
	// Margin is the clearance around a compartment versus the box wall and
	// other compartments.
	Margin float64 `yaml:"margin"`

	// FullWalls, when true, generates a real divider panel on every side of
	// a compartment, including edges that touch the outer box wall (which
	// otherwise get a bare, panel-less edge since the box itself is assumed
	// to provide that wall). Overridable per group via Group.FullWalls.
	FullWalls bool `yaml:"fullWalls"`

	// JointType selects how panels meet where they cross; empty behaves as
	// JointTypeNotch. Overridable per group via Group.JointType.
	JointType JointType `yaml:"jointType"`

	// Dividers, when true, generates internal divider panels between the
	// shelf-packed rows within a single compartment. Unlike FullWalls/
	// JointType, this defaults ON: an omitted defaults.dividers (nil)
	// resolves to true via ResolvedDividers. Set it to a literal `false` to
	// opt out globally. Overridable per group via Group.Dividers.
	Dividers *bool `yaml:"dividers"`

	// Expand, when true, lets compartments stretch to the box edge
	// along their row (see Group.Expand). Defaults off. Overridable per group.
	Expand bool `yaml:"expand"`

	// Removable, when true, makes each tray a self-contained open box (its
	// own four walls, so it can be lifted out) instead of part of the
	// shared interlocking grid. Overridable per group.
	Removable bool `yaml:"removable"`

	// Floor, when true, adds a floor plate under a tray; wall height is
	// reduced by the material thickness. Overridable per group.
	Floor bool `yaml:"floor"`

	// FillRemaining grows the spatially-last compartment (bottom-most, then
	// right-most) to reach the box's true edges, so leftover box space is
	// enclosed within the design rather than left as an open gap. Global:
	// there is no per-group override.
	FillRemaining bool `yaml:"fillRemaining"`

	// ExternalHeightReduction lowers external panels (full-walls boundary
	// walls and removable tray walls). Overridable per group.
	ExternalHeightReduction HeightReduction `yaml:"externalHeightReduction"`

	// DividerHeightReduction lowers divider panels (grid and internal
	// dividers). Overridable per group.
	DividerHeightReduction HeightReduction `yaml:"dividerHeightReduction"`
}

// ResolvedDividers reports Defaults.Dividers, treating an omitted
// defaults.dividers (nil) as true.
func (d Defaults) ResolvedDividers() bool {
	if d.Dividers == nil {
		return true
	}
	return *d.Dividers
}

// Component is one kind of physical game piece/box to store.
type Component struct {
	// ID is the unique component id referenced by Group.Components.
	ID string `yaml:"id"`
	// Name is an optional display name.
	Name string `yaml:"name"`
	// Width, Depth and Height are the piece's bounding box (> 0).
	Width  float64 `yaml:"width"`
	Depth  float64 `yaml:"depth"`
	Height float64 `yaml:"height"`
	// Qty is how many identical pieces there are (> 0).
	Qty int `yaml:"qty"`
	// AllowRotate permits 90 degree in-plane rotation; nil means true.
	AllowRotate *bool `yaml:"allowRotate"`
}

// AllowsRotate reports whether this component may be rotated in-plane during
// packing. Defaults to true when unspecified.
func (c Component) AllowsRotate() bool {
	if c.AllowRotate == nil {
		return true
	}
	return *c.AllowRotate
}

// Group is an explicit, named compartment: a set of component ids that must
// be packed together into their own region of the box (a "bandage").
type Group struct {
	// ID is the unique group id; it must not look like a generated auto-N id.
	ID string `yaml:"id"`
	// Name is an optional display name.
	Name string `yaml:"name"`
	// Components lists the ids of the components packed into this group.
	Components []string `yaml:"components"`
	// Padding overrides Defaults.Padding for this group; nil inherits.
	Padding *float64 `yaml:"padding"`
	// Margin overrides Defaults.Margin for this group; nil inherits.
	Margin *float64 `yaml:"margin"`

	// FullWalls overrides Defaults.FullWalls for this group; nil inherits
	// the manifest-level default.
	FullWalls *bool `yaml:"fullWalls"`

	// JointType overrides Defaults.JointType for this group; nil inherits
	// the manifest-level default.
	JointType *JointType `yaml:"jointType"`

	// Dividers overrides Defaults.ResolvedDividers() for this group; nil
	// inherits the manifest-level default.
	Dividers *bool `yaml:"dividers"`

	// Expand overrides Defaults.Expand for this group; nil inherits. An
	// expanding group stretches in width (never depth, never moving its
	// packed content) to fill the rest of its row; with dividers on, the extra space becomes a new
	// empty cell separated by a divider.
	Expand *bool `yaml:"expand"`

	// Removable overrides Defaults.Removable for this group; nil inherits.
	Removable *bool `yaml:"removable"`

	// Floor overrides Defaults.Floor for this group; nil inherits.
	Floor *bool `yaml:"floor"`

	// ExternalHeightReduction overrides Defaults.ExternalHeightReduction for
	// this group; nil inherits.
	ExternalHeightReduction *HeightReduction `yaml:"externalHeightReduction"`

	// DividerHeightReduction overrides Defaults.DividerHeightReduction for
	// this group; nil inherits.
	DividerHeightReduction *HeightReduction `yaml:"dividerHeightReduction"`
}
