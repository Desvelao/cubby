package pack

import "sort"

// expandCompartments stretches every Expand compartment along its row: the
// compartments are grouped into bands by Bounds.Y, and each band's leftover
// width (box width minus its rightmost edge) is split equally among that
// band's expandable compartments, shifting the ones to their right. Depth is
// never changed, and packed content never moves, so the grown area is empty
// space; CoreW records the pre-growth width.
func expandCompartments(cs []CompartmentResult, boxW float64) {
	bands := map[float64][]int{}
	var ys []float64
	for i := range cs {
		if cs[i].Expand {
			cs[i].CoreW = cs[i].Bounds.W
		}
		key := cs[i].Bounds.Y
		matched := false
		for _, y := range ys {
			if abs(y-key) <= epsilon {
				key, matched = y, true
				break
			}
		}
		if !matched {
			ys = append(ys, key)
		}
		bands[key] = append(bands[key], i)
	}

	for _, y := range ys {
		idx := bands[y]
		sort.Slice(idx, func(a, b int) bool { return cs[idx[a]].Bounds.X < cs[idx[b]].Bounds.X })
		right, n := 0.0, 0
		for _, i := range idx {
			right = max(right, cs[i].Bounds.X+cs[i].Bounds.W)
			if cs[i].Expand {
				n++
			}
		}
		if n == 0 {
			continue
		}
		share := max(boxW-right, 0) / float64(n)
		shift := 0.0
		for _, i := range idx {
			cs[i].Bounds.X += shift
			if cs[i].Expand {
				cs[i].Bounds.W += share
				cs[i].Remaining = shrinkSpace(cs[i].Remaining, share, 0, share*cs[i].Bounds.D)
				shift += share
			}
		}
	}
}

// shrinkSpace removes growth that has been absorbed into a compartment's
// Bounds from its Remaining slack (the slack is now inside Bounds, so it is no
// longer free space outside them). Every field is clamped at 0, which also
// keeps group compartments (whose Remaining is always empty) at zero.
func shrinkSpace(s Space, w, d, area float64) Space {
	return Space{
		Width: max(s.Width-w, 0),
		Depth: max(s.Depth-d, 0),
		Area:  max(s.Area-area, 0),
	}
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
