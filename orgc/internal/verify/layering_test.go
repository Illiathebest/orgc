package verify

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The core knows nothing of vendors (docs/adr/ADR-008, decision 5): the IR,
// lowering, verification and the backend interface itself must not import a
// platform package. Only cmd/orgc wires platforms in. This is the claim the
// thesis makes about the architecture, so it is checked, not trusted.
func TestCoreImportsNoPlatform(t *testing.T) {
	for _, dir := range []string{"../ir", "../lower", ".", "../backend", "../render"} {
		files, err := filepath.Glob(filepath.Join(dir, "*.go"))
		if err != nil || len(files) == 0 {
			t.Fatalf("%s: файлів не знайдено (%v)", dir, err)
		}
		for _, f := range files {
			if strings.HasSuffix(f, "_test.go") {
				continue
			}
			ast, err := parser.ParseFile(token.NewFileSet(), f, nil, parser.ImportsOnly)
			if err != nil {
				t.Fatal(err)
			}
			for _, imp := range ast.Imports {
				path, _ := strconv.Unquote(imp.Path.Value)
				if strings.HasPrefix(path, "orgc/internal/backend/") {
					t.Errorf("%s імпортує платформу %s — ядро не має знати вендорів", f, path)
				}
			}
		}
	}
}
