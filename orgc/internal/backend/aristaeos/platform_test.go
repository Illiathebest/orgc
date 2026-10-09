package aristaeos

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
		{"4.29.2F", true}, // the m-profile switch
		{"4.30.1M", true},
		{"4.28.10.1M", true},
		{"4.29.2", true},
		{"4.29", false},
		{"5.0.0F", false},
		{"3.9.1F", false},
		{"4.29.2f", false}, // the train letter is upper case
		{"6.48", false},    // a RouterOS version on the wrong device
		{"", false},
	}
	for _, tt := range tests {
		ok, supported := Platform{}.CheckVersion(tt.v)
		if ok != tt.ok {
			t.Errorf("CheckVersion(%q) = %v, want %v", tt.v, ok, tt.ok)
		}
		if supported != "Arista EOS 4.x" {
			t.Errorf("CheckVersion(%q): діапазон %q", tt.v, supported)
		}
	}
}

func TestManagementPorts(t *testing.T) {
	got := Platform{}.ManagementPorts("4.29.2F")
	if len(got) != 1 || got[0] != (ir.PortSpec{Proto: ir.TCP, Port: 22}) {
		t.Errorf("ManagementPorts = %v, want [tcp/22]", got)
	}
	if got := (Platform{}).ManagementPorts("5.0.0F"); got != nil {
		t.Errorf("ManagementPorts(5.0.0F) = %v, want nil", got)
	}
}

// EOS ACLs are stateless: a model rule, which is stateful, cannot be
// enforced as written. W002 depends on the platform saying so.
func TestCapabilities(t *testing.T) {
	caps := Platform{}.Capabilities("4.29.2F")
	if !caps.Has(backend.L3Gateway) {
		t.Error("EOS маршрутизує між VLAN")
	}
	if caps.Has(backend.StatefulFilter) {
		t.Error("ACL в EOS не зберігають стан — StatefulFilter бути не може")
	}
	if c := (Platform{}).Capabilities("5.0.0F"); c != 0 {
		t.Errorf("Capabilities(5.0.0F) = %v, want порожньо", c)
	}
}
