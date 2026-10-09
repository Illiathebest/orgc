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
	model := filepath.Join(t.TempDir(), "organization.yml")
	if err := os.WriteFile(model, []byte("organization: test\n"), 0o644); err != nil {
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

		// Until internal/model lands (plan step 13) a readable file still
		// yields a non-zero code: verify must not report success it has
		// not established. This expectation changes when step 13 does.
		{"verify readable file, not implemented", []string{"verify", model}, exitInvoked},
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
