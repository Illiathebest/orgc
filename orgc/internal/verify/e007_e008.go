package verify

import (
	"fmt"
	"net/netip"

	"orgc/internal/backend"
	"orgc/internal/diag"
	"orgc/internal/ir"
)

// checkE007 reports an inconsistent address plan (docs/data-model.md):
//
//   - the gateway is a host address of its network and outside the pool;
//   - the pool is not inverted, and both bounds are host addresses of the
//     network;
//   - every static address is a host address of a network of its own site,
//     outside that network's pool; the control node ip lies in the control
//     node's network;
//   - static addresses and gateways do not collide, except that the mgmt_ip
//     of a site's L3 gateway may equal the gateway of its mgmt network, and
//     the control node may be one of the servers.
//
// Two cases belong to other codes and are skipped: whether a device mgmt_ip
// lies in a network at all (E004 asks for more — a mgmt network of its site),
// and any network too small for what it holds (E008), where misplaced
// addresses are only a symptom.
func checkE007(m *ir.Model, _ backend.Platforms) []diag.Diagnostic {
	var out []diag.Diagnostic
	bad := func(path, msg string) {
		out = append(out, report(m, diag.E007, diag.Error, path, msg))
	}

	sts := statics(m)
	full := map[*ir.Network]bool{}
	for _, n := range m.Networks {
		if overCapacity(n, sts) {
			full[n] = true
			continue
		}

		if g := n.Gateway; g.IsValid() {
			switch {
			case !contains(n.Prefix, g):
				bad(n.Path+".gateway", fmt.Sprintf("шлюз %s лежить поза мережею %s", g, n.Prefix))
			case !isHost(n.Prefix, g):
				bad(n.Path+".gateway", fmt.Sprintf("шлюз %s є адресою мережі чи широкомовною адресою %s", g, n.Prefix))
			case inPool(n.Pool, g):
				bad(n.Path+".gateway", fmt.Sprintf("шлюз %s потрапляє в DHCP-пул %s–%s", g, n.Pool.From, n.Pool.To))
			}
		}

		if p := n.Pool; p != nil {
			if u32(p.From) > u32(p.To) {
				bad(n.Path+".dhcp_pool", fmt.Sprintf("DHCP-пул перевернуто: початок %s більший за кінець %s", p.From, p.To))
			} else {
				for _, b := range []struct {
					field string
					addr  netip.Addr
				}{{"from", p.From}, {"to", p.To}} {
					switch {
					case !contains(n.Prefix, b.addr):
						bad(n.Path+".dhcp_pool."+b.field, fmt.Sprintf("межа пулу %s лежить поза мережею %s", b.addr, n.Prefix))
					case !isHost(n.Prefix, b.addr):
						bad(n.Path+".dhcp_pool."+b.field, fmt.Sprintf("межа пулу %s є адресою мережі чи широкомовною адресою %s", b.addr, n.Prefix))
					}
				}
			}
		}
	}

	// taken records who holds each address of each network, in the order
	// of statics(): gateways first, then the control node, devices, servers.
	type holder struct {
		what, path string
		server     bool
		control    bool
		gatewayOf  *ir.Network
	}
	taken := map[*ir.Network]map[uint32]holder{}
	slot := func(n *ir.Network) map[uint32]holder {
		if taken[n] == nil {
			taken[n] = map[uint32]holder{}
			if n.Gateway.IsValid() && contains(n.Prefix, n.Gateway) {
				taken[n][u32(n.Gateway)] = holder{what: "шлюз мережі " + n.Name, path: n.Path + ".gateway", gatewayOf: n}
			}
		}
		return taken[n]
	}

	for _, s := range sts {
		if s.device != nil && s.net == nil {
			continue // E004
		}
		if s.net == nil {
			if s.control {
				bad(s.path, fmt.Sprintf("адреса вузла керування %s не належить його мережі %q (%s)",
					s.addr, m.ControlNode.Network.Net.Name, m.ControlNode.Network.Net.Prefix))
			} else {
				bad(s.path, fmt.Sprintf("адреса %s не належить жодній мережі свого сайту", s.addr))
			}
			continue
		}
		n := s.net
		if full[n] {
			continue // E008
		}
		switch {
		case !isHost(n.Prefix, s.addr):
			bad(s.path, fmt.Sprintf("адреса %s є адресою мережі чи широкомовною адресою %s", s.addr, n.Prefix))
			continue
		case inPool(n.Pool, s.addr):
			bad(s.path, fmt.Sprintf("статична адреса %s потрапляє в DHCP-пул %s–%s", s.addr, n.Pool.From, n.Pool.To))
			continue
		}

		held := slot(n)
		first, busy := held[u32(s.addr)]
		if !busy {
			held[u32(s.addr)] = holder{what: s.what, path: s.path, server: s.server, control: s.control}
			continue
		}
		if permitted(first.gatewayOf, first.control, first.server, s, n) {
			// The address now has its one permitted second holder; anyone
			// else arriving at it collides with that holder.
			held[u32(s.addr)] = holder{what: s.what, path: s.path, server: s.server, control: s.control}
			continue
		}
		bad(s.path, fmt.Sprintf("адресу %s уже зайнято: %s%s", s.addr, first.what, line(m, first.path)))
	}

	return out
}

// permitted lists the collisions model v2 allows.
func permitted(gatewayOf *ir.Network, firstControl, firstServer bool, s static, n *ir.Network) bool {
	// The site's L3 gateway is reached on the gateway address of its mgmt
	// network (rb750 at 192.168.10.1 in the examples). Any device in the
	// gateway role qualifies: whether the site has exactly one is E003's
	// question, not this one.
	if gatewayOf != nil && s.device != nil && n.Role == ir.RoleMgmt && s.device.Role.RoutesL3() {
		return true
	}
	// The control node may be one of the servers (ctrl-01 in the examples).
	return (firstControl && s.server) || (firstServer && s.control)
}

// checkE008 reports a network too small for what it holds: host addresses,
// 2^(32-prefix) - 2, must cover the pool plus every distinct gateway and
// static address outside it.
func checkE008(m *ir.Model, _ backend.Platforms) []diag.Diagnostic {
	var out []diag.Diagnostic
	sts := statics(m)
	for _, n := range m.Networks {
		need, have := demand(n, sts), hostCapacity(n.Prefix)
		if need <= have {
			continue
		}
		out = append(out, report(m, diag.E008, diag.Error, n.Path+".cidr", fmt.Sprintf(
			"префікс /%d має %d адрес для хостів, а мережі %q потрібно %d (DHCP-пул: %d, шлюз і статичні адреси: %d)",
			n.Prefix.Bits(), have, n.Name, need, poolSize(n.Pool), need-poolSize(n.Pool))))
	}
	return out
}
