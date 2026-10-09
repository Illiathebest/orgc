// Package backend defines the interface a target platform implements, plus the
// registry of available backends. The core asks the backend for anything
// platform-specific — including the management ports used by E006 — so that no
// vendor knowledge leaks into the core.
package backend
