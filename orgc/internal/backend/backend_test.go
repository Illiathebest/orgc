package backend

import (
	"strings"
	"testing"

	"orgc/internal/ir"
)

type fake struct{ t ir.DeviceType }

func (f fake) Type() ir.DeviceType                { return f.t }
func (fake) CheckVersion(string) (bool, string)   { return true, "будь-яка" }
func (fake) ManagementPorts(string) []ir.PortSpec { return nil }
func (fake) Capabilities(string) CapSet           { return 0 }
func (fake) Render(*ir.Model, *ir.Device) []File  { return nil }

func TestRegistry(t *testing.T) {
	r, err := NewRegistry(fake{"zeta"}, fake{"alpha"})
	if err != nil {
		t.Fatal(err)
	}
	if p, ok := r.Lookup("zeta"); !ok || p.Type() != "zeta" {
		t.Errorf("Lookup(zeta) = %v, %v", p, ok)
	}
	if _, ok := r.Lookup("cisco_ios"); ok {
		t.Error("Lookup знайшов платформу, якої не реєстрували")
	}
	if got := r.Types(); len(got) != 2 || got[0] != "alpha" || got[1] != "zeta" {
		t.Errorf("Types() = %v, want відсортовано [alpha zeta]", got)
	}
}

// Two platforms for one device type would make the choice depend on
// registration order — a defect of orgc that must not pass silently.
func TestRegistryRejectsDuplicateType(t *testing.T) {
	_, err := NewRegistry(fake{"mikrotik"}, fake{"mikrotik"})
	if err == nil || !strings.Contains(err.Error(), "mikrotik") {
		t.Errorf("err = %v, want помилку з назвою типу", err)
	}
}

func TestCapSet(t *testing.T) {
	have := Caps(L3Gateway, NAT)
	want := Caps(StatefulFilter, L3Gateway, DHCPServer)

	if !have.Has(NAT) || have.Has(DHCPServer) {
		t.Errorf("Has: %b", have)
	}

	// Missing lists in declaration order, regardless of how want was built.
	got := have.Missing(want)
	if len(got) != 2 || got[0] != DHCPServer || got[1] != StatefulFilter {
		t.Errorf("Missing = %v, want [DHCPServer StatefulFilter]", got)
	}
	if m := want.Missing(0); m != nil {
		t.Errorf("нічого не потрібно — нічого не бракує, отримано %v", m)
	}

	if s := Caps(NAT, L3Gateway).String(); s != "маршрутизація між мережами сайту, трансляція адрес до інтернету" {
		t.Errorf("String() = %q", s)
	}
}

// Every capability needs a Ukrainian name: a W002 diagnostic prints it.
func TestEveryCapabilityIsNamed(t *testing.T) {
	for c := Capability(0); c < capabilityCount; c++ {
		if capabilityNames[c] == "" {
			t.Errorf("можливість %d без назви", c)
		}
	}
	if capabilityCount > 64 {
		t.Error("CapSet — uint64: більше 64 можливостей не вміститься")
	}
}
