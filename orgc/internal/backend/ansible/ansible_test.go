package ansible

import (
	"path/filepath"
	"strings"
	"testing"

	"orgc/internal/backend"
	"orgc/internal/backend/aristaeos"
	"orgc/internal/backend/routeros"
	"orgc/internal/golden"
	"orgc/internal/ir"
	"orgc/internal/lower"
	"orgc/internal/model"
)

func lowerModel(t *testing.T, path string) *ir.Model {
	t.Helper()
	doc, ds, err := model.Load(path)
	if err != nil || doc == nil {
		t.Fatalf("%s: err = %v, ds = %v", path, err, ds)
	}
	m, err := lower.Lower(doc)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func platforms(t *testing.T) backend.Platforms {
	t.Helper()
	r, err := backend.NewRegistry(routeros.Platform{}, aristaeos.Platform{})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// strip drops the "ansible/" prefix: a reference directory holds only this
// generator's part of the output tree.
func strip(files []backend.File) []backend.File {
	out := make([]backend.File, 0, len(files))
	for _, f := range files {
		out = append(out, backend.File{Path: strings.TrimPrefix(f.Path, "ansible/"), Data: f.Data})
	}
	return out
}

// The staging inventory is no longer written by hand: it is the output of
// orgc for the model of the lab stand (docs/adr/ADR-009, decision 4), and
// this test holds the two byte for byte. Rewrite it with -update.
func TestStagingIsGeneratedFromLabModel(t *testing.T) {
	m := lowerModel(t, filepath.Join("..", "..", "..", "testdata", "valid", "lab.yml"))
	// The staging files are rendered from examples/organization.lab.yml;
	// testdata/valid/lab.yml is its byte-identical copy, so the header
	// names the example.
	m.File = "organization.lab.yml"
	golden.Compare(t, filepath.Join("..", "..", "..", "..", "ansible", "inventories", "staging"),
		strip(Generator{}.Generate(m, platforms(t))))
}

func TestGroupsFollowPlatforms(t *testing.T) {
	m := lowerModel(t, filepath.Join("..", "..", "..", "testdata", "valid", "m-profile.yml"))
	files := Generator{}.Generate(m, platforms(t))

	var paths []string
	var inv string
	for _, f := range files {
		paths = append(paths, f.Path)
		if f.Path == "ansible/hosts.yml" {
			inv = string(f.Data)
		}
	}
	want := "ansible/group_vars/arista_switches/main.yml ansible/group_vars/linux_servers/main.yml ansible/group_vars/mikrotik_routers/main.yml ansible/hosts.yml"
	if got := strings.Join(paths, " "); got != want {
		t.Errorf("файли:\n got %s\nwant %s", got, want)
	}
	// Groups sorted, hosts in model order.
	i, j, k := strings.Index(inv, "arista_switches:"), strings.Index(inv, "linux_servers:"), strings.Index(inv, "mikrotik_routers:")
	if !(i >= 0 && i < j && j < k) {
		t.Errorf("порядок груп:\n%s", inv)
	}
	if strings.Index(inv, "core-01:") > strings.Index(inv, "acc-03:") {
		t.Error("хости групи мають іти в порядку моделі")
	}
	if !strings.Contains(inv, "rb-branch:\n          ansible_host: 192.168.110.1\n") {
		t.Errorf("rb-branch у групі mikrotik_routers з mgmt_ip:\n%s", inv)
	}
}

// Secrets never reach the inventory: only a lookup or a vault reference.
func TestNoSecretsInOutput(t *testing.T) {
	m := lowerModel(t, filepath.Join("..", "..", "..", "testdata", "valid", "m-profile.yml"))
	for _, f := range (Generator{}).Generate(m, platforms(t)) {
		for _, line := range strings.Split(string(f.Data), "\n") {
			if strings.Contains(line, "password") && !strings.Contains(line, "{{") {
				t.Errorf("%s: пароль не через lookup чи vault: %s", f.Path, line)
			}
		}
	}
}

func TestScalar(t *testing.T) {
	for in, want := range map[string]string{
		"192.168.1.50":                  "192.168.1.50",
		"ansible.netcommon.network_cli": "ansible.netcommon.network_cli",
		"true":                          "true",
		"{{ vault_arista_password }}":   `"{{ vault_arista_password }}"`,
		`a "quoted" \ value`:            `"a \"quoted\" \\ value"`,
		"-leading":                      `"-leading"`,
	} {
		if got := scalar(in); got != want {
			t.Errorf("scalar(%q) = %s, want %s", in, got, want)
		}
	}
}

func TestDeterministic(t *testing.T) {
	m := lowerModel(t, filepath.Join("..", "..", "..", "testdata", "valid", "m-profile.yml"))
	first := Generator{}.Generate(m, platforms(t))
	for i := 0; i < 20; i++ {
		again := Generator{}.Generate(m, platforms(t))
		for k := range first {
			if string(again[k].Data) != string(first[k].Data) || again[k].Path != first[k].Path {
				t.Fatalf("прогін %d: %s відрізняється", i, first[k].Path)
			}
		}
	}
}
