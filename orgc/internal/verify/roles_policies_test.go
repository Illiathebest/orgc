package verify

import (
	"strings"
	"testing"

	"orgc/internal/diag"
)

var (
	e004 = []Check{{Code: diag.E004, Run: checkE004}}
	w003 = []Check{{Code: diag.W003, Run: checkW003}}

	// rules runs every check that judges a rule, so the tests below see
	// which single code a rule ends up with.
	rules = []Check{
		{Code: diag.E005, Run: checkE005},
		{Code: diag.W001, Run: checkW001},
		{Code: diag.W004, Run: checkW004},
	}
)

const minimalRule = "    - { from: servers, to: mgmt, ports: [22, 8728], action: allow }\n"

func mprofile() string { return testdata("valid", "m-profile.yml") }

func TestE004(t *testing.T) {
	t.Run("друга mgmt-мережа сайту теж годиться", func(t *testing.T) {
		m := variant(t, minimal(),
			minimalServers, minimalServers+"      - { vlan: 11, name: mgmt-2, role: mgmt, cidr: 192.168.11.0/24 }\n",
			minimalDevEnd, minimalDevEnd+"      - { name: sw-01, type: arista_eos, os_version: \"4.29.2F\", role: access_switch, mgmt_ip: 192.168.11.2 }\n")
		if ds := runChecks(t, m, e004); len(ds) != 0 {
			t.Errorf("отримано:\n%s", render(ds))
		}
	})
	t.Run("кожен пристрій сайту без mgmt-мережі — окрема діагностика", func(t *testing.T) {
		m := variant(t, mprofile(), "name: mgmt-branch\n        role: mgmt", "name: mgmt-branch\n        role: users")
		if ds := runChecks(t, m, e004); codes(ds) != "E004 E004 E004" {
			t.Errorf("у філії три пристрої, отримано:\n%s", render(ds))
		}
	})
}

func TestE005AllowsWhatIsNotDefaultDenyBreaking(t *testing.T) {
	for _, r := range []string{
		"{ from: any, to: servers, ports: [443], action: allow }",
		"{ from: servers, to: any, ports: [443], action: allow }",
		"{ from: servers, to: internet, action: allow }",
	} {
		m := variant(t, minimal(), minimalRule, minimalRule+"    - "+r+"\n")
		if ds := runChecks(t, m, []Check{rules[0]}); len(ds) != 0 {
			t.Errorf("%s:\n%s", r, render(ds))
		}
	}
}

// One rule, one code: E005 over W004 over W001.
func TestOneCodePerRule(t *testing.T) {
	guest := minimalServers + "      - { vlan: 40, name: guest, role: guest, cidr: 192.168.40.0/24, gateway: 192.168.40.1, isolated: true }\n"
	tests := []struct {
		rule string
		want string
	}{
		{"{ from: guest, to: mgmt, ports: [8728], action: allow }", "W004"},
		{"{ from: any, to: mgmt, ports: [8728], action: allow }", "W004"},
		{"{ from: guest, to: any, ports: [443], action: allow }", "W004"},
		{"{ from: any, to: any, ports: [443], action: allow }", "E005"},
		{"{ from: internet, to: guest, ports: [443], action: allow }", "E005"},
		{"{ from: any, to: servers, ports: [443], action: allow }", "W001"},
		{"{ from: guest, to: servers, ports: [443], action: allow }", "W001"},
		{"{ from: guest, to: internet, action: allow }", ""},
		{"{ from: servers, to: any, ports: [443], action: allow }", ""},
	}
	for _, tt := range tests {
		t.Run(tt.rule, func(t *testing.T) {
			m := variant(t, minimal(), minimalServers, guest, minimalRule, minimalRule+"    - "+tt.rule+"\n")
			if ds := runChecks(t, m, rules); codes(ds) != tt.want {
				t.Errorf("коди %q, want %q:\n%s", codes(ds), tt.want, render(ds))
			}
		})
	}
}

func TestW001(t *testing.T) {
	t.Run("ізольований guest з виходом в internet — m-profile", func(t *testing.T) {
		if ds := runChecks(t, lowerFile(t, mprofile()), rules); len(ds) != 0 {
			t.Errorf("отримано:\n%s", render(ds))
		}
	})
	t.Run("any до внутрішньої мережі без guest-мереж — не W001", func(t *testing.T) {
		m := variant(t, minimal(), minimalRule, minimalRule+"    - { from: any, to: servers, ports: [443], action: allow }\n")
		if ds := runChecks(t, m, rules); len(ds) != 0 {
			t.Errorf("отримано:\n%s", render(ds))
		}
	})
}

func TestW003(t *testing.T) {
	t.Run("профіль S канали не перевіряє", func(t *testing.T) {
		if ds := runChecks(t, lowerFile(t, minimal()), w003); len(ds) != 0 {
			t.Errorf("отримано:\n%s", render(ds))
		}
	})
	t.Run("профіль M з резервним каналом у кожному сайті", func(t *testing.T) {
		if ds := runChecks(t, lowerFile(t, mprofile()), w003); len(ds) != 0 {
			t.Errorf("отримано:\n%s", render(ds))
		}
	})
	t.Run("лише сайт без backup", func(t *testing.T) {
		m := variant(t, mprofile(), "{ name: lte-branch, kind: backup,", "{ name: lte-branch, kind: primary,")
		ds := runChecks(t, m, w003)
		if codes(ds) != "W003" || !strings.Contains(ds[0].Message, `"branch"`) {
			t.Errorf("отримано:\n%s", render(ds))
		}
	})
}

func TestW004(t *testing.T) {
	t.Run("вузол керування до mgmt і до any — не W004", func(t *testing.T) {
		m := variant(t, minimal(), minimalRule, minimalRule+"    - { from: servers, to: any, ports: [443], action: allow }\n")
		if ds := runChecks(t, m, rules); len(ds) != 0 {
			t.Errorf("отримано:\n%s", render(ds))
		}
	})
	t.Run("до any без mgmt-мереж у моделі — не W004", func(t *testing.T) {
		m := variant(t, minimal(),
			"name: mgmt, role: mgmt,", "name: mgmt, role: users,",
			minimalRule, minimalRule+"    - { from: mgmt, to: any, ports: [443], action: allow }\n")
		if ds := runChecks(t, m, []Check{rules[2]}); len(ds) != 0 {
			t.Errorf("отримано:\n%s", render(ds))
		}
	})
	t.Run("невідома мережа вузла керування — справа E003", func(t *testing.T) {
		m := variant(t, minimal(), "  network: servers\n", "  network: srvers\n")
		if ds := runChecks(t, m, []Check{rules[2]}); len(ds) != 0 {
			t.Errorf("отримано:\n%s", render(ds))
		}
	})
}
