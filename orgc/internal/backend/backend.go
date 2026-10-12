// Package backend defines what the core may ask of a target platform, plus
// the registry of available platforms. The core asks the backend for
// anything platform-specific — the supported OS versions, the management
// ports E006 checks, the capabilities W002 checks — so that no vendor
// knowledge leaks into the core (docs/adr/ADR-008, decision 5).
//
// Rendering is split the same way: a platform renders its own devices, and
// generators render what describes the model as a whole (render.go).
package backend

import (
	"fmt"
	"sort"

	"orgc/internal/ir"
)

// Platform is a target platform as verification sees it. Every method takes
// the device's os_version as written in the model: the format of a version
// belongs to the platform, and only the platform parses it.
type Platform interface {
	// Type is the device.type value this platform serves.
	Type() ir.DeviceType

	// CheckVersion reports whether a device with this version can be
	// rendered at all. supported describes the accepted range in words the
	// administrator can act on, e.g. "RouterOS 6.41–6.x"; it is returned
	// whatever ok is, so a diagnostic can always name it.
	CheckVersion(version string) (ok bool, supported string)

	// ManagementPorts lists the transport Ansible applies configuration
	// over — what E006 needs open from the control node. It is nil for a
	// version CheckVersion rejects.
	ManagementPorts(version string) []ir.PortSpec

	// Capabilities lists what the platform can do at this version. It is
	// empty for a version CheckVersion rejects.
	Capabilities(version string) CapSet

	// Render produces the configuration of one device of this platform
	// (docs/adr/ADR-009, decision 1). It is called only on a model that
	// passed verification, so the device's version is supported.
	Render(m *ir.Model, d *ir.Device) []File
}

// Platforms finds the platform serving a device type.
type Platforms interface {
	Lookup(t ir.DeviceType) (Platform, bool)
}

// Registry is the set of platforms orgc was built with. It is assembled
// outside the core — in cmd/orgc — so the core never imports a vendor
// package.
type Registry struct {
	byType map[ir.DeviceType]Platform
}

// NewRegistry builds a registry. Two platforms claiming the same device type
// are a defect of orgc, not of a model, hence the error.
func NewRegistry(ps ...Platform) (*Registry, error) {
	r := &Registry{byType: make(map[ir.DeviceType]Platform, len(ps))}
	for _, p := range ps {
		t := p.Type()
		if _, dup := r.byType[t]; dup {
			return nil, fmt.Errorf("тип пристрою %q обслуговують дві платформи", t)
		}
		r.byType[t] = p
	}
	return r, nil
}

// Lookup returns the platform for a device type.
func (r *Registry) Lookup(t ir.DeviceType) (Platform, bool) {
	p, ok := r.byType[t]
	return p, ok
}

// Types lists the registered device types in sorted order.
func (r *Registry) Types() []ir.DeviceType {
	ts := make([]ir.DeviceType, 0, len(r.byType))
	for t := range r.byType {
		ts = append(ts, t)
	}
	sort.Slice(ts, func(i, j int) bool { return ts[i] < ts[j] })
	return ts
}
