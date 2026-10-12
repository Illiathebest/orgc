package verify

import (
	"strings"
	"testing"
)

func sprofile() string { return testdata("valid", "s-profile.yml") }

const (
	sLinkAcc01 = "      - { a: core-01:Ethernet2, b: acc-01:Ethernet1, vlans: [mgmt, servers] }\n"
	sLinkAcc02 = "      - { a: core-01:Ethernet3, b: acc-02:Ethernet1 }\n"
)

// Positive and negative cases of the connection checks, one code at a time
// and with every check registered, so that each case also proves no other
// code fires.
func TestConnectionChecks(t *testing.T) {
	tests := []struct {
		name  string
		base  string
		edits []string
		want  string
		msg   string // fragment of the first diagnostic, when one is expected
	}{
		{"те саме імʼя порту на різних пристроях — норма", sprofile(), nil, "", ""},
		{"access-порт на самому шлюзі", sprofile(),
			[]string{"        mgmt_ip: 192.168.10.1\n", "        mgmt_ip: 192.168.10.1\n        access_ports:\n          - { ports: [ether3], network: users }\n"}, "", ""},
		{"сервер на порту core-комутатора", sprofile(),
			[]string{"port: acc-01:Ethernet6 }", "port: core-01:Ethernet6 }"}, "", ""},
		{"порти SFP+ на RouterOS і модульні на EOS", sprofile(),
			[]string{"{ a: rb750:ether2, b: core-01:Ethernet1 }", "{ a: rb750:sfp-sfpplus1, b: core-01:Ethernet1/1 }"}, "", ""},
		{"Management1 як кінець зʼєднання", sprofile(),
			[]string{"b: acc-01:Ethernet1, vlans", "b: acc-01:Management1, vlans"}, "W002", "Management1"},
		{"порт сервера на пристрої іншого сайту", mprofile(),
			[]string{"port: sw-branch-01:Ethernet5 }", "port: acc-01:Ethernet7 }"}, "E003", `сайту "hq"`},
		{"кінець зʼєднання на пристрої іншого сайту", mprofile(),
			[]string{"{ a: core-br-01:Ethernet2, b: sw-branch-01:Ethernet1 }", "{ a: core-br-01:Ethernet2, b: acc-03:Ethernet9 }"}, "E003", "зʼєднання між сайтами"},
		{"мережа іншого сайту у vlans", mprofile(),
			[]string{"vlans: [mgmt, users, guest] }", "vlans: [mgmt, users, guest, users-branch] }"}, "E003", `сайту "branch"`},
		{"порт використано і як access, і як trunk", sprofile(),
			[]string{"{ ports: [Ethernet6], network: guest }", "{ ports: [Ethernet1], network: guest }"}, "E002", "уже використано"},
		{"пропуск рівня: access одразу під core", sprofile(), nil, "", ""},
		{"distribution за access-комутатором", sprofile(),
			[]string{sLinkAcc01, "      - { a: core-01:Ethernet2, b: acc-01:Ethernet1 }\n",
				"        role: access_switch\n        mgmt_ip: 192.168.10.4\n", "        role: distribution_switch\n        mgmt_ip: 192.168.10.4\n",
				sLinkAcc02, "      - { a: acc-01:Ethernet2, b: acc-02:Ethernet1 }\n"}, "W005", "distribution_switch"},
		{"ієрархія m-profile без петель і з повною звʼязністю", mprofile(), nil, "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ds := runChecks(t, variant(t, tt.base, tt.edits...), Checks())
			if codes(ds) != tt.want {
				t.Fatalf("коди %q, want %q:\n%s", codes(ds), tt.want, render(ds))
			}
			if tt.msg != "" && !strings.Contains(ds[0].Message, tt.msg) {
				t.Errorf("повідомлення без %q: %s", tt.msg, ds[0])
			}
		})
	}
}

// One defect, one code: a broken topology is reported once, and the checks
// that need a whole tree stay silent about the site.
func TestConnectionCascades(t *testing.T) {
	tests := []struct {
		name  string
		edits []string
		want  string
	}{
		{"петля: ні E009, ні W005, ні E006 у сайті",
			[]string{sLinkAcc02, sLinkAcc02 + "      - { a: acc-01:Ethernet2, b: acc-02:Ethernet2 }\n",
				"{ from: servers, to: mgmt, ports: [22, 8728],", "{ from: servers, to: mgmt, ports: [8728],"},
			"E010"},
		{"нерозвʼязаний кінець: ні E009, ні E010",
			[]string{sLinkAcc02, "      - { a: core-01:Ethernet3, b: acc-09:Ethernet1 }\n" +
				"      - { a: acc-01:Ethernet2, b: acc-02:Ethernet2 }\n"},
			"E003"},
		{"пристрій без шляху: одна діагностика, без E006 і без доставки на його порти",
			[]string{sLinkAcc02, ""}, "E009"},
		{"мережу вузла керування не доставлено: E009 на портах, E006 мовчить",
			[]string{sLinkAcc01, "      - { a: core-01:Ethernet2, b: acc-01:Ethernet1, vlans: [mgmt] }\n"},
			"E009 E009"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ds := runChecks(t, variant(t, sprofile(), tt.edits...), Checks())
			if codes(ds) != tt.want {
				t.Errorf("коди %q, want %q:\n%s", codes(ds), tt.want, render(ds))
			}
		})
	}
}

// The L2 part of E006: mgmt must be allowed on every trunk from the gateway
// down to the device.
func TestE006L2(t *testing.T) {
	t.Run("без vlans зʼєднання несе й mgmt", func(t *testing.T) {
		if ds := runChecks(t, lowerFile(t, sprofile()), e006); len(ds) != 0 {
			t.Errorf("отримано:\n%s", render(ds))
		}
	})
	t.Run("mgmt прибрано вище — E006 на всіх нижче, позиція на першому зʼєднанні без mgmt", func(t *testing.T) {
		m := variant(t, mprofile(), "      - { a: core-01:Ethernet2, b: dist-01:Ethernet1 }\n",
			"      - { a: core-01:Ethernet2, b: dist-01:Ethernet1, vlans: [users, servers] }\n")
		ds := runChecks(t, m, e006)
		if codes(ds) != "E006 E006 E006" {
			t.Fatalf("dist-01, acc-01 і acc-02 за цим зʼєднанням:\n%s", render(ds))
		}
		for _, d := range ds {
			if d.Path != "sites[0].links[1].vlans" {
				t.Errorf("позиція %s, want sites[0].links[1].vlans", d.Path)
			}
		}
	})
	t.Run("шлюз сам себе не блокує", func(t *testing.T) {
		m := variant(t, mprofile(), "      - { a: rb750:ether2, b: core-01:Ethernet1 }\n",
			"      - { a: rb750:ether2, b: core-01:Ethernet1, vlans: [users, servers, guest] }\n")
		for _, d := range runChecks(t, m, e006) {
			if strings.Contains(d.Message, `"rb750"`) {
				t.Errorf("E006 на шлюзі: %s", d)
			}
		}
	})
	t.Run("сайт без зʼєднань — третя умова тривіальна", func(t *testing.T) {
		if ds := runChecks(t, lowerFile(t, testdata("valid", "lab.yml")), Checks()); len(ds) != 0 {
			t.Errorf("отримано:\n%s", render(ds))
		}
	})
}
