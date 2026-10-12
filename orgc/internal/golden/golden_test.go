package golden

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"orgc/internal/backend"
)

// recorder collects what Compare reports instead of failing the test.
type recorder struct{ msgs []string }

func (r *recorder) Helper()                   {}
func (r *recorder) Errorf(f string, a ...any) { r.msgs = append(r.msgs, fmt.Sprintf(f, a...)) }
func (r *recorder) Fatalf(f string, a ...any) { r.msgs = append(r.msgs, "FATAL "+fmt.Sprintf(f, a...)) }
func (r *recorder) has(sub string) bool       { return strings.Contains(strings.Join(r.msgs, "\n"), sub) }

func write(t *testing.T, dir, rel, data string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func reference(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	write(t, dir, "configs/hq/rb750.rsc", "a\n")
	write(t, dir, "ansible/hosts.yml", "b\n")
	return dir
}

func TestCompareVerdicts(t *testing.T) {
	same := []backend.File{
		{Path: "ansible/hosts.yml", Data: []byte("b\n")},
		{Path: "configs/hq/rb750.rsc", Data: []byte("a\n")},
	}
	tests := []struct {
		name  string
		files []backend.File
		want  string // a fragment of the report; empty means no report
	}{
		{"збігається", same, ""},
		{"змінений файл", []backend.File{same[0], {Path: "configs/hq/rb750.rsc", Data: []byte("a2\n")}},
			"configs/hq/rb750.rsc: відрізняється"},
		{"бракує файла", same[:1], "configs/hq/rb750.rsc: є в еталоні"},
		{"зайвий файл", append(append([]backend.File{}, same...), backend.File{Path: "x.cfg"}),
			"x.cfg: згенеровано, але немає"},
		{"CRLF замість LF — теж розбіжність", []backend.File{same[0], {Path: "configs/hq/rb750.rsc", Data: []byte("a\r\n")}},
			"відрізняється"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &recorder{}
			Compare(r, reference(t), tt.files)
			switch {
			case tt.want == "" && len(r.msgs) != 0:
				t.Errorf("зайві зауваження: %v", r.msgs)
			case tt.want != "" && !r.has(tt.want):
				t.Errorf("немає %q у %v", tt.want, r.msgs)
			}
		})
	}
}

func TestMissingReferenceIsFatal(t *testing.T) {
	r := &recorder{}
	Compare(r, filepath.Join(t.TempDir(), "absent"), nil)
	if !r.has("FATAL") || !r.has("-update") {
		t.Errorf("зауваження: %v", r.msgs)
	}
}

func TestUpdateRewritesReference(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "golden")
	write(t, dir, "stale.cfg", "old")

	*update = true
	defer func() { *update = false }()
	Compare(t, dir, []backend.File{{Path: "configs/a.cfg", Data: []byte("new\n")}})

	if _, err := os.Stat(filepath.Join(dir, "stale.cfg")); err == nil {
		t.Error("застарілий файл еталона лишився після -update")
	}
	data, err := os.ReadFile(filepath.Join(dir, "configs", "a.cfg"))
	if err != nil || string(data) != "new\n" {
		t.Errorf("еталон = %q, %v", data, err)
	}
}
