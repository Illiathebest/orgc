package verify

import (
	"fmt"
	"strconv"

	"orgc/internal/backend"
	"orgc/internal/diag"
	"orgc/internal/ir"
)

// checkE002 reports identifiers that must be unique and are not. Each scope
// follows docs/data-model.md:
//
//   - VLAN ID — within a site: sites are separate L2 domains;
//   - site names and network names — across the model: policies refer to a
//     network by bare name, so the name must mean one thing everywhere;
//   - device and server names — one shared space across the model: both
//     become host names of the Ansible inventory;
//   - uplink names — within a site.
//
// The first definition wins and every later one is reported, pointing back
// at the first, so a name used three times yields two diagnostics.
func checkE002(m *ir.Model, _ backend.Platforms) []diag.Diagnostic {
	var out []diag.Diagnostic
	dup := func(path, msg string) {
		out = append(out, report(m, diag.E002, diag.Error, path, msg))
	}

	sites := map[string]*ir.Site{}
	for _, s := range m.Sites {
		if first, ok := sites[s.Name]; ok {
			dup(s.Path+".name", fmt.Sprintf("сайт з імʼям %q уже описано%s", s.Name, line(m, first.Path)))
			continue
		}
		sites[s.Name] = s
	}

	networks := map[string]*ir.Network{}
	for _, n := range m.Networks {
		if first, ok := networks[n.Name]; ok {
			dup(n.Path+".name", fmt.Sprintf(
				"мережа з імʼям %q уже є в сайті %q%s; імена мереж унікальні в усій моделі, бо політики посилаються на них без сайту",
				n.Name, first.Site.Name, line(m, first.Path)))
			continue
		}
		networks[n.Name] = n
	}

	// Devices and servers share one name space, so they are walked as one
	// list: devices first, in file order, then servers.
	type host struct {
		kind, name, path string
	}
	var hosts []host
	for _, d := range m.Devices {
		hosts = append(hosts, host{"пристрій", d.Name, d.Path})
	}
	for _, s := range m.Servers {
		hosts = append(hosts, host{"сервер", s.Name, s.Path})
	}
	seen := map[string]host{}
	for _, h := range hosts {
		if first, ok := seen[h.name]; ok {
			dup(h.path+".name", fmt.Sprintf(
				"імʼя %q уже має %s%s; пристрої й сервери ділять один простір імен, бо стають іменами хостів інвентарю",
				h.name, first.kind, line(m, first.path)))
			continue
		}
		seen[h.name] = h
	}

	for _, s := range m.Sites {
		vlans := map[int]*ir.Network{}
		for _, n := range s.Networks {
			if first, ok := vlans[n.VLAN]; ok {
				dup(n.Path+".vlan", fmt.Sprintf(
					"VLAN %d уже використано в сайті %q мережею %q%s",
					n.VLAN, s.Name, first.Name, line(m, first.Path)))
				continue
			}
			vlans[n.VLAN] = n
		}

		uplinks := map[string]*ir.Uplink{}
		for _, u := range s.Uplinks {
			if first, ok := uplinks[u.Name]; ok {
				dup(u.Path+".name", fmt.Sprintf(
					"канал з імʼям %q уже є в сайті %q%s", u.Name, s.Name, line(m, first.Path)))
				continue
			}
			uplinks[u.Name] = u
		}
	}

	return out
}

// line renders " (рядок N)" for the element at path, so that a diagnostic
// about a duplicate can point at the original. It is empty when the
// position is unknown.
func line(m *ir.Model, path string) string {
	if p := m.At(path); p.Known() {
		return " (рядок " + strconv.Itoa(p.Line) + ")"
	}
	return ""
}
