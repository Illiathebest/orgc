package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"orgc/internal/backend"
	"orgc/internal/ir"
)

func testdataFile(parts ...string) string {
	return filepath.Join(append([]string{"..", "..", "testdata"}, parts...)...)
}

func runRenderOn(t *testing.T, model, out string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := run([]string{"render", "-o", out, model}, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

// fileGen is a generator with fixed output, standing in for real backends.
type fileGen struct{ files []backend.File }

func (fileGen) Name() string                                           { return "test" }
func (g fileGen) Generate(*ir.Model, backend.Platforms) []backend.File { return g.files }

func withGenerators(t *testing.T, gens ...backend.Generator) {
	t.Helper()
	saved := generators
	generators = func() []backend.Generator { return gens }
	t.Cleanup(func() { generators = saved })
}

// A model with errors, structural or semantic, yields diagnostics, exit 1,
// and not a single file — the output directory is not even created.
func TestRenderRefusesModelWithErrors(t *testing.T) {
	for _, f := range []string{
		testdataFile("broken", "s005-device-type.yml"),
		testdataFile("broken", "e006-ssh-only.yml"),
		testdataFile("broken", "w002-version-out-of-range.yml"),
	} {
		t.Run(filepath.Base(f), func(t *testing.T) {
			out := filepath.Join(t.TempDir(), "out")
			code, stdout, _ := runRenderOn(t, f, out)
			if code != exitModel {
				t.Errorf("код %d, want %d", code, exitModel)
			}
			if !strings.Contains(stdout, "Знайдено") {
				t.Errorf("немає підсумку діагностик:\n%s", stdout)
			}
			if _, err := os.Stat(out); err == nil {
				t.Error("каталог виходу створено попри помилки")
			}
		})
	}
}

// Warnings do not stop generation; they are still printed.
func TestRenderGeneratesDespiteWarnings(t *testing.T) {
	out := filepath.Join(t.TempDir(), "out")
	code, stdout, stderr := runRenderOn(t, testdataFile("broken", "w003-no-backup-uplink.yml"), out)
	if code != exitOK {
		t.Fatalf("код %d, stderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, "W003") || !strings.Contains(stdout, "Згенеровано") {
		t.Errorf("вивід:\n%s", stdout)
	}
	if _, err := os.Stat(filepath.Join(out, ".orgc-output")); err != nil {
		t.Error("немає маркера в каталозі виходу")
	}
}

func TestRenderWritesAndReplacesTree(t *testing.T) {
	out := filepath.Join(t.TempDir(), "out")
	model := testdataFile("valid", "m-profile.yml")

	withGenerators(t, fileGen{[]backend.File{{Path: "a/old.txt", Data: []byte("1\n")}, {Path: "b.txt", Data: []byte("1\n")}}})
	if code, _, stderr := runRenderOn(t, model, out); code != exitOK {
		t.Fatalf("перший прогін: код %d, %s", code, stderr)
	}

	withGenerators(t, fileGen{[]backend.File{{Path: "b.txt", Data: []byte("2\n")}}})
	code, stdout, stderr := runRenderOn(t, model, out)
	if code != exitOK {
		t.Fatalf("другий прогін: код %d, %s", code, stderr)
	}
	if !strings.Contains(stdout, "Згенеровано файлів: 1") {
		t.Errorf("вивід: %s", stdout)
	}
	if _, err := os.Stat(filepath.Join(out, "a", "old.txt")); err == nil {
		t.Error("файл, якого другий прогін не генерує, лишився")
	}
	if data, _ := os.ReadFile(filepath.Join(out, "b.txt")); string(data) != "2\n" {
		t.Errorf("b.txt = %q", data)
	}
}

func TestRenderRefusesForeignDirectory(t *testing.T) {
	out := t.TempDir()
	notes := filepath.Join(out, "notes.txt")
	if err := os.WriteFile(notes, []byte("моє"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := runRenderOn(t, testdataFile("valid", "minimal.yml"), out)
	if code != exitInvoked || !strings.Contains(stderr, "створений не orgc") {
		t.Errorf("код %d, stderr: %s", code, stderr)
	}
	if strings.Contains(stdout, "Згенеровано") {
		t.Errorf("stdout: %s", stdout)
	}
	if data, _ := os.ReadFile(notes); string(data) != "моє" {
		t.Error("чужий файл змінено")
	}
}

// render and verify share one pipeline, so they can never disagree about a
// model: for every model of the corpus, render fails exactly when verify does.
func TestRenderAgreesWithVerify(t *testing.T) {
	var files []string
	for _, dir := range []string{"valid", "broken"} {
		m, _ := filepath.Glob(testdataFile(dir, "*.yml"))
		files = append(files, m...)
	}
	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			var a, b bytes.Buffer
			verifyCode := run([]string{"verify", f}, &a, &b)
			renderCode, _, _ := runRenderOn(t, f, filepath.Join(t.TempDir(), "out"))
			if verifyCode != renderCode {
				t.Errorf("verify → %d, render → %d", verifyCode, renderCode)
			}
		})
	}
}
