package pack

import (
	"fmt"
	"sort"

	"github.com/Desvelao/cubby/internal/manifest"
)

// componentMM is a manifest.Component with dimensions already converted to
// millimeters.
type componentMM struct {
	id, name             string
	width, depth, height float64
	qty                  int
	allowRotate          bool
}

// LayoutBox packs a manifest into a BoxResult: each explicit group becomes
// its own compartment, and any components not listed in a group are
// auto-packed into the box's remaining space.
func LayoutBox(m *manifest.Manifest) (BoxResult, error) {
	interior, err := m.Box.Interior.ToMM(m.Box.Units)
	if err != nil {
		return BoxResult{}, err
	}

	material, err := m.Material.ToMM(m.Box.Units)
	if err != nil {
		return BoxResult{}, err
	}

	components := map[string]componentMM{}
	var order []string
	for _, c := range m.Components {
		dims, err := manifest.Dimensions{Width: c.Width, Depth: c.Depth, Height: c.Height}.ToMM(m.Box.Units)
		if err != nil {
			return BoxResult{}, err
		}
		components[c.ID] = componentMM{
			id: c.ID, name: c.Name,
			width: dims.Width, depth: dims.Depth, height: dims.Height,
			qty: c.Qty, allowRotate: c.AllowsRotate(),
		}
		order = append(order, c.ID)
	}

	defaultPadding, err := manifest.ScalarToMM(m.Defaults.Padding, m.Box.Units)
	if err != nil {
		return BoxResult{}, err
	}
	defaultMargin, err := manifest.ScalarToMM(m.Defaults.Margin, m.Box.Units)
	if err != nil {
		return BoxResult{}, err
	}
	defaultDividers := m.Defaults.ResolvedDividers()
	defaultExternal, err := resolveReduction(m.Defaults.ExternalHeightReduction, m.Box.Units)
	if err != nil {
		return BoxResult{}, err
	}
	defaultDivider, err := resolveReduction(m.Defaults.DividerHeightReduction, m.Box.Units)
	if err != nil {
		return BoxResult{}, err
	}

	grouped := map[string]bool{}
	var compartmentItems []Item
	compartmentByID := map[string]CompartmentResult{}
	groupComponents := map[string][]string{}
	// groupInnerReason keeps the per-component rejection reasons of each
	// group's own packing, for groups later dropped by box packing.
	groupInnerReason := map[string]map[string]string{}
	// droppedMissing holds the rejected components of groups that ended up with
	// nothing placed; such groups produce no compartment at all.
	var droppedMissing []MissingItem

	// In the shared-grid mode any tray with a floor raises the whole grid by
	// one thickness (see render/slotted), so it costs every tray that height.
	anyFloor := m.Defaults.Floor
	for _, g := range m.Groups {
		if g.Floor != nil && *g.Floor {
			anyFloor = true
		}
	}

	for _, g := range m.Groups {
		padding := defaultPadding
		if g.Padding != nil {
			padding, err = manifest.ScalarToMM(*g.Padding, m.Box.Units)
			if err != nil {
				return BoxResult{}, err
			}
		}
		margin := defaultMargin
		if g.Margin != nil {
			margin, err = manifest.ScalarToMM(*g.Margin, m.Box.Units)
			if err != nil {
				return BoxResult{}, err
			}
		}
		fullWalls := m.Defaults.FullWalls
		if g.FullWalls != nil {
			fullWalls = *g.FullWalls
		}
		jointType := m.Defaults.JointType
		if g.JointType != nil {
			jointType = *g.JointType
		}
		dividers := defaultDividers
		if g.Dividers != nil {
			dividers = *g.Dividers
		}

		expand := m.Defaults.Expand
		if g.Expand != nil {
			expand = *g.Expand
		}

		removable := m.Defaults.Removable
		if g.Removable != nil {
			removable = *g.Removable
		}
		floor := m.Defaults.Floor
		if g.Floor != nil {
			floor = *g.Floor
		}
		external := defaultExternal
		if g.ExternalHeightReduction != nil {
			external, err = resolveReduction(*g.ExternalHeightReduction, m.Box.Units)
			if err != nil {
				return BoxResult{}, err
			}
		}
		divider := defaultDivider
		if g.DividerHeightReduction != nil {
			divider, err = resolveReduction(*g.DividerHeightReduction, m.Box.Units)
			if err != nil {
				return BoxResult{}, err
			}
		}
		wallT := 0.0
		if removable {
			wallT = material.Thickness
		}

		// Components must fit under the floor plate the walls stand on.
		maxInnerH := interior.Height
		if (removable && floor) || (!removable && anyFloor) {
			maxInnerH -= material.Thickness
		}

		var items []Item
		groupComponents[g.ID] = g.Components
		for _, cid := range g.Components {
			c, ok := components[cid]
			if !ok {
				return BoxResult{}, fmt.Errorf("group %q references unknown component %q", g.ID, cid)
			}
			grouped[cid] = true
			items = append(items, Item{ID: c.id, Qty: c.qty, W: c.width, D: c.depth, H: c.height, AllowRotate: c.allowRotate})
		}

		// maxD here is deliberately just a large wrapping bound (the box
		// interior depth), not the compartment's real extent: a group
		// compartment always sizes itself tightly to its own content
		// (Bounds == used + margin), so inner.Remaining is a meaningless
		// artifact of that wrapping bound and must not be surfaced.
		inset := margin + wallT
		inner := PackShelf(items, interior.Width-2*inset, interior.Depth-2*inset, maxInnerH, padding)
		if len(inner.Rows) == 0 {
			// Nothing placeable: emitting a zero-content tray would feed a
			// degenerate item to box packing and the renderer. Report the
			// components instead.
			droppedMissing = append(droppedMissing, inner.Missing...)
			continue
		}
		// Height comes from what was actually placed, so a rejected (e.g.
		// too-tall) component cannot make the whole tray fail box packing.
		maxH := 0.0
		for _, row := range inner.Rows {
			for _, pi := range row.Items {
				if pi.Height > maxH {
					maxH = pi.Height
				}
			}
		}
		cr := CompartmentResult{
			ID: g.ID, Name: g.Name, Kind: "group",
			Bounds:         Rect{W: inner.UsedW + 2*inset, D: inner.UsedD + 2*inset},
			Rows:           inner.Rows,
			UsedW:          inner.UsedW,
			UsedD:          inner.UsedD,
			Remaining:      Space{},
			Missing:        inner.Missing,
			FullWalls:      fullWalls,
			JointType:      jointType,
			Dividers:       dividers,
			Expand:         expand,
			Removable:      removable,
			Floor:          floor,
			WallT:          wallT,
			ContentOffsetX: inset,
			ContentOffsetY: inset,

			ExternalReduction: external,
			DividerReduction:  divider,
		}
		compartmentByID[g.ID] = cr
		if len(inner.Missing) > 0 {
			reasons := map[string]string{}
			for _, mi := range inner.Missing {
				reasons[mi.ComponentID] = mi.Reason
			}
			groupInnerReason[g.ID] = reasons
		}
		compartmentItems = append(compartmentItems, Item{
			ID: g.ID, Qty: 1, W: cr.Bounds.W, D: cr.Bounds.D, H: maxH, AllowRotate: false,
		})
	}

	// Arrange group-compartments within the box interior.
	boxPack := PackShelf(compartmentItems, interior.Width, interior.Depth, interior.Height, 0)

	var compartments []CompartmentResult
	for _, row := range boxPack.Rows {
		for _, placed := range row.Items {
			cr := compartmentByID[placed.ID]
			cr.Bounds.X = placed.Rect.X
			cr.Bounds.Y = placed.Rect.Y
			compartments = append(compartments, cr)
		}
	}

	// Groups that did not fit are reported as their member components, since
	// the group ID is not a component ID. A member the group's own packing
	// already rejected keeps that more specific reason (e.g. too-tall); the
	// box-level reason applies to the rest. Nothing of a dropped group is
	// placed, so every member is fully rejected.
	var totalMissing []MissingItem
	for _, mi := range boxPack.Missing {
		for _, cid := range groupComponents[mi.ComponentID] {
			c := components[cid]
			reason := mi.Reason
			if inner, ok := groupInnerReason[mi.ComponentID][cid]; ok {
				reason = inner
			}
			totalMissing = append(totalMissing, MissingItem{ComponentID: cid, Requested: c.qty, Placed: 0, Rejected: c.qty, Reason: reason})
		}
	}

	// Leftover free rectangles: trailing gap per row + bottom gap below all rows.
	var freeRects []Rect
	for _, row := range boxPack.Rows {
		if gap := interior.Width - row.UsedW; gap > epsilon {
			freeRects = append(freeRects, Rect{X: row.UsedW, Y: row.Y, W: gap, D: row.Height})
		}
	}
	if len(boxPack.Rows) > 0 {
		last := boxPack.Rows[len(boxPack.Rows)-1]
		if gap := interior.Depth - (last.Y + last.Height); gap > epsilon {
			freeRects = append(freeRects, Rect{X: 0, Y: last.Y + last.Height, W: interior.Width, D: gap})
		}
	} else {
		freeRects = append(freeRects, Rect{X: 0, Y: 0, W: interior.Width, D: interior.Depth})
	}
	sort.Slice(freeRects, func(i, j int) bool { return freeRects[i].W*freeRects[i].D > freeRects[j].W*freeRects[j].D })

	// Auto-pack ungrouped components into leftover rects, largest first.
	//
	// Policy (current behaviour):
	//   - Free rects are the trailing gap of each group row plus the bottom gap
	//     below all rows (the whole interior when there are no groups), sorted
	//     by area, largest first, not by position. Each rect is shelf-packed in
	//     turn with whatever is still unplaced; a rect that receives items
	//     becomes auto-1, auto-2, ... (numbered in placement order, listed by
	//     position afterwards), so a component can spill across two or more
	//     rects.
	//   - Only defaults apply: padding, removable, floor, fullWalls, jointType,
	//     dividers, expand. Per-group settings are ignored, with one exception:
	//     with non-removable defaults, a floor on any group lowers the shared
	//     grid and so reduces the auto height limit by one thickness (anyFloor).
	//   - With defaults.removable each auto tray has walls, so every rect is
	//     shrunk by 2*autoT (one thickness per side) on both axes before packing.
	//   - Only the first failure reason per component is kept (from the largest
	//     rect tried first), even if later rects fail differently; a leftover
	//     with no recorded reason is reported as "no-space".
	var ungrouped []Item
	for _, id := range order {
		if grouped[id] {
			continue
		}
		c := components[id]
		ungrouped = append(ungrouped, Item{ID: c.id, Qty: c.qty, W: c.width, D: c.depth, H: c.height, AllowRotate: c.allowRotate})
	}

	// autoReason keeps the first (largest free rect) reason each ungrouped
	// component failed with.
	autoReason := map[string]string{}
	autoIdx := 0
	for _, fr := range freeRects {
		if len(ungrouped) == 0 {
			break
		}
		autoT := 0.0
		if m.Defaults.Removable {
			autoT = material.Thickness
		}
		autoH := interior.Height
		if (m.Defaults.Removable && m.Defaults.Floor) || (!m.Defaults.Removable && anyFloor) {
			autoH -= material.Thickness
		}
		res := PackShelf(ungrouped, fr.W-2*autoT, fr.D-2*autoT, autoH, defaultPadding)
		for _, mi := range res.Missing {
			if _, ok := autoReason[mi.ComponentID]; !ok {
				autoReason[mi.ComponentID] = mi.Reason
			}
		}
		if len(res.Rows) == 0 {
			continue
		}
		autoIdx++
		compartments = append(compartments, CompartmentResult{
			ID: manifest.AutoCompartmentID(autoIdx), Name: fmt.Sprintf("Auto compartment %d", autoIdx), Kind: "auto",
			Bounds:         Rect{X: fr.X, Y: fr.Y, W: res.UsedW + 2*autoT, D: res.UsedD + 2*autoT},
			Rows:           res.Rows,
			UsedW:          res.UsedW,
			UsedD:          res.UsedD,
			Remaining:      Space{Width: fr.W - res.UsedW - 2*autoT, Depth: fr.D - res.UsedD - 2*autoT, Area: fr.W*fr.D - (res.UsedW+2*autoT)*(res.UsedD+2*autoT)},
			Missing:        nil,
			FullWalls:      m.Defaults.FullWalls,
			JointType:      m.Defaults.JointType,
			Dividers:       defaultDividers,
			Expand:         m.Defaults.Expand,
			Removable:      m.Defaults.Removable,
			Floor:          m.Defaults.Floor,
			WallT:          autoT,
			ContentOffsetX: autoT,
			ContentOffsetY: autoT,

			ExternalReduction: defaultExternal,
			DividerReduction:  defaultDivider,
		})

		placedIDs := map[string]int{}
		for _, r := range res.Rows {
			for _, it := range r.Items {
				placedIDs[it.ID]++
			}
		}
		var remaining []Item
		for _, it := range ungrouped {
			left := it.Qty - placedIDs[it.ID]
			if left > 0 {
				it.Qty = left
				remaining = append(remaining, it)
			}
		}
		ungrouped = remaining
	}
	for _, it := range ungrouped {
		reason := autoReason[it.ID]
		if reason == "" {
			reason = "no-space"
		}
		requested := components[it.ID].qty
		totalMissing = append(totalMissing, MissingItem{ComponentID: it.ID, Requested: requested, Placed: requested - it.Qty, Rejected: it.Qty, Reason: reason})
	}
	totalMissing = append(totalMissing, droppedMissing...)
	for _, cr := range compartments {
		totalMissing = append(totalMissing, cr.Missing...)
	}
	sort.Slice(totalMissing, func(i, j int) bool { return totalMissing[i].ComponentID < totalMissing[j].ComponentID })

	sort.Slice(compartments, func(i, j int) bool {
		if compartments[i].Bounds.Y != compartments[j].Bounds.Y {
			return compartments[i].Bounds.Y < compartments[j].Bounds.Y
		}
		return compartments[i].Bounds.X < compartments[j].Bounds.X
	})

	expandCompartments(compartments, interior.Width)

	// FillRemaining grows the spatially-last compartment (bottom-most, then
	// right-most) to reach the box's true edges. This is always safe even
	// when it shares a row/column with siblings: being last-by-position
	// guarantees nothing exists to its right or below it, so there's never
	// an overlap — at worst one row becomes taller on one side, which the
	// render backend handles like any other differently-sized neighbor.
	if m.Defaults.FillRemaining && len(compartments) > 0 {
		last := &compartments[len(compartments)-1]
		growW := interior.Width - (last.Bounds.X + last.Bounds.W)
		growD := interior.Depth - (last.Bounds.Y + last.Bounds.D)
		if growW < 0 {
			growW = 0
		}
		if growD < 0 {
			growD = 0
		}
		if growW > 0 || growD > 0 {
			oldArea := last.Bounds.W * last.Bounds.D
			last.Bounds.W += growW
			last.Bounds.D += growD
			// The grown strip is now inside Bounds, so it leaves Remaining.
			last.Remaining = shrinkSpace(last.Remaining, growW, growD, last.Bounds.W*last.Bounds.D-oldArea)
		}
	}

	usedW, usedD := boxPack.UsedW, boxPack.UsedD
	for _, cr := range compartments {
		if right := cr.Bounds.X + cr.Bounds.W; right > usedW {
			usedW = right
		}
		if bottom := cr.Bounds.Y + cr.Bounds.D; bottom > usedD {
			usedD = bottom
		}
	}

	var project *manifest.Project
	if !m.Project.IsZero() {
		p := m.Project
		p.Tags = append([]string(nil), p.Tags...)
		project = &p
	}

	return BoxResult{
		BoxName:      m.Box.Name,
		Material:     material,
		Project:      project,
		InteriorW:    interior.Width,
		InteriorD:    interior.Depth,
		InteriorH:    interior.Height,
		Compartments: compartments,
		UsedW:        usedW,
		UsedD:        usedD,
		Remaining: Space{
			Width: interior.Width - usedW,
			Depth: interior.Depth - usedD,
			Area:  interior.Width*interior.Depth - usedW*usedD,
		},
		TotalMissing: totalMissing,
	}, nil
}

// resolveReduction converts a manifest height reduction to millimeters.
func resolveReduction(r manifest.HeightReduction, units manifest.Units) (HeightReduction, error) {
	amount, err := manifest.ScalarToMM(r.Amount, units)
	if err != nil {
		return HeightReduction{}, &ReductionError{Err: err}
	}
	return HeightReduction{AmountMM: amount, Percent: r.Percent, Panels: r.Panels}, nil
}
