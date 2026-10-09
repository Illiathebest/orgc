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

// Capabilities are deliberately conservative. EOS routes between VLANs, but
// its ACLs are stateless, so it cannot enforce the model's stateful rules as
// written. DHCP server, NAT and client isolation depend on the release and
// the hardware, and are left out until confirmed against the EOS manual and
// cEOS (plan step 2.11): until then W002 warns where it perhaps need not,
// rather than staying silent where it should not.
func (p Platform) Capabilities(v string) backend.CapSet {
	if ok, _ := p.CheckVersion(v); !ok {
		return 0
	}
	return backend.Caps(backend.L3Gateway)
}
