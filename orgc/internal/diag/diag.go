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
//	E  — invariant violations (E001-E008), the scientific core;
//	W  — invariant warnings (W001-W004);
//	S  — structural defects, i.e. what the JSON Schema catches before any
//	     invariant runs. E and W were already taken by the invariants, so
//	     structural codes needed a prefix of their own.
type Code string

// Structural codes. They are grouped by what the administrator has to do
// about them, not by JSON Schema keyword: a single "the value is not allowed
// here" class is more useful than nine keyword-shaped ones. The mapping from
// validator keywords to these codes lives in internal/model (plan step 13).
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
