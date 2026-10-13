// Package model parses organization.yml into Go structures and validates it
// against the embedded JSON Schema, binding every finding to a line and
// column in the source file.
//
// The structures mirror the document, not the semantics: enumerated values
// stay plain strings here. Turning "mgmt" into a typed role, and a CIDR
// string into a prefix, is the job of the intermediate representation —
// this package only answers whether the file says what the schema allows.
package model

// Organization is a whole organization.yml.
type Organization struct {
	Organization string      `yaml:"organization"`
	Profile      string      `yaml:"profile"`
	Sites        []Site      `yaml:"sites"`
	ControlNode  ControlNode `yaml:"control_node"`
	Policies     *Policies   `yaml:"policies,omitempty"`
}

// ControlNode is the node configurations are applied from — the starting
// point of the management-reachability check (E006).
type ControlNode struct {
	Site    string `yaml:"site"`
	Network string `yaml:"network"`
	IP      string `yaml:"ip"`
}

// Site is one physical location.
type Site struct {
	Name string `yaml:"name"`

	// AllowOverlap permits this site's prefixes to overlap with those of
	// other sites. It never permits an overlap inside the site itself.
	AllowOverlap bool `yaml:"allow_overlap,omitempty"`

	Networks []Network `yaml:"networks"`
	Uplinks  []Uplink  `yaml:"uplinks,omitempty"`
	Devices  []Device  `yaml:"devices,omitempty"`
	Servers  []Server  `yaml:"servers,omitempty"`

	// Links are the cables between device ports. A site without links is
	// built as a star from its L3 gateway by platform convention; a site
	// with links describes its topology completely.
	Links []Link `yaml:"links,omitempty"`
}

// Link is one cable. Both ends are trunk ports, written "device:port".
type Link struct {
	A string `yaml:"a"`
	B string `yaml:"b"`

	// VLANs names the networks of the site the trunk carries; absent means
	// all of them.
	VLANs []string `yaml:"vlans,omitempty"`

	// Standby marks a redundant link: it closes one cycle with the tree of
	// active links and STP keeps it blocked until a link of that cycle fails.
	Standby bool `yaml:"standby,omitempty"`
}

// Network is one VLAN segment. Name is unique across the whole model, not
// just the site: policies refer to a segment by bare name.
type Network struct {
	VLAN     int       `yaml:"vlan"`
	Name     string    `yaml:"name"`
	Role     string    `yaml:"role"`
	CIDR     string    `yaml:"cidr"`
	Gateway  string    `yaml:"gateway,omitempty"`
	DHCPPool *DHCPPool `yaml:"dhcp_pool,omitempty"`

	// Isolated forbids traffic between hosts of this very network.
	// Isolation between segments already comes from default-deny.
	Isolated bool `yaml:"isolated,omitempty"`
}

// DHCPPool is the address range handed out by DHCP.
type DHCPPool struct {
	From string `yaml:"from"`
	To   string `yaml:"to"`
}

// Uplink is an external link of a site. It carries no addressing: model v2
// records only that the link exists.
type Uplink struct {
	Name string `yaml:"name"`
	Kind string `yaml:"kind"`

	// Port is the port of the site's L3 gateway the link leaves through,
	// "device:port"; empty means the platform's convention.
	Port string `yaml:"port,omitempty"`
}

// Device is a network device.
type Device struct {
	Name      string `yaml:"name"`
	Type      string `yaml:"type"`
	OSVersion string `yaml:"os_version"`
	Role      string `yaml:"role"`
	MgmtIP    string `yaml:"mgmt_ip"`

	// AccessPorts are untagged ports for hosts the model does not name.
	AccessPorts []AccessPorts `yaml:"access_ports,omitempty"`
}

// AccessPorts puts a group of ports into the VLAN of one network.
type AccessPorts struct {
	Ports   []string `yaml:"ports"`
	Network string   `yaml:"network"`
}

// Server is a workload host.
type Server struct {
	Name string   `yaml:"name"`
	Role []string `yaml:"role"`
	IP   string   `yaml:"ip"`

	// Port is the access port the server is plugged into, "device:port".
	Port string `yaml:"port,omitempty"`
}

// Policies describes permitted traffic. Absence of a rule is a denial.
type Policies struct {
	InterVLAN []Rule `yaml:"inter_vlan,omitempty"`
}

// Rule opens one flow. Rules are stateful and non-transitive, and their
// order carries no meaning.
type Rule struct {
	From   string `yaml:"from"`
	To     string `yaml:"to"`
	Ports  []int  `yaml:"ports,omitempty"`
	Proto  string `yaml:"proto,omitempty"`
	Action string `yaml:"action"`
}
