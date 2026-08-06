package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestVersionFlagPrintsAndExits(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	if err := run([]string{"-version"}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("run -version: %v (stderr %q)", err, stderr.String())
	}

	got := stdout.String()
	if !strings.HasPrefix(got, "kanzlei ") || !strings.Contains(got, "\ncommit ") || !strings.Contains(got, "\ngo ") {
		t.Errorf("-version printed %q, want the version block", got)
	}
	if stderr.Len() != 0 {
		t.Errorf("-version wrote to stderr: %q", stderr.String())
	}
}

func TestUnknownFlagIsRefused(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	// Nothing is served if the arguments were not understood. A process that
	// starts anyway is a process running with settings nobody asked for.
	if err := run([]string{"-not-a-flag"}, strings.NewReader(""), &stdout, &stderr); err == nil {
		t.Fatal("an unknown flag was accepted")
	}
	if stdout.Len() != 0 {
		t.Errorf("a refused invocation wrote to stdout: %q", stdout.String())
	}
}

func TestAStrayArgumentIsRefused(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	// `kanzlei serve` is somebody expecting a subcommand this binary does not
	// have. Ignoring it would start the process and leave them believing the
	// word did something.
	err := run([]string{"serve"}, strings.NewReader(""), &stdout, &stderr)
	if err == nil {
		t.Fatal("a stray argument was accepted")
	}
	if !strings.Contains(err.Error(), `"serve"`) {
		t.Errorf("error = %v, want it to name the argument", err)
	}
}
