// Package golden compares a rendered tree with the reference tree kept in
// orgc/testdata/golden (docs/adr/ADR-009, decision 7). It is imported by
// tests only.
//
// The comparison is byte for byte and covers the whole tree: a missing file,
// an extra file and a changed file all fail. Run the tests with -update to
// rewrite the reference from the current output after a deliberate change;
// the diff of testdata/golden then shows exactly what the change did.
package golden

import (
	"bytes"
	"flag"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"orgc/internal/backend"
)

var update = flag.Bool("update", false, "переписати еталони в testdata/golden з поточного виходу")

// T is the part of *testing.T the comparison needs; tests of this package
// pass a recorder to see its verdicts.
type T interface {
	Helper()
	Errorf(format string, args ...any)
	Fatalf(format string, args ...any)
}

// Compare checks files against the reference directory dir. With -update it
// replaces the directory's content with files instead.
func Compare(t T, dir string, files []backend.File) {
	t.Helper()
	if *update {
		if err := os.RemoveAll(dir); err != nil {
			t.Fatalf("%v", err)
		}
		for _, f := range files {
			p := filepath.Join(dir, filepath.FromSlash(f.Path))
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				t.Fatalf("%v", err)
			}
			if err := os.WriteFile(p, f.Data, 0o644); err != nil {
				t.Fatalf("%v", err)
			}
		}
		return
	}

	want, err := read(dir)
	if err != nil {
		t.Fatalf("еталон %s не прочитано: %v (оновити: go test -update)", dir, err)
	}
	got := map[string][]byte{}
	for _, f := range files {
		got[f.Path] = f.Data
	}

	for _, p := range sortedKeys(want) {
		data, ok := got[p]
		switch {
		case !ok:
			t.Errorf("%s: є в еталоні, але не згенеровано", p)
		case !bytes.Equal(data, want[p]):
			t.Errorf("%s: відрізняється від еталона\n--- згенеровано\n%s\n--- еталон\n%s", p, data, want[p])
		}
	}
	for _, p := range sortedKeys(got) {
		if _, ok := want[p]; !ok {
			t.Errorf("%s: згенеровано, але немає в еталоні", p)
		}
	}
}

// read loads every file under dir, keyed by slash-separated relative path.
func read(dir string) (map[string][]byte, error) {
	out := map[string][]byte{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		out[filepath.ToSlash(rel)] = data
		return nil
	})
	return out, err
}

func sortedKeys(m map[string][]byte) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}
