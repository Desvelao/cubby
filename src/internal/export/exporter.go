// Package export defines the shared exporter interface implemented by each
// output format (console, csv, svg, dxf, stl, step).
package export

import (
	"errors"
	"io"

	"github.com/Desvelao/cubby/internal/geometry"
	"github.com/Desvelao/cubby/internal/manifest"
	"github.com/Desvelao/cubby/internal/pack"
)

// Exporter renders a packed box layout (and, for formats that need it, its
// generated panel geometry) to w. Panels is nil for formats that only need
// the packing result (e.g. console).
type Exporter interface {
	Format() string
	Export(w io.Writer, box pack.BoxResult, panels []geometry.Panel, mat manifest.Material) error
}

// ErrNoPanels is wrapped by the errors that 3D exporters (stl, step) return
// when there is no panel geometry to write, so callers can recognise the case.
var ErrNoPanels = errors.New("no panels to export")
