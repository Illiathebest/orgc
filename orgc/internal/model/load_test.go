package model

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"orgc/internal/diag"
)

// minimal is the smallest document the schema accepts. Tests derive broken
// variants from it so that each one fails for exactly one reason.
const minimal = `organization: "Навчальна лабораторія"
profile: S
control_node:
  site: hq
  network: servers
  ip: 192.168.30.10
sites:
  - name: hq
    networks:
      - { vlan: 30, name: servers, role: servers, cidr: 192.168.30.0/24 }
    devices:
      - name: rb750
        type: mikrotik
        os_version: "6.48"
        role: edge_router
        mgmt_ip: 192.168.30.1
`

func TestLoadBytesAcceptsValidModel(t *testing.T) {
	org, ds, err := LoadBytes("organization.yml", []byte(minimal))
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if len(ds) != 0 {
		t.Fatalf("діагностик %d, want 0:\n%s", len(ds), render(t, ds))
	}
	if org == nil {
		t.Fatal("org = nil для коректної моделі")
	}

	if org.Profile != "S" {
		t.Errorf("Profile = %q, want %q", org.Profile, "S")
	}
	if org.ControlNode.IP != "192.168.30.10" {
		t.Errorf("ControlNode.IP = %q", org.ControlNode.IP)
	}
	if len(org.Sites) != 1 || org.Sites[0].Name != "hq" {
		t.Fatalf("Sites = %+v", org.Sites)
	}
	if got := org.Sites[0].Networks[0].VLAN; got != 30 {
		t.Errorf("VLAN = %d, want 30", got)
	}
	if got := org.Sites[0].Devices[0].OSVersion; got != "6.48" {
		t.Errorf("OSVersion = %q, want %q", got, "6.48")
	}
}

// Each case breaks the minimal model in exactly one place, so the assertion
// is that this code is raised on this line — not merely that something
// failed.
func TestLoadBytesReportsCodeAndLine(t *testing.T) {
	tests := []struct {
		name string
		src  string
		code diag.Code
		line int
		want string // substring of the message
	}{
		{
			name: "профіль поза переліком",
			src:  strings.Replace(minimal, "profile: S", "profile: L", 1),
			code: diag.SNotInEnum, line: 2, want: `значення "L" неприпустиме`,
		},
		{
			name: "невідоме поле",
			src:  minimal + "modules:\n  network_fabric: true\n",
			code: diag.SUnknownField, line: 17, want: `невідоме поле "modules"`,
		},
		{
			name: "неправильний тип",
			src:  strings.Replace(minimal, "    networks:", "    allow_overlap: yes-please\n    networks:", 1),
			code: diag.STypeMismatch, line: 9, want: "очікується булеве значення, отримано рядок",
		},
		{
			name: "некоректна IPv4",
			src:  strings.Replace(minimal, "mgmt_ip: 192.168.30.1", "mgmt_ip: 999.1.1.1", 1),
			code: diag.SFormatMismatch, line: 16, want: "не є коректною IPv4-адресою",
		},
		{
			name: "vlan поза діапазоном",
			src:  strings.Replace(minimal, "vlan: 30", "vlan: 9999", 1),
			code: diag.SOutOfRange, line: 10, want: "більше за максимум 4094",
		},
		{
			name: "зарезервоване імʼя мережі",
			src:  strings.Replace(minimal, "name: servers, role", "name: any, role", 1),
			code: diag.SValueForbidden, line: 10, want: `значення "any" тут заборонено`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ds, err := LoadBytes("organization.yml", []byte(tt.src))
			if err != nil {
				t.Fatalf("err = %v", err)
			}
			var found *diag.Diagnostic
			for i := range ds {
				if ds[i].Code == tt.code {
					found = &ds[i]
					break
				}
			}
			if found == nil {
				t.Fatalf("коду %s немає серед діагностик:\n%s", tt.code, render(t, ds))
			}
			if found.Line != tt.line {
				t.Errorf("рядок = %d, want %d (%s)", found.Line, tt.line, found)
			}
			if !strings.Contains(found.Message, tt.want) {
				t.Errorf("повідомлення = %q, має містити %q", found.Message, tt.want)
			}
		})
	}
}

// A missing required field at the top level belongs to the document as a
// whole. Pointing at a line would blame whichever field happens to come
// first, so the position stays unknown.
func TestTopLevelFindingHasNoPosition(t *testing.T) {
	src := strings.Replace(minimal,
		"control_node:\n  site: hq\n  network: servers\n  ip: 192.168.30.10\n", "", 1)

	_, ds, err := LoadBytes("organization.yml", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(ds) != 1 {
		t.Fatalf("діагностик %d, want 1:\n%s", len(ds), render(t, ds))
	}
	d := ds[0]
	if d.Code != diag.SRequiredMissing {
		t.Errorf("код = %s, want %s", d.Code, diag.SRequiredMissing)
	}
	if d.Line != 0 || d.Column != 0 {
		t.Errorf("позиція = %d:%d, має бути невідома", d.Line, d.Column)
	}
	if !strings.Contains(d.Message, "control_node") {
		t.Errorf("повідомлення не називає поле: %q", d.Message)
	}
}

func TestRequiredPluralWording(t *testing.T) {
	src := strings.Replace(minimal,
		"      - name: rb750\n        type: mikrotik\n        os_version: \"6.48\"\n        role: edge_router\n        mgmt_ip: 192.168.30.1\n",
		"      - { name: rb750, type: mikrotik, role: edge_router }\n", 1)

	_, ds, err := LoadBytes("organization.yml", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(ds) != 1 {
		t.Fatalf("діагностик %d, want 1:\n%s", len(ds), render(t, ds))
	}
	if got := ds[0].Message; !strings.HasPrefix(got, "відсутні обовʼязкові поля") {
		t.Errorf("повідомлення = %q, очікувалася множина", got)
	}
}

func TestLoadBytesOnBrokenYAML(t *testing.T) {
	src := "organization: test\nsites:\n  - name: hq\n   bad_indent: 1\n"

	_, ds, err := LoadBytes("organization.yml", []byte(src))
	if err != nil {
		t.Fatalf("err = %v, синтаксична помилка має йти діагностикою", err)
	}
	if len(ds) != 1 {
		t.Fatalf("діагностик %d, want 1:\n%s", len(ds), render(t, ds))
	}
	d := ds[0]
	if d.Code != diag.SYAMLSyntax {
		t.Errorf("код = %s, want %s", d.Code, diag.SYAMLSyntax)
	}
	if d.Line != 4 {
		t.Errorf("рядок = %d, want 4", d.Line)
	}
	if strings.Contains(d.Message, "\n") {
		t.Errorf("повідомлення містить уривок джерела, має бути один рядок: %q", d.Message)
	}
	if strings.Contains(d.Message, "[4:4]") {
		t.Errorf("позиція має бути в полях, а не в тексті: %q", d.Message)
	}
}

func TestLoadBytesOnEmptyFile(t *testing.T) {
	_, ds, err := LoadBytes("organization.yml", []byte("\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(ds) != 1 || ds[0].Code != diag.SYAMLSyntax {
		t.Fatalf("очікувалася одна діагностика S001, отримано:\n%s", render(t, ds))
	}
}

func TestLoadMissingFileIsAnError(t *testing.T) {
	org, ds, err := Load(filepath.Join(t.TempDir(), "absent.yml"))
	if err == nil {
		t.Fatal("err = nil для відсутнього файлу")
	}
	if org != nil || ds != nil {
		t.Errorf("org = %v, ds = %v; очікувалося nil, nil", org, ds)
	}
}

// Byte-identical output for identical input is a rule of the project, and
// the validator's own ordering does not provide it.
func TestLoadIsDeterministic(t *testing.T) {
	src := []byte(strings.Replace(
		strings.Replace(minimal, "profile: S", "profile: L", 1),
		"vlan: 30", "vlan: 9999", 1) + "modules:\n  network_fabric: true\n")

	_, first, err := LoadBytes("organization.yml", src)
	if err != nil {
		t.Fatal(err)
	}
	want := render(t, first)

	for i := 0; i < 20; i++ {
		_, ds, err := LoadBytes("organization.yml", src)
		if err != nil {
			t.Fatal(err)
		}
		if got := render(t, ds); got != want {
			t.Fatalf("прогін %d дав інший вивід:\n%s\nпроти\n%s", i, got, want)
		}
	}
}

func render(t *testing.T, ds []diag.Diagnostic) string {
	t.Helper()
	var b bytes.Buffer
	if err := diag.Render(&b, ds); err != nil {
		t.Fatal(err)
	}
	return b.String()
}
