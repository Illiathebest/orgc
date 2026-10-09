package verify

import (
	"fmt"
	"net/netip"

	"orgc/internal/backend"
	"orgc/internal/diag"
	"orgc/internal/ir"
)

// checkE004 reports a device whose mgmt_ip is not in a mgmt network of its
// own site. An address in another site's mgmt network is an error too:
// management would then run through someone else's site. A site with
// devices and no mgmt network gets one diagnostic per device, which is the
// definition, not a cascade.
func checkE004(m *ir.Model, _ backend.Platforms) []diag.Diagnostic {
	var out []diag.Diagnostic
	for _, d := range m.Devices {
		if mgmtNetworkOf(d) != nil {
			continue
		}
		var msg string
		if own := networkOf(d.Site, d.MgmtIP); own != nil {
			msg = fmt.Sprintf("mgmt_ip %s лежить у мережі %q з роллю %s; керування має йти через мережу з роллю mgmt сайту %q",
				d.MgmtIP, own.Name, own.Role, d.Site.Name)
		} else if other := mgmtAnywhere(m, d.MgmtIP); other != nil {
			msg = fmt.Sprintf("mgmt_ip %s належить mgmt-мережі %q іншого сайту %q; керування йшло б через чужий сайт",
				d.MgmtIP, other.Name, other.Site.Name)
		} else {
			msg = fmt.Sprintf("mgmt_ip %s не належить жодній мережі з роллю mgmt сайту %q", d.MgmtIP, d.Site.Name)
		}
		out = append(out, report(m, diag.E004, diag.Error, d.Path+".mgmt_ip", msg))
	}
	return out
}

// mgmtNetworkOf returns N_m of E006: the mgmt network of the device's site
// holding its mgmt_ip.
func mgmtNetworkOf(d *ir.Device) *ir.Network {
	for _, n := range d.Site.Networks {
		if n.Role == ir.RoleMgmt && contains(n.Prefix, d.MgmtIP) {
			return n
		}
	}
	return nil
}

// mgmtAnywhere finds a mgmt network of any site holding the address.
func mgmtAnywhere(m *ir.Model, a netip.Addr) *ir.Network {
	for _, n := range m.Networks {
		if n.Role == ir.RoleMgmt && contains(n.Prefix, a) {
			return n
		}
	}
	return nil
}

// checkW003 warns about a site of a profile M model without a backup
// uplink. Profile S does not check uplinks at all.
func checkW003(m *ir.Model, _ backend.Platforms) []diag.Diagnostic {
	if m.Profile != ir.ProfileM {
		return nil
	}
	var out []diag.Diagnostic
	for _, s := range m.Sites {
		backup := false
		for _, u := range s.Uplinks {
			if u.Kind == ir.UplinkBackup {
				backup = true
				break
			}
		}
		if !backup {
			out = append(out, report(m, diag.W003, diag.Warning, s.Path+".name",
				fmt.Sprintf("сайт %q профілю M не має резервного каналу (uplinks з kind: backup)", s.Name)))
		}
	}
	return out
}
