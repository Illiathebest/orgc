// Package diag defines a diagnostic: code, severity, Ukrainian message, path
// to the model element, and position in the source file.
//
// Ordering is the package's responsibility, not the caller's: the JSON Schema
// validator emits findings in map-iteration order, which differs between runs.
// Diagnostics are sorted by (file, line, column, code, path) so that the same
// input always produces byte-identical output.
//
// Filled in at plan step 12.
package diag
