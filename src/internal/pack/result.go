package pack

import (
	"path"

	"github.com/Desvelao/cubby/internal/manifest"
)

// Rect is an axis-aligned placement footprint, in millimeters, relative to
// the origin of whatever it was packed into.
type Rect struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	W float64 `json:"w"`
	D float64 `json:"d"`
}

// Space describes leftover room after packing.
type Space struct {
	Width float64 `json:"width"`
	Depth float64 `json:"depth"`
	Area  float64 `json:"area"`
}

// MissingItem records component instances that could not be placed.
type MissingItem struct {
	ComponentID string `json:"componentId"`
	Requested   int    `json:"requested"`
	Placed      int    `json:"placed"`
	Rejected    int    `json:"rejected"`
	Reason      string `json:"reason"` // "too-tall" | "too-wide" | "too-deep" | "no-space"
}

// PlacedItem is one packed instance's footprint within its row.
type PlacedItem struct {
	ID       string  `json:"id"`
	Instance int     `json:"instance"`
	Rect     Rect    `json:"rect"`
	Height   float64 `json:"height"`
	Rotated  bool    `json:"rotated"`
}

// ShelfRow is one row of a shelf-packed layout. UsedW is the row's total
// consumed width including padding on both sides of each item.
type ShelfRow struct {
	Y      float64      `json:"y"`
	Height float64      `json:"height"`
	UsedW  float64      `json:"usedW"`
	Items  []PlacedItem `json:"items"`
}

// ShelfPackResult is the outcome of packing a set of Items via PackShelf.
type ShelfPackResult struct {
	Rows      []ShelfRow
	UsedW     float64
	UsedD     float64
	MaxW      float64
	MaxD      float64
	Remaining Space
	Missing   []MissingItem
}

// CompartmentResult is one packed compartment ("bandage") within the box.
type CompartmentResult struct {
	ID        string        `json:"id"`
	Name      string        `json:"name"`
	Kind      string        `json:"kind"` // "group" | "auto"
	Bounds    Rect          `json:"bounds"`
	Rows      []ShelfRow    `json:"rows"`
	UsedW     float64       `json:"usedW"`
	UsedD     float64       `json:"usedD"`
	Remaining Space         `json:"remaining"`
	Missing   []MissingItem `json:"missing"`

	// FullWalls resolves manifest.Defaults.FullWalls / Group.FullWalls: when
	// true, the render backend generates real divider panels on every side
	// of this compartment, including edges that touch the outer box wall.
	FullWalls bool `json:"fullWalls"`

	// JointType resolves manifest.Defaults.JointType / Group.JointType: an
	// empty value behaves as manifest.JointTypeNotch.
	JointType manifest.JointType `json:"jointType"`

	// Dividers resolves manifest.Defaults.Dividers / Group.Dividers
	// (defaulting to true when unset): when true, the render backend
	// generates internal divider panels between this compartment's own
	// shelf-packed Rows.
	Dividers bool `json:"dividers"`

	// ContentOffsetX, ContentOffsetY locate this compartment's packed row
	// content (the local frame Rows[i].Y and each row's Items are expressed
	// in) relative to Bounds.X/Y. Equals this compartment's resolved margin
	// for "group" compartments (whose Bounds already include margin on
	// every side), and zero for "auto" compartments (whose Bounds are
	// already tight to content). Set once at pack time rather than derived
	// from Bounds/UsedW at render time, because FillRemaining can later
	// grow a trailing compartment's Bounds.W/D without moving its packed
	// content -- a derived offset would then wrongly shift that content.
	ContentOffsetX float64 `json:"contentOffsetX"`
	ContentOffsetY float64 `json:"contentOffsetY"`

	// Expand resolves manifest.Defaults.Expand / Group.Expand. When set,
	// the layout may stretch Bounds.W along its row to the box edge; CoreW
	// then holds the pre-growth width so the render backend can divide the
	// extra space off as its own cell.
	Expand bool    `json:"expand"`
	CoreW  float64 `json:"coreW"`

	// Removable resolves manifest.Defaults.Removable / Group.Removable: the
	// tray is rendered as its own open box of wall thickness WallT (already
	// included in Bounds and ContentOffset). Floor adds a floor plate.
	Removable bool    `json:"removable"`
	Floor     bool    `json:"floor"`
	WallT     float64 `json:"wallT"`

	// ExternalReduction and DividerReduction resolve the manifest height
	// reduction settings (amount already in mm).
	ExternalReduction HeightReduction `json:"externalReduction"`
	DividerReduction  HeightReduction `json:"dividerReduction"`
}

// ReductionError reports a height-reduction setting that the manifest cannot
// honor (an unconvertible amount, or one too large for the panel it trims). It
// marks the problem as manifest-caused so callers can tell it from internal
// or I/O failures.
type ReductionError struct{ Err error }

func (e *ReductionError) Error() string { return e.Err.Error() }
func (e *ReductionError) Unwrap() error { return e.Err }

// HeightReduction is a resolved panel-height reduction: AmountMM or Percent
// of the panel height, optionally limited to panels whose id matches one of
// Panels (`*` globs); empty Panels means every panel of the class.
type HeightReduction struct {
	AmountMM float64  `json:"amountMM,omitempty"`
	Percent  float64  `json:"percent,omitempty"`
	Panels   []string `json:"panels,omitempty"`
}

// For returns how much to cut from a panel of the given id and height.
func (r HeightReduction) For(id string, height float64) float64 {
	if r.AmountMM == 0 && r.Percent == 0 {
		return 0
	}
	if len(r.Panels) > 0 {
		match := false
		for _, p := range r.Panels {
			if ok, _ := path.Match(p, id); ok {
				match = true
				break
			}
		}
		if !match {
			return 0
		}
	}
	return r.AmountMM + height*r.Percent/100
}

// BoxResult is the full packed layout of a box.
type BoxResult struct {
	BoxName      string              `json:"boxName"`
	InteriorW    float64             `json:"interiorW"`
	InteriorD    float64             `json:"interiorD"`
	InteriorH    float64             `json:"interiorH"`
	Compartments []CompartmentResult `json:"compartments"`
	UsedW        float64             `json:"usedW"`
	UsedD        float64             `json:"usedD"`
	Remaining    Space               `json:"remaining"`
	TotalMissing []MissingItem       `json:"totalMissing"`

	// Material is the manifest's material normalized to millimeters (thickness
	// and kerf converted from box.units). Render and export use this, never
	// the raw manifest material.
	Material manifest.Material `json:"material"`

	// Project is the manifest's optional project metadata (name, author,
	// license, ...). It is nil, and omitted from JSON, when the manifest has
	// no project block or the block sets no field.
	Project *manifest.Project `json:"project,omitempty"`
}
