package diag

import (
	"bytes"
	"strings"
	"testing"
)

func TestDiagnosticString(t *testing.T) {
	tests := []struct {
		name string
		d    Diagnostic
		want string
	}{
		{
			name: "повна діагностика",
			d: Diagnostic{
				Code: SNotInEnum, Severity: Error,
				Message: "значення поза переліком дозволених",
				File:    "organization.yml", Line: 23, Column: 42,
				Path: "sites[0].networks[2].role",
			},
			want: "organization.yml:23:42: помилка S005: значення поза переліком дозволених — sites[0].networks[2].role",
		},
		{
			name: "без позиції і без шляху — про документ загалом",
			d: Diagnostic{
				Code: SRequiredMissing, Severity: Error,
				Message: "відсутнє обовʼязкове поле control_node",
				File:    "organization.yml",
			},
			want: "organization.yml: помилка S002: відсутнє обовʼязкове поле control_node",
		},
		{
			name: "рядок без колонки",
			d: Diagnostic{
				Code: SYAMLSyntax, Severity: Error,
				File: "organization.yml", Line: 7,
			},
			want: "organization.yml:7: помилка S001: файл не є коректним YAML",
		},
		{
			name: "попередження інваріанта",
			d: Diagnostic{
				Code: "W003", Severity: Warning,
				Message: "сайт профілю M не має резервного каналу",
				File:    "organization.yml", Line: 12, Column: 5,
				Path: "sites[1]",
			},
			want: "organization.yml:12:5: попередження W003: сайт профілю M не має резервного каналу — sites[1]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.d.String(); got != tt.want {
				t.Errorf("\n отримано %q\n очікувано %q", got, tt.want)
			}
		})
	}
}

// Render must sort on its own: a caller that forgets to sort would otherwise
// produce different bytes for the same model on every run.
func TestRenderSortsAndUsesLF(t *testing.T) {
	ds := []Diagnostic{
		{File: "organization.yml", Line: 23, Code: SNotInEnum, Severity: Error},
		{File: "organization.yml", Line: 7, Code: SYAMLSyntax, Severity: Error},
		{File: "organization.yml", Line: 12, Code: SUnknownField, Severity: Error},
	}

	var buf bytes.Buffer
	if err := Render(&buf, ds); err != nil {
		t.Fatal(err)
	}
	out := buf.String()

	if strings.Contains(out, "\r") {
		t.Error("у виводі є CR — закінчення рядків мають бути LF")
	}

	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("рядків %d, want 3: %q", len(lines), out)
	}
	for i, want := range []string{":7:", ":12:", ":23:"} {
		if !strings.Contains(lines[i], want) {
			t.Errorf("рядок %d = %q, очікувався з %q", i, lines[i], want)
		}
	}
}

// Render must not reorder the caller's slice: the caller may still need it,
// and a hidden side effect is a poor trade for avoiding one copy.
func TestRenderDoesNotMutateInput(t *testing.T) {
	ds := []Diagnostic{
		{File: "a.yml", Line: 9, Code: SNotInEnum},
		{File: "a.yml", Line: 2, Code: SUnknownField},
	}
	before := []Diagnostic{ds[0], ds[1]}

	if err := Render(&bytes.Buffer{}, ds); err != nil {
		t.Fatal(err)
	}
	for i := range before {
		if ds[i] != before[i] {
			t.Errorf("вхідний зріз змінено на позиції %d: %+v", i, ds[i])
		}
	}
}

func TestRenderIsByteIdenticalAcrossRuns(t *testing.T) {
	ds := []Diagnostic{
		{File: "organization.yml", Line: 23, Column: 42, Code: SNotInEnum, Severity: Error, Path: "b"},
		{File: "organization.yml", Line: 23, Column: 42, Code: SNotInEnum, Severity: Error, Path: "a"},
		{File: "organization.yml", Line: 23, Column: 42, Code: STypeMismatch, Severity: Error},
	}

	var first bytes.Buffer
	if err := Render(&first, ds); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		var again bytes.Buffer
		if err := Render(&again, ds); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(first.Bytes(), again.Bytes()) {
			t.Fatalf("прогін %d дав інший вивід:\n%s\nпроти\n%s", i, again.String(), first.String())
		}
	}
}

func TestSummary(t *testing.T) {
	err := func(n int) []Diagnostic {
		ds := make([]Diagnostic, n)
		for i := range ds {
			ds[i] = Diagnostic{Code: "E001", Severity: Error}
		}
		return ds
	}
	warn := func(n int) []Diagnostic {
		ds := make([]Diagnostic, n)
		for i := range ds {
			ds[i] = Diagnostic{Code: "W001", Severity: Warning}
		}
		return ds
	}

	tests := []struct {
		name string
		ds   []Diagnostic
		want string
	}{
		{"порожньо", nil, "OK — помилок не знайдено"},
		{"1 помилка", err(1), "Знайдено 1 помилку"},
		{"2 помилки", err(2), "Знайдено 2 помилки"},
		{"4 помилки", err(4), "Знайдено 4 помилки"},
		{"5 помилок", err(5), "Знайдено 5 помилок"},
		{"11 помилок — виняток", err(11), "Знайдено 11 помилок"},
		{"12 помилок — виняток", err(12), "Знайдено 12 помилок"},
		{"14 помилок — виняток", err(14), "Знайдено 14 помилок"},
		{"21 помилка", err(21), "Знайдено 21 помилку"},
		{"22 помилки", err(22), "Знайдено 22 помилки"},
		{"25 помилок", err(25), "Знайдено 25 помилок"},
		{"1 попередження", warn(1), "Знайдено 1 попередження"},
		{"2 попередження", warn(2), "Знайдено 2 попередження"},
		{"5 попереджень", warn(5), "Знайдено 5 попереджень"},
		{"і те, і те", append(err(1), warn(3)...), "Знайдено 1 помилку і 3 попередження"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Summary(tt.ds); got != tt.want {
				t.Errorf("\n отримано %q\n очікувано %q", got, tt.want)
			}
		})
	}
}
