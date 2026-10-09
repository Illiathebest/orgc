package verify

import (
	"fmt"
	"strings"

	"orgc/internal/backend"
	"orgc/internal/diag"
	"orgc/internal/ir"
)

// checkE003 reports references that do not resolve (docs/data-model.md):
//
//   - control_node.site → a site of the model;
//   - control_node.network → a network of that very site;
//   - from/to of a rule → a network of the model, any or internet;
//   - a network with a gateway → exactly one L3 gateway of its site, by the
//     model v2 convention one device with role edge_router or firewall.
//
// Lowering has already tried every reference and left the unresolved ones
// with a nil target; this check only reports them. It has to come early:
// the other invariants skip whatever it reports.
func checkE003(m *ir.Model, _ backend.Platforms) []diag.Diagnostic {
	var out []diag.Diagnostic
	bad := func(path, msg string) {
		out = append(out, report(m, diag.E003, diag.Error, path, msg))
	}

	cn := m.ControlNode
	switch {
	case cn.Site.Site == nil:
		// The network is looked up within the site, so with no site there
		// is nothing more to say about it: one defect, one diagnostic.
		bad(cn.Path+".site", fmt.Sprintf("вузол керування посилається на сайт %q, якого немає в моделі", cn.Site.Name))
	case cn.Network.Net == nil:
		if other := networkNamed(m, cn.Network.Name); other != nil {
			bad(cn.Path+".network", fmt.Sprintf(
				"мережа %q належить сайту %q, а вузол керування вказано в сайті %q",
				cn.Network.Name, other.Site.Name, cn.Site.Name))
		} else {
			bad(cn.Path+".network", fmt.Sprintf(
				"вузол керування посилається на мережу %q, якої немає в сайті %q", cn.Network.Name, cn.Site.Name))
		}
	}

	for _, r := range m.Rules {
		for _, side := range []struct {
			field string
			end   ir.Endpoint
		}{{"from", r.From}, {"to", r.To}} {
			if !side.end.Resolved() {
				bad(r.Path+"."+side.field, fmt.Sprintf(
					"правило посилається на мережу %q, якої немає в моделі; посилання має називати мережу, any або internet",
					side.end.Name))
			}
		}
	}

	for _, s := range m.Sites {
		routed := firstRouted(s)
		if routed == nil {
			continue
		}
		switch c := s.GatewayCandidates; len(c) {
		case 1:
		case 0:
			bad(routed.Path+".gateway", fmt.Sprintf(
				"мережа %q має шлюз, але в сайті %q немає L3-шлюзу (пристрою з роллю edge_router або firewall)",
				routed.Name, s.Name))
		default:
			names := make([]string, len(c))
			for i, d := range c {
				names[i] = d.Name
			}
			// Pointing at the second candidate: the first one alone would
			// have been a valid gateway.
			bad(c[1].Path+".role", fmt.Sprintf(
				"у сайті %q кілька L3-шлюзів (%s); шлюзом мереж сайту має бути рівно один пристрій з роллю edge_router або firewall",
				s.Name, strings.Join(names, ", ")))
		}
	}

	return out
}

// firstRouted returns the first network of the site that has a gateway, or
// nil when no network needs one.
func firstRouted(s *ir.Site) *ir.Network {
	for _, n := range s.Networks {
		if n.Gateway.IsValid() {
			return n
		}
	}
	return nil
}

// networkNamed finds the first network with the name anywhere in the model.
func networkNamed(m *ir.Model, name string) *ir.Network {
	for _, n := range m.Networks {
		if n.Name == name {
			return n
		}
	}
	return nil
}
