package prometheus

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"orgc/internal/backend"
	"orgc/internal/golden"
	"orgc/internal/ir"
	"orgc/internal/lower"
	"orgc/internal/model"
)

func lowerModel(t *testing.T, name string) *ir.Model {
	t.Helper()
	doc, ds, err := model.Load(filepath.Join("..", "..", "..", "testdata", "valid", name))
	if err != nil || doc == nil {
		t.Fatalf("err = %v, ds = %v", err, ds)
	}
	m, err := lower.Lower(doc)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func strip(files []backend.File) []backend.File {
	out := make([]backend.File, 0, len(files))
	for _, f := range files {
		out = append(out, backend.File{Path: strings.TrimPrefix(f.Path, "prometheus/targets/"), Data: f.Data})
	}
	return out
}

// The targets Prometheus on mon-01 reads are the output of orgc for the
// model of the lab stand, as the staging inventory is. -update rewrites them.
func TestMonitoringTargetsAreGeneratedFromLabModel(t *testing.T) {
	golden.Compare(t, filepath.Join("..", "..", "..", "..", "bootstrap", "mon-01", "prometheus", "targets"),
		strip(Generator{}.Generate(lowerModel(t, "lab.yml"), nil)))
}

func TestTargetsAndLabels(t *testing.T) {
	files := Generator{}.Generate(lowerModel(t, "m-profile.yml"), nil)
	if len(files) != 2 || files[0].Path != "prometheus/targets/node.json" || files[1].Path != "prometheus/targets/snmp.json" {
		t.Fatalf("файли: %+v", files)
	}

	var node, snmp []group
	if err := json.Unmarshal(files[0].Data, &node); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(files[1].Data, &snmp); err != nil {
		t.Fatal(err)
	}
	if len(node) != 3 || node[2].Targets[0] != "192.168.130.10:9100" || node[2].Labels["site"] != "branch" ||
		node[2].Labels["roles"] != "docker_host,app" {
		t.Errorf("node: %+v", node)
	}
	if len(snmp) != 10 || snmp[0].Targets[0] != "192.168.10.1" || snmp[0].Labels["platform"] != "mikrotik" ||
		snmp[1].Labels["role"] != "core_switch" || snmp[1].Labels["platform"] != "arista_eos" {
		t.Errorf("snmp: %+v", snmp[:2])
	}
}

// A job without hosts produces no file, rather than an empty one.
func TestEmptyJobsAreOmitted(t *testing.T) {
	m := lowerModel(t, "minimal.yml")
	files := Generator{}.Generate(m, nil)
	if len(files) != 1 || files[0].Path != "prometheus/targets/snmp.json" {
		t.Errorf("minimal без серверів: %+v", files)
	}
}

func TestDeterministic(t *testing.T) {
	m := lowerModel(t, "m-profile.yml")
	first := Generator{}.Generate(m, nil)
	for i := 0; i < 20; i++ {
		for k, f := range (Generator{}).Generate(m, nil) {
			if string(f.Data) != string(first[k].Data) {
				t.Fatalf("прогін %d: %s", i, f.Path)
			}
		}
	}
	if strings.ContainsRune(string(first[0].Data), '\r') || !strings.HasSuffix(string(first[0].Data), "]\n") {
		t.Error("JSON має закінчуватися LF")
	}
}
