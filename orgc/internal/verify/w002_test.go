package verify

import (
	"testing"

	"orgc/internal/backend"
	"orgc/internal/diag"
)

var w002 = []Check{{Code: diag.W002, Run: checkW002}}

// What the model requires of the site's gateway, built up one feature at a
// time from minimal (two networks, one rule servers → mgmt).
func TestW002Required(t *testing.T) {
	gw := func(t *testing.T, edits ...string) backend.CapSet {
		t.Helper()
		m := variant(t, minimal(), edits...)
		return required(m, m.Sites[0].Gateway)
	}
	base := backend.Caps(backend.L3Gateway, backend.StatefulFilter)

	tests := []struct {
		name  string
		edits []string
		want  backend.CapSet
	}{
		{"маршрутизація і правило між мережами сайту", nil, base},
		{"без правил фільтрація зі станом не потрібна", []string{"\npolicies:\n  inter_vlan:\n" + minimalRule, ""},
			backend.Caps(backend.L3Gateway)},
		{"DHCP-пул", []string{"gateway: 192.168.30.1 }", "gateway: 192.168.30.1, dhcp_pool: { from: 192.168.30.100, to: 192.168.30.200 } }"},
			base | backend.Caps(backend.DHCPServer)},
		{"вихід в internet", []string{minimalRule, minimalRule + "    - { from: servers, to: internet, action: allow }\n"},
			base | backend.Caps(backend.NAT)},
		{"ізольована мережа", []string{"gateway: 192.168.30.1 }", "gateway: 192.168.30.1, isolated: true }"},
			base | backend.Caps(backend.ClientIsolation)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := gw(t, tt.edits...); got != tt.want {
				t.Errorf("required = [%v], want [%v]", got, tt.want)
			}
		})
	}

	t.Run("не шлюз нічого не вимагає", func(t *testing.T) {
		m := lowerFile(t, mprofile())
		for _, d := range m.Devices {
			if d.Role.RoutesL3() {
				continue
			}
			if got := required(m, d); got != 0 {
				t.Errorf("%s: required = [%v]", d.Name, got)
			}
		}
	})
}

func TestW002(t *testing.T) {
	t.Run("Arista як комутатор доступу в m-profile — без попереджень", func(t *testing.T) {
		if ds := runChecks(t, lowerFile(t, mprofile()), w002); len(ds) != 0 {
			t.Errorf("отримано:\n%s", render(ds))
		}
	})
	t.Run("тестова збірка RouterOS — рівень помилки, без переліку можливостей", func(t *testing.T) {
		m := variant(t, minimal(), `os_version: "6.48"`, `os_version: "6.49beta11"`)
		ds := runChecks(t, m, w002)
		if codes(ds) != "W002" || ds[0].Severity != diag.Error {
			t.Errorf("отримано:\n%s", render(ds))
		}
	})
}
