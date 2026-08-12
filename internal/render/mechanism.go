package render

// Mechanism is a rendering mechanism (Section 26.1 / REQ-096).
//
// Product-wide there are exactly three: static, template, gomod.
// Catalog [[files]] entries admit only static|template; gomod is the typed
// go.mod generator and is not a catalog file render mode.
type Mechanism string

const (
	// MechanismStatic copies catalog source bytes verbatim.
	MechanismStatic Mechanism = "static"
	// MechanismTemplate is restricted complete-file text/template rendering.
	MechanismTemplate Mechanism = "template"
	// MechanismGomod is typed go.mod generation via x/mod/modfile.
	MechanismGomod Mechanism = "gomod"
)

// String returns the mechanism name.
func (m Mechanism) String() string { return string(m) }

// Valid reports whether m is one of the three product mechanisms.
func (m Mechanism) Valid() bool {
	switch m {
	case MechanismStatic, MechanismTemplate, MechanismGomod:
		return true
	default:
		return false
	}
}

// Mechanisms returns the closed product set in stable order (REQ-096).
// Exhaustive: any new mechanism must appear here and in Valid.
func Mechanisms() []Mechanism {
	return []Mechanism{
		MechanismStatic,
		MechanismTemplate,
		MechanismGomod,
	}
}
