// Package schema carries the contract of the organization model: the JSON
// Schema every organization.yml is validated against.
//
// The schema file lives in this directory rather than at the repository root
// because go:embed cannot reach outside the directory of its own package.
// See docs/adr/ADR-006-go-dependencies-schema-embedding.md.
package schema

import _ "embed"

// Organization is the JSON Schema (draft 2020-12) describing organization.yml.
//
// It is embedded into the binary so that orgc verify runs on a machine that
// has no copy of the repository — a precondition of experiment E1 and a
// deliberate guard against depending on the lab.
//
//go:embed organization.schema.json
var Organization []byte
