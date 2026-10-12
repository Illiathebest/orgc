package lower

import (
	"fmt"
	"strings"
	"testing"

	"orgc/internal/ir"
)

func names(ds []*ir.Device) string {
	var out []string
	for _, d := range ds {
		out = append(out, d.Name)
	}
	return strings.Join(out, " ")
}

func device(t *testing.T, m *ir.Model, name string) *ir.Device {
	t.Helper()
	for _, d := range m.Devices {
		if d.Name == name {
			return d
		}
	}
	t.Fatalf("пристрою %s немає", name)
	return nil
}

func network(t *testing.T, m *ir.Model, name string) *ir.Network {
	t.Helper()
	for _, n := range m.Networks {
		if n.Name == name {
			return n
		}
	}
	t.Fatalf("мережі %s немає", name)
	return nil
}

func pathText(ls []*ir.Link) string {
	var out []string
	for _, l := range ls {
		out = append(out, l.A.String()+"–"+l.B.String())
	}
	return strings.Join(out, " ")
}

// The HQ of m-profile: rb750 → core-01 → dist-01 → acc-01, acc-02;
// core-01 → dist-02 → acc-03, with VLANs pruned towards access.
func TestL2TreeOfMProfile(t *testing.T) {
	m := lower(t, load(t, testdata("valid", "m-profile.yml")))
	hq := m.Sites[0]
	tr := hq.L2
	if tr == nil || tr.Incomplete || len(tr.Extra) != 0 {
		t.Fatalf("дерево HQ: %+v", tr)
	}
	if tr.Root != hq.Gateway || tr.Root.Name != "rb750" {
		t.Errorf("корінь = %v", tr.Root)
	}
	if got := names(tr.Order); got != "rb750 core-01 dist-01 dist-02 acc-01 acc-02 acc-03" {
		t.Errorf("порядок обходу: %s", got)
	}

	acc02 := device(t, m, "acc-02")
	if got := pathText(tr.Path(acc02)); got != "rb750:ether2–core-01:Ethernet1 core-01:Ethernet2–dist-01:Ethernet1 dist-01:Ethernet3–acc-02:Ethernet1" {
		t.Errorf("шлях до acc-02: %s", got)
	}
	if p := tr.Path(tr.Root); p == nil || len(p) != 0 {
		t.Errorf("шлях до кореня має бути порожнім, а не nil: %v", p)
	}

	carried := []struct {
		dev, net string
		want     bool
	}{
		{"core-01", "guest", true}, // no vlans: everything
		{"acc-01", "servers", true},
		{"acc-01", "users", false}, // pruned on dist-01–acc-01
		{"acc-02", "users", true},
		{"acc-02", "servers", false},
		{"acc-03", "guest", true},
		{"acc-03", "mgmt", true},
	}
	for _, c := range carried {
		if got := tr.Carried(device(t, m, c.dev), network(t, m, c.net)); got != c.want {
			t.Errorf("Carried(%s, %s) = %v, want %v", c.dev, c.net, got, c.want)
		}
	}
	if b := tr.Blocking(device(t, m, "acc-01"), network(t, m, "users")); b == nil || b.A.String() != "dist-01:Ethernet2" {
		t.Errorf("перше зʼєднання без users на шляху до acc-01: %v", b)
	}
	if b := tr.Blocking(acc02, network(t, m, "mgmt")); b != nil {
		t.Errorf("mgmt доходить до acc-02, а Blocking = %v", b)
	}
	// A network of the other site is never carried in this one.
	if tr.Carried(acc02, network(t, m, "users-branch")) {
		t.Error("мережа філії не може бути дозволена в HQ")
	}
}

func TestPortsResolveWithinSite(t *testing.T) {
	m := lower(t, load(t, testdata("valid", "m-profile.yml")))
	hq := m.Sites[0]

	if up := hq.Uplinks[0].Port; !up.Resolved() || up.Device.Name != "rb750" || up.Port != "ether1" {
		t.Errorf("порт каналу: %+v", up)
	}
	ctrl := m.Servers[0]
	if !ctrl.Port.Resolved() || ctrl.Port.Device.Name != "acc-01" || ctrl.Port.Port != "Ethernet5" {
		t.Errorf("порт ctrl-01: %+v", ctrl.Port)
	}
	acc03 := device(t, m, "acc-03")
	if len(acc03.AccessPorts) != 2 || acc03.AccessPorts[1].Network.Net == nil ||
		acc03.AccessPorts[1].Network.Net.Name != "guest" || acc03.AccessPorts[1].Device != acc03 {
		t.Errorf("access-порти acc-03: %+v", acc03.AccessPorts)
	}
	if l := hq.Links[3]; len(l.VLANs) != 2 || l.VLANs[1].Net == nil || l.VLANs[1].Net.Name != "servers" {
		t.Errorf("vlans зʼєднання dist-01–acc-01: %+v", l.VLANs)
	}
	if hq.Links[0].VLANs != nil {
		t.Error("зʼєднання без vlans має нести всі мережі (VLANs = nil)")
	}
}

func TestSitesWithoutLinksHaveNoTree(t *testing.T) {
	for _, f := range []string{"lab.yml", "minimal.yml"} {
		m := lower(t, load(t, testdata("valid", f)))
		for _, s := range m.Sites {
			if s.L2 != nil || len(s.Links) != 0 {
				t.Errorf("%s/%s: зірка за конвенцією не має дерева", f, s.Name)
			}
		}
	}
}

func TestL2TreeSetsAsideWhatIsNotATree(t *testing.T) {
	sprofile := testdata("valid", "s-profile.yml")
	extra := func(t *testing.T, line string) *ir.L2Tree {
		t.Helper()
		m := lower(t, loadVariant(t, sprofile,
			"      - { a: core-01:Ethernet3, b: acc-02:Ethernet1 }\n",
			"      - { a: core-01:Ethernet3, b: acc-02:Ethernet1 }\n"+line))
		return m.Sites[0].L2
	}

	tests := []struct {
		name  string
		line  string
		fault ir.LinkFault
	}{
		{"цикл", "      - { a: acc-01:Ethernet2, b: acc-02:Ethernet2 }\n", ir.FaultCycle},
		{"пристрій сам із собою", "      - { a: acc-01:Ethernet2, b: acc-01:Ethernet3 }\n", ir.FaultSelf},
		{"друге зʼєднання між парою", "      - { a: core-01:Ethernet4, b: acc-02:Ethernet2 }\n", ir.FaultParallel},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tr := extra(t, tt.line)
			if len(tr.Extra) != 1 || tr.Extra[0].Fault != tt.fault || tr.Extra[0].Link.Path != "sites[0].links[3]" {
				t.Fatalf("Extra = %+v", tr.Extra)
			}
			// The tree itself stays whole: every device still has its one path.
			if got := names(tr.Order); got != "rb750 core-01 acc-01 acc-02" {
				t.Errorf("порядок обходу: %s", got)
			}
		})
	}

	t.Run("цикл серед пристроїв без шляху до шлюзу", func(t *testing.T) {
		m := lower(t, loadVariant(t, sprofile,
			"      - { a: rb750:ether2, b: core-01:Ethernet1 }\n",
			"      - { a: acc-01:Ethernet2, b: acc-02:Ethernet2 }\n"))
		tr := m.Sites[0].L2
		if len(tr.Extra) != 1 || tr.Extra[0].Fault != ir.FaultCycle {
			t.Errorf("Extra = %+v", tr.Extra)
		}
		if got := names(tr.Order); got != "rb750" {
			t.Errorf("від шлюзу досяжний лише він сам, а Order = %s", got)
		}
		if tr.Reached(device(t, m, "acc-01")) || tr.Path(device(t, m, "acc-01")) != nil {
			t.Error("acc-01 не має шляху до шлюзу")
		}
	})
}

// An end that does not resolve leaves the reference in place for E003 and
// marks the tree incomplete; lowering does not stop.
func TestUnresolvedLinkEnd(t *testing.T) {
	m := lower(t, loadVariant(t, testdata("valid", "s-profile.yml"),
		"{ a: core-01:Ethernet3, b: acc-02:Ethernet1 }", "{ a: core-01:Ethernet3, b: acc-09:Ethernet1 }"))
	s := m.Sites[0]
	l := s.Links[2]
	if l.B.Resolved() || l.B.DeviceName != "acc-09" || l.B.Port != "Ethernet1" {
		t.Errorf("кінець: %+v", l.B)
	}
	if !s.L2.Incomplete {
		t.Error("дерево з нерозвʼязаним кінцем має бути позначене Incomplete")
	}
	if !m.At(l.B.Path).Known() {
		t.Error("нерозвʼязаний кінець без позиції — E003 лишиться без рядка")
	}
}

func TestConnectionsHavePositions(t *testing.T) {
	m := lower(t, load(t, testdata("valid", "m-profile.yml")))
	var paths []string
	for _, s := range m.Sites {
		for _, l := range s.Links {
			paths = append(paths, l.Path, l.A.Path, l.B.Path)
			if l.VLANs != nil {
				paths = append(paths, l.Path+".vlans")
			}
		}
		for _, u := range s.Uplinks {
			if u.Port != nil {
				paths = append(paths, u.Port.Path)
			}
		}
		for _, sv := range s.Servers {
			if sv.Port != nil {
				paths = append(paths, sv.Port.Path)
			}
		}
		for _, d := range s.Devices {
			for _, ap := range d.AccessPorts {
				paths = append(paths, ap.Path, ap.Path+".network")
			}
		}
	}
	if len(paths) < 20 {
		t.Fatalf("замало шляхів: %d", len(paths))
	}
	for _, p := range paths {
		if !m.At(p).Known() {
			t.Errorf("шлях %q без позиції", p)
		}
	}
}

func TestL2IsDeterministic(t *testing.T) {
	dump := func() string {
		m := lower(t, load(t, testdata("valid", "m-profile.yml")))
		var b strings.Builder
		for _, s := range m.Sites {
			tr := s.L2
			fmt.Fprintf(&b, "%s: %s\n", s.Name, names(tr.Order))
			for _, d := range tr.Order {
				fmt.Fprintf(&b, "  %s ← %s\n", d.Name, pathText(tr.Path(d)))
				for _, n := range s.Networks {
					fmt.Fprintf(&b, "    %s=%v", n.Name, tr.Carried(d, n))
				}
				b.WriteString("\n")
			}
		}
		return b.String()
	}
	first := dump()
	for i := 0; i < 20; i++ {
		if got := dump(); got != first {
			t.Fatalf("прогін %d дав інше дерево", i)
		}
	}
}
