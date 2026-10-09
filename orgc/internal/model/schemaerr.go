package model

import (
	"fmt"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"

	"orgc/internal/diag"
)

// collect turns a validation failure into diagnostics.
//
// The tree is walked to its leaves rather than read from the flat output:
// the schema is built on $defs, so a flat listing stops at the $ref boundary
// and reports "reference failed" instead of the enum or pattern that
// actually failed (docs/adr/ADR-006).
func collect(ve *jsonschema.ValidationError, file string, loc *locator) []diag.Diagnostic {
	var out []diag.Diagnostic
	walk(ve, file, loc, &out)
	return out
}

func walk(ve *jsonschema.ValidationError, file string, loc *locator, out *[]diag.Diagnostic) {
	if ve == nil {
		return
	}
	if len(ve.Causes) > 0 {
		for _, c := range ve.Causes {
			walk(c, file, loc, out)
		}
		return
	}
	*out = append(*out, describe(ve, file, loc))
}

// describe renders one leaf failure in Ukrainian.
func describe(ve *jsonschema.ValidationError, file string, loc *locator) diag.Diagnostic {
	scalar, _ := loc.scalarAt(ve.InstanceLocation)
	code, msg, key := classify(ve, scalar)

	var pos position
	switch {
	case key != "":
		// The finding is about a key, not a value: point at the key.
		pos = loc.keyAt(ve.InstanceLocation, key)
	case len(ve.InstanceLocation) == 0:
		// A finding about the document as a whole, such as a missing
		// top-level field. The mapping node starts at the first value in
		// the file, and pointing there would blame an unrelated line, so
		// the position is left unknown and the file alone is reported.
		pos = position{}
	default:
		pos = loc.valueAt(ve.InstanceLocation)
	}

	return diag.Diagnostic{
		Code:     code,
		Severity: code.DefaultSeverity(),
		Message:  msg,
		File:     file,
		Line:     pos.line,
		Column:   pos.column,
		Path:     diag.PathFromPointer("/" + strings.Join(ve.InstanceLocation, "/")),
	}
}

// classify maps a validator finding onto a structural code and Ukrainian
// text. The third result names a key the finding is about, so that the
// position can point at the key instead of the enclosing mapping.
func classify(ve *jsonschema.ValidationError, scalar string) (diag.Code, string, string) {
	switch k := ve.ErrorKind.(type) {

	case *kind.Required:
		if len(k.Missing) == 1 {
			return diag.SRequiredMissing,
				"відсутнє обовʼязкове поле " + quoteList(k.Missing), ""
		}
		return diag.SRequiredMissing,
			"відсутні обовʼязкові поля " + quoteList(k.Missing), ""

	case *kind.AdditionalProperties:
		key := ""
		if len(k.Properties) == 1 {
			key = k.Properties[0]
		}
		return diag.SUnknownField,
			"невідоме поле " + quoteList(k.Properties) + "; схема таких не визначає", key

	case *kind.Type:
		return diag.STypeMismatch,
			fmt.Sprintf("очікується %s, отримано %s", typeNames(k.Want), typeName(k.Got)), ""

	case *kind.Enum:
		return diag.SNotInEnum,
			fmt.Sprintf("значення %s неприпустиме; дозволені: %s",
				quoteValue(k.Got), joinValues(k.Want)), ""

	case *kind.Pattern:
		return diag.SFormatMismatch, patternMessage(ve.SchemaURL, k.Got), ""

	case *kind.Format:
		return diag.SFormatMismatch,
			fmt.Sprintf("значення %s не є коректним %s", quoteValue(k.Got), k.Want), ""

	case *kind.Minimum:
		return diag.SOutOfRange,
			fmt.Sprintf("значення %s менше за мінімум %s", k.Got.RatString(), k.Want.RatString()), ""
	case *kind.Maximum:
		return diag.SOutOfRange,
			fmt.Sprintf("значення %s більше за максимум %s", k.Got.RatString(), k.Want.RatString()), ""
	case *kind.ExclusiveMinimum:
		return diag.SOutOfRange,
			fmt.Sprintf("значення %s має бути більшим за %s", k.Got.RatString(), k.Want.RatString()), ""
	case *kind.ExclusiveMaximum:
		return diag.SOutOfRange,
			fmt.Sprintf("значення %s має бути меншим за %s", k.Got.RatString(), k.Want.RatString()), ""

	case *kind.UniqueItems:
		return diag.SArrayConstraint,
			fmt.Sprintf("елементи списку на позиціях %d і %d однакові; дублікати заборонено",
				k.Duplicates[0], k.Duplicates[1]), ""
	case *kind.MinItems:
		return diag.SArrayConstraint,
			fmt.Sprintf("у списку %d елементів, потрібно щонайменше %d", k.Got, k.Want), ""
	case *kind.MaxItems:
		return diag.SArrayConstraint,
			fmt.Sprintf("у списку %d елементів, дозволено щонайбільше %d", k.Got, k.Want), ""
	case *kind.MinLength:
		if k.Want == 1 {
			return diag.SArrayConstraint, "значення не може бути порожнім", ""
		}
		return diag.SArrayConstraint,
			fmt.Sprintf("довжина значення %d символів, потрібно щонайменше %d", k.Got, k.Want), ""
	case *kind.MaxLength:
		return diag.SArrayConstraint,
			fmt.Sprintf("довжина значення %d символів, дозволено щонайбільше %d", k.Got, k.Want), ""

	case *kind.DependentRequired:
		return diag.SFieldDependency,
			fmt.Sprintf("поле %q потребує поля %s", k.Prop, quoteList(k.Missing)), ""

	case *kind.Not:
		// "not" carries no data of its own, so the offending value is read
		// back from the source to make the message actionable.
		if scalar != "" {
			return diag.SValueForbidden,
				fmt.Sprintf("значення %q тут заборонено", scalar), ""
		}
		return diag.SValueForbidden, "таке значення тут заборонено", ""
	case *kind.FalseSchema:
		return diag.SValueForbidden, "у цьому місці жодне значення не дозволено", ""

	default:
		// Safety net. A test asserts this never fires on the project's
		// broken-model corpus; if it does, the mapping above has a gap.
		kw := "невідоме обмеження"
		if p := ve.ErrorKind.KeywordPath(); len(p) > 0 {
			kw = strings.Join(p, "/")
		}
		return diag.SOther, "порушено обмеження схеми: " + kw, ""
	}
}

// patternMessage turns a regular-expression failure into something an
// administrator can act on.
//
// The schema keeps its formats in $defs, and the validator reports which one
// failed in SchemaURL. Naming the format from there beats printing the regex
// and avoids copying the expressions into Go, where they would drift.
func patternMessage(schemaURL, got string) string {
	switch defName(schemaURL) {
	case "ipv4":
		return fmt.Sprintf("значення %s не є коректною IPv4-адресою", quoteValue(got))
	case "cidr":
		return fmt.Sprintf("значення %s не є коректною мережею у нотації CIDR", quoteValue(got))
	case "identifier":
		return fmt.Sprintf("значення %s не є коректним ідентифікатором: малі латинські літери, "+
			"цифри, дефіс і підкреслення, починається з літери", quoteValue(got))
	default:
		return fmt.Sprintf("значення %s не відповідає очікуваному формату", quoteValue(got))
	}
}

// defName extracts the $defs entry a schema location points at, e.g.
// "urn:orgc:organization-schema#/$defs/ipv4" → "ipv4".
func defName(schemaURL string) string {
	const marker = "/$defs/"
	i := strings.LastIndex(schemaURL, marker)
	if i < 0 {
		return ""
	}
	name := schemaURL[i+len(marker):]
	if j := strings.IndexByte(name, '/'); j >= 0 {
		name = name[:j]
	}
	return name
}

// typeName gives the Ukrainian name of a JSON type.
func typeName(t string) string {
	switch t {
	case "object":
		return "обʼєкт"
	case "array":
		return "список"
	case "string":
		return "рядок"
	case "integer":
		return "ціле число"
	case "number":
		return "число"
	case "boolean":
		return "булеве значення"
	case "null":
		return "порожнє значення"
	default:
		return t
	}
}

func typeNames(ts []string) string {
	names := make([]string, len(ts))
	for i, t := range ts {
		names[i] = typeName(t)
	}
	return strings.Join(names, " або ")
}

func quoteValue(v any) string {
	if s, ok := v.(string); ok {
		return fmt.Sprintf("%q", s)
	}
	return fmt.Sprintf("%v", v)
}

func joinValues(vs []any) string {
	parts := make([]string, len(vs))
	for i, v := range vs {
		parts[i] = quoteValue(v)
	}
	return strings.Join(parts, ", ")
}

func quoteList(ss []string) string {
	parts := make([]string, len(ss))
	for i, s := range ss {
		parts[i] = fmt.Sprintf("%q", s)
	}
	return strings.Join(parts, ", ")
}
