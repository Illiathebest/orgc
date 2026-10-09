package backend

import "strings"

// Capability is something a model can require of a device. The names are
// vendor-neutral and owned by the core: the core decides from the IR what a
// device needs, the platform says what it has, and W002 is the difference.
type Capability uint

const (
	// L3Gateway: route between the site's networks — VLAN interfaces with
	// the gateway addresses.
	L3Gateway Capability = iota

	// DHCPServer: hand out addresses from a network's dhcp_pool.
	DHCPServer

	// NAT: translate source addresses on the way to the internet.
	NAT

	// StatefulFilter: admit return traffic of permitted connections. Model
	// rules are stateful, so a gateway that filters statelessly cannot
	// enforce them as written.
	StatefulFilter

	// ClientIsolation: forbid traffic between hosts of one network
	// (isolated: true).
	ClientIsolation

	capabilityCount // keep last
)

var capabilityNames = [capabilityCount]string{
	L3Gateway:       "маршрутизація між мережами сайту",
	DHCPServer:      "DHCP-сервер",
	NAT:             "трансляція адрес до інтернету",
	StatefulFilter:  "фільтрація зі станом зʼєднань",
	ClientIsolation: "ізоляція клієнтів усередині мережі",
}

// String names the capability in Ukrainian, for diagnostics.
func (c Capability) String() string {
	if c < capabilityCount {
		return capabilityNames[c]
	}
	return "невідома можливість"
}

// CapSet is a set of capabilities. A bit set rather than a map: it iterates
// in a fixed order, so whatever is printed from it is deterministic.
type CapSet uint64

// Caps builds a set.
func Caps(cs ...Capability) CapSet {
	var s CapSet
	for _, c := range cs {
		s |= 1 << c
	}
	return s
}

// Has reports whether the set contains c.
func (s CapSet) Has(c Capability) bool { return s&(1<<c) != 0 }

// Missing lists, in declaration order, what want has and s does not.
func (s CapSet) Missing(want CapSet) []Capability {
	var out []Capability
	for c := Capability(0); c < capabilityCount; c++ {
		if want.Has(c) && !s.Has(c) {
			out = append(out, c)
		}
	}
	return out
}

// String lists the capabilities in declaration order.
func (s CapSet) String() string {
	var names []string
	for c := Capability(0); c < capabilityCount; c++ {
		if s.Has(c) {
			names = append(names, c.String())
		}
	}
	return strings.Join(names, ", ")
}
