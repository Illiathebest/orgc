// Package containerlab generates the topology of the emulated lab:
// containerlab/lab.clab.yml (docs/adr/ADR-009, decision 6 and its
// amendment; docs/adr/ADR-010).
//
// Every device becomes a node of the kind and image its platform names,
// with its generated configuration as the startup config. Links come from
// the model: the links of a site that describes them, otherwise the star
// convention — each switch's first port to the next trunk port of the
// gateway. A server plugged into a port becomes a Linux node with its
// address and default route from the model, so the control node has a
// stand-in inside the emulation. Uplinks are not links of the topology:
// containerlab's management network already joins every node and the
// outside, which is what the model assumes between sites, and vrnetlab
// RouterOS keeps ether1 for that network.
//
// The management network is fixed, and every node gets its address in it by
// position — devices, then servers, in model order — so the inventory of the
// emulation is known before deployment.
package containerlab

import (
	"fmt"
	"net/netip"
	"path"
	"strings"

	"orgc/internal/backend"
	"orgc/internal/ir"
)

// Generator renders the topology.
type Generator struct{}

var _ backend.Generator = Generator{}

// Name identifies the generator.
func (Generator) Name() string { return "containerlab" }

// File is where the topology lands in the output tree. The startup configs
// it points to are siblings under configs/.
const File = "containerlab/lab.clab.yml"

// The management network of the lab (docs/adr/ADR-010): containerlab's
// default 172.20.20.0/24 overlaps the address pools of Docker on the hosts
// the network is routed to. The bridge takes .1; nodes start at firstHost.
const (
	mgmtNetwork = "orgc-mgmt"
	mgmtSubnet  = "10.99.1.0/24"
	firstHost   = 11
)

// serverImage is the image of the Linux node that stands in for a server.
const serverImage = "alpine:3.22"

// conventionTrunks are the gateway ports the star convention hands out to
// switches, per platform family, in order (ADR-009, decision 3).
var conventionTrunks = map[string][]string{
	"mikrotik_ros": {"ether2", "ether3", "ether4"},
	"arista_ceos":  {"Ethernet2", "Ethernet3", "Ethernet4", "Ethernet5", "Ethernet6", "Ethernet7", "Ethernet8"},
}

type endpoint struct {
	device *ir.Device
	port   string
}

// Generate renders one topology for the whole model.
func (Generator) Generate(m *ir.Model, ps backend.Platforms) []backend.File {
	node := func(d *ir.Device) backend.LabNode {
		p, ok := ps.Lookup(d.Type)
		if !ok {
			return nil
		}
		n, _ := p.(backend.LabNode)
		return n
	}

	var skipped []string
	prefix := netip.MustParsePrefix(mgmtSubnet)
	next := prefix.Addr()
	for i := 0; i < firstHost; i++ {
		next = next.Next()
	}
	// mgmt hands out the next address, or none once the subnet is used up:
	// containerlab then picks one itself.
	mgmt := func(name string) string {
		a := next
		next = next.Next()
		if !prefix.Contains(next) { // a is the broadcast address or beyond
			skipped = append(skipped, "фіксована адреса керування "+name)
			return ""
		}
		return "      mgmt-ipv4: " + a.String() + "\n"
	}

	var b strings.Builder
	b.WriteString("# Згенеровано orgc з " + path.Base(strings.ReplaceAll(m.File, `\`, "/")) + ". Не редагувати вручну.\n")
	b.WriteString("---\nname: lab\nmgmt:\n  network: " + mgmtNetwork + "\n  ipv4-subnet: " + mgmtSubnet + "\ntopology:\n  nodes:\n")
	for _, d := range m.Devices {
		n := node(d)
		if n == nil {
			continue
		}
		ext := ".rsc"
		if n.LabKind() == "arista_ceos" {
			ext = ".cfg"
		}
		b.WriteString(fmt.Sprintf("    %s:\n      kind: %s\n      image: %s\n%s      startup-config: ../configs/%s/%s%s\n",
			d.Name, n.LabKind(), n.LabImage(d.OSVersion), mgmt(d.Name), d.Site.Name, d.Name, ext))
	}

	// A server is a node when its port is on a node; it reaches its network
	// through eth1, its first data interface.
	var hosts []*ir.Server
	for _, sv := range m.Servers {
		if !sv.Port.Resolved() || node(sv.Port.Device) == nil {
			continue
		}
		n := networkOf(sv.Site, sv.IP)
		if n == nil {
			continue
		}
		hosts = append(hosts, sv)
		b.WriteString(fmt.Sprintf("    %s:\n      kind: linux\n      image: %s\n%s      exec:\n        - ip address add %s/%d dev eth1\n",
			sv.Name, serverImage, mgmt(sv.Name), sv.IP, n.Prefix.Bits()))
		if n.Gateway.IsValid() {
			b.WriteString(fmt.Sprintf("        - ip route replace default via %s dev eth1\n", n.Gateway))
		}
	}

	var links [][2]endpoint
	// standby marks the links STP keeps blocked: the ones to leave up when
	// a failover is tested.
	standby := map[[2]endpoint]bool{}
	for _, s := range m.Sites {
		if len(s.Links) > 0 {
			for _, l := range s.Links {
				if l.Resolved() {
					e := [2]endpoint{{l.A.Device, l.A.Port}, {l.B.Device, l.B.Port}}
					links = append(links, e)
					standby[e] = l.Standby
				}
			}
			continue
		}
		gw := s.Gateway
		if gw == nil || node(gw) == nil {
			continue
		}
		free := conventionTrunks[node(gw).LabKind()]
		for _, d := range s.Devices {
			if d == gw || node(d) == nil {
				continue
			}
			if len(free) == 0 {
				skipped = append(skipped, d.Name)
				continue
			}
			first := "ether1"
			if node(d).LabKind() == "arista_ceos" {
				first = "Ethernet1"
			}
			links = append(links, [2]endpoint{{gw, free[0]}, {d, first}})
			free = free[1:]
		}
	}

	var lines []string
	for _, l := range links {
		a, okA := node(l[0].device).LabInterface(l[0].port)
		z, okZ := node(l[1].device).LabInterface(l[1].port)
		if !okA || !okZ {
			skipped = append(skipped, l[0].device.Name+":"+l[0].port+" – "+l[1].device.Name+":"+l[1].port)
			continue
		}
		mark := ""
		if standby[l] {
			mark = " # резервне"
		}
		lines = append(lines, fmt.Sprintf("    - endpoints: [\"%s:%s\", \"%s:%s\"]%s\n", l[0].device.Name, a, l[1].device.Name, z, mark))
	}
	for _, sv := range hosts {
		p := sv.Port
		ifc, ok := node(p.Device).LabInterface(p.Port)
		if !ok {
			skipped = append(skipped, p.String()+" – "+sv.Name)
			continue
		}
		lines = append(lines, fmt.Sprintf("    - endpoints: [\"%s:%s\", \"%s:eth1\"]\n", p.Device.Name, ifc, sv.Name))
	}

	// An access or uplink port with nothing plugged in gets a dummy link:
	// cEOS and vrnetlab create a port only for an interface the container
	// has. A missing access port cannot carry its VLAN for the check, and a
	// RouterOS script naming a missing uplink port stops at that line.
	plugged := map[endpoint]bool{}
	for _, l := range links {
		plugged[l[0]], plugged[l[1]] = true, true
	}
	for _, sv := range hosts {
		plugged[endpoint{sv.Port.Device, sv.Port.Port}] = true
	}
	dummy := func(d *ir.Device, port string) {
		e := endpoint{d, port}
		if plugged[e] || node(d) == nil {
			return
		}
		plugged[e] = true
		if ifc, ok := node(d).LabInterface(port); ok {
			lines = append(lines, fmt.Sprintf("    - type: dummy\n      endpoint: { node: %s, interface: %s }\n", d.Name, ifc))
		}
	}
	for _, d := range m.Devices {
		for _, ap := range d.AccessPorts {
			for _, p := range ap.Ports {
				dummy(d, p)
			}
		}
	}
	for _, sv := range m.Servers {
		if sv.Port.Resolved() {
			dummy(sv.Port.Device, sv.Port.Port)
		}
	}
	for _, s := range m.Sites {
		for _, u := range s.Uplinks {
			if u.Port.Resolved() {
				dummy(u.Port.Device, u.Port.Port)
			}
		}
	}
	if len(lines) > 0 {
		b.WriteString("  links:\n")
		for _, s := range lines {
			b.WriteString(s)
		}
	}
	for _, s := range skipped {
		b.WriteString("# не відтворено в емуляції: " + s + "\n")
	}
	return []backend.File{{Path: File, Data: []byte(b.String())}}
}

// networkOf finds the network of the site holding an address.
func networkOf(s *ir.Site, a netip.Addr) *ir.Network {
	for _, n := range s.Networks {
		if n.Prefix.Masked().Contains(a) {
			return n
		}
	}
	return nil
}
