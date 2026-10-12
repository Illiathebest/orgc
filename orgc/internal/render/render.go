// Package render turns a verified IR into the output tree and writes it
// (docs/adr/ADR-009, decisions 1, 2 and 7).
//
// It knows nothing of any platform: devices are rendered by their
// platform's Render, everything model-wide by generators. This package only
// collects the files, checks that together they form a sane tree, and writes
// that tree so that a second run with the same input leaves exactly the same
// bytes on disk — and nothing else.
package render

import (
	"fmt"
	"path"
	"sort"
	"strings"

	"orgc/internal/backend"
	"orgc/internal/ir"
)

// Files renders the model: every device through its platform, in file
// order, then every generator, in registry order. The result is sorted by
// path, so the order of rendering never shows in the output.
//
// The error is never about the model — verification has passed. It means a
// backend produced something that cannot be a file of the tree: a path
// outside it, or two files at one path. That is a defect of orgc.
func Files(m *ir.Model, ps backend.Platforms, gens []backend.Generator) ([]backend.File, error) {
	var files []backend.File
	for _, d := range m.Devices {
		p, ok := ps.Lookup(d.Type)
		if !ok {
			return nil, fmt.Errorf("для типу пристрою %q немає бекенда", d.Type)
		}
		files = append(files, p.Render(m, d)...)
	}
	for _, g := range gens {
		files = append(files, g.Generate(m, ps)...)
	}

	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	for i, f := range files {
		if err := checkPath(f.Path); err != nil {
			return nil, err
		}
		if i > 0 && files[i-1].Path == f.Path {
			return nil, fmt.Errorf("два файли з одним шляхом %q", f.Path)
		}
	}
	return files, nil
}

// checkPath accepts a clean, relative, slash-separated path that stays inside
// the output directory and does not collide with the ownership marker.
func checkPath(p string) error {
	switch {
	case p == "":
		return fmt.Errorf("файл з порожнім шляхом")
	case strings.Contains(p, `\`):
		return fmt.Errorf("шлях %q містить зворотну косу риску; роздільник — /", p)
	case path.IsAbs(p) || strings.Contains(p, ":"):
		return fmt.Errorf("шлях %q абсолютний", p)
	case path.Clean(p) != p || p == "." || p == ".." || strings.HasPrefix(p, "../"):
		return fmt.Errorf("шлях %q не канонічний або виходить за межі каталогу", p)
	case p == Marker:
		return fmt.Errorf("шлях %q зарезервовано для маркера каталогу", p)
	}
	return nil
}
