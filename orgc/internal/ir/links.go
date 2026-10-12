package ir

// PortRef is a reference to a port of a device, written "device:port".
type PortRef struct {
	// DeviceName and Port are the two halves as written.
	DeviceName string
	Port       string

	// Device is the referenced device of the same site, nil when the name
	// does not resolve there. E003 reports it.
	Device *Device

	// Origin is the field holding the reference, e.g. "sites[0].links[2].a".
	Origin
}

// Resolved reports whether the device of the reference was found.
func (p *PortRef) Resolved() bool { return p != nil && p.Device != nil }

// String renders the reference as written.
func (p *PortRef) String() string { return p.DeviceName + ":" + p.Port }

// Link is one cable between two trunk ports of the same site.
type Link struct {
	A, B PortRef

	// VLANs are the networks the trunk carries. Nil means every network of
	// the site; a name that does not resolve keeps a nil Net (E003).
	VLANs []NetRef

	Site *Site
	Origin
}

// Carries reports whether the trunk carries the network. An unresolved name
// in VLANs carries nothing.
func (l *Link) Carries(n *Network) bool {
	if l.VLANs == nil {
		return true
	}
	for _, v := range l.VLANs {
		if v.Net == n {
			return true
		}
	}
	return false
}

// Resolved reports whether both ends were found.
func (l *Link) Resolved() bool { return l.A.Resolved() && l.B.Resolved() }

// Other returns the device at the other end from d.
func (l *Link) Other(d *Device) *Device {
	if l.A.Device == d {
		return l.B.Device
	}
	return l.A.Device
}

// AccessPorts puts ports of one device into the VLAN of one network.
type AccessPorts struct {
	Ports   []string
	Network NetRef // looked up among the networks of the device's site
	Device  *Device
	Origin
}

// LinkFault says why a link is not part of the tree.
type LinkFault int

const (
	FaultCycle    LinkFault = iota // closes a cycle with links earlier in the file
	FaultSelf                      // both ends on one device
	FaultParallel                  // a second link between the same two devices
)

// ExtraLink is a link the tree cannot contain.
type ExtraLink struct {
	Link  *Link
	Fault LinkFault
}

// L2Tree is a site's topology rooted at its L3 gateway. It is what the
// connection checks and the L2 part of E006 read, and what the backends
// will configure trunks from.
//
// Links that would make the graph anything but a forest are set aside in
// Extra, in file order: the link that closes a cycle is the later one. The
// remaining links form a forest, and the tree is the part of it reachable
// from the gateway, so the path from the gateway to a device is unique.
type L2Tree struct {
	// Root is the site's gateway; nil when the site has none or several
	// (E003), in which case nothing is reached.
	Root *Device

	// Order lists reached devices breadth first, the root first.
	Order []*Device

	// Up is the link from a reached device towards the root.
	Up map[*Device]*Link

	Extra []ExtraLink

	// Incomplete is set when some link has an end or a VLAN name that did
	// not resolve. The tree is built from what resolved, but checks that
	// need the whole topology skip the site: E003 explains it.
	Incomplete bool

	// carried holds, per reached device, the networks every link on its
	// path carries, as a bit set over the site's networks.
	carried map[*Device][]uint64
	index   map[*Network]int
}

// Reached reports whether the device has a path to the root.
func (t *L2Tree) Reached(d *Device) bool { _, ok := t.carried[d]; return ok }

// Path returns the links from the root down to d; empty for the root itself
// and nil for a device that is not reached.
func (t *L2Tree) Path(d *Device) []*Link {
	if !t.Reached(d) {
		return nil
	}
	var up []*Link
	for v := d; v != t.Root; {
		l := t.Up[v]
		up = append(up, l)
		v = l.Other(v)
	}
	path := make([]*Link, len(up))
	for i, l := range up {
		path[len(up)-1-i] = l
	}
	return path
}

// Carried reports whether every link on the path from the root to d
// carries the network. It is false for a device that is not reached and for
// a network of another site.
func (t *L2Tree) Carried(d *Device, n *Network) bool {
	set, ok := t.carried[d]
	if !ok {
		return false
	}
	i, ok := t.index[n]
	return ok && set[i/64]&(1<<(i%64)) != 0
}

// Blocking returns the first link on the path from the root to d that does
// not carry the network, or nil when the network is carried all the way.
func (t *L2Tree) Blocking(d *Device, n *Network) *Link {
	for _, l := range t.Path(d) {
		if !l.Carries(n) {
			return l
		}
	}
	return nil
}

// BuildL2 builds the tree of a site. Lowering calls it once the site's
// devices and links are resolved.
//
// Cycles are found with a union-find pass over every resolved link in file
// order, so a cycle among devices the gateway cannot reach is found too.
// One breadth-first walk from the root then fixes the parent links and
// intersects the carried networks: O(|D| + |L|·⌈|N|/64⌉) for a site with D
// devices, L links and N networks.
func BuildL2(s *Site) *L2Tree {
	t := &L2Tree{
		Root:    s.Gateway,
		Up:      map[*Device]*Link{},
		carried: map[*Device][]uint64{},
		index:   map[*Network]int{},
	}
	words := (len(s.Networks) + 63) / 64
	for i, n := range s.Networks {
		t.index[n] = i
	}

	parent := map[*Device]*Device{}
	var find func(*Device) *Device
	find = func(d *Device) *Device {
		p, ok := parent[d]
		if !ok || p == d {
			parent[d] = d
			return d
		}
		r := find(p)
		parent[d] = r
		return r
	}

	type pair struct{ a, b *Device }
	seen := map[pair]bool{}
	adj := map[*Device][]*Link{}
	for _, l := range s.Links {
		for _, v := range l.VLANs {
			if v.Net == nil {
				t.Incomplete = true
			}
		}
		if !l.Resolved() {
			t.Incomplete = true
			continue
		}
		a, b := l.A.Device, l.B.Device
		switch {
		case a == b:
			t.Extra = append(t.Extra, ExtraLink{l, FaultSelf})
			continue
		case seen[pair{a, b}]:
			t.Extra = append(t.Extra, ExtraLink{l, FaultParallel})
			continue
		}
		seen[pair{a, b}], seen[pair{b, a}] = true, true
		if ra, rb := find(a), find(b); ra == rb {
			t.Extra = append(t.Extra, ExtraLink{l, FaultCycle})
			continue
		} else {
			parent[ra] = rb
		}
		adj[a] = append(adj[a], l)
		adj[b] = append(adj[b], l)
	}

	if t.Root == nil {
		return t
	}
	all := make([]uint64, words)
	for i := range s.Networks {
		all[i/64] |= 1 << (i % 64)
	}
	t.carried[t.Root] = all
	t.Order = []*Device{t.Root}
	for q := 0; q < len(t.Order); q++ {
		v := t.Order[q]
		for _, l := range adj[v] {
			w := l.Other(v)
			if t.Reached(w) {
				continue
			}
			set := make([]uint64, words)
			for i, n := range s.Networks {
				if l.Carries(n) {
					set[i/64] |= 1 << (i % 64)
				}
			}
			for i := range set {
				set[i] &= t.carried[v][i]
			}
			t.carried[w] = set
			t.Up[w] = l
			t.Order = append(t.Order, w)
		}
	}
	return t
}
