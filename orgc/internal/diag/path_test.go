package diag

import "testing"

func TestPathFromPointer(t *testing.T) {
	tests := []struct {
		ptr  string
		want string
	}{
		{"", ""},
		{"/", ""},
		{"/profile", "profile"},
		{"/sites/0", "sites[0]"},
		{"/sites/0/name", "sites[0].name"},
		{"/sites/0/networks/2/role", "sites[0].networks[2].role"},
		{"/policies/inter_vlan/1/from", "policies.inter_vlan[1].from"},
		{"/sites/0/networks/4/dhcp_pool/to", "sites[0].networks[4].dhcp_pool.to"},
		{"/control_node/ip", "control_node.ip"},
		{"/sites/10/devices/123", "sites[10].devices[123]"},

		// RFC 6901 escapes. ~01 must come out as ~1, which only holds if
		// ~1 is resolved before ~0.
		{"/a~1b", "a/b"},
		{"/a~0b", "a~b"},
		{"/a~01b", "a~1b"},

		// Not indices: a leading zero and a sign make it a field name.
		{"/sites/01", "sites.01"},
		{"/sites/-1", "sites.-1"},
	}

	for _, tt := range tests {
		if got := PathFromPointer(tt.ptr); got != tt.want {
			t.Errorf("PathFromPointer(%q) = %q, want %q", tt.ptr, got, tt.want)
		}
	}
}

// The pointers below are the ones the validator actually produced against
// examples/organization.broken.yml while ADR-006 was being written.
func TestPathFromPointerOnRealValidatorOutput(t *testing.T) {
	tests := map[string]string{
		"/profile":                     "profile",
		"/sites/0/name":                "sites[0].name",
		"/sites/0/allow_overlap":       "sites[0].allow_overlap",
		"/sites/0/uplinks/0/kind":      "sites[0].uplinks[0].kind",
		"/sites/0/networks/0/vlan":     "sites[0].networks[0].vlan",
		"/sites/0/networks/5/name":     "sites[0].networks[5].name",
		"/sites/0/devices/2":           "sites[0].devices[2]",
		"/sites/0/servers/1/role/0":    "sites[0].servers[1].role[0]",
		"/policies/inter_vlan/3/ports": "policies.inter_vlan[3].ports",
		"/policies/inter_vlan/4":       "policies.inter_vlan[4]",
	}
	for ptr, want := range tests {
		if got := PathFromPointer(ptr); got != want {
			t.Errorf("PathFromPointer(%q) = %q, want %q", ptr, got, want)
		}
	}
}

func TestIsIndex(t *testing.T) {
	yes := []string{"0", "1", "9", "10", "123"}
	no := []string{"", "01", "-1", "+1", "1a", "a", "1.0", " 1"}

	for _, s := range yes {
		if !isIndex(s) {
			t.Errorf("isIndex(%q) = false, want true", s)
		}
	}
	for _, s := range no {
		if isIndex(s) {
			t.Errorf("isIndex(%q) = true, want false", s)
		}
	}
}
