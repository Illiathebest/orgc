package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"orgc/internal/backend"
	"orgc/internal/diag"
	"orgc/internal/ir"
	"orgc/internal/verify"
	"orgc/schema"
)

// withInvariants swaps the registry for the duration of a test.
func withInvariants(t *testing.T, checks ...verify.Check) {
	t.Helper()
	saved := invariants
	invariants = func() []verify.Check { return checks }
	t.Cleanup(func() { invariants = saved })
}

// finding is a check that reports one diagnostic on the first network, so
// that the exit code and the output can be tested before any real
// invariant exists.
func finding(code diag.Code, sev diag.Severity) verify.Check {
	return verify.Check{Code: code, Run: func(m *ir.Model, _ backend.Platforms) []diag.Diagnostic {
		path := m.Networks[0].Path + ".cidr"
		pos := m.At(path)
		return []diag.Diagnostic{{
			Code: code, Severity: sev, Message: "тестова знахідка",
			File: m.File, Line: pos.Line, Column: pos.Column, Path: path,
		}}
	}}
}

func runVerifyOn(t *testing.T, src string) (int, string, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "organization.yml")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"verify", path}, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

// With no invariants registered, verify behaves exactly as the structural
// layer alone did before lowering and invariants joined the pipeline — on
// every model of the corpus. Lowering adds no findings of its own.
func TestEmptyRegistryChangesNothing(t *testing.T) {
	withInvariants(t)

	files, _ := filepath.Glob(filepath.Join("..", "..", "testdata", "valid", "*.yml"))
	broken, _ := filepath.Glob(filepath.Join("..", "..", "testdata", "broken", "*.yml"))
	if len(files) == 0 || len(broken) == 0 {
		t.Fatal("корпус testdata не знайдено")
	}

	for _, f := range append(files, broken...) {
		name := filepath.Base(f)
		t.Run(name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			got := run([]string{"verify", f}, &stdout, &stderr)

			// Structural fixtures fail; everything else — valid models and
			// invariant fixtures, whose checks do not exist yet — passes.
			want := exitOK
			structural := strings.HasPrefix(name, "s") || name == "aggregate.yml"
			if filepath.Base(filepath.Dir(f)) == "broken" && structural {
				want = exitModel
			}
			if got != want {
				t.Errorf("код виходу = %d, want %d\nstdout: %s\nstderr: %s",
					got, want, stdout.String(), stderr.String())
			}
			if stderr.Len() != 0 {
				t.Errorf("stderr: %s", stderr.String())
			}
		})
	}
}

// ADR-008, decision 4: warnings alone leave the model valid, and severity —
// not the code prefix — decides, so W002 at error level fails the run.
func TestExitCodeFollowsSeverity(t *testing.T) {
	tests := []struct {
		name    string
		checks  []verify.Check
		want    int
		summary string
	}{
		{"лише попередження", []verify.Check{finding("W003", diag.Warning)}, exitOK, "Знайдено 1 попередження"},
		{"W002 рівня error", []verify.Check{finding("W002", diag.Error)}, exitModel, "Знайдено 1 помилку"},
		{"помилка інваріанта", []verify.Check{finding("E006", diag.Error)}, exitModel, "Знайдено 1 помилку"},
		{"помилка і попередження", []verify.Check{
			finding("E001", diag.Error), finding("W001", diag.Warning),
		}, exitModel, "Знайдено 1 помилку і 1 попередження"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withInvariants(t, tt.checks...)
			code, out, errOut := runVerifyOn(t, validModel)
			if code != tt.want {
				t.Errorf("код виходу = %d, want %d\n%s%s", code, tt.want, out, errOut)
			}
			if !strings.Contains(out, tt.summary) {
				t.Errorf("підсумок не містить %q:\n%s", tt.summary, out)
			}
		})
	}
}

// An invariant finding reaches the administrator with a line, through the
// position index that now outlives Load.
func TestInvariantFindingCarriesPosition(t *testing.T) {
	withInvariants(t, finding("E001", diag.Error))
	_, out, _ := runVerifyOn(t, validModel)

	want := "organization.yml:10:57: помилка E001: тестова знахідка — sites[0].networks[0].cidr"
	if !strings.Contains(out, want) {
		t.Errorf("вивід не містить\n  %s\nотримано:\n%s", want, out)
	}
}

// A model with structural errors has no definite meaning, so the invariants
// must not even run on it.
func TestStructuralErrorsStopThePipeline(t *testing.T) {
	ran := false
	withInvariants(t, verify.Check{Code: "E001", Run: func(*ir.Model, backend.Platforms) []diag.Diagnostic {
		ran = true
		return nil
	}})

	code, _, _ := runVerifyOn(t, "organization: test\n")
	if code != exitModel {
		t.Errorf("код виходу = %d, want %d", code, exitModel)
	}
	if ran {
		t.Error("інваріант виконався на структурно зламаній моделі")
	}
}

// A check that breaks its contract is a defect of orgc, reported like any
// other: exit 2 and a message on stderr, not a verdict about the model.
func TestBrokenCheckIsAnOrgcDefect(t *testing.T) {
	withInvariants(t, verify.Check{Code: "E001", Run: finding("E002", diag.Error).Run})

	code, out, errOut := runVerifyOn(t, validModel)
	if code != exitInvoked {
		t.Errorf("код виходу = %d, want %d", code, exitInvoked)
	}
	if out != "" || !strings.Contains(errOut, "E002") {
		t.Errorf("stdout = %q, stderr = %q", out, errOut)
	}
}

// Every device type the schema admits must have a platform, or E006 and
// W002 would have nothing to ask about a perfectly valid device. The list is
// read from the schema itself, so adding a type there without a backend
// fails here.
func TestEverySchemaDeviceTypeHasAPlatform(t *testing.T) {
	var s struct {
		Defs map[string]struct {
			Properties map[string]struct {
				Enum []string `json:"enum"`
			} `json:"properties"`
		} `json:"$defs"`
	}
	if err := json.Unmarshal(schema.Organization, &s); err != nil {
		t.Fatal(err)
	}
	types := s.Defs["device"].Properties["type"].Enum
	if len(types) == 0 {
		t.Fatal("у схемі не знайдено переліку device.type")
	}

	reg, err := platforms()
	if err != nil {
		t.Fatal(err)
	}
	for _, typ := range types {
		if _, ok := reg.Lookup(ir.DeviceType(typ)); !ok {
			t.Errorf("тип пристрою %q є в схемі, але жодна платформа його не обслуговує", typ)
		}
	}
	if got := len(reg.Types()); got != len(types) {
		t.Errorf("платформ %d, типів у схемі %d — зайва платформа без типу в схемі?", got, len(types))
	}
}
