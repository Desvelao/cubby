// Package render turns a packed box layout into cuttable/renderable panel
// geometry via a pluggable Backend.
package render

import (
	"github.com/Desvelao/cubby/internal/geometry"
	"github.com/Desvelao/cubby/internal/manifest"
	"github.com/Desvelao/cubby/internal/pack"
)

// RenderOptions carries the knobs that affect how a Backend generates
// geometry, independent of the packing result itself.
//
// It is currently empty: panel height comes from the box interior height and
// the manifest's height reduction settings.
type RenderOptions struct{}

// Backend converts a packed box layout into cuttable/renderable panel
// geometry.
//
// v1 ships exactly one implementation: slotted egg-crate divider panels
// (see the slotted subpackage) — flat 2D panels with interlocking notches,
// extruded to material thickness for 3D export.
//
// FUTURE (v2, NOT implemented here): a second Backend modeling a solid
// 3D-printed tray with component-shaped pockets subtracted from a block
// (fundamentally different, subtractive-solid geometry, for people who want
// to 3D print an insert rather than assemble cut panels). It will implement
// this same interface; internal/pack needs no changes to support it.
type Backend interface {
	Name() string
	Render(box pack.BoxResult, mat manifest.Material, opts RenderOptions) ([]geometry.Panel, error)
}
