package verify

import (
	"fmt"
	"strings"

	"orgc/internal/backend"
	"orgc/internal/diag"
	"orgc/internal/ir"
)

// checkW006 warns where a standby link would not keep a device manageable:
// when an active link of the cycle the standby link closes fails, STP
// unblocks the standby link, and every device below the failed link reaches
// the gateway along a new path. That path must still carry the device's
// mgmt network, otherwise the failure cuts the device off although a
// redundant cable is in place — typically, the mgmt network is missing from
// the standby link's vlans.
//
// Whichever active link of the cycle on a device's path fails, the new path
// is the same: up to the far end of the standby link, across it, then down
// the tree to the device. So one path per standby link and device is
// checked, against the topmost cycle link on the device's path:
// O(Σ|S|·|D|·depth) for a site with S standby links and D devices. Devices
// cut off by the same missing VLAN are reported together, one diagnostic
// per link that lacks it, positioned on that link's vlans.
//
// "May" lose management: when another standby link closes a cycle through
// the same failed link, which of the two STP unblocks depends on bridge IDs
// — MAC addresses the model does not have — so the model cannot promise the
// good one. With a single standby link for the failure, the loss is certain.
//
// A warning: in normal operation management works. Sites with a broken
// topology (E003, E010) and devices whose mgmt network is not carried even
// now (E006) are skipped.
func checkW006(m *ir.Model, _ backend.Platforms) []diag.Diagnostic {
	var out []diag.Diagnostic
	for _, s := range m.Sites {
		if !whole(s) {
			continue
		}
		t := s.L2
		for _, sl := range t.Standby {
			u, v := sl.A.Device, sl.B.Device
			pu, pv := t.Path(u), t.Path(v)
			k := commonPrefix(pu, pv)
			side := map[*ir.Link]*ir.Device{} // cycle link → standby end below it
			for _, l := range pu[k:] {
				side[l] = u
			}
			for _, l := range pv[k:] {
				side[l] = v
			}

			// One finding per link that lacks the mgmt network: the devices
			// it would cut off, grouped by the active link whose failure
			// does it.
			type cut struct {
				failed  *ir.Link
				devices []string
			}
			type finding struct {
				blocking *ir.Link
				mgmt     *ir.Network
				cuts     []*cut
			}
			var found []*finding
			for _, d := range t.Order[1:] {
				nm := mgmtNetworkOf(d)
				if nm == nil || t.Blocking(d, nm) != nil {
					continue // E004 or E006
				}
				var failed *ir.Link
				for _, l := range t.Path(d) {
					if side[l] != nil {
						failed = l
						break
					}
				}
				if failed == nil {
					continue // the path of d does not change
				}
				near := side[failed]
				far := u
				if near == u {
					far = v
				}
				alt := append(append(append([]*ir.Link{}, t.Path(far)...), sl), between(t, near, d)...)
				var blocking *ir.Link
				for _, l := range alt {
					if !l.Carries(nm) {
						blocking = l
						break
					}
				}
				if blocking == nil {
					continue
				}
				var f *finding
				for _, x := range found {
					if x.blocking == blocking && x.mgmt == nm {
						f = x
					}
				}
				if f == nil {
					f = &finding{blocking: blocking, mgmt: nm}
					found = append(found, f)
				}
				var c *cut
				for _, x := range f.cuts {
					if x.failed == failed {
						c = x
					}
				}
				if c == nil {
					c = &cut{failed: failed}
					f.cuts = append(f.cuts, c)
				}
				c.devices = append(c.devices, fmt.Sprintf("%q", d.Name))
			}

			for _, f := range found {
				var parts []string
				for _, c := range f.cuts {
					parts = append(parts, fmt.Sprintf("при відмові зʼєднання %s – %s — %s",
						c.failed.A.String(), c.failed.B.String(), strings.Join(c.devices, ", ")))
				}
				how := "резервне зʼєднання " + sl.A.String() + " – " + sl.B.String()
				if f.blocking != sl {
					how = "зʼєднання " + f.blocking.A.String() + " – " + f.blocking.B.String() + " на резервному шляху через " + sl.A.String() + " – " + sl.B.String()
				}
				path := f.blocking.Path
				if f.blocking.VLANs != nil {
					path += ".vlans"
				}
				out = append(out, report(m, diag.W006, diag.Warning, path, fmt.Sprintf(
					"%s не пропускає mgmt-мережу %q, тож пристрої можуть втратити керування: %s",
					how, f.mgmt.Name, strings.Join(parts, "; "))))
			}
		}
	}
	return out
}

func commonPrefix(a, b []*ir.Link) int {
	k := 0
	for k < len(a) && k < len(b) && a[k] == b[k] {
		k++
	}
	return k
}

// between returns the tree path from device a to device b: up from a to
// their lowest common ancestor, then down to b.
func between(t *ir.L2Tree, a, b *ir.Device) []*ir.Link {
	pa, pb := t.Path(a), t.Path(b)
	k := commonPrefix(pa, pb)
	var out []*ir.Link
	for i := len(pa) - 1; i >= k; i-- {
		out = append(out, pa[i])
	}
	return append(out, pb[k:]...)
}
