package aristaeos

import (
	"fmt"
	"net/netip"
	"path"
	"sort"
	"strconv"
	"strings"

	"orgc/internal/backend"
	"orgc/internal/ir"
)

// Render produces the .cfg of an EOS device (docs/adr/ADR-009).
//
// A switch gets the VLANs that actually reach it, trunks with the allowed
// VLANs of their links, access ports, an SVI in its mgmt network and a
// default route through the site gateway. A device that is the site's L3
// gateway also routes: SVIs with the gateway addresses, DHCP servers, and
// stateless ACLs built from the rules. What EOS cannot do — stateful
// filtering, NAT — is not invented: a comment names the missing capability
// where the configuration would be, as W002 has already warned.
func (p Platform) Render(m *ir.Model, d *ir.Device) []backend.File {
	c := &config{p: p, m: m, d: d, site: d.Site, gateway: d.Site.Gateway == d}
	c.plan()
	c.header()
	c.spanningTree()
	c.vlans()
	c.interfaces()
	if c.gateway {
		c.routing()
		c.dhcp()
		c.acl()
	} else {
		c.route()
	}
	c.line("end")
	return []backend.File{{
		Path: path.Join("configs", d.Site.Name, d.Name+".cfg"),
		Data: []byte(c.b.String()),
	}}
}

type portKind int

const (
	uplink portKind = iota
	trunk
	access
)

type port struct {
	name    string
	kind    portKind
	nets    []*ir.Network
	standby bool // trunk of a standby link: STP keeps it blocked
	comment string
}

type config struct {
	p       Platform
	m       *ir.Model
	d       *ir.Device
	site    *ir.Site
	gateway bool
	ports   []port
	b       strings.Builder
}

func (c *config) line(s string) { c.b.WriteString(s + "\n") }

// plan decides every port: from the model for a site with links, by
// convention otherwise — Ethernet1 is the trunk to the gateway on a switch
// and the routed uplink on a gateway (ADR-009, decision 3).
func (c *config) plan() {
	seen := map[string]bool{}
	add := func(p port) {
		if !seen[p.name] {
			seen[p.name] = true
			c.ports = append(c.ports, p)
		}
	}
	if c.gateway {
		for _, u := range c.site.Uplinks {
			name := "Ethernet1"
			if u.Port != nil {
				if u.Port.Device != c.d {
					continue
				}
				name = u.Port.Port
			}
			add(port{name: name, kind: uplink, comment: "uplink " + u.Name})
		}
	}
	if len(c.site.Links) > 0 {
		for _, l := range c.site.Links {
			for _, end := range []struct{ here, there *ir.PortRef }{{&l.A, &l.B}, {&l.B, &l.A}} {
				if end.here.Device == c.d {
					comment := "trunk to " + end.there.String()
					if l.Standby {
						comment = "standby trunk to " + end.there.String()
					}
					add(port{name: end.here.Port, kind: trunk, nets: carried(l, c.site), standby: l.Standby, comment: comment})
				}
			}
		}
	} else if c.gateway {
		add(port{name: "Ethernet1", kind: uplink, comment: "uplink (convention)"})
		add(port{name: "Ethernet2", kind: trunk, nets: c.site.Networks, comment: "trunk (convention)"})
	} else {
		add(port{name: "Ethernet1", kind: trunk, nets: c.site.Networks, comment: "trunk to gateway (convention)"})
	}
	for _, ap := range c.d.AccessPorts {
		if ap.Network.Net == nil {
			continue
		}
		for _, name := range ap.Ports {
			add(port{name: name, kind: access, nets: []*ir.Network{ap.Network.Net}, comment: "access " + ap.Network.Name})
		}
	}
	for _, s := range c.site.Servers {
		if s.Port != nil && s.Port.Device == c.d {
			if n := networkOf(c.site, s.IP); n != nil {
				add(port{name: s.Port.Port, kind: access, nets: []*ir.Network{n}, comment: "server " + s.Name})
			}
		}
	}
}

func carried(l *ir.Link, s *ir.Site) []*ir.Network {
	var out []*ir.Network
	for _, n := range s.Networks {
		if l.Carries(n) {
			out = append(out, n)
		}
	}
	return out
}

func networkOf(s *ir.Site, a netip.Addr) *ir.Network {
	for _, n := range s.Networks {
		if n.Prefix.Masked().Contains(a) {
			return n
		}
	}
	return nil
}

func (c *config) header() {
	c.line("! Згенеровано orgc з " + path.Base(strings.ReplaceAll(c.m.File, `\`, "/")) + ". Не редагувати вручну.")
	c.line(fmt.Sprintf("! Пристрій %s (%s), сайт %s, EOS %s.", c.d.Name, c.d.Role, c.site.Name, c.d.OSVersion))
	c.line("! Обліковий запис для Ansible (ansible-net) і його секрет задаються поза orgc.")
	c.line("!")
	c.line("hostname " + c.d.Name)
	c.line("!")
}

// present lists the site's networks that reach the device: those on any of
// its ports, plus its mgmt network and, on the gateway, every network it
// routes. Order is the site's.
func (c *config) present() []*ir.Network {
	want := map[*ir.Network]bool{}
	for _, p := range c.ports {
		for _, n := range p.nets {
			want[n] = true
		}
	}
	var out []*ir.Network
	for _, n := range c.site.Networks {
		if want[n] || n.Prefix.Masked().Contains(c.d.MgmtIP) || (c.gateway && n.Gateway.IsValid()) {
			out = append(out, n)
		}
	}
	return out
}

// RSTP on every switch (docs/adr/ADR-009, amendment 2026-10-14): the
// site's gateway is the root, and a standby link's ports carry the highest
// cost, so that once STP converges the active topology is the tree verify
// judged. One instance for all VLANs, in place of the MSTP EOS starts with.
const (
	rootPriority = 4096
	standbyCost  = 200000000
)

func (c *config) spanningTree() {
	c.line("spanning-tree mode rstp")
	if c.gateway {
		c.line("spanning-tree priority " + strconv.Itoa(rootPriority))
	}
	c.line("!")
}

func (c *config) vlans() {
	for _, n := range c.present() {
		c.line("vlan " + strconv.Itoa(n.VLAN))
		c.line("   name " + n.Name)
	}
	c.line("!")
	c.isolation()
}

// isolated lists the isolated networks with access ports on this device.
func (c *config) isolated() []*ir.Network {
	var out []*ir.Network
	seen := map[*ir.Network]bool{}
	for _, p := range c.ports {
		if p.kind == access && p.nets[0].Isolated && !seen[p.nets[0]] {
			seen[p.nets[0]] = true
			out = append(out, p.nets[0])
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].VLAN < out[j].VLAN })
	return out
}

// isolation keeps hosts of an isolated network apart with an inbound ACL on
// its access ports: traffic to the gateway, DHCP and everything outside the
// network passes, traffic to other hosts of the network does not. A port ACL
// filters switched traffic too, and it needs no secondary VLAN — which the
// model does not have, and which a private VLAN would require on every trunk
// up to the gateway.
func (c *config) isolation() {
	for _, n := range c.isolated() {
		c.line("ip access-list orgc-isolate-" + n.Name)
		seq := 10
		if n.Gateway.IsValid() {
			c.line(fmt.Sprintf("   %d permit ip any host %s", seq, n.Gateway))
			seq += 10
		}
		c.line(fmt.Sprintf("   %d permit udp any any eq bootps", seq))
		c.line(fmt.Sprintf("   %d deny ip any %s", seq+10, n.Prefix.Masked()))
		c.line(fmt.Sprintf("   %d permit ip any any", seq+20))
		c.line("!")
	}
}

func vlanList(nets []*ir.Network) string {
	var ids []int
	for _, n := range nets {
		ids = append(ids, n.VLAN)
	}
	sort.Ints(ids)
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = strconv.Itoa(id)
	}
	return strings.Join(parts, ",")
}

func (c *config) interfaces() {
	for _, p := range c.ports {
		c.line("interface " + p.name)
		c.line("   description " + p.comment)
		switch p.kind {
		case uplink:
			c.line("   no switchport")
			c.line("   ip address dhcp")
		case trunk:
			c.line("   switchport mode trunk")
			c.line("   switchport trunk allowed vlan " + vlanList(p.nets))
			if p.standby {
				c.line("   spanning-tree cost " + strconv.Itoa(standbyCost))
			}
		case access:
			n := p.nets[0]
			c.line("   switchport access vlan " + strconv.Itoa(n.VLAN))
			if n.Isolated {
				c.line("   ip access-group orgc-isolate-" + n.Name + " in")
			}
		}
		c.line("!")
	}
	for _, n := range c.present() {
		var addrs []netip.Addr
		if c.gateway && n.Gateway.IsValid() {
			addrs = append(addrs, n.Gateway)
		}
		if n.Prefix.Masked().Contains(c.d.MgmtIP) && (len(addrs) == 0 || addrs[0] != c.d.MgmtIP) {
			addrs = append(addrs, c.d.MgmtIP)
		}
		if len(addrs) == 0 {
			continue
		}
		c.line("interface Vlan" + strconv.Itoa(n.VLAN))
		c.line("   description " + n.Name)
		for i, a := range addrs {
			suffix := ""
			if i > 0 {
				suffix = " secondary"
			}
			c.line(fmt.Sprintf("   ip address %s/%d%s", a, n.Prefix.Bits(), suffix))
		}
		if c.gateway {
			if n.Pool != nil {
				c.line("   dhcp server ipv4")
			}
			c.line("   ip access-group orgc-in in")
		}
		c.line("!")
	}
}

func (c *config) route() {
	for _, n := range c.site.Networks {
		if n.Prefix.Masked().Contains(c.d.MgmtIP) && n.Gateway.IsValid() {
			c.line("ip route 0.0.0.0/0 " + n.Gateway.String())
			c.line("!")
			return
		}
	}
}

func (c *config) routing() {
	c.line("ip routing")
	c.line("!")
}

func (c *config) dhcp() {
	var pooled []*ir.Network
	for _, n := range c.site.Networks {
		if n.Pool != nil && n.Gateway.IsValid() {
			pooled = append(pooled, n)
		}
	}
	if len(pooled) == 0 {
		return
	}
	c.line("dhcp server")
	for _, n := range pooled {
		c.line("   subnet " + n.Prefix.Masked().String())
		c.line(fmt.Sprintf("      range %s %s", n.Pool.From, n.Pool.To))
		c.line("      default-gateway " + n.Gateway.String())
	}
	c.line("!")
}

// acl builds the inbound ACL of the gateway's SVIs from the rules touching
// the site. EOS ACLs are stateless: return traffic of TCP sessions passes
// on "established", other return traffic does not, and that is said in the
// configuration rather than hidden.
func (c *config) acl() {
	touches := func(e ir.Endpoint) bool {
		return e.Kind == ir.EndpointAny || (e.Kind == ir.EndpointNetwork && e.Net != nil && e.Net.Site == c.site)
	}
	var rules []*ir.Rule
	internet := false
	for _, r := range c.m.Rules {
		if !r.From.Resolved() || !r.To.Resolved() {
			continue
		}
		if r.To.Kind == ir.EndpointInternet {
			if touches(r.From) {
				internet = true
			}
			continue
		}
		if touches(r.From) || touches(r.To) {
			rules = append(rules, r)
		}
	}
	sort.SliceStable(rules, func(i, j int) bool { return key(rules[i]) < key(rules[j]) })

	c.line("! W002: фільтрація зі станом недоступна — ACL без стану; зворотний трафік TCP пропускає established, інших протоколів — ні.")
	if internet {
		c.line("! W002: трансляція адрес (NAT) недоступна — правила виходу в internet не згенеровано.")
	}
	c.line("ip access-list orgc-in")
	seq := 10
	add := func(s string) {
		c.line(fmt.Sprintf("   %d %s", seq, s))
		seq += 10
	}
	add("permit tcp any any established")
	if nc := c.m.ControlNode.Network.Net; nc != nil {
		// The management port stays open to the control node network, as on
		// RouterOS: E006 may hold without a rule (N_c = N_m), and the
		// configuration must agree with it.
		var mp []string
		for _, pp := range c.p.ManagementPorts(c.d.OSVersion) {
			mp = append(mp, strconv.Itoa(pp.Port))
		}
		if len(mp) > 0 {
			add(fmt.Sprintf("permit tcp %s host %s eq %s", nc.Prefix.Masked(), c.d.MgmtIP, strings.Join(mp, " ")))
		}
	}
	for _, r := range rules {
		for _, spec := range specs(r.Ports) {
			add(fmt.Sprintf("permit %s %s %s%s", spec.proto, addr(r.From), addr(r.To), spec.ports))
		}
	}
	for _, n := range c.site.Networks {
		if n.Pool != nil && n.Gateway.IsValid() {
			add("permit udp any any eq bootps")
			break
		}
	}
	add("deny ip any any")
	c.line("!")
}

func key(r *ir.Rule) string {
	return r.From.Name + "\x00" + r.To.Name
}

func addr(e ir.Endpoint) string {
	if e.Kind == ir.EndpointAny {
		return "any"
	}
	return e.Net.Prefix.Masked().String()
}

type spec struct{ proto, ports string }

func specs(s ir.PortSet) []spec {
	if s.Any {
		return []spec{{"ip", ""}}
	}
	nums := make([]string, len(s.Ports))
	for i, p := range s.Ports {
		nums[i] = strconv.Itoa(p.Port)
	}
	return []spec{{string(s.Ports[0].Proto), " eq " + strings.Join(nums, " ")}}
}
