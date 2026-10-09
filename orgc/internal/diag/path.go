package diag

import (
	"strconv"
	"strings"
)

// PathFromPointer turns a JSON Pointer (RFC 6901), which is what a JSON
// Schema validator reports, into the dotted form an administrator recognises
// from the YAML file:
//
//	/sites/0/networks/2/role   →  sites[0].networks[2].role
//	/policies/inter_vlan/1/from →  policies.inter_vlan[1].from
//	""                          →  "" (the document as a whole)
//
// Numeric segments become indices because that is how a list element is
// referred to in speech; everything else becomes a field name.
func PathFromPointer(ptr string) string {
	if ptr == "" || ptr == "/" {
		return ""
	}

	var b strings.Builder
	for _, raw := range strings.Split(strings.TrimPrefix(ptr, "/"), "/") {
		seg := unescapePointer(raw)
		if isIndex(seg) {
			b.WriteByte('[')
			b.WriteString(seg)
			b.WriteByte(']')
			continue
		}
		if b.Len() > 0 {
			b.WriteByte('.')
		}
		b.WriteString(seg)
	}
	return b.String()
}

// unescapePointer resolves the two RFC 6901 escapes. Order matters: ~01 must
// decode to ~1, so ~1 is resolved before ~0.
func unescapePointer(s string) string {
	s = strings.ReplaceAll(s, "~1", "/")
	return strings.ReplaceAll(s, "~0", "~")
}

// isIndex reports whether a segment is a non-negative decimal integer.
// strconv.Atoi alone would accept "+1" and "-1", which are field names here,
// not indices.
func isIndex(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	if len(s) > 1 && s[0] == '0' {
		return false // "01" is a field name, not an index
	}
	_, err := strconv.Atoi(s)
	return err == nil
}
