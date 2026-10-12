package verify

import (
	"fmt"
	"strings"

	"orgc/internal/backend"
	"orgc/internal/diag"
	"orgc/internal/ir"
)

// checkW002 asks each device's platform two things (docs/data-model.md):
//
//   - whether the os_version is supported at all. If not, the device cannot
//     be rendered, and W002 is raised to error level;
//   - whether the platform has every capability the model requires of the
//     device. A missing one is a warning;
//   - whether every port the model names on the device is a valid port
//     name for the platform. An invalid one cannot be rendered: error.
//
// The platform says what it has; what the model requires is decided here,
// from the IR, so that the core owns the vocabulary and no vendor knowledge
// is needed. Only the site's L3 gateway carries requirements in model v2;
// a site with no single gateway is E003's.
func checkW002(m *ir.Model, ps backend.Platforms) []diag.Diagnostic {
	var out []diag.Diagnostic
	for _, d := range m.Devices {
		p, ok := ps.Lookup(d.Type)
		if !ok {
			// The schema admits only types orgc has platforms for, and a
			// test holds the two lists together; reaching this means a
			// registry was built without one.
			out = append(out, report(m, diag.W002, diag.Error, d.Path+".type",
				fmt.Sprintf("для типу пристрою %q немає бекенда", d.Type)))
			continue
		}
		if ok, supported := p.CheckVersion(d.OSVersion); !ok {
			out = append(out, report(m, diag.W002, diag.Error, d.Path+".os_version", fmt.Sprintf(
				"версія ОС %q не підтримується: бекенд генерує конфігурації для %s, тож пристрій %q згенерувати неможливо",
				d.OSVersion, supported, d.Name)))
			continue
		}

		missing := p.Capabilities(d.OSVersion).Missing(required(m, d))
		if len(missing) == 0 {
			continue
		}
		names := make([]string, len(missing))
		for i, c := range missing {
			names[i] = c.String()
		}
		out = append(out, report(m, diag.W002, diag.Warning, d.Path+".type", fmt.Sprintf(
			"платформа %s %s не підтримує того, що модель вимагає від L3-шлюзу сайту %q: %s",
			d.Type, d.OSVersion, d.Site.Name, strings.Join(names, ", "))))
	}
	out = append(out, portNames(m, ps)...)
	return out
}

// required lists what the model asks of a device. The site's L3 gateway
// routes between its networks, and further:
//
//   - a network of the site with a dhcp_pool needs a DHCP server;
//   - a rule from a network of the site (or from any) to the internet needs
//     address translation;
//   - any rule touching a network of the site (or any) needs stateful
//     filtering, because model rules are stateful;
//   - an isolated network of the site needs client isolation.
func required(m *ir.Model, d *ir.Device) backend.CapSet {
	s := d.Site
	if s.Gateway != d {
		return 0
	}
	need := backend.Caps(backend.L3Gateway)
	for _, n := range s.Networks {
		if n.Pool != nil {
			need |= backend.Caps(backend.DHCPServer)
		}
		if n.Isolated {
			need |= backend.Caps(backend.ClientIsolation)
		}
	}
	touches := func(e ir.Endpoint) bool {
		return e.Kind == ir.EndpointAny || (e.Kind == ir.EndpointNetwork && e.Net != nil && e.Net.Site == s)
	}
	for _, r := range m.Rules {
		if !resolved(r) {
			continue
		}
		if r.To.Kind == ir.EndpointInternet && touches(r.From) {
			need |= backend.Caps(backend.NAT)
		}
		if touches(r.From) || touches(r.To) {
			need |= backend.Caps(backend.StatefulFilter)
		}
	}
	return need
}
