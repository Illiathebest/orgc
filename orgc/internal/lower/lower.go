// Package lower turns a parsed model into the intermediate representation:
// it resolves references, parses addresses and finds the L3 gateway of each
// site (docs/adr/ADR-008, decisions 1 and 2).
//
// Lowering never rejects a model. A reference that does not resolve stays in
// the IR with its name and a nil target, and E003 reports it; the other
// invariants still run, so one pass shows the administrator every defect.
package lower

import (
	"fmt"
	"net/netip"
	"sort"
	"strconv"
	"strings"

	"orgc/internal/ir"
	"orgc/internal/model"
)

// Reserved segment references. The schema forbids them as network names, so
// they cannot collide with a real network.
const (
	refAny      = "any"
	refInternet = "internet"
)

// Lower builds the IR of a document that passed the schema.
//
// The error is never about the model. It means the schema accepted an
// address that net/netip cannot parse — the schema and the IR disagree,
// which is a defect of orgc and must not be papered over: a zero address
// would make every address invariant skip the element in silence.
func Lower(doc *model.Document) (*ir.Model, error) {
	l := &lowerer{
		m: &ir.Model{
			File:         doc.File,
			Organization: doc.Org.Organization,
			Profile:      ir.Profile(doc.Org.Profile),
			Positions:    doc.Positions,
		},
		sites:    map[string]*ir.Site{},
		networks: map[string]*ir.Network{},
	}

	for i := range doc.Org.Sites {
		l.site(&doc.Org.Sites[i], "sites["+strconv.Itoa(i)+"]")
	}
	l.controlNode(&doc.Org.ControlNode)
	if doc.Org.Policies != nil {
		for i := range doc.Org.Policies.InterVLAN {
			l.rule(&doc.Org.Policies.InterVLAN[i], "policies.inter_vlan["+strconv.Itoa(i)+"]")
		}
	}

	if l.err != nil {
		return nil, l.err
	}
	return l.m, nil
}

type lowerer struct {
	m *ir.Model

	// Lookup tables for reference resolution. Duplicate names are E002's
	// concern; until it reports them, a reference binds to the first
	// definition in the file, which is deterministic and what a reader
	// scanning top-down would assume.
	sites    map[string]*ir.Site
	networks map[string]*ir.Network

	err error // first address that failed to parse
}

func (l *lowerer) site(s *model.Site, path string) {
	site := &ir.Site{
		Name:         s.Name,
		AllowOverlap: s.AllowOverlap,
		Origin:       ir.Origin{Path: path},
	}
	l.m.Sites = append(l.m.Sites, site)
	if _, dup := l.sites[s.Name]; !dup {
		l.sites[s.Name] = site
	}

	for i, u := range s.Uplinks {
		site.Uplinks = append(site.Uplinks, &ir.Uplink{
			Name:   u.Name,
			Kind:   ir.UplinkKind(u.Kind),
			Site:   site,
			Origin: ir.Origin{Path: path + ".uplinks[" + strconv.Itoa(i) + "]"},
		})
	}

	for i := range s.Networks {
		n := l.network(&s.Networks[i], path+".networks["+strconv.Itoa(i)+"]", site)
		site.Networks = append(site.Networks, n)
		l.m.Networks = append(l.m.Networks, n)
	}

	for i, d := range s.Devices {
		p := path + ".devices[" + strconv.Itoa(i) + "]"
		dev := &ir.Device{
			Name:      d.Name,
			Type:      ir.DeviceType(d.Type),
			OSVersion: d.OSVersion,
			Role:      ir.DeviceRole(d.Role),
			MgmtIP:    l.addr(d.MgmtIP, p+".mgmt_ip"),
			Site:      site,
			Origin:    ir.Origin{Path: p},
		}
		site.Devices = append(site.Devices, dev)
		l.m.Devices = append(l.m.Devices, dev)
		if dev.Role.RoutesL3() {
			site.GatewayCandidates = append(site.GatewayCandidates, dev)
		}
	}
	if len(site.GatewayCandidates) == 1 {
		site.Gateway = site.GatewayCandidates[0]
	}

	for i, sv := range s.Servers {
		p := path + ".servers[" + strconv.Itoa(i) + "]"
		srv := &ir.Server{
			Name:   sv.Name,
			IP:     l.addr(sv.IP, p+".ip"),
			Site:   site,
			Origin: ir.Origin{Path: p},
		}
		for _, r := range sv.Role {
			srv.Roles = append(srv.Roles, ir.ServerRole(r))
		}
		site.Servers = append(site.Servers, srv)
		l.m.Servers = append(l.m.Servers, srv)
	}

	l.connections(s, path, site)
}

// connections resolves everything that names a port: uplink ports, server
// ports, access ports and links. Ports are looked up among the devices of
// the same site only — the model does not describe links between sites —
// so this runs once all of them are known. It ends by building the site's
// L2 tree when the site describes its links.
func (l *lowerer) connections(s *model.Site, path string, site *ir.Site) {
	devices := map[string]*ir.Device{}
	for _, d := range site.Devices {
		if _, dup := devices[d.Name]; !dup {
			devices[d.Name] = d
		}
	}
	networks := map[string]*ir.Network{}
	for _, n := range site.Networks {
		if _, dup := networks[n.Name]; !dup {
			networks[n.Name] = n
		}
	}
	port := func(ref, p string) *ir.PortRef {
		name, iface, _ := strings.Cut(ref, ":")
		return &ir.PortRef{DeviceName: name, Port: iface, Device: devices[name], Origin: ir.Origin{Path: p}}
	}

	for i, u := range s.Uplinks {
		if u.Port != "" {
			site.Uplinks[i].Port = port(u.Port, site.Uplinks[i].Path+".port")
		}
	}
	for i, sv := range s.Servers {
		if sv.Port != "" {
			site.Servers[i].Port = port(sv.Port, site.Servers[i].Path+".port")
		}
	}
	for i, d := range s.Devices {
		dev := site.Devices[i]
		for k, ap := range d.AccessPorts {
			dev.AccessPorts = append(dev.AccessPorts, &ir.AccessPorts{
				Ports:   ap.Ports,
				Network: ir.NetRef{Name: ap.Network, Net: networks[ap.Network]},
				Device:  dev,
				Origin:  ir.Origin{Path: dev.Path + ".access_ports[" + strconv.Itoa(k) + "]"},
			})
		}
	}

	for i, lk := range s.Links {
		p := path + ".links[" + strconv.Itoa(i) + "]"
		link := &ir.Link{
			A:      *port(lk.A, p+".a"),
			B:      *port(lk.B, p+".b"),
			Site:   site,
			Origin: ir.Origin{Path: p},
		}
		if lk.VLANs != nil {
			link.VLANs = make([]ir.NetRef, len(lk.VLANs))
			for k, name := range lk.VLANs {
				link.VLANs[k] = ir.NetRef{Name: name, Net: networks[name]}
			}
		}
		site.Links = append(site.Links, link)
	}
	if len(site.Links) > 0 {
		site.L2 = ir.BuildL2(site)
	}
}

func (l *lowerer) network(n *model.Network, path string, site *ir.Site) *ir.Network {
	net := &ir.Network{
		Name:     n.Name,
		VLAN:     n.VLAN,
		Role:     ir.Role(n.Role),
		Prefix:   l.prefix(n.CIDR, path+".cidr"),
		Isolated: n.Isolated,
		Site:     site,
		Origin:   ir.Origin{Path: path},
	}
	if n.Gateway != "" {
		net.Gateway = l.addr(n.Gateway, path+".gateway")
	}
	if n.DHCPPool != nil {
		net.Pool = &ir.Range{
			From: l.addr(n.DHCPPool.From, path+".dhcp_pool.from"),
			To:   l.addr(n.DHCPPool.To, path+".dhcp_pool.to"),
		}
	}
	if _, dup := l.networks[n.Name]; !dup {
		l.networks[n.Name] = net
	}
	return net
}

func (l *lowerer) controlNode(c *model.ControlNode) {
	const path = "control_node"
	cn := ir.ControlNode{
		Site:    ir.SiteRef{Name: c.Site, Site: l.sites[c.Site]},
		Network: ir.NetRef{Name: c.Network},
		IP:      l.addr(c.IP, path+".ip"),
		Origin:  ir.Origin{Path: path},
	}
	// The network must belong to the named site; a network of the same
	// name elsewhere does not count, and E003 tells the two cases apart.
	if s := cn.Site.Site; s != nil {
		for _, n := range s.Networks {
			if n.Name == c.Network {
				cn.Network.Net = n
				break
			}
		}
	}
	l.m.ControlNode = cn
}

func (l *lowerer) rule(r *model.Rule, path string) {
	l.m.Rules = append(l.m.Rules, &ir.Rule{
		From:   l.endpoint(r.From),
		To:     l.endpoint(r.To),
		Ports:  portSet(r.Ports, r.Proto),
		Origin: ir.Origin{Path: path},
	})
}

func (l *lowerer) endpoint(ref string) ir.Endpoint {
	switch ref {
	case refAny:
		return ir.Endpoint{Kind: ir.EndpointAny, Name: ref}
	case refInternet:
		return ir.Endpoint{Kind: ir.EndpointInternet, Name: ref}
	default:
		return ir.Endpoint{Kind: ir.EndpointNetwork, Name: ref, Net: l.networks[ref]}
	}
}

// portSet normalises the ports of a rule. No ports means any traffic at all;
// a protocol defaults to TCP, as the schema documents.
func portSet(ports []int, proto string) ir.PortSet {
	if len(ports) == 0 {
		return ir.PortSet{Any: true}
	}
	p := ir.TCP
	if proto != "" {
		p = ir.Proto(proto)
	}
	set := ir.PortSet{Ports: make([]ir.PortSpec, 0, len(ports))}
	for _, n := range ports {
		set.Ports = append(set.Ports, ir.PortSpec{Proto: p, Port: n})
	}
	sort.Slice(set.Ports, func(i, j int) bool {
		a, b := set.Ports[i], set.Ports[j]
		if a.Proto != b.Proto {
			return a.Proto < b.Proto
		}
		return a.Port < b.Port
	})
	return set
}

func (l *lowerer) addr(s, path string) netip.Addr {
	a, err := netip.ParseAddr(s)
	if err != nil {
		l.fail(path, s, err)
	}
	return a
}

func (l *lowerer) prefix(s, path string) netip.Prefix {
	p, err := netip.ParsePrefix(s)
	if err != nil {
		l.fail(path, s, err)
	}
	return p
}

func (l *lowerer) fail(path, value string, err error) {
	if l.err == nil {
		l.err = fmt.Errorf("модель пройшла схему, але %s = %q не розбирається: %w", path, value, err)
	}
}
