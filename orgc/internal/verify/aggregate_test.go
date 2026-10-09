package verify

import (
	"bytes"
	"testing"

	"orgc/internal/diag"
)

// The stage criterion: a model that passes the schema but breaks every
// invariant is caught by exactly its codes — twelve diagnostics, one per
// code, each on the line of its defect — and never by a generic error.
func TestInvariantsAggregate(t *testing.T) {
	want := []struct {
		code diag.Code
		line int
		sev  diag.Severity
	}{
		{diag.W003, 16, diag.Warning},
		{diag.W001, 32, diag.Warning},
		{diag.E001, 37, diag.Error},
		{diag.E002, 39, diag.Error},
		{diag.E008, 44, diag.Error},
		{diag.E007, 48, diag.Error},
		{diag.E004, 60, diag.Error},
		{diag.W002, 64, diag.Error},
		{diag.E006, 100, diag.Error},
		{diag.E003, 103, diag.Error},
		{diag.W004, 105, diag.Warning},
		{diag.E005, 110, diag.Error},
	}

	ds := runChecks(t, lowerFile(t, testdata("broken", "invariants-aggregate.yml")), Checks())
	if len(ds) != len(want) {
		t.Fatalf("діагностик %d, want %d:\n%s", len(ds), len(want), render(ds))
	}
	seen := map[diag.Code]bool{}
	for i, w := range want {
		d := ds[i]
		if d.Code != w.code || d.Line != w.line || d.Severity != w.sev {
			t.Errorf("#%d: %s рядок %d рівень %v, want %s рядок %d рівень %v\n  %s",
				i, d.Code, d.Line, d.Severity, w.code, w.line, w.sev, d)
		}
		seen[d.Code] = true
	}
	for _, c := range Checks() {
		if !seen[c.Code] {
			t.Errorf("зведений приклад не порушує %s", c.Code)
		}
	}
}

// Same input, byte-identical output, over repeated runs of the whole
// pipeline from source to rendered diagnostics.
func TestAggregateOutputIsDeterministic(t *testing.T) {
	out := func() []byte {
		var b bytes.Buffer
		ds := runChecks(t, lowerFile(t, testdata("broken", "invariants-aggregate.yml")), Checks())
		if err := diag.Render(&b, ds); err != nil {
			t.Fatal(err)
		}
		return b.Bytes()
	}
	first := out()
	if bytes.IndexByte(first, '\r') >= 0 {
		t.Error("у виводі є CR — закінчення рядків мають бути LF")
	}
	for i := 0; i < 20; i++ {
		if got := out(); !bytes.Equal(got, first) {
			t.Fatalf("прогін %d дав інший вивід", i)
		}
	}
}
