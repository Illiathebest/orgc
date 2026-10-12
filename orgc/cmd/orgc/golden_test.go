package main

import (
	"path/filepath"
	"testing"

	"orgc/internal/golden"
	"orgc/internal/lower"
	"orgc/internal/model"
	"orgc/internal/render"
)

// The whole output tree of each example model, every backend at once,
// compared byte for byte with testdata/golden/<model>/. One reference tree
// per model, because backends share directories of the tree (configs/).
// After a deliberate change of generation: go test ./... -update.
func TestGoldenTrees(t *testing.T) {
	for _, name := range []string{"s-profile", "m-profile"} {
		t.Run(name, func(t *testing.T) {
			doc, ds, err := model.Load(testdataFile("valid", name+".yml"))
			if err != nil || doc == nil {
				t.Fatalf("err = %v, ds = %v", err, ds)
			}
			m, err := lower.Lower(doc)
			if err != nil {
				t.Fatal(err)
			}
			reg, err := platforms()
			if err != nil {
				t.Fatal(err)
			}
			files, err := render.Files(m, reg, generators())
			if err != nil {
				t.Fatal(err)
			}
			golden.Compare(t, filepath.Join("..", "..", "testdata", "golden", name), files)
		})
	}
}
