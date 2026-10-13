package verify

import (
	"fmt"
	"strconv"
	"strings"

	"orgc/internal/backend"
	"orgc/internal/diag"
	"orgc/internal/ir"
)

// checkE006 verifies management reachability (docs/adr/ADR-008, decision 6).
// For every device d with mgmt network N_m, given the control node network
// N_c and the management ports P(d) its platform declares, the flow
// N_c → N_m on a port of P(d) must be
//
//  1. permitted: N_c = N_m, or a rule with from ∈ {N_c, any}, to ∈ {N_m,
//     any} admits a port of P(d);
//  2. routed: N_m is reachable from N_c in the topology graph;
//  3. carried: in a site that describes its links, every link on the path
//     of the site's L2 tree from the gateway down to d allows N_m.
//
// The graph has networks and site L3 gateways as vertices. A network with a
// gateway address is attached to the gateway of its site; a network without
// one has no L3 interface and stays isolated. Gateways of all sites are
// connected to each other: model v2 does not describe inter-site links and
// assumes a full mesh. Rules filter the flow on every gateway of the path;
// they never create an edge, because they are not transitive.
//
// One breadth-first search from N_c gives the reachable networks; each
// device then costs one scan of the rules: O(|V| + |E| + |D|·|R|).
//
// Skipped, because another code reports it: an unresolved control node
// network (E003), a device without a mgmt network of its site (E004), an
// OS version the platform rejects (W002), a path that would cross a site
// whose gateway is missing or ambiguous while its networks have gateway
// addresses (E003), and a missing rule when a rule with an unresolved end
// could have been it (E003).
func checkE006(m *ir.Model, ps backend.Platforms) []diag.Diagnostic {
	nc := m.ControlNode.Network.Net
	if nc == nil {
		return nil
	}
	reach := reachable(m, nc)
	controlNodeCut := controlNodeNotDelivered(m)

	// A rule names a network by its name; with two networks of one name
	// (E002) it is not known which one the rule meant, so whether it admits
	// management is not judged.
	named := map[string]int{}
	for _, n := range m.Networks {
		named[n.Name]++
	}
	if named[nc.Name] > 1 {
		return nil
	}

	var out []diag.Diagnostic
	for _, d := range m.Devices {
		nm := mgmtNetworkOf(d)
		if nm == nil {
			continue
		}
		p, ok := ps.Lookup(d.Type)
		if !ok {
			continue
		}
		if ok, _ := p.CheckVersion(d.OSVersion); !ok {
			continue
		}
		if nm == nc || named[nm.Name] > 1 {
			continue
		}
		ports := p.ManagementPorts(d.OSVersion)

		if !reach[nm] {
			if gatewayAmbiguous(nc.Site) || gatewayAmbiguous(nm.Site) {
				continue
			}
			out = append(out, report(m, diag.E006, diag.Error, d.Path+".name", fmt.Sprintf(
				"пристрій %q недосяжний для керування: немає L3-шляху від мережі вузла керування %q до mgmt-мережі %q (%s)",
				d.Name, nc.Name, nm.Name, whyUnrouted(nc, nm))))
			continue
		}

		if t := d.Site.L2; t != nil {
			if !whole(d.Site) || !t.Reached(d) || controlNodeCut {
				continue // E003, E010 or E009
			}
			if b := t.Blocking(d, nm); b != nil {
				out = append(out, report(m, diag.E006, diag.Error, b.Path+".vlans", fmt.Sprintf(
					"пристрій %q недосяжний для керування: зʼєднання %s – %s не пропускає mgmt-мережу %q",
					d.Name, b.A.String(), b.B.String(), nm.Name)))
				continue
			}
		}

		var near *ir.Rule
		admitted := false
		for _, r := range m.Rules {
			if !resolved(r) || !endIs(r.From, nc) || !endIs(r.To, nm) {
				continue
			}
			if near == nil {
				near = r
			}
			for _, port := range ports {
				if r.Ports.Admits(port) {
					admitted = true
				}
			}
		}
		switch {
		case admitted:
		case near == nil && couldBeTheRule(m, nc, nm):
			// An unresolved rule might be the one meant to open management;
			// E003 reports it, and the fix of that typo may well fix this.
		case near == nil:
			out = append(out, report(m, diag.E006, diag.Error, d.Path+".name", fmt.Sprintf(
				"пристрій %q недосяжний для керування: жодне правило не відкриває мережі %q доступ до mgmt-мережі %q; потрібен %s",
				d.Name, nc.Name, nm.Name, portList(ports))))
		default:
			out = append(out, report(m, diag.E006, diag.Error, near.Path, fmt.Sprintf(
				"правило відкриває %s, а керування пристроєм %q (%s) іде через %s; після застосування конфігурації засіб розгортання втратить доступ",
				portSetText(near.Ports), d.Name, d.Type, portList(ports))))
		}
	}
	return out
}

// controlNodeNotDelivered tells whether the control node is a server
// plugged into a port its network does not reach — E009 reports that, and
// management of the whole site is moot until it is fixed.
func controlNodeNotDelivered(m *ir.Model) bool {
	cn := m.ControlNode
	for _, sv := range m.Servers {
		if sv.IP != cn.IP || sv.Port == nil || !sv.Port.Resolved() || cn.Network.Net == nil {
			continue
		}
		t := sv.Site.L2
		if t == nil || !whole(sv.Site) {
			return false
		}
		return !t.Carried(sv.Port.Device, cn.Network.Net)
	}
	return false
}

// reachable runs a breadth-first search over the topology graph from the
// control node network and returns the networks it reaches.
func reachable(m *ir.Model, from *ir.Network) map[*ir.Network]bool {
	type vertex struct {
		net *ir.Network
		gw  *ir.Site // the L3 gateway of this site
	}
	adj := map[vertex][]vertex{}
	var gateways []vertex
	for _, s := range m.Sites {
		if s.Gateway == nil {
			continue
		}
		g := vertex{gw: s}
		gateways = append(gateways, g)
		for _, n := range s.Networks {
			if n.Gateway.IsValid() {
				v := vertex{net: n}
				adj[v] = append(adj[v], g)
				adj[g] = append(adj[g], v)
			}
		}
	}
	for _, a := range gateways {
		for _, b := range gateways {
			if a != b {
				adj[a] = append(adj[a], b)
			}
		}
	}

	seen := map[vertex]bool{{net: from}: true}
	queue := []vertex{{net: from}}
	out := map[*ir.Network]bool{from: true}
	for len(queue) > 0 {
		v := queue[0]
		queue = queue[1:]
		for _, w := range adj[v] {
			if seen[w] {
				continue
			}
			seen[w] = true
			if w.net != nil {
				out[w.net] = true
			}
			queue = append(queue, w)
		}
	}
	return out
}

// gatewayAmbiguous tells whether E003 already reports the site's gateway:
// some network needs one and the site has none or several.
func gatewayAmbiguous(s *ir.Site) bool {
	return firstRouted(s) != nil && len(s.GatewayCandidates) != 1
}

// whyUnrouted names the end of the flow that has no L3 interface.
func whyUnrouted(nc, nm *ir.Network) string {
	var ends []string
	for _, n := range []*ir.Network{nc, nm} {
		if !n.Gateway.IsValid() {
			ends = append(ends, fmt.Sprintf("мережа %q не має gateway і не маршрутизується", n.Name))
		} else if n.Site.Gateway == nil {
			ends = append(ends, fmt.Sprintf("у сайті %q немає L3-шлюзу", n.Site.Name))
		}
	}
	return strings.Join(ends, "; ")
}

// couldBeTheRule tells whether some rule with an unresolved end would cover
// the flow nc → nm if that end were resolved.
func couldBeTheRule(m *ir.Model, nc, nm *ir.Network) bool {
	for _, r := range m.Rules {
		if resolved(r) {
			continue
		}
		fromOK := !r.From.Resolved() || endIs(r.From, nc)
		toOK := !r.To.Resolved() || endIs(r.To, nm)
		if fromOK && toOK {
			return true
		}
	}
	return false
}

// endIs tells whether a resolved endpoint covers the network.
func endIs(e ir.Endpoint, n *ir.Network) bool {
	return e.Kind == ir.EndpointAny || (e.Kind == ir.EndpointNetwork && e.Net == n)
}

func portList(ps []ir.PortSpec) string {
	parts := make([]string, len(ps))
	for i, p := range ps {
		parts[i] = string(p.Proto) + "/" + strconv.Itoa(p.Port)
	}
	return strings.Join(parts, " або ")
}

func portSetText(s ir.PortSet) string {
	if s.Any {
		return "будь-який трафік"
	}
	return portList(s.Ports)
}
