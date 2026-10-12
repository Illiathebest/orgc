// Package diag defines a diagnostic: code, severity, Ukrainian message, path
// to the model element, and position in the source file.
//
// Ordering is the package's responsibility, not the caller's: the JSON Schema
// validator emits findings in map-iteration order, which differs between runs
// (see docs/adr/ADR-006). Diagnostics are sorted here so that the same input
// always produces byte-identical output.
package diag

import "sort"

// Severity says how a finding affects the exit code: an Error fails the
// model, a Warning does not.
type Severity int

const (
	Error Severity = iota
	Warning
)

// String renders the severity for the administrator.
func (s Severity) String() string {
	switch s {
	case Warning:
		return "попередження"
	default:
		return "помилка"
	}
}

// Code identifies a class of defect.
//
// Three prefixes are in use and they do not overlap:
//
//	E  — invariant violations (E001-E010), the scientific core;
//	W  — invariant warnings (W001-W005);
//	S  — structural defects, i.e. what the JSON Schema catches before any
//	     invariant runs. E and W were already taken by the invariants, so
//	     structural codes needed a prefix of their own.
type Code string

// Structural codes. They are grouped by what the administrator has to do
// about them, not by JSON Schema keyword: a single "the value is not allowed
// here" class is more useful than nine keyword-shaped ones. The mapping from
// validator keywords to these codes lives in internal/model.
const (
	SYAMLSyntax      Code = "S001" // the file is not valid YAML
	SRequiredMissing Code = "S002" // a required field is absent
	SUnknownField    Code = "S003" // a field the schema does not define
	STypeMismatch    Code = "S004" // wrong value type
	SNotInEnum       Code = "S005" // value outside the allowed set
	SFormatMismatch  Code = "S006" // value does not match the required format
	SOutOfRange      Code = "S007" // numeric value outside its bounds
	SArrayConstraint Code = "S008" // array constraint violated (duplicates, length)
	SFieldDependency Code = "S009" // a field requires another field
	SValueForbidden  Code = "S010" // value explicitly disallowed (reserved name)

	// SOther is the safety net for a validator finding this package has no
	// Ukrainian wording for. It should never reach an administrator: a test
	// asserts that the whole broken-model corpus produces no S099. If one
	// appears, the mapping in internal/model has a gap — the code exists so
	// that the gap is reported with a position instead of silently dropped.
	SOther Code = "S099"
)

// Invariant codes, defined in docs/data-model.md. An invariant always builds
// a concrete message, so these codes need no titles.
const (
	E001 Code = "E001" // CIDR overlap
	E002 Code = "E002" // identifier uniqueness
	E003 Code = "E003" // reference integrity
	E004 Code = "E004" // mgmt_ip in a mgmt network of its own site
	E005 Code = "E005" // default-deny preserved
	E006 Code = "E006" // management reachability
	E007 Code = "E007" // address plan consistency
	E008 Code = "E008" // prefix capacity
	E009 Code = "E009" // L2 connectivity of a site with links
	E010 Code = "E010" // a site's links form a tree rooted at the gateway
	W001 Code = "W001" // guest network not isolated
	W002 Code = "W002" // platform lacks a capability or the OS version
	W003 Code = "W003" // profile M site without a backup uplink
	W004 Code = "W004" // mgmt network open beyond the control node
	W005 Code = "W005" // role levels grow along the tree from the gateway
)

// titles give each code a short Ukrainian name. A Diagnostic normally carries
// a concrete Message; the title is the fallback when it has none, so that a
// diagnostic is never printed as a bare code.
var titles = map[Code]string{
	SYAMLSyntax:      "файл не є коректним YAML",
	SRequiredMissing: "відсутнє обовʼязкове поле",
	SUnknownField:    "невідоме поле",
	STypeMismatch:    "неправильний тип значення",
	SNotInEnum:       "значення поза переліком дозволених",
	SFormatMismatch:  "значення не відповідає формату",
	SOutOfRange:      "значення поза допустимим діапазоном",
	SArrayConstraint: "порушено обмеження списку",
	SFieldDependency: "поле потребує іншого поля",
	SValueForbidden:  "значення заборонено",
	SOther:           "структурна помилка моделі",
}

// Title returns the Ukrainian name of the defect class, or an empty string
// for a code the package does not know (invariant codes carry their own text).
func (c Code) Title() string { return titles[c] }

// DefaultSeverity reports how a code is normally classified.
//
// It is a default, not a rule: W002 is raised at error level when the device
// OS version falls outside what its backend supports, because then the device
// cannot be rendered at all. That is why Diagnostic carries Severity itself
// instead of deriving it from the prefix.
func (c Code) DefaultSeverity() Severity {
	if len(c) > 0 && c[0] == 'W' {
		return Warning
	}
	return Error
}

// Pos is a place in the source file. Line and Column are 1-based; the zero
// Pos means the position is unknown.
type Pos struct {
	Line   int
	Column int
}

// Known reports whether the position points somewhere in the file.
func (p Pos) Known() bool { return p.Line > 0 }

// Diagnostic is one finding about the model.
type Diagnostic struct {
	Code     Code
	Severity Severity

	// Message is the Ukrainian text shown to the administrator, without a
	// trailing period. Empty means "use the code's title".
	Message string

	// File, Line and Column locate the finding in the source. Line and
	// Column are 1-based; zero means the position is unknown, which happens
	// for findings about the document as a whole.
	File   string
	Line   int
	Column int

	// Path points at the model element, in the readable form produced by
	// PathFromPointer, e.g. "sites[0].networks[2].role". Empty for
	// document-level findings.
	Path string
}

// Text returns the message, falling back to the code title.
func (d Diagnostic) Text() string {
	if d.Message != "" {
		return d.Message
	}
	if t := d.Code.Title(); t != "" {
		return t
	}
	return "невідома помилка"
}

// Sort orders diagnostics so that the same input always yields the same
// output. Position comes first because that is the order an administrator
// reads the file in; the remaining keys only break ties, but they must be
// total — two findings on one line are common, and leaving their order to
// the validator would break byte-identical output.
func Sort(ds []Diagnostic) {
	sort.SliceStable(ds, func(i, j int) bool {
		a, b := ds[i], ds[j]
		switch {
		case a.File != b.File:
			return a.File < b.File
		case a.Line != b.Line:
			return a.Line < b.Line
		case a.Column != b.Column:
			return a.Column < b.Column
		case a.Code != b.Code:
			return a.Code < b.Code
		case a.Path != b.Path:
			return a.Path < b.Path
		default:
			return a.Text() < b.Text()
		}
	})
}

// HasErrors reports whether anything here should fail the run. Warnings
// alone leave the model valid.
func HasErrors(ds []Diagnostic) bool {
	for _, d := range ds {
		if d.Severity == Error {
			return true
		}
	}
	return false
}

// Count returns how many errors and warnings the set holds.
func Count(ds []Diagnostic) (errors, warnings int) {
	for _, d := range ds {
		if d.Severity == Warning {
			warnings++
		} else {
			errors++
		}
	}
	return errors, warnings
}
