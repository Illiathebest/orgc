package diag

import (
	"math/rand"
	"testing"
)

func TestSeverityString(t *testing.T) {
	if got, want := Error.String(), "помилка"; got != want {
		t.Errorf("Error = %q, want %q", got, want)
	}
	if got, want := Warning.String(), "попередження"; got != want {
		t.Errorf("Warning = %q, want %q", got, want)
	}
}

func TestDefaultSeverityByPrefix(t *testing.T) {
	tests := []struct {
		code Code
		want Severity
	}{
		{"E001", Error},
		{"E006", Error},
		{"W001", Warning},
		{"W004", Warning},
		{SRequiredMissing, Error},
		{SYAMLSyntax, Error},
		{"", Error},
	}
	for _, tt := range tests {
		if got := tt.code.DefaultSeverity(); got != tt.want {
			t.Errorf("Code(%q).DefaultSeverity() = %v, want %v", tt.code, got, tt.want)
		}
	}
}

// W002 is raised at error level when the OS version falls outside what the
// backend supports: the device cannot be rendered at all. Severity therefore
// has to be settable per diagnostic, not derived from the code.
func TestSeverityCanOverrideDefault(t *testing.T) {
	d := Diagnostic{Code: "W002", Severity: Error, Message: "версія ОС поза діапазоном бекенда"}
	if d.Severity != Error {
		t.Fatalf("Severity = %v, want Error", d.Severity)
	}
	if got := Code("W002").DefaultSeverity(); got != Warning {
		t.Errorf("типовий рівень W002 = %v, want Warning", got)
	}
	if !HasErrors([]Diagnostic{d}) {
		t.Error("HasErrors = false, хоча діагностика має рівень помилки")
	}
}

// Every structural code must carry a Ukrainian title: a diagnostic should
// never reach the administrator as a bare code.
func TestEveryStructuralCodeHasTitle(t *testing.T) {
	codes := []Code{
		SYAMLSyntax, SRequiredMissing, SUnknownField, STypeMismatch,
		SNotInEnum, SFormatMismatch, SOutOfRange, SArrayConstraint,
		SFieldDependency, SValueForbidden,
	}
	seen := make(map[string]Code, len(codes))
	for _, c := range codes {
		title := c.Title()
		if title == "" {
			t.Errorf("код %q без заголовка", c)
			continue
		}
		if prev, dup := seen[title]; dup {
			t.Errorf("коди %q і %q мають однаковий заголовок %q", prev, c, title)
		}
		seen[title] = c
	}
	if len(seen) != len(codes) {
		t.Errorf("унікальних заголовків %d, кодів %d", len(seen), len(codes))
	}
}

func TestTextFallsBackToTitle(t *testing.T) {
	if got, want := (Diagnostic{Code: SNotInEnum}).Text(), "значення поза переліком дозволених"; got != want {
		t.Errorf("Text() = %q, want %q", got, want)
	}
	if got, want := (Diagnostic{Code: SNotInEnum, Message: "профіль має бути S або M"}).Text(),
		"профіль має бути S або M"; got != want {
		t.Errorf("Text() = %q, want %q", got, want)
	}
	if got, want := (Diagnostic{Code: "E006"}).Text(), "невідома помилка"; got != want {
		t.Errorf("Text() для коду без заголовка = %q, want %q", got, want)
	}
}

func TestCount(t *testing.T) {
	ds := []Diagnostic{
		{Code: "E001", Severity: Error},
		{Code: "W001", Severity: Warning},
		{Code: "E002", Severity: Error},
		{Code: "W002", Severity: Error},
	}
	e, w := Count(ds)
	if e != 3 || w != 1 {
		t.Errorf("Count = (%d, %d), want (3, 1)", e, w)
	}
}

func TestHasErrorsIgnoresWarnings(t *testing.T) {
	warnings := []Diagnostic{{Code: "W001", Severity: Warning}, {Code: "W003", Severity: Warning}}
	if HasErrors(warnings) {
		t.Error("HasErrors = true для самих лише попереджень")
	}
	if HasErrors(nil) {
		t.Error("HasErrors = true для порожнього набору")
	}
}

// The whole point of the package: the validator hands findings over in
// map-iteration order, so sorting must impose a total order that does not
// depend on how they arrived.
func TestSortIsTotalAndInputOrderIndependent(t *testing.T) {
	// Messages are spelled out even where the fallback title would do: the
	// last sort key compares Text(), and relying on the fallback made an
	// earlier version of this test assert the wrong order.
	want := []Diagnostic{
		{File: "a.yml", Line: 1, Column: 1, Code: "S002", Message: "аа"},
		{File: "organization.yml", Line: 10, Column: 5, Code: "S005", Message: "аа"},
		{File: "organization.yml", Line: 23, Column: 42, Code: "S004", Message: "аа"},
		{File: "organization.yml", Line: 23, Column: 42, Code: "S005", Path: "a", Message: "аа"},
		{File: "organization.yml", Line: 23, Column: 42, Code: "S005", Path: "b", Message: "аа"},
		{File: "organization.yml", Line: 23, Column: 42, Code: "S005", Path: "b", Message: "бб"},
		{File: "organization.yml", Line: 99, Code: "S002", Message: "аа"},
	}

	rng := rand.New(rand.NewSource(1))
	for attempt := 0; attempt < 50; attempt++ {
		shuffled := make([]Diagnostic, len(want))
		copy(shuffled, want)
		rng.Shuffle(len(shuffled), func(i, j int) {
			shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
		})

		Sort(shuffled)
		for i := range want {
			if shuffled[i] != want[i] {
				t.Fatalf("спроба %d, позиція %d:\n отримано %+v\n очікувано %+v",
					attempt, i, shuffled[i], want[i])
			}
		}
	}
}

func TestSortPutsUnknownPositionFirstInItsFile(t *testing.T) {
	ds := []Diagnostic{
		{File: "organization.yml", Line: 5, Code: "S005"},
		{File: "organization.yml", Line: 0, Code: "S002"},
	}
	Sort(ds)
	if ds[0].Line != 0 {
		t.Errorf("діагностика без позиції має йти першою у своєму файлі, отримано рядок %d", ds[0].Line)
	}
}
