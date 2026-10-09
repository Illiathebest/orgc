package verify

import (
	"strings"
	"testing"

	"orgc/internal/backend"
	"orgc/internal/diag"
	"orgc/internal/ir"
)

func model() *ir.Model {
	return &ir.Model{
		File: "organization.yml",
		Positions: map[string]diag.Pos{
			"sites[0].networks[0].cidr": {Line: 10, Column: 57},
			"sites[0].networks[1].cidr": {Line: 11, Column: 57},
		},
	}
}

func emit(code diag.Code, paths ...string) Check {
	return Check{Code: code, Run: func(m *ir.Model, _ backend.Platforms) []diag.Diagnostic {
		var out []diag.Diagnostic
		for _, p := range paths {
			out = append(out, report(m, code, code.DefaultSeverity(), p, "знахідка"))
		}
		return out
	}}
}

func TestReportLocatesThroughIR(t *testing.T) {
	d := report(model(), "E001", diag.Error, "sites[0].networks[1].cidr", "перетин")
	want := diag.Diagnostic{
		Code: "E001", Severity: diag.Error, Message: "перетин",
		File: "organization.yml", Line: 11, Column: 57, Path: "sites[0].networks[1].cidr",
	}
	if d != want {
		t.Errorf("report = %+v\nwant     %+v", d, want)
	}

	// A path the file never had keeps its path but no line, which is how
	// the formatter recognises an unknown position.
	if d := report(model(), "E001", diag.Error, "nowhere", "x"); d.Line != 0 || d.Path != "nowhere" {
		t.Errorf("невідомий шлях: %+v", d)
	}
}

// Output order is the file's, not the registry's: a later check reporting
// an earlier line must come first.
func TestRunSortsAcrossChecks(t *testing.T) {
	ds, err := Run(model(), nil, []Check{
		emit("W001", "sites[0].networks[1].cidr"),
		emit("E001", "sites[0].networks[0].cidr"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(ds) != 2 || ds[0].Code != "E001" || ds[1].Code != "W001" {
		t.Errorf("порядок: %v", ds)
	}
}

func TestRunRejectsForeignCode(t *testing.T) {
	bad := emit("E002", "sites[0].networks[0].cidr")
	bad.Code = "E001"

	_, err := Run(model(), nil, []Check{bad})
	if err == nil || !strings.Contains(err.Error(), "E002") {
		t.Errorf("err = %v, want відмову з назвою чужого коду", err)
	}
}

func TestRunWithNoChecks(t *testing.T) {
	ds, err := Run(model(), nil, Checks())
	if err != nil || len(ds) != 0 {
		t.Errorf("ds = %v, err = %v", ds, err)
	}
}

// Every registered check is unique and carries an invariant code. The
// registry is empty until the invariants are implemented; this guards it
// as it fills.
func TestRegistryCodesAreUniqueInvariants(t *testing.T) {
	seen := map[diag.Code]bool{}
	for _, c := range Checks() {
		if seen[c.Code] {
			t.Errorf("код %s зареєстровано двічі", c.Code)
		}
		seen[c.Code] = true
		if len(c.Code) != 4 || (c.Code[0] != 'E' && c.Code[0] != 'W') {
			t.Errorf("%q не є кодом інваріанта", c.Code)
		}
		if c.Run == nil {
			t.Errorf("%s без функції перевірки", c.Code)
		}
	}
}
