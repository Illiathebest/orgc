// Package verify implements the invariants E001-E010 and W001-W006 over the
// intermediate representation. E006, reachability of management, is the core
// check of the work.
//
// The contract is fixed by docs/adr/ADR-008, decision 3:
//
//   - An invariant is a pure function of the IR and the platforms. It reads
//     no files, changes nothing, and knows nothing of other invariants.
//   - The registry runs them in a fixed order. Output order does not depend
//     on it — the result is sorted — but the computation is reproducible
//     step by step, and E1 can time each check on its own.
//   - No cascades. An invariant skips an element whose prerequisite is
//     already broken and reported under another code, so that each defect
//     yields exactly one code; E2 relies on that.
//   - Invariants never see a model with structural errors: the caller stops
//     before lowering.
package verify

import (
	"fmt"

	"orgc/internal/backend"
	"orgc/internal/diag"
	"orgc/internal/ir"
)

// Check is one invariant.
type Check struct {
	// Code is the only code Run may emit. Run enforces it: a check that
	// reported under someone else's code would break "one defect, one
	// code" in a way no fixture would notice.
	Code diag.Code
	Run  func(m *ir.Model, p backend.Platforms) []diag.Diagnostic
}

// Checks returns the registry in run order. The order is that of ADR-008:
// identifiers and references first, since everything else relies on them;
// then the address plan, roles and policies, platforms; E006 once both
// addresses and platforms are known; warnings last.
//
//	E002 E003 E001 E007 E008 E010 E009 E004 E005 W002 E006 W001 W003 W004 W005 W006
//
// Entries are added as the invariants are implemented.
func Checks() []Check {
	return []Check{
		{Code: diag.E002, Run: checkE002},
		{Code: diag.E003, Run: checkE003},
		{Code: diag.E001, Run: checkE001},
		{Code: diag.E007, Run: checkE007},
		{Code: diag.E008, Run: checkE008},
		{Code: diag.E010, Run: checkE010},
		{Code: diag.E009, Run: checkE009},
		{Code: diag.E004, Run: checkE004},
		{Code: diag.E005, Run: checkE005},
		{Code: diag.W002, Run: checkW002},
		{Code: diag.E006, Run: checkE006},
		{Code: diag.W001, Run: checkW001},
		{Code: diag.W003, Run: checkW003},
		{Code: diag.W004, Run: checkW004},
		{Code: diag.W005, Run: checkW005},
		{Code: diag.W006, Run: checkW006},
	}
}

// Run applies the checks to the model and returns their findings sorted.
//
// The error is never about the model: it means a check broke its contract,
// which is a defect of orgc, the same class as a failure of Load or Lower.
func Run(m *ir.Model, p backend.Platforms, checks []Check) ([]diag.Diagnostic, error) {
	var out []diag.Diagnostic
	for _, c := range checks {
		for _, d := range c.Run(m, p) {
			if d.Code != c.Code {
				return nil, fmt.Errorf("перевірка %s видала діагностику з чужим кодом %s", c.Code, d.Code)
			}
			out = append(out, d)
		}
	}
	diag.Sort(out)
	return out, nil
}

// report builds a diagnostic about the value at path, located through the
// IR's position index. Every invariant reports through it, so none can
// forget the file name or the line.
func report(m *ir.Model, code diag.Code, sev diag.Severity, path, msg string) diag.Diagnostic {
	pos := m.At(path)
	return diag.Diagnostic{
		Code:     code,
		Severity: sev,
		Message:  msg,
		File:     m.File,
		Line:     pos.Line,
		Column:   pos.Column,
		Path:     path,
	}
}
