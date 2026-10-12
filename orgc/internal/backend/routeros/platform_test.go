package routeros

import (
	"testing"

	"orgc/internal/backend"
	"orgc/internal/ir"
)

func TestCheckVersion(t *testing.T) {
	tests := []struct {
		v  string
		ok bool
	}{
		{"6.48", true}, // the lab's RB750
		{"6.48.6", true},
		{"6.41", true}, // the floor: bridge VLAN filtering
		{"6.41.0", true},
		{"6.49.10", true},
		{"6.40", false},
		{"6.40.9", false},
		{"5.26", false},
		{"7.15", false}, // different syntax
		{"7.1.1", false},
		{"6.49beta11", false}, // test builds are not rendered
		{"6", false},
		{"v6.48", false},
		{"", false},
	}
	for _, tt := range tests {
		ok, supported := Platform{}.CheckVersion(tt.v)
		if ok != tt.ok {
			t.Errorf("CheckVersion(%q) = %v, want %v", tt.v, ok, tt.ok)
		}
		if supported != "RouterOS 6.41–6.x" {
			t.Errorf("CheckVersion(%q): діапазон %q — його треба повертати завжди", tt.v, supported)
		}
	}
}

// The management transport is the RouterOS API, not SSH. This is the fact
// the E006 demonstration case rests on: s-profile with ports [22] only.
func TestManagementPorts(t *testing.T) {
	got := Platform{}.ManagementPorts("6.48")
	if len(got) != 1 || got[0] != (ir.PortSpec{Proto: ir.TCP, Port: 8728}) {
		t.Errorf("ManagementPorts(6.48) = %v, want [tcp/8728]", got)
	}
	for _, p := range got {
		if p.Port == 22 {
			t.Error("SSH не є транспортом Ansible для RouterOS")
		}
	}
	if got := (Platform{}).ManagementPorts("7.15"); got != nil {
		t.Errorf("ManagementPorts(7.15) = %v, want nil для непідтримуваної версії", got)
	}
}

func TestCapabilities(t *testing.T) {
	caps := Platform{}.Capabilities("6.48")
	for _, c := range []backend.Capability{
		backend.L3Gateway, backend.DHCPServer, backend.NAT,
		backend.StatefulFilter, backend.ClientIsolation,
	} {
		if !caps.Has(c) {
			t.Errorf("RouterOS 6.48 має мати %q", c)
		}
	}
	if c := (Platform{}).Capabilities("7.15"); c != 0 {
		t.Errorf("Capabilities(7.15) = %v, want порожньо", c)
	}
}

func TestCheckPort(t *testing.T) {
	for name, want := range map[string]bool{
		"ether1": true, "ether13": true, "sfp1": true, "sfp-sfpplus1": true,
		"ether0": false, "Ethernet1": false, "eth1": false, "bridge": false, "wlan1": false, "": false,
	} {
		if ok, expected := (Platform{}).CheckPort(name); ok != want || expected == "" {
			t.Errorf("CheckPort(%q) = %v, %q; want %v", name, ok, expected, want)
		}
	}
}
