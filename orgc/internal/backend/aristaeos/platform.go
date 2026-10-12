// Package aristaeos renders .cfg configuration for Arista EOS. In the lab the
// target runs as cEOS under containerlab, standing in for physical switches.
//
// So far it implements only what verification asks of a platform: the
// supported versions, the management transport and the capabilities.
package aristaeos

import (
	"regexp"
	"strconv"

	"orgc/internal/backend"
	"orgc/internal/ir"
)

// Platform is Arista EOS.
type Platform struct{}

var _ backend.Platform = Platform{}

// Type matches device.type in the model.
func (Platform) Type() ir.DeviceType { return "arista_eos" }

const supported = "Arista EOS 4.x"

// version is the EOS release form: major.minor.patch, an optional fourth
// number, and the release train letter — 4.29.2F, 4.30.1M, 4.28.10.1M.
var version = regexp.MustCompile(`^(\d+)\.(\d+)\.(\d+)(?:\.(\d+))?([A-Z]+)?$`)

// CheckVersion accepts EOS 4.x: the VLAN, SVI and ACL syntax the backend
// writes is the same across that major line.
func (Platform) CheckVersion(v string) (bool, string) {
	m := version.FindStringSubmatch(v)
	if m == nil {
		return false, supported
	}
	major, _ := strconv.Atoi(m[1])
	return major == 4, supported
}

// ManagementPorts is SSH: arista.eos modules work over
// ansible.netcommon.network_cli.
func (p Platform) ManagementPorts(v string) []ir.PortSpec {
	if ok, _ := p.CheckVersion(v); !ok {
		return nil
	}
	return []ir.PortSpec{{Proto: ir.TCP, Port: 22}}
}

// Capabilities follow the EOS User Manual and Arista's feature notes:
//
//   - L3Gateway: routing between VLAN interfaces;
//   - DHCPServer: the built-in DHCPv4 server appeared in EOS 4.22.1;
//   - ClientIsolation: private VLANs with an isolated secondary VLAN, whose
//     hosts cannot reach each other by bridging.
//
// Two are absent on purpose. EOS ACLs are stateless, so the model's
// stateful rules cannot be enforced as written (no StatefulFilter). NAT
// depends on the hardware platform and is not available on cEOS, the
// emulated target of the lab.
func (p Platform) Capabilities(v string) backend.CapSet {
	if ok, _ := p.CheckVersion(v); !ok {
		return 0
	}
	caps := backend.Caps(backend.L3Gateway, backend.ClientIsolation)
	if atLeast(v, 4, 22, 1) {
		caps |= backend.Caps(backend.DHCPServer)
	}
	return caps
}

// atLeast compares the numeric part of an accepted version with a release.
func atLeast(v string, major, minor, patch int) bool {
	m := version.FindStringSubmatch(v)
	got := [3]int{}
	for i := range got {
		got[i], _ = strconv.Atoi(m[i+1])
	}
	want := [3]int{major, minor, patch}
	for i := range got {
		if got[i] != want[i] {
			return got[i] > want[i]
		}
	}
	return true
}

// Render produces the device configuration. Generation is not implemented
// yet; until it is, a device contributes no files.
func (Platform) Render(*ir.Model, *ir.Device) []backend.File { return nil }

// port matches EOS front-panel interfaces: Ethernet<N>, or Ethernet<N>/<M>
// on modular and breakout ports. Management1 does not match: it is the
// out-of-band management port and carries neither trunk nor access VLANs.
var port = regexp.MustCompile(`^Ethernet[1-9][0-9]*(/[1-9][0-9]*)?$`)

// CheckPort accepts Ethernet<N> and Ethernet<N>/<M>.
func (Platform) CheckPort(name string) (bool, string) {
	return port.MatchString(name), "Ethernet<N> чи Ethernet<N>/<M> (Management1 — лише порт керування)"
}
