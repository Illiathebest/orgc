package model

import (
	"os"
	"path/filepath"
	"testing"

	"orgc/internal/diag"
)

// The corpus lives at orgc/testdata, one directory above the packages that
// use it, because the plan keeps it shared: internal/verify will exercise
// the same models against the invariants.
func testdata(parts ...string) string {
	return filepath.Join(append([]string{"..", "..", "testdata"}, parts...)...)
}

// expected pins what each one-defect model must produce. A line of zero
// means the finding belongs to the document as a whole and carries no
// position.
//
// The point of one defect per file is that a negative test proves which
// code fired. A test that only asserted "something failed" would stay green
// if a schema edit silently replaced one defect class with another.
var expected = map[string]struct {
	code diag.Code
	line int
}{
	"s001-bad-yaml.yml":               {diag.SYAMLSyntax, 6},
	"s002-missing-control-node.yml":   {diag.SRequiredMissing, 0},
	"s002-missing-device-fields.yml":  {diag.SRequiredMissing, 18},
	"s002-missing-network-role.yml":   {diag.SRequiredMissing, 16},
	"s003-unknown-rule-field.yml":     {diag.SUnknownField, 26},
	"s003-unknown-top-level.yml":      {diag.SUnknownField, 24},
	"s004-allow-overlap-not-bool.yml": {diag.STypeMismatch, 14},
	"s004-server-role-not-array.yml":  {diag.STypeMismatch, 18},
	"s005-device-type.yml":            {diag.SNotInEnum, 19},
	"s005-network-role.yml":           {diag.SNotInEnum, 16},
	"s005-profile.yml":                {diag.SNotInEnum, 5},
	"s005-rule-action.yml":            {diag.SNotInEnum, 26},
	"s005-server-role.yml":            {diag.SNotInEnum, 18},
	"s005-uplink-kind.yml":            {diag.SNotInEnum, 15},
	"s006-bad-cidr.yml":               {diag.SFormatMismatch, 16},
	"s006-bad-identifier.yml":         {diag.SFormatMismatch, 13},
	"s006-bad-ipv4.yml":               {diag.SFormatMismatch, 22},
	"s006-unknown-segment.yml":        {diag.SFormatMismatch, 26},
	"s007-vlan-too-large.yml":         {diag.SOutOfRange, 16},
	"s008-empty-organization.yml":     {diag.SArrayConstraint, 4},
	"s008-ports-duplicate.yml":        {diag.SArrayConstraint, 26},
	"s009-proto-without-ports.yml":    {diag.SFieldDependency, 26},
	"s010-network-named-any.yml":      {diag.SValueForbidden, 16},
}

// aggregateName is the combined model: every defect of the corpus in one
// file, which is also what CI validates through examples/.
const aggregateName = "aggregate.yml"

func TestValidCorpusLoadsClean(t *testing.T) {
	files, err := filepath.Glob(testdata("valid", "*.yml"))
	if err != nil || len(files) == 0 {
		t.Fatalf("у testdata/valid немає моделей: %v", err)
	}

	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			doc, ds, err := Load(f)
			if err != nil {
				t.Fatalf("err = %v", err)
			}
			if len(ds) != 0 {
				t.Fatalf("діагностик %d, want 0:\n%s", len(ds), render(t, ds))
			}
			if doc == nil {
				t.Fatal("doc = nil для коректної моделі")
			}
			if doc.Org.Organization == "" || len(doc.Org.Sites) == 0 || doc.Org.ControlNode.IP == "" {
				t.Errorf("модель розібралася порожньою: %+v", doc.Org)
			}
		})
	}
}

func TestBrokenCorpusMatchesTable(t *testing.T) {
	// Only s*.yml are structural fixtures. e*.yml and w*.yml break an
	// invariant and must pass the schema; TestSemanticFixturesAreStructurallyClean
	// holds them to that.
	files, err := filepath.Glob(testdata("broken", "s*.yml"))
	if err != nil || len(files) == 0 {
		t.Fatalf("у testdata/broken немає моделей: %v", err)
	}

	seen := make(map[string]bool, len(files))

	for _, f := range files {
		name := filepath.Base(f)
		if name == aggregateName {
			continue
		}
		seen[name] = true

		t.Run(name, func(t *testing.T) {
			want, ok := expected[name]
			if !ok {
				t.Fatalf("файл є, а очікування для нього немає — додайте запис у expected")
			}

			doc, ds, err := Load(f)
			if err != nil {
				t.Fatalf("err = %v", err)
			}
			if doc != nil {
				t.Error("doc != nil для зламаної моделі")
			}
			if len(ds) != 1 {
				t.Fatalf("діагностик %d, want 1 — у файлі має бути рівно один дефект:\n%s",
					len(ds), render(t, ds))
			}

			d := ds[0]
			if d.Code != want.code {
				t.Errorf("код = %s, want %s\n  %s", d.Code, want.code, d)
			}
			if d.Line != want.line {
				t.Errorf("рядок = %d, want %d\n  %s", d.Line, want.line, d)
			}
			if d.Severity != diag.Error {
				t.Errorf("рівень = %v, структурний дефект має бути помилкою", d.Severity)
			}
		})
	}

	for name := range expected {
		if !seen[name] {
			t.Errorf("очікування для %s є, а файла немає", name)
		}
	}
}

// A fixture for an invariant must fail that invariant and nothing else. If
// it also broke the schema, the pipeline would stop at the structural layer
// and the invariant would never run — the negative test would stay green
// while proving nothing.
func TestSemanticFixturesAreStructurallyClean(t *testing.T) {
	var files []string
	for _, pattern := range []string{"e*.yml", "w*.yml"} {
		m, err := filepath.Glob(testdata("broken", pattern))
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, m...)
	}

	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			doc, ds, err := Load(f)
			if err != nil {
				t.Fatalf("err = %v", err)
			}
			if len(ds) != 0 || doc == nil {
				t.Fatalf("семантичний приклад не проходить схему:\n%s", render(t, ds))
			}
		})
	}
}

// The file name carries the code, so the name must not drift from what the
// file actually produces: a reader picks fixtures by name.
func TestBrokenFileNamesMatchTheirCodes(t *testing.T) {
	for name, want := range expected {
		if len(name) < 4 {
			t.Errorf("назва %q закоротка", name)
			continue
		}
		prefix := name[:4] // "s001"
		if got := "S" + prefix[1:]; got != string(want.code) {
			t.Errorf("файл %s названий як %s, а очікується %s", name, got, want.code)
		}
	}
}

func TestAggregateCoversEveryStructuralCode(t *testing.T) {
	path := testdata("broken", aggregateName)

	doc, ds, err := Load(path)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if doc != nil {
		t.Error("doc != nil, хоча зведена модель має помилки")
	}
	if !diag.HasErrors(ds) {
		t.Fatal("зведена модель не дала жодної помилки")
	}

	seen := make(map[diag.Code]bool, len(ds))
	for _, d := range ds {
		seen[d.Code] = true

		if d.Code == diag.SOther {
			t.Errorf("непокритий вид помилки валідатора — у відображенні дірка:\n  %s", d)
		}
		if d.Message == "" {
			t.Errorf("порожнє повідомлення: %s", d)
		}
		// Only a finding about the document as a whole may lack a position.
		if d.Line == 0 && d.Path != "" {
			t.Errorf("діагностика зі шляхом, але без рядка: %s", d)
		}
	}

	// S001 needs a file that does not parse, so it cannot appear in a
	// document that also has to exercise everything else.
	for _, c := range []diag.Code{
		diag.SRequiredMissing, diag.SUnknownField, diag.STypeMismatch,
		diag.SNotInEnum, diag.SFormatMismatch, diag.SOutOfRange,
		diag.SArrayConstraint, diag.SFieldDependency, diag.SValueForbidden,
	} {
		if !seen[c] {
			t.Errorf("код %s (%s) не трапився у зведеній моделі", c, c.Title())
		}
	}
}

// Until stage 3 moves CI onto orgc verify, examples/ stays the input of the
// validate stage while testdata/ is what the tests read. Two copies of the
// same fixture are exactly the kind of drift this project exists to catch,
// so they are pinned to each other here. The check goes away with examples/.
func TestTestdataMatchesExamples(t *testing.T) {
	pairs := []struct{ dir, name, example string }{
		{"valid", "s-profile.yml", "organization.s-profile.yml"},
		{"valid", "m-profile.yml", "organization.m-profile.yml"},
		{"broken", aggregateName, "organization.broken.yml"},
	}

	for _, p := range pairs {
		want, err := os.ReadFile(filepath.Join("..", "..", "..", "examples", p.example))
		if err != nil {
			t.Skipf("examples/ недоступні (%v) — перевірка синхронності пропущена", err)
		}
		got, err := os.ReadFile(testdata(p.dir, p.name))
		if err != nil {
			t.Fatalf("не прочитати testdata/%s/%s: %v", p.dir, p.name, err)
		}
		if string(got) != string(want) {
			t.Errorf("testdata/%s/%s розійшовся з examples/%s — копії мають збігатися байт у байт",
				p.dir, p.name, p.example)
		}
	}
}
