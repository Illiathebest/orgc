package diag

import (
	"fmt"
	"io"
	"strconv"
	"strings"
)

// String renders one diagnostic as a single line.
//
// The line starts with file:line:column so that editors and CI logs can jump
// straight to the defect — the same convention compilers use. Position is
// omitted when unknown, which happens for findings about the document as a
// whole, such as a missing top-level field.
//
//	organization.yml:23:42: помилка S005: значення поза переліком дозволених — sites[0].networks[2].role
//	organization.yml: помилка S002: відсутнє обовʼязкове поле control_node
func (d Diagnostic) String() string {
	var b strings.Builder

	if d.File != "" {
		b.WriteString(d.File)
		if d.Line > 0 {
			b.WriteByte(':')
			b.WriteString(strconv.Itoa(d.Line))
			if d.Column > 0 {
				b.WriteByte(':')
				b.WriteString(strconv.Itoa(d.Column))
			}
		}
		b.WriteString(": ")
	}

	b.WriteString(d.Severity.String())
	if d.Code != "" {
		b.WriteByte(' ')
		b.WriteString(string(d.Code))
	}
	b.WriteString(": ")
	b.WriteString(d.Text())

	if d.Path != "" {
		b.WriteString(" — ")
		b.WriteString(d.Path)
	}
	return b.String()
}

// Render sorts the diagnostics and writes them one per line.
//
// Sorting happens here rather than at the call site so that no caller can
// forget it: byte-identical output for identical input is a rule of the
// project, and the validator's own order is not reproducible.
//
// Line endings are LF on every platform, for the same reason.
func Render(w io.Writer, ds []Diagnostic) error {
	sorted := make([]Diagnostic, len(ds))
	copy(sorted, ds)
	Sort(sorted)

	for _, d := range sorted {
		if _, err := io.WriteString(w, d.String()+"\n"); err != nil {
			return err
		}
	}
	return nil
}

// Summary is the closing line of a run.
func Summary(ds []Diagnostic) string {
	errors, warnings := Count(ds)

	switch {
	case errors == 0 && warnings == 0:
		return "OK — помилок не знайдено"
	case warnings == 0:
		return fmt.Sprintf("Знайдено %d %s", errors, pluralErrors(errors))
	case errors == 0:
		return fmt.Sprintf("Знайдено %d %s", warnings, pluralWarnings(warnings))
	default:
		return fmt.Sprintf("Знайдено %d %s і %d %s",
			errors, pluralErrors(errors), warnings, pluralWarnings(warnings))
	}
}

// pluralErrors and pluralWarnings pick the Ukrainian form after "Знайдено",
// which governs the accusative: 1 помилку, 2 помилки, 5 помилок.
func pluralErrors(n int) string {
	return plural(n, "помилку", "помилки", "помилок")
}
func pluralWarnings(n int) string {
	return plural(n, "попередження", "попередження", "попереджень")
}

// plural applies the Ukrainian rule. The 11-14 exception is checked first:
// 11 ends in 1 but still takes the "many" form.
func plural(n int, one, few, many string) string {
	n = abs(n)
	if r := n % 100; r >= 11 && r <= 14 {
		return many
	}
	switch n % 10 {
	case 1:
		return one
	case 2, 3, 4:
		return few
	default:
		return many
	}
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
