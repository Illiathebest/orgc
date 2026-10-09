package model

import (
	"strconv"

	"github.com/goccy/go-yaml/ast"

	"orgc/internal/diag"
)

// position is a place in the source file. A zero position means "unknown".
type position struct {
	line   int
	column int
}

// locator answers where a model element sits in the source. The validator
// reports findings by instance location; without this the administrator
// would get a path and no line, which on a model of two hundred devices is
// almost no help at all.
type locator struct {
	root ast.Node
}

func newLocator(root ast.Node) *locator { return &locator{root: root} }

// valueAt returns the position of the value at the given instance location,
// e.g. ["sites", "0", "networks", "2", "role"].
func (l *locator) valueAt(segments []string) position {
	n := l.resolve(segments)
	return posOf(n)
}

// keyAt returns the position of a named key inside the mapping at the given
// location. It is used for findings about a key rather than a value — an
// unknown field, for one: pointing at the mapping would send the reader to
// its first line instead of the offending key.
//
// It falls back to the mapping's own position when the key is not found.
func (l *locator) keyAt(segments []string, key string) position {
	n := l.resolve(segments)
	if m, ok := mappingOf(n); ok {
		for _, v := range m {
			if v.Key != nil && v.Key.GetToken() != nil && v.Key.GetToken().Value == key {
				return posOf(v.Key)
			}
		}
	}
	return posOf(n)
}

// resolve walks the AST down the instance location. When a segment cannot be
// followed — which happens for a finding about a value the parser never saw
// — it returns the deepest node reached, so the reader still lands nearby
// instead of at the top of the file.
func (l *locator) resolve(segments []string) ast.Node {
	n := l.root
	if n == nil {
		return nil
	}
	for _, seg := range segments {
		next, ok := step(n, seg)
		if !ok {
			return n
		}
		n = next
	}
	return n
}

// step descends one segment.
func step(n ast.Node, seg string) (ast.Node, bool) {
	if m, ok := mappingOf(n); ok {
		for _, v := range m {
			if v.Key != nil && v.Key.GetToken() != nil && v.Key.GetToken().Value == seg {
				return v.Value, true
			}
		}
		return nil, false
	}

	if s, ok := n.(*ast.SequenceNode); ok {
		i, err := strconv.Atoi(seg)
		if err != nil || i < 0 || i >= len(s.Values) {
			return nil, false
		}
		return s.Values[i], true
	}

	return nil, false
}

// mappingOf normalises the two shapes goccy uses for a mapping: a node with
// several pairs, and a bare pair when the mapping holds exactly one.
func mappingOf(n ast.Node) ([]*ast.MappingValueNode, bool) {
	switch t := n.(type) {
	case *ast.MappingNode:
		return t.Values, true
	case *ast.MappingValueNode:
		return []*ast.MappingValueNode{t}, true
	default:
		return nil, false
	}
}

// scalarAt returns the scalar written at the location, when there is one.
// Some findings carry no value of their own — "not" says only that the value
// is forbidden — so the text has to be read back from the source.
func (l *locator) scalarAt(segments []string) (string, bool) {
	n := l.resolve(segments)
	if n == nil {
		return "", false
	}
	if _, isMap := mappingOf(n); isMap {
		return "", false
	}
	if _, isSeq := n.(*ast.SequenceNode); isSeq {
		return "", false
	}
	tok := n.GetToken()
	if tok == nil {
		return "", false
	}
	return tok.Value, true
}

func posOf(n ast.Node) position {
	if n == nil {
		return position{}
	}
	tok := n.GetToken()
	if tok == nil || tok.Position == nil {
		return position{}
	}
	return position{line: tok.Position.Line, column: tok.Position.Column}
}

// Positions maps a readable path of the model — the same form diagnostics
// use, e.g. "sites[0].networks[2].cidr" — to where that value sits in the
// source.
//
// It exists so that positions outlive Load. The schema layer resolves a
// position the moment it reports a finding; invariants run later, over the
// intermediate representation, and would otherwise be left with a path and
// no line (docs/adr/ADR-008, decision 1).
//
// It is only ever read by key, never iterated, so being a map does not put
// the byte-identical output at risk.
type Positions map[string]diag.Pos

// At returns the position recorded for the path, or the zero Pos when there
// is none: the document root, or a value reached only through an alias.
func (p Positions) At(path string) diag.Pos { return p[path] }

// indexPositions walks the document once and records the position of every
// mapping, sequence and scalar by its readable path. The root itself is left
// out on purpose: a finding about the document as a whole carries no line
// (see describe).
func indexPositions(root ast.Node) Positions {
	idx := Positions{}
	var walk func(n ast.Node, path string)
	walk = func(n ast.Node, path string) {
		n = unwrap(n)
		if n == nil {
			return
		}

		if m, ok := mappingOf(n); ok {
			if path != "" && len(m) > 0 {
				// A mapping is located by its first key. goccy places a
				// one-pair mapping on its colon and a flow mapping on the
				// brace; the key is where a reader looks in both cases.
				idx[path] = toPos(posOf(m[0].Key))
			}
			for _, v := range m {
				if v.Key == nil || v.Key.GetToken() == nil {
					continue
				}
				key := v.Key.GetToken().Value
				child := key
				if path != "" {
					child = path + "." + key
				}
				walk(v.Value, child)
			}
			return
		}

		if path != "" {
			idx[path] = toPos(posOf(n))
		}
		if s, ok := n.(*ast.SequenceNode); ok {
			for i, v := range s.Values {
				walk(v, path+"["+strconv.Itoa(i)+"]")
			}
		}
	}
	walk(root, "")
	return idx
}

// unwrap looks through an anchor or a tag to the value it decorates. An
// alias is left as is: its position is the alias, and the values behind it
// belong to the anchored node elsewhere in the file.
func unwrap(n ast.Node) ast.Node {
	for {
		switch t := n.(type) {
		case *ast.AnchorNode:
			n = t.Value
		case *ast.TagNode:
			n = t.Value
		default:
			return n
		}
	}
}

func toPos(p position) diag.Pos { return diag.Pos{Line: p.line, Column: p.column} }
