package render

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"orgc/internal/backend"
)

// Marker is the file that marks a directory as orgc's own. Its presence is
// what allows a run to replace the directory's whole content.
const Marker = ".orgc-output"

// markerText is the marker's content: fixed, with no time or version, so the
// marker never makes two runs differ.
const markerText = "Каталог створено orgc render. Увесь його вміст перезаписується при кожному запуску.\n"

// ErrNotOwned means the output directory exists, holds files, and was not
// created by orgc. Replacing it could delete someone's work, so Write
// refuses (docs/adr/ADR-009, decision 2).
var ErrNotOwned = errors.New("каталог не порожній і створений не orgc")

// Write replaces the output directory with exactly the given files, plus
// the marker. Files a previous run produced and this one does not are gone
// afterwards: a diff of two runs shows real changes only.
//
// The new tree is built next to the old one and swapped in by renaming, so
// an interrupted run never leaves a half-written directory under the
// original name.
func Write(dir string, files []backend.File) error {
	// "out/" and "out" are the same directory; without cleaning, the
	// staging tree would land inside the one it replaces.
	dir = filepath.Clean(dir)
	if err := owned(dir); err != nil {
		return err
	}

	staging := dir + ".orgc-new"
	retired := dir + ".orgc-old"
	for _, leftover := range []string{staging, retired} {
		if err := os.RemoveAll(leftover); err != nil {
			return err
		}
	}

	if err := os.MkdirAll(staging, 0o755); err != nil {
		return err
	}
	all := append([]backend.File{{Path: Marker, Data: []byte(markerText)}}, files...)
	for _, f := range all {
		target := filepath.Join(staging, filepath.FromSlash(f.Path))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(target, f.Data, 0o644); err != nil {
			return err
		}
	}

	if _, err := os.Stat(dir); err == nil {
		if err := os.Rename(dir, retired); err != nil {
			return err
		}
	}
	if err := os.Rename(staging, dir); err != nil {
		return err
	}
	return os.RemoveAll(retired)
}

// owned reports whether dir may be replaced: it does not exist, is empty, or
// carries the marker.
func owned(dir string) error {
	entries, err := os.ReadDir(dir)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return nil
	case err != nil:
		return err
	case len(entries) == 0:
		return nil
	}
	if _, err := os.Stat(filepath.Join(dir, Marker)); err == nil {
		return nil
	}
	return fmt.Errorf("%w: %s", ErrNotOwned, dir)
}
