package containerlab

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"orgc/internal/backend"
	"orgc/internal/backend/aristaeos"
	"orgc/internal/backend/routeros"
	"orgc/internal/lower"
	"orgc/internal/model"
)

func platforms(t *testing.T) backend.Platforms {
	t.Helper()
	r, err := backend.NewRegistry(routeros.Platform{}, aristaeos.Platform{})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func topology(t *testing.T, name string, edits ...string) string {
	t.Helper()
	path := filepath.Join("..", "..", "..", "testdata", "valid", name)
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s := string(src)
	for i := 0; i+1 < len(edits); i += 2 {
		if n := strings.Count(s, edits[i]); n != 1 {
			t.Fatalf("фрагмент %q трапляється %d разів", edits[i], n)
		}
		s = strings.Replace(s, edits[i], edits[i+1], 1)
	}
	doc, ds, err := model.LoadBytes(name, []byte(s))
	if err != nil || doc == nil {
		t.Fatalf("err = %v, ds = %v", err, ds)
	}
	m, err := lower.Lower(doc)
	if err != nil {
		t.Fatal(err)
	}
	files := Generator{}.Generate(m, platforms(t))
	if len(files) != 1 || files[0].Path != File {
		t.Fatalf("файли: %+v", files)
	}
	return string(files[0].Data)
}

func has(t *testing.T, topo, s string) {
	t.Helper()
	if !strings.Contains(topo, s) {
		t.Errorf("у топології немає:\n%s", s)
	}
}

func TestNodesAndLinksFromModel(t *testing.T) {
	topo := topology(t, "s-profile.yml")
	has(t, topo, "    rb750:\n      kind: mikrotik_ros\n      image: vrnetlab/mikrotik_routeros:6.48\n      startup-config: ../configs/hq/rb750.rsc\n")
	has(t, topo, "    core-01:\n      kind: arista_ceos\n      image: ceos:4.29.2F\n      startup-config: ../configs/hq/core-01.cfg\n")
	has(t, topo, `    - endpoints: ["rb750:eth1", "core-01:eth1"]`+"\n")
	has(t, topo, `    - endpoints: ["core-01:eth3", "acc-02:eth1"]`+"\n")
	for _, srv := range []string{"ctrl-01", "mon-01"} {
		if strings.Contains(topo, "    "+srv+":") {
			t.Errorf("сервер %s не є вузлом топології", srv)
		}
	}
}

// A site without links follows the star convention: each switch's first
// port to the next free trunk port of the gateway.
func TestStarConvention(t *testing.T) {
	topo := topology(t, "minimal.yml",
		"        mgmt_ip: 192.168.10.1\n",
		"        mgmt_ip: 192.168.10.1\n"+
			"      - { name: sw-01, type: arista_eos, os_version: \"4.29.2F\", role: access_switch, mgmt_ip: 192.168.10.2 }\n"+
			"      - { name: sw-02, type: mikrotik, os_version: \"6.48\", role: access_switch, mgmt_ip: 192.168.10.3 }\n")
	has(t, topo, `    - endpoints: ["rb750:eth1", "sw-01:eth1"]`+"\n")
	// ether1 of a RouterOS switch is containerlab's management port, so
	// that link cannot be reproduced; it is named, not dropped silently.
	has(t, topo, "# не відтворено в емуляції: rb750:ether3 – sw-02:ether1\n")
}

func TestPortMapping(t *testing.T) {
	for _, c := range []struct {
		node backend.LabNode
		port string
		want string
		ok   bool
	}{
		{routeros.Platform{}, "ether2", "eth1", true},
		{routeros.Platform{}, "ether5", "eth4", true},
		{routeros.Platform{}, "ether1", "", false},
		{routeros.Platform{}, "sfp1", "", false},
		{aristaeos.Platform{}, "Ethernet1", "eth1", true},
		{aristaeos.Platform{}, "Ethernet49/1", "eth49_1", true},
		{aristaeos.Platform{}, "Management1", "", false},
	} {
		got, ok := c.node.LabInterface(c.port)
		if got != c.want || ok != c.ok {
			t.Errorf("%T %s → %q, %v; want %q, %v", c.node, c.port, got, ok, c.want, c.ok)
		}
	}
}

func TestShape(t *testing.T) {
	topo := topology(t, "m-profile.yml")
	if !strings.HasPrefix(topo, "# Згенеровано orgc з m-profile.yml. Не редагувати вручну.\n---\nname: lab\ntopology:\n") {
		t.Errorf("заголовок:\n%s", topo[:120])
	}
	if strings.ContainsRune(topo, '\r') {
		t.Error("CR у топології")
	}
	if strings.Contains(topo, "wan") {
		t.Error("моста wan немає: міжсайтову звʼязність дає мережа керування containerlab")
	}
	for i := 0; i < 10; i++ {
		if topology(t, "m-profile.yml") != topo {
			t.Fatalf("прогін %d дав іншу топологію", i)
		}
	}
}
