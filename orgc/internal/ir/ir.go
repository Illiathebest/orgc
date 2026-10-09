// Package ir holds the vendor-neutral intermediate representation: the model
// after its references are resolved and its addresses parsed. Nothing here
// knows about RouterOS, EOS or any other platform.
//
// The IR is what the invariants run over (docs/adr/ADR-008). Three properties
// matter more than convenience:
//
//   - It survives a defective model. A reference that does not resolve keeps
//     its name and a nil target; reporting it is E003's job, and every other
//     invariant still runs.
//   - Every element knows where it came from. Origin carries the readable
//     path, and Model.At turns any path below it into a line and column.
//   - It is deterministic. Elements keep the order of the source file; maps
//     appear only where they are read by key and never iterated.
package ir

import (
	"net/netip"

	"orgc/internal/diag"
)

// Model is a whole lowered organization.
type Model struct {
	// File is the name diagnostics are reported against.
	File string

	Organization string
	Profile      Profile

	// Sites, Networks, Devices and Servers are listed in source order.
	// Networks, Devices and Servers span all sites — site after site, and
	// within a site in the order they are written — because several
	// invariants range over the whole model regardless of site.
	Sites    []*Site
	Networks []*Network
	Devices  []*Device
	Servers  []*Server

	ControlNode ControlNode
	Rules       []*Rule

	// Positions maps a readable path to its place in the source. It is
	// read by key only.
	Positions map[string]diag.Pos
}

// At returns the source position of a path, e.g. n.Path + ".cidr". The zero
// Pos means unknown: a model built in code, or a path the file never had.
func (m *Model) At(path string) diag.Pos { return m.Positions[path] }

// Origin records which element of the model an IR element was lowered from.
type Origin struct {
	// Path is the readable path, e.g. "sites[0].networks[2]".
	Path string
}

// Profile is the scale of the organization.
type Profile string

const (
	ProfileS Profile = "S"
	ProfileM Profile = "M"
)

// Site is one physical location.
type Site struct {
	Name         string
	AllowOverlap bool

	Networks []*Network
	Devices  []*Device
	Servers  []*Server
	Uplinks  []*Uplink

	// Gateway is the site's L3 gateway: by the model v2 convention, the one
	// device whose role is edge_router or firewall. It is set only when
	// exactly one such device exists, so that checks relying on it never
	// have to guess.
	Gateway *Device

	// GatewayCandidates lists every edge_router and firewall of the site.
	// E003 reads it to tell "no gateway" from "two gateways"; nothing else
	// should.
	GatewayCandidates []*Device

	Origin
}

// UplinkKind says whether a link is the main one or a reserve.
type UplinkKind string

const (
	UplinkPrimary UplinkKind = "primary"
	UplinkBackup  UplinkKind = "backup"
)

// Uplink is an external link of a site. Model v2 gives it no addressing.
type Uplink struct {
	Name string
	Kind UplinkKind
	Site *Site
	Origin
}

// Role is the purpose of a network segment.
type Role string

const (
	RoleMgmt    Role = "mgmt"
	RoleUsers   Role = "users"
	RoleServers Role = "servers"
	RoleGuest   Role = "guest"
)

// Network is one VLAN segment.
type Network struct {
	Name string
	VLAN int
	Role Role

	// Prefix is kept exactly as written, host bits included, so that a
	// check can still see them; Overlaps and Contains are unaffected.
	Prefix netip.Prefix

	// Gateway is the zero Addr when the network has none.
	Gateway netip.Addr

	// Pool is nil when the network has no DHCP pool.
	Pool *Range

	// Isolated forbids traffic between hosts of this very network.
	Isolated bool

	Site *Site
	Origin
}

// Range is an inclusive address range.
type Range struct {
	From netip.Addr
	To   netip.Addr
}

// DeviceType is the platform family of a device. Its values are the model's
// own vocabulary; which backend serves which type is decided outside the
// core.
type DeviceType string

// DeviceRole is the function of a device in its site.
type DeviceRole string

const (
	DeviceEdgeRouter         DeviceRole = "edge_router"
	DeviceFirewall           DeviceRole = "firewall"
	DeviceCoreSwitch         DeviceRole = "core_switch"
	DeviceDistributionSwitch DeviceRole = "distribution_switch"
	DeviceAccessSwitch       DeviceRole = "access_switch"
)

// RoutesL3 reports whether the role makes a device the L3 gateway of its
// site under the model v2 convention.
func (r DeviceRole) RoutesL3() bool {
	return r == DeviceEdgeRouter || r == DeviceFirewall
}

// Device is a network device.
type Device struct {
	Name string
	Type DeviceType

	// OSVersion stays a string: its format belongs to the platform
	// ("6.48.6" against "4.29.2F"), and only the backend parses it.
	OSVersion string

	Role   DeviceRole
	MgmtIP netip.Addr
	Site   *Site
	Origin
}

// ServerRole is a workload a server carries.
type ServerRole string

// Server is a workload host.
type Server struct {
	Name  string
	Roles []ServerRole
	IP    netip.Addr
	Site  *Site
	Origin
}

// ControlNode is where configuration is applied from — the starting point of
// E006. It need not be one of the servers.
type ControlNode struct {
	// Site and Network are references; either may be unresolved. Network is
	// looked up among the networks of Site only, so it is unresolved
	// whenever Site is.
	Site    SiteRef
	Network NetRef

	IP netip.Addr
	Origin
}

// SiteRef is a reference to a site by name.
type SiteRef struct {
	Name string
	Site *Site // nil when unresolved
}

// NetRef is a reference to a network by name.
type NetRef struct {
	Name string
	Net  *Network // nil when unresolved
}

// EndpointKind says what a rule endpoint refers to.
type EndpointKind int

const (
	EndpointNetwork  EndpointKind = iota // one network of the model
	EndpointAny                          // every network of the model, not the internet
	EndpointInternet                     // everything outside the model
)

// Endpoint is one side of a rule.
type Endpoint struct {
	Kind EndpointKind

	// Name is the segment reference as written.
	Name string

	// Net is the referenced network for EndpointNetwork, nil when the name
	// does not resolve. It is always nil for the other kinds.
	Net *Network
}

// Resolved reports whether the endpoint means something. Only a network
// reference can fail to.
func (e Endpoint) Resolved() bool { return e.Kind != EndpointNetwork || e.Net != nil }

// Proto is a transport protocol of a rule.
type Proto string

const (
	TCP Proto = "tcp"
	UDP Proto = "udp"
)

// PortSpec is one destination port of one protocol.
type PortSpec struct {
	Proto Proto
	Port  int
}

// PortSet is the traffic a rule admits.
type PortSet struct {
	// Any is true when the rule lists no ports: every protocol and port,
	// ICMP included.
	Any bool

	// Ports is sorted by protocol, then port. Empty when Any is set.
	Ports []PortSpec
}

// Admits reports whether the set lets a flow to the given port through.
func (s PortSet) Admits(p PortSpec) bool {
	if s.Any {
		return true
	}
	for _, q := range s.Ports {
		if q == p {
			return true
		}
	}
	return false
}

// Rule opens one flow. Rules are stateful and non-transitive; their order
// carries no meaning, so the IR keeps source order only for stable output.
// There is no action: the schema admits nothing but allow.
type Rule struct {
	From  Endpoint
	To    Endpoint
	Ports PortSet
	Origin
}
