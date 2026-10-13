package verify

import (
	"fmt"
	"strconv"

	"orgc/internal/backend"
	"orgc/internal/diag"
	"orgc/internal/ir"
)

// portUse is one place the model names a port of a device.
type portUse struct {
	device *ir.Device // nil when the reference did not resolve (E003)
	port   string
	path   string // the field naming the port
	what   string // how the use is named in a message
}

// portUses lists every port the model names, site by site: uplink ports,
// access ports, server ports, then both ends of every link. E002 (each port
// used once) and W002 (port names valid for the platform) both walk it.
func portUses(m *ir.Model) []portUse {
	var out []portUse
	ref := func(r *ir.PortRef, what string) {
		out = append(out, portUse{device: r.Device, port: r.Port, path: r.Path, what: what})
	}
	for _, s := range m.Sites {
		for _, u := range s.Uplinks {
			if u.Port != nil {
				ref(u.Port, "канал "+u.Name)
			}
		}
		for _, d := range s.Devices {
			for _, ap := range d.AccessPorts {
				for i, p := range ap.Ports {
					out = append(out, portUse{device: d, port: p,
						path: ap.Path + ".ports[" + strconv.Itoa(i) + "]",
						what: "access-порт мережі " + ap.Network.Name})
				}
			}
		}
		for _, sv := range s.Servers {
			if sv.Port != nil {
				ref(sv.Port, "сервер "+sv.Name)
			}
		}
		for _, l := range s.Links {
			ref(&l.A, "зʼєднання "+l.A.String()+" – "+l.B.String())
			ref(&l.B, "зʼєднання "+l.A.String()+" – "+l.B.String())
		}
	}
	return out
}

// usedPortsOnce is the E002 part about ports: a port of a device is named
// at most once across links, access ports, server ports and uplink ports.
func usedPortsOnce(m *ir.Model, dup func(path, msg string)) {
	type key struct {
		d    *ir.Device
		port string
	}
	first := map[key]portUse{}
	for _, u := range portUses(m) {
		if u.device == nil {
			continue // E003
		}
		k := key{u.device, u.port}
		if f, ok := first[k]; ok {
			dup(u.path, fmt.Sprintf("порт %s:%s уже використано: %s%s", u.device.Name, u.port, f.what, line(m, f.path)))
			continue
		}
		first[k] = u
	}
}

// connectionReferences is the E003 part about connections: link ends and
// server ports name devices of the same site, VLAN names and access-port
// networks name networks of the site, and an uplink port belongs to the
// site's L3 gateway.
func connectionReferences(m *ir.Model, bad func(path, msg string)) {
	device := func(r *ir.PortRef, s *ir.Site, what string) {
		if r.Resolved() {
			return
		}
		if other := deviceNamed(m, r.DeviceName); other != nil {
			bad(r.Path, fmt.Sprintf("%s посилається на пристрій %q сайту %q; зʼєднання між сайтами модель не описує", what, r.DeviceName, other.Site.Name))
			return
		}
		bad(r.Path, fmt.Sprintf("%s посилається на пристрій %q, якого немає в сайті %q", what, r.DeviceName, s.Name))
	}
	network := func(ref ir.NetRef, s *ir.Site, path string) {
		if ref.Net != nil {
			return
		}
		if other := networkNamed(m, ref.Name); other != nil {
			bad(path, fmt.Sprintf("мережа %q належить сайту %q, а не %q", ref.Name, other.Site.Name, s.Name))
			return
		}
		bad(path, fmt.Sprintf("мережі %q немає в сайті %q", ref.Name, s.Name))
	}

	for _, s := range m.Sites {
		for _, u := range s.Uplinks {
			if u.Port == nil {
				continue
			}
			if !u.Port.Resolved() {
				device(u.Port, s, "порт каналу")
			} else if !u.Port.Device.Role.RoutesL3() {
				bad(u.Port.Path, fmt.Sprintf("канал %q виходить через %s, а порт каналу має належати L3-шлюзу сайту (edge_router чи firewall)", u.Name, u.Port))
			}
		}
		for _, d := range s.Devices {
			for _, ap := range d.AccessPorts {
				network(ap.Network, s, ap.Path+".network")
			}
		}
		for _, sv := range s.Servers {
			if sv.Port != nil {
				device(sv.Port, s, "порт сервера")
			}
		}
		for _, l := range s.Links {
			device(&l.A, s, "кінець зʼєднання")
			device(&l.B, s, "кінець зʼєднання")
			for i, v := range l.VLANs {
				network(v, s, l.Path+".vlans["+strconv.Itoa(i)+"]")
			}
		}
	}
}

// portNames is the W002 part about ports: every port name must match the
// pattern its device's platform declares. An invalid name cannot be
// rendered, so the level is error.
func portNames(m *ir.Model, ps backend.Platforms) []diag.Diagnostic {
	var out []diag.Diagnostic
	for _, u := range portUses(m) {
		if u.device == nil {
			continue // E003
		}
		p, ok := ps.Lookup(u.device.Type)
		if !ok {
			continue // reported by checkW002 itself
		}
		if ok, expected := p.CheckPort(u.port); !ok {
			out = append(out, report(m, diag.W002, diag.Error, u.path, fmt.Sprintf(
				"порт %q неприпустимий для платформи %s пристрою %q: очікується %s", u.port, u.device.Type, u.device.Name, expected)))
		}
	}
	return out
}

func deviceNamed(m *ir.Model, name string) *ir.Device {
	for _, d := range m.Devices {
		if d.Name == name {
			return d
		}
	}
	return nil
}

// whole reports whether a site's tree can be judged at all: the site
// describes links, has its one gateway, every reference resolved (else
// E003) and the links form a tree (else E010).
func whole(s *ir.Site) bool {
	t := s.L2
	return t != nil && t.Root != nil && !t.Incomplete && len(t.Extra) == 0
}

// checkE010 reports links that keep a site's topology from being a tree
// rooted at its gateway: a cycle, a link from a device to itself, a second
// link between the same two devices. The link reported is the one later in
// the file — the one whose removal restores the tree.
func checkE010(m *ir.Model, _ backend.Platforms) []diag.Diagnostic {
	var out []diag.Diagnostic
	for _, s := range m.Sites {
		if s.L2 == nil || s.L2.Incomplete {
			continue // no links, or E003
		}
		for _, e := range s.L2.Extra {
			l := e.Link
			var msg string
			switch e.Fault {
			case ir.FaultSelf:
				msg = fmt.Sprintf("зʼєднання %s – %s зʼєднує пристрій %q сам із собою", l.A.String(), l.B.String(), l.A.DeviceName)
			case ir.FaultParallel:
				if l.Standby {
					msg = fmt.Sprintf("резервне зʼєднання %s – %s — друге резервне між пристроями %q і %q", l.A.String(), l.B.String(), l.A.DeviceName, l.B.DeviceName)
				} else {
					msg = fmt.Sprintf("зʼєднання %s – %s — друге між пристроями %q і %q; без STP це петля, а резервне зʼєднання позначають standby: true", l.A.String(), l.B.String(), l.A.DeviceName, l.B.DeviceName)
				}
			case ir.FaultStandbyOpen:
				out := l.A.DeviceName
				if s.L2.Reached(l.A.Device) {
					out = l.B.DeviceName
				}
				msg = fmt.Sprintf("резервне зʼєднання %s – %s не замикає циклу з деревом активних зʼєднань: пристрій %q не має активного шляху до шлюзу %q, тож STP не заблокує це зʼєднання", l.A.String(), l.B.String(), out, s.L2.Root.Name)
			default:
				msg = fmt.Sprintf("зʼєднання %s – %s замикає петлю в топології сайту %q; активні зʼєднання мають утворювати дерево з коренем у L3-шлюзі, а резервне позначають standby: true", l.A.String(), l.B.String(), s.Name)
			}
			out = append(out, report(m, diag.E010, diag.Error, l.Path, msg))
		}
	}
	return out
}

// checkE009 verifies L2 connectivity of a site with links: every device
// has a path to the site's gateway, and the network of every access port
// is carried all the way to it. A device without a path gets one
// diagnostic, not one per port.
func checkE009(m *ir.Model, _ backend.Platforms) []diag.Diagnostic {
	var out []diag.Diagnostic
	for _, s := range m.Sites {
		if !whole(s) {
			continue
		}
		t := s.L2
		for _, d := range s.Devices {
			if !t.Reached(d) {
				out = append(out, report(m, diag.E009, diag.Error, d.Path+".name", fmt.Sprintf(
					"пристрій %q не має L2-шляху до шлюзу %q: його немає в жодному зʼєднанні, що веде до шлюзу", d.Name, t.Root.Name)))
			}
		}
		deliver := func(d *ir.Device, n *ir.Network, path, what string) {
			if d == nil || n == nil || !t.Reached(d) {
				return
			}
			if b := t.Blocking(d, n); b != nil {
				out = append(out, report(m, diag.E009, diag.Error, path, fmt.Sprintf(
					"мережа %q не доходить до %s на %q: зʼєднання %s – %s її не пропускає", n.Name, what, d.Name, b.A.String(), b.B.String())))
			}
		}
		for _, d := range s.Devices {
			for _, ap := range d.AccessPorts {
				deliver(d, ap.Network.Net, ap.Path+".network", "access-портів")
			}
		}
		for _, sv := range s.Servers {
			if sv.Port.Resolved() {
				deliver(sv.Port.Device, networkOf(s, sv.IP), sv.Port.Path, "порту сервера "+sv.Name)
			}
		}
	}
	return out
}

// roleLevel ranks roles from the gateway down: the level must grow along
// every path from the gateway.
func roleLevel(r ir.DeviceRole) int {
	switch r {
	case ir.DeviceCoreSwitch:
		return 1
	case ir.DeviceDistributionSwitch:
		return 2
	case ir.DeviceAccessSwitch:
		return 3
	default: // edge_router, firewall
		return 0
	}
}

// checkW005 warns where the role level does not grow on the way from the
// gateway: an access switch feeding another switch, distribution behind
// access. Skipping a level is fine — Access–Core is the usual small-office
// design. A warning: the topology works, it just contradicts its roles.
func checkW005(m *ir.Model, _ backend.Platforms) []diag.Diagnostic {
	var out []diag.Diagnostic
	for _, s := range m.Sites {
		if !whole(s) {
			continue
		}
		t := s.L2
		for _, d := range t.Order[1:] {
			up := t.Up[d]
			parent := up.Other(d)
			if roleLevel(d.Role) <= roleLevel(parent.Role) {
				out = append(out, report(m, diag.W005, diag.Warning, up.Path, fmt.Sprintf(
					"%q (%s) підключено через %q (%s): на шляху від шлюзу рівень ролі має зростати — core, distribution, access",
					d.Name, d.Role, parent.Name, parent.Role)))
			}
		}
	}
	return out
}
