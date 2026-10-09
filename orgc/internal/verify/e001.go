package verify

import (
	"fmt"

	"orgc/internal/backend"
	"orgc/internal/diag"
	"orgc/internal/ir"
)

// checkE001 reports overlapping prefixes (docs/data-model.md):
//
//   - inside one site an overlap is always an error: two networks on one
//     router with shared addresses mean broken routing;
//   - between sites it is allowed when at least one of the two sites sets
//     allow_overlap (a branch behind NAT reusing a private range);
//   - a rule joining two networks whose overlap was allowed is an error:
//     the addresses it compiles to mean different hosts on each side.
//
// Each overlapping pair is reported once, on the later network.
func checkE001(m *ir.Model, _ backend.Platforms) []diag.Diagnostic {
	var out []diag.Diagnostic
	bad := func(path, msg string) {
		out = append(out, report(m, diag.E001, diag.Error, path, msg))
	}

	type pair struct{ a, b *ir.Network }
	allowed := map[pair]bool{}

	for j, b := range m.Networks {
		for _, a := range m.Networks[:j] {
			if !overlap(a.Prefix, b.Prefix) {
				continue
			}
			switch {
			case a.Site == b.Site:
				bad(b.Path+".cidr", fmt.Sprintf(
					"префікс %s перетинається з мережею %q %s%s; усередині сайту перетин заборонено",
					b.Prefix, a.Name, a.Prefix, line(m, a.Path)))
			case a.Site.AllowOverlap || b.Site.AllowOverlap:
				allowed[pair{a, b}], allowed[pair{b, a}] = true, true
			default:
				bad(b.Path+".cidr", fmt.Sprintf(
					"префікс %s перетинається з мережею %q сайту %q %s%s; між сайтами перетин дозволяє лише allow_overlap",
					b.Prefix, a.Name, a.Site.Name, a.Prefix, line(m, a.Path)))
			}
		}
	}

	for _, r := range m.Rules {
		// Unresolved ends are E003's, any → any is E005's, and the internet
		// is outside the model's address plan.
		if !r.From.Resolved() || !r.To.Resolved() ||
			r.From.Kind == ir.EndpointInternet || r.To.Kind == ir.EndpointInternet ||
			(r.From.Kind == ir.EndpointAny && r.To.Kind == ir.EndpointAny) {
			continue
		}
	search:
		for _, a := range side(m, r.From) {
			for _, b := range side(m, r.To) {
				if allowed[pair{a, b}] {
					bad(r.Path, fmt.Sprintf(
						"правило зʼєднує мережі %q і %q, адреси яких перетинаються (дозволено allow_overlap); адресація правила неоднозначна",
						a.Name, b.Name))
					break search
				}
			}
		}
	}

	return out
}

// side lists the networks a resolved, non-internet endpoint stands for.
func side(m *ir.Model, e ir.Endpoint) []*ir.Network {
	if e.Kind == ir.EndpointAny {
		return m.Networks
	}
	return []*ir.Network{e.Net}
}
