package render

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"orgc/internal/backend"
	"orgc/internal/ir"
)

// platform renders one file per device; files lets a test choose them.
type platform struct {
	files func(d *ir.Device) []backend.File
}

func (platform) Type() ir.DeviceType                               { return "fake" }
func (platform) CheckVersion(string) (bool, string)                { return true, "" }
func (platform) ManagementPorts(string) []ir.PortSpec              { return nil }
func (platform) Capabilities(string) backend.CapSet                { return 0 }
func (p platform) Render(_ *ir.Model, d *ir.Device) []backend.File { return p.files(d) }

type generator struct {
	name  string
	files []backend.File
}

func (g generator) Name() string                                         { return g.name }
func (g generator) Generate(*ir.Model, backend.Platforms) []backend.File { return g.files }

func model() *ir.Model {
	s := &ir.Site{Name: "hq"}
	return &ir.Model{Devices: []*ir.Device{
		{Name: "zeta", Type: "fake", Site: s},
		{Name: "alpha", Type: "fake", Site: s},
	}}
}

func registry(t *testing.T, p platform) backend.Platforms {
	t.Helper()
	r, err := backend.NewRegistry(p)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func perDevice(d *ir.Device) []backend.File {
	return []backend.File{{Path: "configs/" + d.Site.Name + "/" + d.Name + ".cfg", Data: []byte(d.Name + "\n")}}
}

// Devices, then generators, all sorted by path: the order things were
// rendered in never shows in the output.
func TestFilesCollectsAndSorts(t *testing.T) {
	gens := []backend.Generator{
		generator{"b", []backend.File{{Path: "prometheus/targets/devices.json"}}},
		generator{"a", []backend.File{{Path: "ansible/hosts.yml"}}},
	}
	files, err := Files(model(), registry(t, platform{perDevice}), gens)
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, f := range files {
		paths = append(paths, f.Path)
	}
	want := "ansible/hosts.yml configs/hq/alpha.cfg configs/hq/zeta.cfg prometheus/targets/devices.json"
	if got := strings.Join(paths, " "); got != want {
		t.Errorf("шляхи:\n got %s\nwant %s", got, want)
	}
}

// A backend producing a path outside the tree, or two files at one path, is
// a defect of orgc and must stop the run before anything is written.
func TestFilesRejectsBadTrees(t *testing.T) {
	bad := []string{"", "/etc/passwd", "C:/x", `configs\hq\a.cfg`, "../x", "configs/../../x", "configs//a", "./a", Marker}
	for _, p := range bad {
		gens := []backend.Generator{generator{"g", []backend.File{{Path: p}}}}
		if _, err := Files(&ir.Model{}, registry(t, platform{perDevice}), gens); err == nil {
			t.Errorf("шлях %q прийнято", p)
		}
	}

	dup := []backend.Generator{
		generator{"a", []backend.File{{Path: "ansible/hosts.yml"}}},
		generator{"b", []backend.File{{Path: "ansible/hosts.yml"}}},
	}
	if _, err := Files(&ir.Model{}, registry(t, platform{perDevice}), dup); err == nil {
		t.Error("два файли з одним шляхом прийнято")
	}
}

func tree(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		data, err := os.ReadFile(p)
		out[filepath.ToSlash(rel)] = string(data)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestWriteNewAndEmptyDirectory(t *testing.T) {
	for name, prepare := range map[string]func(string){
		"неіснуючий": func(string) {},
		"порожній":   func(d string) { _ = os.MkdirAll(d, 0o755) },
	} {
		t.Run(name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "out")
			prepare(dir)
			if err := Write(dir, []backend.File{{Path: "configs/hq/a.cfg", Data: []byte("a\n")}}); err != nil {
				t.Fatal(err)
			}
			got := tree(t, dir)
			if got["configs/hq/a.cfg"] != "a\n" || got[Marker] != markerText || len(got) != 2 {
				t.Errorf("дерево: %v", got)
			}
		})
	}
}

// A second run replaces the whole tree: a file the new run no longer
// generates disappears, which is what keeps a diff of two runs honest.
func TestWriteReplacesOwnedDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "out")
	if err := Write(dir, []backend.File{{Path: "old/stale.cfg", Data: []byte("x")}, {Path: "keep.cfg", Data: []byte("1")}}); err != nil {
		t.Fatal(err)
	}
	if err := Write(dir+string(filepath.Separator), []backend.File{{Path: "keep.cfg", Data: []byte("2")}}); err != nil {
		t.Fatal(err)
	}
	got := tree(t, dir)
	if len(got) != 2 || got["keep.cfg"] != "2" {
		t.Errorf("після другого прогону: %v", got)
	}
	for _, leftover := range []string{dir + ".orgc-new", dir + ".orgc-old"} {
		if _, err := os.Stat(leftover); err == nil {
			t.Errorf("лишився тимчасовий каталог %s", leftover)
		}
	}
}

// Someone else's directory is never touched.
func TestWriteRefusesForeignDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "out")
	_ = os.MkdirAll(dir, 0o755)
	_ = os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("моє"), 0o644)

	err := Write(dir, []backend.File{{Path: "a.cfg"}})
	if !errors.Is(err, ErrNotOwned) {
		t.Fatalf("err = %v, want ErrNotOwned", err)
	}
	if got := tree(t, dir); len(got) != 1 || got["notes.txt"] != "моє" {
		t.Errorf("чужий каталог змінено: %v", got)
	}
}

// Same files, same bytes on disk, run after run.
func TestWriteIsDeterministic(t *testing.T) {
	files := []backend.File{{Path: "b/x.cfg", Data: []byte("x\n")}, {Path: "a.cfg", Data: []byte("a\n")}}
	dir := filepath.Join(t.TempDir(), "out")
	if err := Write(dir, files); err != nil {
		t.Fatal(err)
	}
	first := tree(t, dir)
	for i := 0; i < 5; i++ {
		if err := Write(dir, files); err != nil {
			t.Fatal(err)
		}
		got := tree(t, dir)
		if len(got) != len(first) {
			t.Fatalf("прогін %d: %v", i, got)
		}
		for p, data := range first {
			if got[p] != data {
				t.Fatalf("прогін %d: %s відрізняється", i, p)
			}
		}
	}
	if strings.ContainsRune(markerText, '\r') {
		t.Error("маркер містить CR")
	}
}
