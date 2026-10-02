package pack

import "sort"

const epsilon = 1e-9

// instance is one unit copy of an Item, canonically oriented for packing.
type instance struct {
	id      string
	idx     int
	fw, fd  float64 // canonical footprint: fw feeds row width, fd feeds row height
	rotated bool
	origW   float64
	origD   float64
	origH   float64
}

// PackShelf places items into a maxW x maxD x maxH region using a
// deterministic row-based (shelf) heuristic: items are sorted largest-depth
// first and packed left-to-right into rows, wrapping to a new row when the
// current row runs out of width.
func PackShelf(items []Item, maxW, maxD, maxH, padding float64) ShelfPackResult {
	requested := map[string]int{}
	var instances []instance
	missing := map[string]*MissingItem{}

	reject := func(id string, qty int, reason string) {
		mi, ok := missing[id]
		if !ok {
			mi = &MissingItem{ComponentID: id, Reason: reason}
			missing[id] = mi
		}
		mi.Rejected += qty
	}

	for _, it := range items {
		requested[it.ID] += it.Qty
		if it.H > maxH+epsilon {
			reject(it.ID, it.Qty, "too-tall")
			continue
		}
		fw, fd, rotated := it.W, it.D, false
		if it.AllowRotate {
			fw, fd = min(it.W, it.D), max(it.W, it.D)
			rotated = fw != it.W
		}
		if fw+2*padding > maxW+epsilon {
			reject(it.ID, it.Qty, "too-wide")
			continue
		}
		if fd+2*padding > maxD+epsilon {
			// Fall back to the other orientation when rotation allows it.
			if it.AllowRotate && fw+2*padding <= maxD+epsilon && fd+2*padding <= maxW+epsilon {
				fw, fd = fd, fw
				rotated = fw != it.W
			} else {
				reject(it.ID, it.Qty, "too-deep")
				continue
			}
		}
		for i := 0; i < it.Qty; i++ {
			instances = append(instances, instance{
				id: it.ID, idx: i, fw: fw, fd: fd, rotated: rotated,
				origW: it.W, origD: it.D, origH: it.H,
			})
		}
	}

	sort.Slice(instances, func(i, j int) bool {
		a, b := instances[i], instances[j]
		if a.fd != b.fd {
			return a.fd > b.fd
		}
		if a.fw != b.fw {
			return a.fw > b.fw
		}
		if a.id != b.id {
			return a.id < b.id
		}
		return a.idx < b.idx
	})

	var rows []ShelfRow
	placed := map[string]int{}

	type rowBuilder struct {
		y, height, usedW float64
		items            []PlacedItem
	}
	var cur *rowBuilder
	cursorY := 0.0
	flush := func() {
		if cur != nil && len(cur.items) > 0 {
			rows = append(rows, ShelfRow{Y: cur.y, Height: cur.height, UsedW: cur.usedW, Items: cur.items})
		}
	}

	for _, inst := range instances {
		ew, ed := inst.fw+2*padding, inst.fd+2*padding

		if cur == nil {
			cur = &rowBuilder{y: cursorY}
		}
		if cur.usedW+ew > maxW+epsilon && len(cur.items) > 0 {
			flush()
			cursorY += cur.height
			cur = &rowBuilder{y: cursorY}
		}
		if cursorY+ed > maxD+epsilon {
			// Only this instance is rejected; later (shallower) ones may still fit.
			reject(inst.id, 1, "no-space")
			continue
		}

		x := cur.usedW + padding
		y := cur.y + padding
		cur.items = append(cur.items, PlacedItem{
			ID: inst.id, Instance: inst.idx,
			Rect:    Rect{X: x, Y: y, W: inst.fw, D: inst.fd},
			Height:  inst.origH,
			Rotated: inst.rotated,
		})
		cur.usedW += ew
		if ed > cur.height {
			cur.height = ed
		}
		placed[inst.id]++
	}
	flush()

	usedW, usedD := 0.0, 0.0
	for _, r := range rows {
		if r.UsedW > usedW {
			usedW = r.UsedW
		}
		if bottom := r.Y + r.Height; bottom > usedD {
			usedD = bottom
		}
	}

	var missingList []MissingItem
	for id, mi := range missing {
		mi.Requested = requested[id]
		mi.Placed = placed[id]
		missingList = append(missingList, *mi)
	}
	sort.Slice(missingList, func(i, j int) bool { return missingList[i].ComponentID < missingList[j].ComponentID })

	return ShelfPackResult{
		Rows:  rows,
		UsedW: usedW,
		UsedD: usedD,
		MaxW:  maxW,
		MaxD:  maxD,
		Remaining: Space{
			Width: maxW - usedW,
			Depth: maxD - usedD,
			Area:  maxW*maxD - usedW*usedD,
		},
		Missing: missingList,
	}
}
