package model

import (
	"os"
	"path/filepath"
	"testing"

	"orgc/internal/diag"
)

// examplesDir is the repository's examples/, a sibling of the module. Plan
// step 14 moves the corpus into orgc/testdata/; until then these are the
// only real models there are, and they are also what CI validates.
func examplePath(t *testing.T, name string) string {
	t.Helper()
	p := filepath.Join("..", "..", "..", "examples", name)
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("не знайдено приклад %s: %v", name, err)
	}
	return p
}

func TestValidExamplesLoadClean(t *testing.T) {
	for _, name := range []string{
		"organization.s-profile.yml",
		"organization.m-profile.yml",
	} {
		t.Run(name, func(t *testing.T) {
			org, ds, err := Load(examplePath(t, name))
			if err != nil {
				t.Fatalf("err = %v", err)
			}
			if len(ds) != 0 {
				t.Fatalf("діагностик %d, want 0:\n%s", len(ds), render(t, ds))
			}
			if org == nil {
				t.Fatal("org = nil")
			}
			if org.Organization == "" || len(org.Sites) == 0 {
				t.Errorf("модель розібралася порожньою: %+v", org)
			}
		})
	}
}

// The broken example carries one defect per numbered comment. The test does
// not pin the exact count — step 14 will own that — but it does pin the two
// properties the step is about: every finding has a code this package knows,
// and every finding that can have a position has one.
func TestBrokenExampleIsFullyDescribed(t *testing.T) {
	path := examplePath(t, "organization.broken.yml")

	org, ds, err := Load(path)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if org != nil {
		t.Error("org != nil, хоча модель має помилки")
	}
	if !diag.HasErrors(ds) {
		t.Fatal("зламаний приклад не дав жодної помилки")
	}

	for _, d := range ds {
		if d.Code == diag.SOther {
			t.Errorf("непокритий вид помилки валідатора — у відображенні дірка:\n  %s", d)
		}
		if d.Message == "" {
			t.Errorf("порожнє повідомлення: %s", d)
		}
		if d.File != path {
			t.Errorf("файл = %q, want %q", d.File, path)
		}
		// Only a finding about the document as a whole may lack a position.
		if d.Line == 0 && d.Path != "" {
			t.Errorf("діагностика зі шляхом, але без рядка: %s", d)
		}
	}
}

// Every structural code the package can produce should be exercised by the
// corpus; otherwise the wording of the unexercised ones is never checked.
func TestBrokenExampleCoversStructuralCodes(t *testing.T) {
	_, ds, err := Load(examplePath(t, "organization.broken.yml"))
	if err != nil {
		t.Fatal(err)
	}

	seen := make(map[diag.Code]bool, len(ds))
	for _, d := range ds {
		seen[d.Code] = true
	}

	// S001 needs malformed YAML and S008's length branch needs an empty
	// string, neither of which belongs in a file that must stay parseable;
	// both are covered by the unit tests instead.
	want := []diag.Code{
		diag.SRequiredMissing, diag.SUnknownField, diag.STypeMismatch,
		diag.SNotInEnum, diag.SFormatMismatch, diag.SOutOfRange,
		diag.SArrayConstraint, diag.SFieldDependency, diag.SValueForbidden,
	}
	for _, c := range want {
		if !seen[c] {
			t.Errorf("код %s (%s) не трапився в зламаному прикладі", c, c.Title())
		}
	}
}
