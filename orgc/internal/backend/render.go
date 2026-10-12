package backend

import "orgc/internal/ir"

// File is one generated file. Path is relative to the output directory and
// uses "/" on every platform, e.g. "configs/hq/rb750.rsc"
// (docs/adr/ADR-009, decision 2).
type File struct {
	Path string
	Data []byte
}

// Generator renders something that describes the model as a whole rather
// than one device: the Ansible inventory, monitoring targets, the lab
// topology. When it needs platform knowledge, it asks the platform through
// an extra interface the platform implements; it never names a vendor.
type Generator interface {
	// Name identifies the generator in diagnostics and tests.
	Name() string

	// Generate renders the model. It has no error result: the model has
	// passed verification, and a device that cannot be rendered has already
	// been reported as W002 at error level.
	Generate(m *ir.Model, p Platforms) []File
}

// Var is one variable of a generated file, kept in a fixed order.
type Var struct {
	Name  string
	Value string
}

// AnsibleTarget is what a platform tells the Ansible generator about its
// devices: the inventory group they belong to and the group's connection
// variables. Secrets never appear here — only a lookup of an environment
// variable or a reference to an ansible-vault variable
// (docs/adr/ADR-009, decision 4).
type AnsibleTarget interface {
	AnsibleGroup() string
	AnsibleVars() []Var
}
