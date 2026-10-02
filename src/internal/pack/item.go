// Package pack implements the deterministic grid/shelf bin-packing engine
// shared by component-in-compartment and compartment-in-box layout.
package pack

// Item is one type of thing to place: either a component (with its
// manifest-declared Qty) or a single compartment (Qty always 1) being
// arranged among its siblings within the box.
type Item struct {
	ID          string
	Qty         int
	W, D, H     float64
	AllowRotate bool
}
