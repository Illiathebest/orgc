package model

import (
	"strings"
	"testing"

	"orgc/internal/diag"
)

// Invariants report against paths in the intermediate representation, so
// every path they can name must resolve to the line a reader expects.
func TestPositionsIndex(t *testing.T) {
	doc, ds, err := LoadBytes("organization.yml", []byte(minimal))
	if err != nil || len(ds) != 0 {
		t.Fatalf("err = %v, ds = %v", err, ds)
	}

	tests := []struct {
		path string
		want diag.Pos
	}{
		// scalars, block and flow style
		{"profile", diag.Pos{Line: 2, Column: 10}},
		{"control_node.ip", diag.Pos{Line: 6, Column: 7}},
		{"sites[0].name", diag.Pos{Line: 8, Column: 11}},
		{"sites[0].networks[0].cidr", diag.Pos{Line: 10, Column: 57}},
		{"sites[0].devices[0].mgmt_ip", diag.Pos{Line: 16, Column: 18}},

		// A mapping is located by its first key, whatever its style.
		{"control_node", diag.Pos{Line: 4, Column: 3}},
		{"sites[0]", diag.Pos{Line: 8, Column: 5}},
		{"sites[0].networks[0]", diag.Pos{Line: 10, Column: 11}},
		{"sites[0].devices[0]", diag.Pos{Line: 12, Column: 9}},

		// A sequence is located by its first entry marker.
		{"sites", diag.Pos{Line: 8, Column: 3}},

		// The document root carries no position by design, and an absent
		// path is not an error: it is simply unknown.
		{"", diag.Pos{}},
		{"policies.inter_vlan[0]", diag.Pos{}},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := doc.Positions.At(tt.path); got != tt.want {
				t.Errorf("At(%q) = %+v, want %+v", tt.path, got, tt.want)
			}
		})
	}
}

// An anchor must not hide the value it decorates from the index.
func TestPositionsSeeThroughAnchors(t *testing.T) {
	src := strings.Replace(minimal, "  ip: 192.168.30.10", "  ip: &cn 192.168.30.10", 1)

	doc, ds, err := LoadBytes("organization.yml", []byte(src))
	if err != nil || len(ds) != 0 {
		t.Fatalf("err = %v, ds = %v", err, ds)
	}
	if got := doc.Positions.At("control_node.ip"); got.Line != 6 {
		t.Errorf("At(control_node.ip) = %+v, want line 6", got)
	}
}

func TestDocumentCarriesFileName(t *testing.T) {
	doc, _, err := LoadBytes("hq.yml", []byte(minimal))
	if err != nil {
		t.Fatal(err)
	}
	if doc.File != "hq.yml" {
		t.Errorf("File = %q, want %q", doc.File, "hq.yml")
	}
}
