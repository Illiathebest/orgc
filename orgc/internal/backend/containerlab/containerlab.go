// Package containerlab generates the topology of the emulated lab:
// containerlab/lab.clab.yml (docs/adr/ADR-009, decision 6 and its
// amendment).
//
// Every device becomes a node of the kind and image its platform names,
// with its generated configuration as the startup config. Links come from
// the model: the links of a site that describes them, otherwise the star
// convention — each switch's first port to the next trunk port of the
// gateway. Servers are VMs, not nodes. Uplinks are not links of the
// topology: containerlab's management network already joins every node and
// the outside, which is what the model assumes between sites, and vrnetlab
// RouterOS keeps ether1 for that network.
package containerlab

import (
	"fmt"
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

	var b strings.Builder
	b.WriteString("# Згенеровано orgc з " + path.Base(strings.ReplaceAll(m.File, `\`, "/")) + ". Не редагувати вручну.\n")
	b.WriteString("---\nname: lab\ntopology:\n  nodes:\n")
	for _, d := range m.Devices {
		n := node(d)
		if n == nil {
			continue
		}
		ext := ".rsc"
		if n.LabKind() == "arista_ceos" {
			ext = ".cfg"
		}
		b.WriteString(fmt.Sprintf("    %s:\n      kind: %s\n      image: %s\n      startup-config: ../configs/%s/%s%s\n",
			d.Name, n.LabKind(), n.LabImage(d.OSVersion), d.Site.Name, d.Name, ext))
	}

	var links [][2]endpoint
	var skipped []string
	for _, s := range m.Sites {
		if len(s.Links) > 0 {
			for _, l := range s.Links {
				if l.Resolved() {
					links = append(links, [2]endpoint{{l.A.Device, l.A.Port}, {l.B.Device, l.B.Port}})
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
		lines = append(lines, fmt.Sprintf("    - endpoints: [\"%s:%s\", \"%s:%s\"]\n", l[0].device.Name, a, l[1].device.Name, z))
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
