package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The exit codes are a contract CI depends on: a wrong model and a wrong
// invocation must never collapse into the same signal.
func TestRunExitCodes(t *testing.T) {
	dir := t.TempDir()

	model := filepath.Join(dir, "organization.yml")
	if err := os.WriteFile(model, []byte(validModel), 0o644); err != nil {
		t.Fatal(err)
	}

	broken := filepath.Join(dir, "broken.yml")
	if err := os.WriteFile(broken, []byte("organization: test\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		args []string
		want int
	}{
		{"help long", []string{"--help"}, exitOK},
		{"help short", []string{"-h"}, exitOK},
		{"help word", []string{"help"}, exitOK},
		{"no arguments", nil, exitInvoked},
		{"unknown command", []string{"bogus"}, exitInvoked},
		{"verify without file", []string{"verify"}, exitInvoked},
		{"verify two files", []string{"verify", model, model}, exitInvoked},
		{"verify missing file", []string{"verify", filepath.Join(t.TempDir(), "absent.yml")}, exitInvoked},
		{"verify directory", []string{"verify", t.TempDir()}, exitInvoked},
		{"render without file", []string{"render"}, exitInvoked},

		// The point of the two codes: a model that is wrong and a command
		// that was called wrong must never look the same to CI.
		{"verify valid model", []string{"verify", model}, exitOK},
		{"verify broken model", []string{"verify", broken}, exitModel},

		// render is still a stub: no backend generates output yet.
		{"render readable file, not implemented", []string{"render", model}, exitInvoked},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if got := run(tt.args, &stdout, &stderr); got != tt.want {
				t.Errorf("run(%q) = %d, want %d\nstderr: %s", tt.args, got, tt.want, stderr.String())
			}
		})
	}
}

// Help goes to stdout and lists both commands; a user who typed --help did
// not make a mistake, so nothing belongs on stderr.
func TestHelpListsBothCommands(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if got := run([]string{"--help"}, &stdout, &stderr); got != exitOK {
		t.Fatalf("exit = %d, want %d", got, exitOK)
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr not empty: %s", stderr.String())
	}
	for _, want := range []string{"verify", "render"} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("help does not mention %q", want)
		}
	}
}

// A bad invocation prints usage to stderr, never stdout: stdout is reserved
// for output a caller may pipe.
func TestBadInvocationKeepsStdoutClean(t *testing.T) {
	var stdout, stderr bytes.Buffer
	run([]string{"bogus"}, &stdout, &stderr)
	if stdout.Len() != 0 {
		t.Errorf("stdout not empty: %s", stdout.String())
	}
	if !strings.Contains(stderr.String(), "bogus") {
		t.Errorf("stderr does not name the unknown command: %s", stderr.String())
	}
}

// validModel is the smallest document the schema accepts.
const validModel = `organization: "Навчальна лабораторія"
profile: S
control_node:
  site: hq
  network: servers
  ip: 192.168.30.10
sites:
  - name: hq
    networks:
      - { vlan: 30, name: servers, role: servers, cidr: 192.168.30.0/24 }
`

// verify writes its findings to stdout, so that a caller can pipe them;
// stderr stays for complaints about how orgc itself was called.
func TestVerifyWritesFindingsToStdout(t *testing.T) {
	path := filepath.Join(t.TempDir(), "broken.yml")
	if err := os.WriteFile(path, []byte("organization: test\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if got := run([]string{"verify", path}, &stdout, &stderr); got != exitModel {
		t.Fatalf("код виходу = %d, want %d\nstderr: %s", got, exitModel, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr не порожній: %s", stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "S002") {
		t.Errorf("у виводі немає коду структурної помилки:\n%s", out)
	}
	if !strings.Contains(out, "Знайдено") {
		t.Errorf("у виводі немає підсумку:\n%s", out)
	}
}

func TestVerifyValidModelSaysOK(t *testing.T) {
	path := filepath.Join(t.TempDir(), "organization.yml")
	if err := os.WriteFile(path, []byte(validModel), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if got := run([]string{"verify", path}, &stdout, &stderr); got != exitOK {
		t.Fatalf("код виходу = %d, want %d\nstdout: %s\nstderr: %s",
			got, exitOK, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "OK") {
		t.Errorf("вивід не містить OK:\n%s", stdout.String())
	}
}
