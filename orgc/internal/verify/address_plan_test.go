package verify

import (
	"strings"
	"testing"

	"orgc/internal/diag"
)

var (
	e001 = []Check{{Code: diag.E001, Run: checkE001}}
	e007 = []Check{{Code: diag.E007, Run: checkE007}}
	e008 = []Check{{Code: diag.E008, Run: checkE008}}

	// addressPlan runs the three together: the edge cases below are about
	// which of them fires, not only whether one does.
	addressPlan = []Check{e001[0], e007[0], e008[0]}
)

const (
	minimalServers = "      - { vlan: 30, name: servers, role: servers, cidr: 192.168.30.0/24, gateway: 192.168.30.1 }\n"
	minimalDevEnd  = "        mgmt_ip: 192.168.10.1\n"
)

func minimal() string { return testdata("valid", "minimal.yml") }

// codes renders the codes found, for compact assertions.
func codes(ds []diag.Diagnostic) string {
	var cs []string
	for _, d := range ds {
		cs = append(cs, string(d.Code))
	}
	return strings.Join(cs, " ")
}

func TestE001AllowOverlapBetweenSites(t *testing.T) {
	branch := func(overlap string) string {
		return minimalDevEnd + "  - name: branch\n" + overlap +
			"    networks:\n      - { vlan: 40, name: lab, role: users, cidr: 192.168.30.0/24 }\n"
	}

	t.Run("з allow_overlap перетин між сайтами дозволено", func(t *testing.T) {
		m := variant(t, minimal(), minimalDevEnd, branch("    allow_overlap: true\n"))
		if ds := runChecks(t, m, addressPlan); len(ds) != 0 {
			t.Errorf("отримано:\n%s", render(ds))
		}
	})
	t.Run("allow_overlap не дозволяє перетину всередині сайту", func(t *testing.T) {
		m := variant(t, minimal(),
			"  - name: hq\n", "  - name: hq\n    allow_overlap: true\n",
			minimalServers, minimalServers+"      - { vlan: 40, name: lab, role: users, cidr: 192.168.30.128/25 }\n")
		if ds := runChecks(t, m, e001); codes(ds) != "E001" {
			t.Errorf("отримано:\n%s", render(ds))
		}
	})

	overlapping := func(t *testing.T, rule string) []diag.Diagnostic {
		t.Helper()
		m := variant(t, minimal(),
			minimalDevEnd, branch("    allow_overlap: true\n"),
			"  inter_vlan:\n", "  inter_vlan:\n"+rule)
		return runChecks(t, m, e001)
	}
	t.Run("правило через any до мережі з дозволеним перетином", func(t *testing.T) {
		ds := overlapping(t, "    - { from: lab, to: any, ports: [443], action: allow }\n")
		if codes(ds) != "E001" || !strings.HasPrefix(ds[0].Path, "policies.inter_vlan[") {
			t.Errorf("отримано:\n%s", render(ds))
		}
	})
	t.Run("any → any — справа E005, не E001", func(t *testing.T) {
		if ds := overlapping(t, "    - { from: any, to: any, ports: [443], action: allow }\n"); len(ds) != 0 {
			t.Errorf("отримано:\n%s", render(ds))
		}
	})
	t.Run("вихід в internet адресний план не зачіпає", func(t *testing.T) {
		if ds := overlapping(t, "    - { from: lab, to: internet, action: allow }\n"); len(ds) != 0 {
			t.Errorf("отримано:\n%s", render(ds))
		}
	})
}

func TestE007Exceptions(t *testing.T) {
	// minimal: rb750, the site's gateway, sits on the gateway of mgmt.
	// s-profile: the control node is the server ctrl-01.
	for _, f := range []string{minimal(), testdata("valid", "s-profile.yml")} {
		if ds := runChecks(t, lowerFile(t, f), addressPlan); len(ds) != 0 {
			t.Errorf("%s:\n%s", f, render(ds))
		}
	}

	t.Run("не-шлюз на адресі шлюзу mgmt — колізія", func(t *testing.T) {
		m := variant(t, minimal(), minimalDevEnd, minimalDevEnd+
			"      - { name: sw-01, type: arista_eos, os_version: \"4.36.2F\", role: access_switch, mgmt_ip: 192.168.10.1 }\n")
		if ds := runChecks(t, m, e007); codes(ds) != "E007" || ds[0].Path != "sites[0].devices[1].mgmt_ip" {
			t.Errorf("отримано:\n%s", render(ds))
		}
	})
	t.Run("два сервери на одній адресі — колізія", func(t *testing.T) {
		m := variant(t, testdata("valid", "s-profile.yml"), "ip: 192.168.30.11", "ip: 192.168.30.10")
		if ds := runChecks(t, m, e007); codes(ds) != "E007" || !strings.Contains(ds[0].Message, "сервер ctrl-01") {
			t.Errorf("отримано:\n%s", render(ds))
		}
	})
	t.Run("шлюз — адреса мережі", func(t *testing.T) {
		m := variant(t, minimal(), "gateway: 192.168.30.1 }", "gateway: 192.168.30.0 }")
		if ds := runChecks(t, m, e007); codes(ds) != "E007" || !strings.HasSuffix(ds[0].Path, ".gateway") {
			t.Errorf("отримано:\n%s", render(ds))
		}
	})
}

// What another code already reports, E007 leaves alone.
func TestE007SkipsOtherCodes(t *testing.T) {
	t.Run("mgmt_ip поза мережами сайту — це E004", func(t *testing.T) {
		m := variant(t, minimal(), minimalDevEnd, "        mgmt_ip: 192.168.99.1\n")
		if ds := runChecks(t, m, e007); len(ds) != 0 {
			t.Errorf("отримано:\n%s", render(ds))
		}
	})
	t.Run("мережа вузла керування не розвʼязана — це E003", func(t *testing.T) {
		m := variant(t, minimal(), "  network: servers\n", "  network: srvers\n")
		if ds := runChecks(t, m, e007); len(ds) != 0 {
			t.Errorf("отримано:\n%s", render(ds))
		}
	})
}

// The boundaries the address plan has to get exactly right.
func TestAddressPlanBoundaries(t *testing.T) {
	net := func(cidr, rest string) string {
		return minimalServers + "      - { vlan: 40, name: lab, role: users, cidr: " + cidr + rest + " }\n"
	}
	server := func(ip string) string {
		return minimalDevEnd + "    servers:\n      - { name: app-01, role: [app], ip: " + ip + " }\n"
	}

	tests := []struct {
		name  string
		edits []string
		want  string
	}{
		{"пул до останньої адреси хоста", []string{
			minimalServers, net("192.168.40.0/24", ", gateway: 192.168.40.1, dhcp_pool: { from: 192.168.40.200, to: 192.168.40.254 }"),
		}, ""},
		{"пул від адреси мережі", []string{
			minimalServers, net("192.168.40.0/24", ", gateway: 192.168.40.254, dhcp_pool: { from: 192.168.40.0, to: 192.168.40.10 }"),
		}, "E007"},
		{"/30 рівно на шлюз і сервер", []string{
			minimalServers, net("192.168.40.0/30", ", gateway: 192.168.40.1"),
			minimalDevEnd, server("192.168.40.2"),
		}, ""},
		{"/30 на шлюз і широкомовну адресу", []string{
			minimalServers, net("192.168.40.0/30", ", gateway: 192.168.40.1"),
			minimalDevEnd, server("192.168.40.3"),
		}, "E007"},
		{"/29 рівно на шлюз і пул з пʼяти адрес", []string{
			minimalServers, net("192.168.40.0/29", ", gateway: 192.168.40.1, dhcp_pool: { from: 192.168.40.2, to: 192.168.40.6 }"),
		}, ""},
		{"/29 на одну адресу більше за ємність", []string{
			minimalServers, net("192.168.40.0/29", ", gateway: 192.168.40.1, dhcp_pool: { from: 192.168.40.2, to: 192.168.40.7 }"),
		}, "E008"},
		{"/32 без адрес — порожня мережа не помилка", []string{
			minimalServers, net("192.168.40.0/32", ""),
		}, ""},
		{"/32 зі шлюзом", []string{
			minimalServers, net("192.168.40.0/32", ", gateway: 192.168.40.0"),
		}, "E008"},
		{"переповнена мережа — лише E008, без симптомів E007", []string{
			minimalServers, net("192.168.40.0/30", ", gateway: 192.168.40.1, dhcp_pool: { from: 192.168.40.2, to: 192.168.40.6 }"),
		}, "E008"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := variant(t, minimal(), tt.edits...)
			if ds := runChecks(t, m, addressPlan); codes(ds) != tt.want {
				t.Errorf("коди %q, want %q:\n%s", codes(ds), tt.want, render(ds))
			}
		})
	}
}

// The control node and a server on the same address count once, as do the
// gateway and the mgmt_ip of the device carrying it: E008 counts addresses,
// not the entries that name them.
func TestE008CountsDistinctAddresses(t *testing.T) {
	m := variant(t, minimal(),
		"cidr: 192.168.30.0/24, gateway: 192.168.30.1 }", "cidr: 192.168.30.0/30, gateway: 192.168.30.1 }",
		"  ip: 192.168.30.10\n", "  ip: 192.168.30.2\n",
		minimalDevEnd, minimalDevEnd+"    servers:\n      - { name: ctrl-01, role: [docker_host], ip: 192.168.30.2 }\n")
	if ds := runChecks(t, m, addressPlan); len(ds) != 0 {
		t.Errorf("/30: шлюз і одна адреса двох записів — 2 хости, а отримано:\n%s", render(ds))
	}
}
