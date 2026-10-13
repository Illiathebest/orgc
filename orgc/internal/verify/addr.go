package verify

import (
	"encoding/binary"
	"net/netip"

	"orgc/internal/ir"
)

// Address arithmetic shared by E001, E007 and E008. Model v2 is IPv4 only;
// the schema guarantees it.

func u32(a netip.Addr) uint32 {
	b := a.As4()
	return binary.BigEndian.Uint32(b[:])
}

// bounds returns the network and broadcast addresses of a prefix. Host bits
// written in the model are ignored, as a router would ignore them.
func bounds(p netip.Prefix) (network, broadcast uint32) {
	network = u32(p.Masked().Addr())
	return network, network | (^uint32(0) >> p.Bits())
}

// hostCapacity is the number of host addresses, 2^(32-prefix) - 2 by the
// definition of E008. /31 and /32 therefore hold no hosts at all: model v2
// describes LAN segments with a gateway, not point-to-point links.
func hostCapacity(p netip.Prefix) uint64 {
	size := uint64(1) << (32 - p.Bits())
	if size <= 2 {
		return 0
	}
	return size - 2
}

func contains(p netip.Prefix, a netip.Addr) bool {
	return p.Masked().Contains(a)
}

// isHost reports whether a is a host address of p: inside it, and neither
// its network nor its broadcast address.
func isHost(p netip.Prefix, a netip.Addr) bool {
	if !contains(p, a) {
		return false
	}
	network, broadcast := bounds(p)
	v := u32(a)
	return v != network && v != broadcast
}

// inPool reports whether a falls inside the pool. An inverted pool contains
// nothing; E007 reports it separately.
func inPool(r *ir.Range, a netip.Addr) bool {
	if r == nil {
		return false
	}
	v := u32(a)
	return u32(r.From) <= v && v <= u32(r.To)
}

// poolSize is the number of addresses the pool hands out, zero for an
// inverted pool.
func poolSize(r *ir.Range) uint64 {
	if r == nil || u32(r.From) > u32(r.To) {
		return 0
	}
	return uint64(u32(r.To)-u32(r.From)) + 1
}

// static is an address assigned by hand: a device mgmt_ip, a server ip, or
// the control node ip.
type static struct {
	addr netip.Addr
	path string // the field holding the address, e.g. "sites[0].servers[1].ip"
	what string // how the owner is named in a message

	device  *ir.Device // set for a device mgmt_ip
	server  bool
	control bool

	// net is the network of the owner's site the address falls into, nil
	// when there is none. The control node is placed only in its own
	// network.
	net *ir.Network
}

// statics lists every static address in a fixed order: the control node,
// then devices and servers in file order. Earlier entries win collisions.
func statics(m *ir.Model) []static {
	var out []static

	cn := m.ControlNode
	if cn.Network.Net != nil {
		s := static{addr: cn.IP, path: cn.Path + ".ip", what: "вузол керування", control: true}
		if contains(cn.Network.Net.Prefix, cn.IP) {
			s.net = cn.Network.Net
		}
		out = append(out, s)
	}
	for _, d := range m.Devices {
		out = append(out, static{
			addr: d.MgmtIP, path: d.Path + ".mgmt_ip", what: "пристрій " + d.Name,
			device: d, net: networkOf(d.Site, d.MgmtIP),
		})
	}
	for _, s := range m.Servers {
		out = append(out, static{
			addr: s.IP, path: s.Path + ".ip", what: "сервер " + s.Name,
			server: true, net: networkOf(s.Site, s.IP),
		})
	}
	return out
}

// networkOf returns the first network of the site holding the address.
func networkOf(s *ir.Site, a netip.Addr) *ir.Network {
	for _, n := range s.Networks {
		if contains(n.Prefix, a) {
			return n
		}
	}
	return nil
}

// demand counts the addresses a network has to hold: its pool, plus every
// distinct gateway or static address inside the prefix and outside the pool.
func demand(n *ir.Network, sts []static) uint64 {
	seen := map[uint32]bool{}
	add := func(a netip.Addr) {
		if contains(n.Prefix, a) && !inPool(n.Pool, a) {
			seen[u32(a)] = true
		}
	}
	if n.Gateway.IsValid() {
		add(n.Gateway)
	}
	for _, s := range sts {
		if s.net == n {
			add(s.addr)
		}
	}
	return poolSize(n.Pool) + uint64(len(seen))
}

// overCapacity reports whether the network is too small for what it holds —
// E008. E007 skips such a network: the misplaced addresses are a symptom.
func overCapacity(n *ir.Network, sts []static) bool {
	return demand(n, sts) > hostCapacity(n.Prefix)
}
