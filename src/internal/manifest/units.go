package manifest

import "fmt"

// mmPerUnit converts one unit of u into millimeters.
func mmPerUnit(u Units) (float64, error) {
	switch u {
	case "", UnitsMM:
		return 1, nil
	case UnitsCM:
		return 10, nil
	case UnitsIN:
		return 25.4, nil
	default:
		return 0, fmt.Errorf("unknown units %q (expected mm, cm, or in)", u)
	}
}

// ToMM converts a Dimensions value authored in u into millimeters.
func (d Dimensions) ToMM(u Units) (Dimensions, error) {
	factor, err := mmPerUnit(u)
	if err != nil {
		return Dimensions{}, err
	}
	return Dimensions{
		Width:  d.Width * factor,
		Depth:  d.Depth * factor,
		Height: d.Height * factor,
	}, nil
}

// ScalarToMM converts a single scalar length authored in u into millimeters.
func ScalarToMM(v float64, u Units) (float64, error) {
	factor, err := mmPerUnit(u)
	if err != nil {
		return 0, err
	}
	return v * factor, nil
}

// ToMM converts a Material whose thickness and kerf are authored in u into
// millimeters. It is the single normalization point for material lengths:
// pack.LayoutBox calls it once and everything downstream works in mm.
func (m Material) ToMM(u Units) (Material, error) {
	t, err := ScalarToMM(m.Thickness, u)
	if err != nil {
		return Material{}, err
	}
	k, err := ScalarToMM(m.Kerf, u)
	if err != nil {
		return Material{}, err
	}
	return Material{Name: m.Name, Thickness: t, Kerf: k}, nil
}
