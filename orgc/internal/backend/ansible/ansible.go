// Package ansible generates the Ansible inventory of a model: hosts.yml and
// group_vars (docs/adr/ADR-009, decision 4).
//
// Groups follow platforms: every server goes to linux_servers, every device
// to the group its platform names. Connection variables of a device group
// come from the platform as well, so the generator never names a vendor.
// Existing playbooks target the groups by name — smoke_routeros.yml uses
// mikrotik_routers, linux.yml uses linux_servers — and keep working on the
// generated inventory unchanged.
package ansible

import (
	"path"
	"regexp"
	"sort"
	"strings"

	"orgc/internal/backend"
	"orgc/internal/ir"
)

// ServerGroup is the group of every server of the model.
const ServerGroup = "linux_servers"

// serverVars connect to servers over SSH as the service account the lab
// provisions on every VM; its key lives on the control node.
var serverVars = []backend.Var{{Name: "ansible_user", Value: "ansible-svc"}}

// Generator renders the inventory.
type Generator struct{}

var _ backend.Generator = Generator{}

// Name identifies the generator.
func (Generator) Name() string { return "ansible" }

type host struct {
	name, addr string
}

type group struct {
	name  string
	hosts []host
	vars  []backend.Var
}

// Generate renders ansible/hosts.yml and one group_vars file per group that
// has hosts. Groups are sorted by name; hosts keep the order of the model.
func (Generator) Generate(m *ir.Model, ps backend.Platforms) []backend.File {
	groups := map[string]*group{}
	add := func(name string, vars []backend.Var, h host) {
		g, ok := groups[name]
		if !ok {
			g = &group{name: name, vars: vars}
			groups[name] = g
		}
		g.hosts = append(g.hosts, h)
	}

	for _, s := range m.Servers {
		add(ServerGroup, serverVars, host{s.Name, s.IP.String()})
	}
	for _, d := range m.Devices {
		p, ok := ps.Lookup(d.Type)
		if !ok {
			continue // verification has already failed for such a device
		}
		t, ok := p.(backend.AnsibleTarget)
		if !ok {
			continue // a platform Ansible does not manage
		}
		add(t.AnsibleGroup(), t.AnsibleVars(), host{d.Name, d.MgmtIP.String()})
	}

	names := make([]string, 0, len(groups))
	for n := range groups {
		names = append(names, n)
	}
	sort.Strings(names)

	head := header(m)
	var inv strings.Builder
	inv.WriteString(head)
	inv.WriteString("all:\n  children:\n")
	files := []backend.File{}
	for _, n := range names {
		g := groups[n]
		inv.WriteString("    " + g.name + ":\n      hosts:\n")
		for _, h := range g.hosts {
			inv.WriteString("        " + h.name + ":\n          ansible_host: " + scalar(h.addr) + "\n")
		}

		var vars strings.Builder
		vars.WriteString(head)
		for _, v := range g.vars {
			vars.WriteString(v.Name + ": " + scalar(v.Value) + "\n")
		}
		files = append(files, backend.File{
			Path: path.Join("ansible", "group_vars", g.name, "main.yml"),
			Data: []byte(vars.String()),
		})
	}
	files = append(files, backend.File{Path: "ansible/hosts.yml", Data: []byte(inv.String())})
	return files
}

// header opens every generated file: where it came from, without time or
// version, then the YAML document marker (docs/adr/ADR-009, decision 7).
func header(m *ir.Model) string {
	return "# Згенеровано orgc з " + path.Base(strings.ReplaceAll(m.File, `\`, "/")) + ". Не редагувати вручну.\n---\n"
}

// plain matches a scalar that YAML reads back as the same string or as the
// boolean it is meant to be: identifiers, addresses, module names, true.
var plain = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_./-]*$`)

// scalar renders a value, double-quoting anything that is not plain, such as
// a Jinja expression.
func scalar(v string) string {
	if plain.MatchString(v) {
		return v
	}
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	return `"` + r.Replace(v) + `"`
}
