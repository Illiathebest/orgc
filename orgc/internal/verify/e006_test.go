package verify

import (
	"strings"
	"testing"

	"orgc/internal/diag"
)

var e006 = []Check{{Code: diag.E006, Run: checkE006}}

// The cases the method has to get right, as listed for E006 in the plan.
func TestE006(t *testing.T) {
	sprofile := testdata("valid", "s-profile.yml")

	t.Run("s-profile з ports: [22] — E006 на MikroTik, бо Ansible керує ним через API", func(t *testing.T) {
		m := variant(t, sprofile, "{ from: servers, to: mgmt, ports: [22, 8728],", "{ from: servers, to: mgmt, ports: [22],")
		ds := runChecks(t, m, Checks())
		if codes(ds) != "E006" || !strings.Contains(ds[0].Message, "tcp/8728") || !strings.Contains(ds[0].Message, "rb750") {
			t.Errorf("отримано:\n%s", render(ds))
		}
	})
	t.Run("N_c = N_m — правило не потрібне", func(t *testing.T) {
		m := variant(t, minimal(),
			"  network: servers\n  ip: 192.168.30.10\n", "  network: mgmt\n  ip: 192.168.10.10\n",
			"\npolicies:\n  inter_vlan:\n"+minimalRule, "")
		if ds := runChecks(t, m, Checks()); len(ds) != 0 {
			t.Errorf("отримано:\n%s", render(ds))
		}
	})
	t.Run("доступ через any", func(t *testing.T) {
		for _, r := range []string{
			"    - { from: any, to: mgmt, ports: [8728], action: allow }\n",
			"    - { from: servers, to: any, ports: [8728], action: allow }\n",
		} {
			m := variant(t, minimal(), minimalRule, r)
			if ds := runChecks(t, m, e006); len(ds) != 0 {
				t.Errorf("%s:\n%s", r, render(ds))
			}
		}
	})
	t.Run("правило без портів пропускає все", func(t *testing.T) {
		m := variant(t, minimal(), minimalRule, "    - { from: servers, to: mgmt, action: allow }\n")
		if ds := runChecks(t, m, e006); len(ds) != 0 {
			t.Errorf("отримано:\n%s", render(ds))
		}
	})
	t.Run("udp/8728 — не той протокол", func(t *testing.T) {
		m := variant(t, minimal(), minimalRule, "    - { from: servers, to: mgmt, ports: [8728], proto: udp, action: allow }\n")
		if ds := runChecks(t, m, e006); codes(ds) != "E006" {
			t.Errorf("отримано:\n%s", render(ds))
		}
	})
	t.Run("профіль M через два шлюзи — OK", func(t *testing.T) {
		if ds := runChecks(t, lowerFile(t, mprofile()), Checks()); len(ds) != 0 {
			t.Errorf("отримано:\n%s", render(ds))
		}
	})
	t.Run("профіль M без правила до філії — E006 на кожен пристрій філії", func(t *testing.T) {
		m := variant(t, mprofile(), "    - { from: servers, to: mgmt-branch, ports: [22, 8728], action: allow }\n", "")
		ds := runChecks(t, m, Checks())
		if codes(ds) != "E006 E006 E006" {
			t.Fatalf("отримано:\n%s", render(ds))
		}
		for i, name := range []string{"rb-branch", "core-br-01", "sw-branch-01"} {
			if !strings.Contains(ds[i].Message, name) {
				t.Errorf("діагностика %d не про %s: %s", i, name, ds[i])
			}
		}
	})
	t.Run("порт залежить від платформи: Arista у філії потребує 22", func(t *testing.T) {
		m := variant(t, mprofile(), "{ from: servers, to: mgmt-branch, ports: [22, 8728],", "{ from: servers, to: mgmt-branch, ports: [8728],")
		ds := runChecks(t, m, e006)
		// Both branch switches are Arista; the router, on the RouterOS API,
		// is still reachable on 8728.
		if codes(ds) != "E006 E006" || !strings.Contains(ds[0].Message, "core-br-01") ||
			!strings.Contains(ds[1].Message, "sw-branch-01") || !strings.Contains(ds[1].Message, "tcp/22") {
			t.Errorf("отримано:\n%s", render(ds))
		}
	})
	t.Run("сайт без L3-шлюзу — лише E003, E006 мовчить", func(t *testing.T) {
		m := variant(t, minimal(), "role: edge_router", "role: access_switch",
			"        mgmt_ip: 192.168.10.1\n", "        mgmt_ip: 192.168.10.2\n")
		if ds := runChecks(t, m, Checks()); codes(ds) != "E003" {
			t.Errorf("отримано:\n%s", render(ds))
		}
	})
}

// E006 is silent wherever another code already explains the failure.
func TestE006SkipsOtherCodes(t *testing.T) {
	tests := []struct {
		name  string
		edits []string
		want  string
	}{
		{"невідома мережа вузла керування", []string{"  network: servers\n", "  network: srvers\n"}, "E003"},
		{"mgmt_ip поза mgmt-мережею", []string{"        mgmt_ip: 192.168.10.1\n", "        mgmt_ip: 192.168.30.2\n"}, "E004"},
		{"версія ОС поза діапазоном", []string{`os_version: "6.48"`, `os_version: "7.15"`}, "W002"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := variant(t, minimal(), append(tt.edits, minimalRule, "    - { from: servers, to: mgmt, ports: [22], action: allow }\n")...)
			if ds := runChecks(t, m, Checks()); codes(ds) != tt.want {
				t.Errorf("коди %q, want %q:\n%s", codes(ds), tt.want, render(ds))
			}
		})
	}
}

// The search itself: networks without a gateway stay off the graph, and
// gateways of different sites are connected.
func TestReachable(t *testing.T) {
	m := lowerFile(t, mprofile())
	reach := reachable(m, m.ControlNode.Network.Net)
	for _, n := range m.Networks {
		if !reach[n] {
			t.Errorf("%s/%s недосяжна в m-profile", n.Site.Name, n.Name)
		}
	}

	m = variant(t, mprofile(), "cidr: 192.168.110.0/24\n        gateway: 192.168.110.1\n", "cidr: 192.168.110.0/24\n")
	reach = reachable(m, m.ControlNode.Network.Net)
	for _, n := range m.Networks {
		if want := n.Name != "mgmt-branch"; reach[n] != want {
			t.Errorf("%s: досяжна = %v, want %v", n.Name, reach[n], want)
		}
	}
}
