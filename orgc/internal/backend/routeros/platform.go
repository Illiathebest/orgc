// Package routeros renders .rsc configuration for MikroTik RouterOS 6.x.
//
// So far it implements only what verification asks of a platform: the
// supported versions, the management transport and the capabilities.
package routeros

import (
	"regexp"
	"strconv"

	"orgc/internal/backend"
	"orgc/internal/ir"
)

// Platform is MikroTik RouterOS.
type Platform struct{}

var _ backend.Platform = Platform{}

// Type matches device.type in the model.
func (Platform) Type() ir.DeviceType { return "mikrotik" }

// The backend writes RouterOS 6 syntax; 7.x changed it for routing and
// several other subsystems. The floor is 6.41, the release that introduced
// VLAN filtering on the bridge — the mechanism the backend uses to put
// several networks on one router. Below it the same model needs an entirely
// different configuration.
const (
	minMinor  = 41
	supported = "RouterOS 6.41–6.x"
)

// version accepts the stable release form: 6.48 or 6.48.6. Test builds
// ("6.49beta11", "7.1rc1") are not something to render a site for.
var version = regexp.MustCompile(`^(\d+)\.(\d+)(?:\.(\d+))?$`)

// CheckVersion accepts RouterOS 6.41 and later 6.x releases.
func (Platform) CheckVersion(v string) (bool, string) {
	m := version.FindStringSubmatch(v)
	if m == nil {
		return false, supported
	}
	major, _ := strconv.Atoi(m[1])
	minor, _ := strconv.Atoi(m[2])
	return major == 6 && minor >= minMinor, supported
}

// ManagementPorts is the RouterOS API. The inventory drives the router with
// community.routeros.api* over plain API, TCP 8728
// (ansible/inventories/staging/group_vars/mikrotik_routers); API-SSL on 8729
// is not used. SSH is not the transport at all, which is exactly how a rule
// opening only port 22 cuts Ansible off from the router (E006).
func (p Platform) ManagementPorts(v string) []ir.PortSpec {
	if ok, _ := p.CheckVersion(v); !ok {
		return nil
	}
	return []ir.PortSpec{{Proto: ir.TCP, Port: 8728}}
}

// Capabilities: everything model v2 can ask of a gateway is available
// throughout 6.41–6.x — connection-tracking firewall, masquerade, DHCP
// server, and bridge port horizon for client isolation.
func (p Platform) Capabilities(v string) backend.CapSet {
	if ok, _ := p.CheckVersion(v); !ok {
		return 0
	}
	return backend.Caps(
		backend.L3Gateway,
		backend.DHCPServer,
		backend.NAT,
		backend.StatefulFilter,
		backend.ClientIsolation,
	)
}

// portName matches RouterOS interface names a model may use: copper Ethernet,
// SFP and SFP+ ports, numbered from 1.
var portName = regexp.MustCompile(`^(ether|sfp|sfp-sfpplus)[1-9][0-9]*$`)

// CheckPort accepts etherN, sfpN and sfp-sfpplusN.
func (Platform) CheckPort(name string) (bool, string) {
	return portName.MatchString(name), "ether<N>, sfp<N> чи sfp-sfpplus<N>"
}

// AnsibleGroup is the inventory group of RouterOS devices. smoke_routeros.yml
// targets it by this name.
func (Platform) AnsibleGroup() string { return "mikrotik_routers" }

// AnsibleVars connect to RouterOS over its API: community.routeros.api
// modules open the connection themselves, hence connection local. The
// password comes from the environment — a masked CI variable or a local
// export — never from the inventory.
func (Platform) AnsibleVars() []backend.Var {
	return []backend.Var{
		{Name: "ansible_connection", Value: "local"},
		{Name: "routeros_api_username", Value: "ansible-ro"},
		{Name: "routeros_api_password", Value: "{{ lookup('ansible.builtin.env', 'ROUTEROS_PASSWORD') }}"},
	}
}
