package manifest

import (
	"fmt"
	"math"
	"net/mail"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// Issue is a single validation problem found in a manifest.
type Issue struct {
	Path    string `json:"path"` // dotted/bracketed path to the offending field, e.g. "groups[0].components[1]"
	Message string `json:"message"`
}

func (i Issue) String() string {
	return fmt.Sprintf("%s: %s", i.Path, i.Message)
}

// ValidationResult is the outcome of validating a manifest.
type ValidationResult struct {
	Issues []Issue `json:"issues"`
}

// OK reports whether the manifest is valid (no issues found).
func (r ValidationResult) OK() bool {
	return len(r.Issues) == 0
}

// autoIDPrefix is the prefix of the ids the packer generates for auto
// compartments (see AutoCompartmentID).
const autoIDPrefix = "auto-"

// AutoCompartmentID returns the id the packer gives the n-th (1-based)
// auto-packed compartment. Group ids must not collide with these.
func AutoCompartmentID(n int) string { return fmt.Sprintf("%s%d", autoIDPrefix, n) }

// IsAutoCompartmentID reports whether id matches the pattern of generated
// auto compartment ids ("auto-" followed by a positive integer).
func IsAutoCompartmentID(id string) bool {
	rest, ok := strings.CutPrefix(id, autoIDPrefix)
	if !ok {
		return false
	}
	n, err := strconv.Atoi(rest)
	return err == nil && n >= 1 && AutoCompartmentID(n) == id
}

// finite reports whether v is neither NaN nor an infinity.
func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

// Validate checks a manifest's schema, cross-references, and value ranges.
// It does not attempt any packing.
func Validate(m *Manifest) ValidationResult {
	var issues []Issue
	add := func(path, format string, args ...any) {
		issues = append(issues, Issue{Path: path, Message: fmt.Sprintf(format, args...)})
	}

	if m.Version != 1 {
		add("version", "unsupported manifest version %d (expected 1)", m.Version)
	}

	validateProject(add, m.Project)

	switch m.Box.Units {
	case "", UnitsMM, UnitsCM, UnitsIN:
	default:
		add("box.units", "unknown units %q (expected mm, cm, or in)", m.Box.Units)
	}
	if m.Box.Name == "" {
		add("box.name", "must not be empty")
	}
	validateDimensions(&issues, "box.interior", m.Box.Interior, true)

	if !finite(m.Material.Thickness) {
		add("material.thickness", "must be a finite number, got %v", m.Material.Thickness)
	} else if m.Material.Thickness <= 0 {
		add("material.thickness", "must be > 0, got %v", m.Material.Thickness)
	}
	switch {
	case !finite(m.Material.Kerf):
		add("material.kerf", "must be a finite number, got %v", m.Material.Kerf)
	case m.Material.Kerf < 0:
		add("material.kerf", "must be >= 0, got %v", m.Material.Kerf)
	case finite(m.Material.Thickness) && m.Material.Thickness > 0 && m.Material.Kerf >= m.Material.Thickness:
		// The slot width is thickness+kerf (geometry.SlotWidth), so a kerf of
		// at least the thickness at least doubles it: almost certainly a
		// units slip (e.g. 3 instead of 0.3).
		add("material.kerf", "must be < material.thickness (%v), got %v; check the units", m.Material.Thickness, m.Material.Kerf)
	}

	if !finite(m.Defaults.Padding) {
		add("defaults.padding", "must be a finite number, got %v", m.Defaults.Padding)
	} else if m.Defaults.Padding < 0 {
		add("defaults.padding", "must be >= 0, got %v", m.Defaults.Padding)
	}
	if !finite(m.Defaults.Margin) {
		add("defaults.margin", "must be a finite number, got %v", m.Defaults.Margin)
	} else if m.Defaults.Margin < 0 {
		add("defaults.margin", "must be >= 0, got %v", m.Defaults.Margin)
	}
	switch m.Defaults.JointType {
	case "", JointTypeNotch, JointTypePlain:
	default:
		add("defaults.jointType", "unknown joint type %q (expected notch or plain)", m.Defaults.JointType)
	}

	for _, hr := range []struct {
		key   string
		r     HeightReduction
		class ReductionClass
	}{{"externalHeightReduction", m.Defaults.ExternalHeightReduction, ExternalPanels}, {"dividerHeightReduction", m.Defaults.DividerHeightReduction, DividerPanels}} {
		if msg := hr.r.Validate(hr.class); msg != "" {
			add("defaults."+hr.key, "%s", msg)
		}
	}

	seenIDs := map[string]bool{}
	for i, c := range m.Components {
		path := fmt.Sprintf("components[%d]", i)
		switch {
		case c.ID == "":
			add(path+".id", "must not be empty")
		case seenIDs[c.ID]:
			add(path+".id", "duplicate component id %q", c.ID)
		default:
			seenIDs[c.ID] = true
		}
		if c.Qty <= 0 {
			add(path+".qty", "must be > 0, got %d", c.Qty)
		}
		validateDimensions(&issues, path, Dimensions{c.Width, c.Depth, c.Height}, true)
	}

	seenGroupIDs := map[string]bool{}
	componentGroup := map[string]string{}
	// removableGroups tracks the resolved removable setting of each group so
	// mixing can be reported early (the renderer rejects it otherwise).
	firstRemovable := -1
	var firstRemovableVal bool
	mixedReported := false
	for i, g := range m.Groups {
		path := fmt.Sprintf("groups[%d]", i)
		switch {
		case g.ID == "":
			add(path+".id", "must not be empty")
		case seenGroupIDs[g.ID]:
			add(path+".id", "duplicate group id %q", g.ID)
		default:
			seenGroupIDs[g.ID] = true
		}
		if IsAutoCompartmentID(g.ID) {
			add(path+".id", "group id %q collides with the generated auto compartment ids (auto-N)", g.ID)
		}
		if len(g.Components) == 0 {
			add(path+".components", "must list at least one component id")
		}
		if g.Padding != nil {
			if !finite(*g.Padding) {
				add(path+".padding", "must be a finite number, got %v", *g.Padding)
			} else if *g.Padding < 0 {
				add(path+".padding", "must be >= 0, got %v", *g.Padding)
			}
		}
		if g.Margin != nil {
			if !finite(*g.Margin) {
				add(path+".margin", "must be a finite number, got %v", *g.Margin)
			} else if *g.Margin < 0 {
				add(path+".margin", "must be >= 0, got %v", *g.Margin)
			}
		}
		for _, hr := range []struct {
			key   string
			r     *HeightReduction
			class ReductionClass
		}{{"externalHeightReduction", g.ExternalHeightReduction, ExternalPanels}, {"dividerHeightReduction", g.DividerHeightReduction, DividerPanels}} {
			if hr.r == nil {
				continue
			}
			if msg := hr.r.Validate(hr.class); msg != "" {
				add(path+"."+hr.key, "%s", msg)
			}
		}
		removable := m.Defaults.Removable
		if g.Removable != nil {
			removable = *g.Removable
		}
		switch {
		case firstRemovable < 0:
			firstRemovable, firstRemovableVal = i, removable
		case removable != firstRemovableVal && !mixedReported:
			mixedReported = true
			add(path+".removable", "cannot mix removable and non-removable trays in one box: groups[%d] is removable=%t but this group is removable=%t (set removable on every tray, or none)", firstRemovable, firstRemovableVal, removable)
		}
		if g.JointType != nil {
			switch *g.JointType {
			case JointTypeNotch, JointTypePlain:
			default:
				add(path+".jointType", "unknown joint type %q (expected notch or plain)", *g.JointType)
			}
		}
		for j, cid := range g.Components {
			cpath := fmt.Sprintf("%s.components[%d]", path, j)
			if !seenIDs[cid] {
				add(cpath, "references unknown component id %q", cid)
				continue
			}
			if owner, ok := componentGroup[cid]; ok {
				if owner == g.ID {
					add(cpath, "component %q is listed more than once in group %q", cid, g.ID)
					continue
				}
				add(cpath, "component %q already belongs to group %q (a component may belong to at most one group)", cid, owner)
				continue
			}
			componentGroup[cid] = g.ID
		}
	}

	// Ungrouped components are auto-packed with the defaults, so they count
	// as one more (defaults-derived) removable setting and margin.
	hasUngrouped := false
	for _, c := range m.Components {
		if _, grouped := componentGroup[c.ID]; !grouped && c.ID != "" {
			hasUngrouped = true
			break
		}
	}
	if hasUngrouped && firstRemovable >= 0 && !mixedReported && m.Defaults.Removable != firstRemovableVal {
		add("defaults.removable", "cannot mix removable and non-removable trays in one box: groups[%d] is removable=%t but ungrouped components (auto compartments) use defaults.removable=%t (set removable on every tray, or none)", firstRemovable, firstRemovableVal, m.Defaults.Removable)
	}

	validateFeasibility(m, hasUngrouped, add)

	if issues == nil {
		issues = []Issue{} // serialise as [] rather than null
	}
	return ValidationResult{Issues: issues}
}

// Project field limits, in characters (runes).
const (
	maxProjectName        = 120
	maxProjectDescription = 1000
	maxProjectNotes       = 2000
	maxProjectField       = 200
	maxProjectTags        = 20
	maxProjectTag         = 50

	// projectDateLayout is the Go layout of an ISO 8601 calendar date.
	projectDateLayout = "2006-01-02"
)

// validateProject checks the optional project metadata block. Every field is
// optional; only values that are set are checked.
func validateProject(add func(path, format string, args ...any), p Project) {
	text := func(field, v string, max int, multiline bool) {
		path := "project." + field
		if n := utf8.RuneCountInString(v); n > max {
			add(path, "must be at most %d characters, got %d", max, n)
		}
		if bad, ok := badProjectRune(v, multiline); ok {
			add(path, "must not contain control character %U", bad)
		}
	}
	text("name", p.Name, maxProjectName, false)
	text("description", p.Description, maxProjectDescription, true)
	text("revision", p.Revision, maxProjectField, false)
	text("author", p.Author, maxProjectField, false)
	text("contact", p.Contact, maxProjectField, false)
	text("license", p.License, maxProjectField, false)
	text("url", p.URL, maxProjectField, false)
	text("created", p.Created, maxProjectField, false)
	text("updated", p.Updated, maxProjectField, false)
	text("notes", p.Notes, maxProjectNotes, true)
	text("game", p.Game, maxProjectField, false)
	text("publisher", p.Publisher, maxProjectField, false)
	text("edition", p.Edition, maxProjectField, false)

	// An omitted revision is fine, but one that is set must say something.
	if p.Revision != "" && strings.TrimSpace(p.Revision) == "" {
		add("project.revision", "must not be blank when set")
	}

	if looksLikeEmail(p.Contact) && !plausibleEmail(p.Contact) {
		add("project.contact", "looks like an email address but is not a valid email address, got %q", p.Contact)
	}

	if p.URL != "" {
		u, err := url.Parse(p.URL)
		switch {
		case err != nil:
			add("project.url", "must be a valid URL, got %q", p.URL)
		case u.Scheme != "http" && u.Scheme != "https":
			add("project.url", "must be an http or https URL, got %q", p.URL)
		case u.Hostname() == "":
			add("project.url", "must include a host, got %q", p.URL)
		}
	}

	var created, updated time.Time
	var createdOK, updatedOK bool
	date := func(field, v string) (time.Time, bool) {
		if v == "" {
			return time.Time{}, false
		}
		t, err := time.Parse(projectDateLayout, v)
		if err != nil || t.Format(projectDateLayout) != v {
			add("project."+field, "must be a date in YYYY-MM-DD format, got %q", v)
			return time.Time{}, false
		}
		return t, true
	}
	created, createdOK = date("created", p.Created)
	updated, updatedOK = date("updated", p.Updated)
	if createdOK && updatedOK && updated.Before(created) {
		add("project.updated", "must not be before project.created (%s), got %s", p.Created, p.Updated)
	}

	if len(p.Tags) > maxProjectTags {
		add("project.tags", "must have at most %d tags, got %d", maxProjectTags, len(p.Tags))
	}
	for i, tag := range p.Tags {
		path := fmt.Sprintf("project.tags[%d]", i)
		switch n := utf8.RuneCountInString(tag); {
		case strings.TrimSpace(tag) == "":
			add(path, "must not be empty")
		case n > maxProjectTag:
			add(path, "must be at most %d characters, got %d", maxProjectTag, n)
		}
		if bad, ok := badProjectRune(tag, false); ok {
			add(path, "must not contain control character %U", bad)
		}
	}
}

// badProjectRune returns the first rune of s that must not appear in project
// text: control characters (newline and tab are allowed when multiline) and
// the Unicode noncharacters U+FFFE/U+FFFF, which XML forbids.
func badProjectRune(s string, multiline bool) (rune, bool) {
	for _, r := range s {
		if multiline && (r == '\n' || r == '\t') {
			continue
		}
		if unicode.IsControl(r) || r == 0xFFFE || r == 0xFFFF {
			return r, true
		}
	}
	return 0, false
}

// looksLikeEmail reports whether a contact value is meant as an email address:
// it contains '@' and is either a bare address (no whitespace) or "Name <addr>".
// Handles ("@jane", "@jane@fosstodon.org"), URLs ("https://medium.com/@jane")
// and other free text are not treated as email.
func looksLikeEmail(s string) bool {
	if !strings.Contains(s, "@") || strings.HasPrefix(s, "@") || strings.Contains(s, "://") {
		return false
	}
	if strings.ContainsFunc(s, unicode.IsSpace) {
		return strings.Contains(s, "<") && strings.HasSuffix(s, ">")
	}
	return true
}

// plausibleEmail reports whether s parses as a single email address (a bare
// "user@host.tld" or "Name <user@host.tld>") whose domain contains a dot.
func plausibleEmail(s string) bool {
	a, err := mail.ParseAddress(s)
	if err != nil {
		return false
	}
	at := strings.LastIndex(a.Address, "@")
	domain := a.Address[at+1:]
	return at > 0 && strings.Contains(domain, ".") && !strings.HasPrefix(domain, ".") && !strings.HasSuffix(domain, ".")
}

// validateFeasibility flags box/material/spacing parameters that leave no room
// to pack anything, mirroring the arithmetic of pack.LayoutBox. It never
// considers individual components: a component that merely does not fit is a
// packing outcome, not a validation error. Fields that already failed their
// range checks are skipped.
func validateFeasibility(m *Manifest, hasUngrouped bool, add func(path, format string, args ...any)) {
	units := m.Box.Units
	if _, err := mmPerUnit(units); err != nil {
		return
	}
	ok := func(v float64) bool { return finite(v) && v >= 0 }
	interior, err := m.Box.Interior.ToMM(units)
	if err != nil || !finite(interior.Width) || !finite(interior.Depth) || !finite(interior.Height) ||
		interior.Width <= 0 || interior.Depth <= 0 || interior.Height <= 0 {
		return
	}
	mm := func(v float64) float64 { r, _ := ScalarToMM(v, units); return r }
	thickOK := finite(m.Material.Thickness) && m.Material.Thickness > 0
	thick := mm(m.Material.Thickness)

	if thickOK {
		if thick >= interior.Width || thick >= interior.Depth {
			add("material.thickness", "%s mm is not smaller than the box interior width/depth (%s x %s mm), so the walls cannot fit", fmtMM(thick), fmtMM(interior.Width), fmtMM(interior.Depth))
		}
		// A floor applies when some group resolves to floor=true (override,
		// else default) or ungrouped components inherit defaults.floor.
		floor := hasUngrouped && m.Defaults.Floor
		for _, g := range m.Groups {
			if g.Floor != nil && *g.Floor || g.Floor == nil && m.Defaults.Floor {
				floor = true
			}
		}
		if floor && thick >= interior.Height {
			add("material.thickness", "%s mm is not smaller than the box interior height (%s mm), so a floor plate leaves no room for contents", fmtMM(thick), fmtMM(interior.Height))
		}
	}

	// Each group insets its usable area by margin + wall thickness (removable
	// trays only) on every side, exactly as LayoutBox computes it.
	check := func(path string, margin float64, removable bool) bool {
		if !ok(margin) {
			return false
		}
		wall := 0.0
		if removable && thickOK {
			wall = thick
		}
		inset := mm(margin) + wall
		if 2*inset >= interior.Width || 2*inset >= interior.Depth {
			add(path, "2 x (margin %s mm + wall %s mm) = %s mm is not smaller than the box interior width/depth (%s x %s mm), leaving no usable space", fmtMM(mm(margin)), fmtMM(wall), fmtMM(2*inset), fmtMM(interior.Width), fmtMM(interior.Depth))
			return true
		}
		return false
	}
	defaultReported := false
	for i, g := range m.Groups {
		removable := m.Defaults.Removable
		if g.Removable != nil {
			removable = *g.Removable
		}
		if g.Margin != nil {
			check(fmt.Sprintf("groups[%d].margin", i), *g.Margin, removable)
		} else if !defaultReported {
			// Reported once, on the defaults field the groups inherit.
			defaultReported = check("defaults.margin", m.Defaults.Margin, removable)
		}
	}
	// Ungrouped components are auto-packed with the defaults.
	if hasUngrouped && !defaultReported {
		check("defaults.margin", m.Defaults.Margin, m.Defaults.Removable)
	}

	validateReductionFeasibility(m, hasUngrouped, interior.Height, thickOK, thick, mm, add)
}

// validateReductionFeasibility flags absolute height reductions that can never
// fit: the amount (in mm) is not smaller than the tallest panel it could trim,
// the box interior height minus the floor thickness where a floor is in
// effect for every tray using the value. It is deliberately conservative:
// percentages (always < 100) and reductions limited to named panels are
// skipped, as are values no tray uses, external reductions on trays without
// external panels, and the half-height rule for notched panels (whether a
// panel is notched is only known at render time).
func validateReductionFeasibility(m *Manifest, hasUngrouped bool, interiorH float64, thickOK bool, thick float64, mm func(float64) float64, add func(path, format string, args ...any)) {
	type tray struct {
		floor, removable, fullWalls bool
		ext, div                    bool // tray inherits the default reduction
	}
	var trays []tray
	anyFloor := false
	resolve := func(g *Group) tray {
		t := tray{floor: m.Defaults.Floor, removable: m.Defaults.Removable, fullWalls: m.Defaults.FullWalls, ext: true, div: true}
		if g != nil {
			if g.Floor != nil {
				t.floor = *g.Floor
			}
			if g.Removable != nil {
				t.removable = *g.Removable
			}
			if g.FullWalls != nil {
				t.fullWalls = *g.FullWalls
			}
			t.ext, t.div = g.ExternalHeightReduction == nil, g.DividerHeightReduction == nil
		}
		anyFloor = anyFloor || t.floor
		return t
	}
	for i := range m.Groups {
		trays = append(trays, resolve(&m.Groups[i]))
	}
	if hasUngrouped {
		trays = append(trays, resolve(nil))
	}
	// base is the tallest a panel of tray t can be: the interior height less
	// the floor thickness when that tray's panels stand on a floor (a shared
	// grid is raised by any tray's floor).
	base := func(t tray) float64 {
		if thickOK && (t.removable && t.floor || !t.removable && anyFloor) {
			return interiorH - thick
		}
		return interiorH
	}
	// flag reports the reduction r at path p when it is not smaller than the
	// panel base of every tray in users.
	flag := func(p string, r HeightReduction, users []tray) {
		if len(users) == 0 || len(r.Panels) > 0 || !finite(r.Amount) || r.Amount <= 0 {
			return
		}
		amount := mm(r.Amount)
		limit := 0.0
		for _, t := range users {
			limit = math.Max(limit, base(t))
		}
		if amount >= limit {
			add(p, "reduction %s mm leaves no panel height (interior height %s mm, available panel height %s mm)", fmtMM(amount), fmtMM(interiorH), fmtMM(limit))
		}
	}
	// External reductions only trim removable tray walls and fullWalls boundary walls.
	hasExternal := func(t tray) bool { return t.removable || t.fullWalls }

	var defExt, defDiv []tray
	for i, t := range trays {
		if t.ext && hasExternal(t) {
			defExt = append(defExt, t)
		}
		if t.div {
			defDiv = append(defDiv, t)
		}
		if i >= len(m.Groups) {
			continue
		}
		g := m.Groups[i]
		if g.ExternalHeightReduction != nil && hasExternal(t) {
			flag(fmt.Sprintf("groups[%d].externalHeightReduction", i), *g.ExternalHeightReduction, []tray{t})
		}
		if g.DividerHeightReduction != nil {
			flag(fmt.Sprintf("groups[%d].dividerHeightReduction", i), *g.DividerHeightReduction, []tray{t})
		}
	}
	flag("defaults.externalHeightReduction", m.Defaults.ExternalHeightReduction, defExt)
	flag("defaults.dividerHeightReduction", m.Defaults.DividerHeightReduction, defDiv)
}

func fmtMM(v float64) string { return strconv.FormatFloat(v, 'g', 6, 64) }

func validateDimensions(issues *[]Issue, path string, d Dimensions, requirePositive bool) {
	check := func(field string, v float64) {
		if !finite(v) {
			*issues = append(*issues, Issue{Path: path + "." + field, Message: fmt.Sprintf("must be a finite number, got %v", v)})
			return
		}
		if requirePositive && v <= 0 {
			*issues = append(*issues, Issue{Path: path + "." + field, Message: fmt.Sprintf("must be > 0, got %v", v)})
		}
	}
	check("width", d.Width)
	check("depth", d.Depth)
	check("height", d.Height)
}
