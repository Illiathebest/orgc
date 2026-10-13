package verify

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"orgc/internal/backend"
	"orgc/internal/backend/aristaeos"
	"orgc/internal/backend/routeros"
	"orgc/internal/diag"
	"orgc/internal/ir"
	"orgc/internal/lower"
	"orgc/internal/model"
)

func testdata(parts ...string) string {
	return filepath.Join(append([]string{"..", "..", "testdata"}, parts...)...)
}

// platforms is the registry orgc ships with. Tests may import platform
// packages; the layering test guards non-test files only.
func platforms(t *testing.T) backend.Platforms {
	t.Helper()
	r, err := backend.NewRegistry(routeros.Platform{}, aristaeos.Platform{})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// lowerSource runs the pipeline up to the IR on a model in memory.
func lowerSource(t *testing.T, name, src string) *ir.Model {
	t.Helper()
	doc, ds, err := model.LoadBytes(name, []byte(src))
	if err != nil || doc == nil {
		t.Fatalf("%s не пройшов схему: err = %v, ds = %v", name, err, ds)
	}
	m, err := lower.Lower(doc)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func lowerFile(t *testing.T, path string) *ir.Model {
	t.Helper()
	return variant(t, path)
}

// variant applies textual edits to a fixture and lowers the result. Each old
// string must occur exactly once, so a fixture change cannot silently turn a
// test into a no-op.
func variant(t *testing.T, path string, edits ...string) *ir.Model {
	t.Helper()
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s := string(src)
	for i := 0; i+1 < len(edits); i += 2 {
		if n := strings.Count(s, edits[i]); n != 1 {
			t.Fatalf("фрагмент %q трапляється %d разів, want 1", edits[i], n)
		}
		s = strings.Replace(s, edits[i], edits[i+1], 1)
	}
	return lowerSource(t, filepath.Base(path), s)
}

func runChecks(t *testing.T, m *ir.Model, checks []Check) []diag.Diagnostic {
	t.Helper()
	ds, err := Run(m, platforms(t), checks)
	if err != nil {
		t.Fatal(err)
	}
	return ds
}

// expected pins what each invariant fixture must produce: exactly one
// diagnostic, of the code in its name, on this line. Exactly one is the
// point: a fixture that also tripped another invariant would no longer
// prove which one caught the defect, and the no-cascade rule of ADR-008
// would break unnoticed.
var expected = map[string]struct {
	code diag.Code
	line int
}{
	"e001-overlap-between-sites.yml":        {diag.E001, 25},
	"e001-overlap-in-site.yml":              {diag.E001, 17},
	"e001-rule-across-overlap.yml":          {diag.E001, 31},
	"e002-duplicate-network-name.yml":       {diag.E002, 17},
	"e002-duplicate-site-name.yml":          {diag.E002, 23},
	"e002-duplicate-uplink.yml":             {diag.E002, 16},
	"e002-duplicate-vlan.yml":               {diag.E002, 16},
	"e002-host-name-clash.yml":              {diag.E002, 24},
	"e002-port-used-twice.yml":              {diag.E002, 67},
	"e003-access-network-unknown.yml":       {diag.E003, 64},
	"e003-link-unknown-device.yml":          {diag.E003, 74},
	"e003-link-unknown-vlan.yml":            {diag.E003, 73},
	"e003-uplink-port-not-gateway.yml":      {diag.E003, 16},
	"e006-mgmt-pruned.yml":                  {diag.E006, 94},
	"e009-device-not-connected.yml":         {diag.E009, 57},
	"e009-vlan-not-delivered.yml":           {diag.E009, 64},
	"e010-loop.yml":                         {diag.E010, 75},
	"w002-port-name.yml":                    {diag.W002, 66},
	"w005-access-feeds-access.yml":          {diag.W005, 74},
	"w006-standby-no-mgmt.yml":              {diag.W006, 101},
	"e003-control-network-other-site.yml":   {diag.E003, 9},
	"e003-no-l3-gateway.yml":                {diag.E003, 15},
	"e003-two-l3-gateways.yml":              {diag.E003, 26},
	"e003-unknown-control-network.yml":      {diag.E003, 9},
	"e003-unknown-control-site.yml":         {diag.E003, 8},
	"e003-unknown-rule-segment.yml":         {diag.E003, 26},
	"e004-mgmt-ip-in-servers-network.yml":   {diag.E004, 22},
	"e004-mgmt-of-other-site.yml":           {diag.E004, 27},
	"e004-no-mgmt-network.yml":              {diag.E004, 22},
	"e005-any-to-any.yml":                   {diag.E005, 27},
	"e005-from-internet.yml":                {diag.E005, 27},
	"e006-mgmt-not-routed.yml":              {diag.E006, 18},
	"e006-no-rule.yml":                      {diag.E006, 18},
	"e006-ssh-only.yml":                     {diag.E006, 26},
	"e007-control-node-outside-network.yml": {diag.E007, 10},
	"e007-gateway-in-pool.yml":              {diag.E007, 16},
	"e007-gateway-outside-cidr.yml":         {diag.E007, 16},
	"e007-pool-inverted.yml":                {diag.E007, 17},
	"e007-pool-on-broadcast.yml":            {diag.E007, 17},
	"e007-pool-outside-cidr.yml":            {diag.E007, 17},
	"e007-server-outside-site.yml":          {diag.E007, 24},
	"e007-static-collision.yml":             {diag.E007, 24},
	"e007-static-in-pool.yml":               {diag.E007, 10},
	"e007-static-network-address.yml":       {diag.E007, 24},
	"e008-prefix-too-small.yml":             {diag.E008, 17},
	"e008-slash31-with-gateway.yml":         {diag.E008, 17},
	"w001-guest-not-isolated.yml":           {diag.W001, 17},
	"w001-guest-to-servers.yml":             {diag.W001, 28},
	"w002-capability-missing.yml":           {diag.W002, 19},
	"w002-version-out-of-range.yml":         {diag.W002, 20},
	"w003-no-backup-uplink.yml":             {diag.W003, 13},
	"w004-mgmt-open-to-users.yml":           {diag.W004, 28},
}

// raisedToError lists fixtures whose code is a warning by default but is
// reported at error level: W002 for an OS version the backend cannot render.
var raisedToError = map[string]bool{
	"w002-version-out-of-range.yml": true,
	"w002-port-name.yml":            true,
}

func invariantFixtures(t *testing.T) []string {
	t.Helper()
	var files []string
	for _, p := range []string{"e*.yml", "w*.yml"} {
		m, err := filepath.Glob(testdata("broken", p))
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, m...)
	}
	return files
}

func TestInvariantFixturesMatchTable(t *testing.T) {
	seen := map[string]bool{}
	for _, f := range invariantFixtures(t) {
		name := filepath.Base(f)
		seen[name] = true
		t.Run(name, func(t *testing.T) {
			want, ok := expected[name]
			if !ok {
				t.Fatal("файл є, а очікування для нього немає — додайте запис у expected")
			}
			if got := diag.Code(strings.ToUpper(name[:4])); got != want.code {
				t.Fatalf("назва файлу каже %s, таблиця — %s", got, want.code)
			}

			ds := runChecks(t, lowerFile(t, f), Checks())
			if len(ds) != 1 {
				t.Fatalf("діагностик %d, want рівно 1:\n%s", len(ds), render(ds))
			}
			d := ds[0]
			if d.Code != want.code || d.Line != want.line {
				t.Errorf("отримано %s у рядку %d, want %s у рядку %d\n  %s",
					d.Code, d.Line, want.code, want.line, d)
			}
			wantSev := want.code.DefaultSeverity()
			if raisedToError[name] {
				wantSev = diag.Error
			}
			if d.Severity != wantSev {
				t.Errorf("рівень %v, want %v: %s", d.Severity, wantSev, d)
			}
			if d.Path == "" || d.Column == 0 {
				t.Errorf("діагностика без шляху чи колонки: %s", d)
			}
		})
	}
	for name := range expected {
		if !seen[name] {
			t.Errorf("очікування для %s є, а файла немає", name)
		}
	}
}

// Every registered invariant must have at least one negative fixture.
func TestEveryCheckHasAFixture(t *testing.T) {
	has := map[diag.Code]bool{}
	for _, e := range expected {
		has[e.code] = true
	}
	for _, c := range Checks() {
		if !has[c.Code] {
			t.Errorf("%s зареєстровано, але в testdata/broken немає прикладу, що його порушує", c.Code)
		}
	}
}

// The positive side for every invariant at once: the valid corpus, which
// includes both example profiles, yields nothing.
func TestValidCorpusIsClean(t *testing.T) {
	files, err := filepath.Glob(testdata("valid", "*.yml"))
	if err != nil || len(files) == 0 {
		t.Fatalf("у testdata/valid немає моделей: %v", err)
	}
	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			if ds := runChecks(t, lowerFile(t, f), Checks()); len(ds) != 0 {
				t.Errorf("коректна модель дала діагностики:\n%s", render(ds))
			}
		})
	}
}

func render(ds []diag.Diagnostic) string {
	var b strings.Builder
	_ = diag.Render(&b, ds)
	return b.String()
}

// The invariant → test table in testdata/README.md is documentation a
// reader trusts; it must name every fixture with the line it reports.
func TestReadmeTableMatchesFixtures(t *testing.T) {
	readme, err := os.ReadFile(testdata("README.md"))
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range expected {
		cell := "`" + name + "`:" + strconv.Itoa(want.line)
		if !strings.Contains(string(readme), cell) {
			t.Errorf("у таблиці testdata/README.md немає %s", cell)
		}
	}
}
