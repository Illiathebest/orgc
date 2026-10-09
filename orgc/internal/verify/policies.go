package verify

import (
	"fmt"

	"orgc/internal/backend"
	"orgc/internal/diag"
	"orgc/internal/ir"
)

// Rules are judged by three codes, at most one per rule, most severe
// first: E005 (the rule undoes default-deny), W004 (it opens a mgmt network
// beyond the control node), W001 (it lets a guest network reach more than
// the internet). A rule like guest → mgmt fits both warnings; it gets W004,
// the one about management. Unresolved ends are E003's and judged by none.

// breaksDefaultDeny tells whether a rule is E005's: any → any, or any rule
// from the internet.
func breaksDefaultDeny(r *ir.Rule) (bool, string, string) {
	switch {
	case r.From.Kind == ir.EndpointAny && r.To.Kind == ir.EndpointAny:
		return true, r.Path, "правило any → any відкриває всі мережі одна одній і скасовує default-deny"
	case r.From.Kind == ir.EndpointInternet:
		return true, r.Path + ".from", "правило from: internet заборонено: вхідний доступ ззовні потребує трансляції портів, якої модель не описує"
	}
	return false, "", ""
}

func resolved(r *ir.Rule) bool { return r.From.Resolved() && r.To.Resolved() }

// checkE005 reports rules that neutralise default-deny.
func checkE005(m *ir.Model, _ backend.Platforms) []diag.Diagnostic {
	var out []diag.Diagnostic
	for _, r := range m.Rules {
		if !resolved(r) {
			continue
		}
		if bad, path, msg := breaksDefaultDeny(r); bad {
			out = append(out, report(m, diag.E005, diag.Error, path, msg))
		}
	}
	return out
}

// opensMgmt tells whether a rule is W004's: it reaches a mgmt network —
// named, or through to: any — from somewhere other than the control node's
// network. Without a resolved control node network there is nothing to
// compare with; that is E003's.
func opensMgmt(m *ir.Model, r *ir.Rule) (bool, string) {
	nc := m.ControlNode.Network.Net
	if nc == nil || !resolved(r) {
		return false, ""
	}
	if ok, _, _ := breaksDefaultDeny(r); ok {
		return false, ""
	}

	var target *ir.Network
	switch r.To.Kind {
	case ir.EndpointNetwork:
		if r.To.Net.Role == ir.RoleMgmt {
			target = r.To.Net
		}
	case ir.EndpointAny:
		for _, n := range m.Networks {
			if n.Role == ir.RoleMgmt {
				target = n
				break
			}
		}
	}
	if target == nil {
		return false, ""
	}

	switch r.From.Kind {
	case ir.EndpointAny:
		return true, fmt.Sprintf("правило з any відкриває mgmt-мережу %q усім мережам, а не лише мережі вузла керування %q", target.Name, nc.Name)
	case ir.EndpointNetwork:
		if r.From.Net != nc {
			return true, fmt.Sprintf("правило відкриває mgmt-мережу %q мережі %q; вузол керування працює з мережі %q", target.Name, r.From.Net.Name, nc.Name)
		}
	}
	return false, ""
}

// checkW004 warns about mgmt networks opened beyond the control node. It is
// a warning: monitoring from a separate network is a legitimate case.
func checkW004(m *ir.Model, _ backend.Platforms) []diag.Diagnostic {
	var out []diag.Diagnostic
	for _, r := range m.Rules {
		if ok, msg := opensMgmt(m, r); ok {
			out = append(out, report(m, diag.W004, diag.Warning, r.Path, msg))
		}
	}
	return out
}

// checkW001 warns about a guest network that is not isolated inside, or
// that a rule lets reach anything but the internet. "From" may be the guest
// network itself or any, which includes it.
func checkW001(m *ir.Model, _ backend.Platforms) []diag.Diagnostic {
	var out []diag.Diagnostic
	var guests []*ir.Network
	for _, n := range m.Networks {
		if n.Role != ir.RoleGuest {
			continue
		}
		guests = append(guests, n)
		if !n.Isolated {
			out = append(out, report(m, diag.W001, diag.Warning, n.Path+".role", fmt.Sprintf(
				"guest-мережа %q без isolated: true, тож гості бачать одне одного", n.Name)))
		}
	}
	if len(guests) == 0 {
		return out
	}

	for _, r := range m.Rules {
		if !resolved(r) || r.To.Kind == ir.EndpointInternet {
			continue
		}
		if bad, _, _ := breaksDefaultDeny(r); bad {
			continue
		}
		if mgmt, _ := opensMgmt(m, r); mgmt {
			continue
		}
		var guest *ir.Network
		switch r.From.Kind {
		case ir.EndpointNetwork:
			if r.From.Net.Role == ir.RoleGuest {
				guest = r.From.Net
			}
		case ir.EndpointAny:
			guest = guests[0]
		}
		if guest == nil {
			continue
		}
		out = append(out, report(m, diag.W001, diag.Warning, r.Path, fmt.Sprintf(
			"правило відкриває guest-мережі %q доступ до %s; гостям має бути доступний лише internet",
			guest.Name, endpointName(r.To))))
	}
	return out
}

func endpointName(e ir.Endpoint) string {
	if e.Kind == ir.EndpointAny {
		return "усіх мереж (any)"
	}
	return fmt.Sprintf("мережі %q", e.Name)
}
