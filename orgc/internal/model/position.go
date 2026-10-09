package model

import (
	"strconv"

	"github.com/goccy/go-yaml/ast"
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
