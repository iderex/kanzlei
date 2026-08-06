package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// profile is a two-block profile: two statements entered, two not. It measures
// 50.0%, which is the number every case below is set against.
const profile = `mode: atomic
github.com/iderex/kanzlei/internal/a/a.go:10.20,12.3 2 1
github.com/iderex/kanzlei/internal/a/a.go:14.20,16.3 2 0
`

// write puts content in a fresh temporary file and returns its path.
func write(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}

func TestARunThatClearsTheFloorSaysSoOnStandardOutput(t *testing.T) {
	var out, errOut strings.Builder
	err := run([]string{
		"-profile", write(t, "coverage.out", profile),
		"-floor", write(t, "coverage-floor.txt", "# measured on the day it landed\n50.0\n"),
	}, &out, &errOut)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(out.String(), "50.0%") {
		t.Fatalf("the pass line %q does not carry the measured number", out.String())
	}
}

// The bite, end to end rather than against the comparison alone: the floor is
// raised by one tenth of a point above what the profile measures, and the
// command has to refuse it.
func TestARunBelowTheFloorIsRefused(t *testing.T) {
	var out, errOut strings.Builder
	err := run([]string{
		"-profile", write(t, "coverage.out", profile),
		"-floor", write(t, "coverage-floor.txt", "50.1\n"),
	}, &out, &errOut)
	if err == nil {
		t.Fatal("a run measuring 50.0% cleared a floor of 50.1%")
	}
	for _, want := range []string{"50.0", "50.1"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("the refusal %q does not name %q", err, want)
		}
	}
	if out.String() != "" {
		t.Fatalf("a refused run wrote %q to standard output", out.String())
	}
}

// A profile that is not there is the shape a broken workflow step produces: the
// test run failed before writing one, or wrote it somewhere else. Both have to
// be refusals rather than a measurement of zero, which would clear a floor of
// zero, and rather than a pass.
func TestAMissingProfileIsRefusedRatherThanReadAsNoCoverage(t *testing.T) {
	var out, errOut strings.Builder
	err := run([]string{
		"-profile", filepath.Join(t.TempDir(), "never-written.out"),
		"-floor", write(t, "coverage-floor.txt", "0\n"),
	}, &out, &errOut)
	if err == nil {
		t.Fatal("a missing profile cleared a floor of 0%")
	}
	if !strings.Contains(err.Error(), "read the profile") {
		t.Fatalf("the refusal %q does not say the profile could not be read", err)
	}
}

func TestAMissingFloorFileIsRefused(t *testing.T) {
	var out, errOut strings.Builder
	err := run([]string{
		"-profile", write(t, "coverage.out", profile),
		"-floor", filepath.Join(t.TempDir(), "absent.txt"),
	}, &out, &errOut)
	if err == nil {
		t.Fatal("a missing floor file was accepted")
	}
	if !strings.Contains(err.Error(), "read the floor") {
		t.Fatalf("the refusal %q does not say the floor could not be read", err)
	}
}

func TestAnUnreadableProfileNamesTheFileItCouldNotRead(t *testing.T) {
	path := write(t, "coverage.out", "mode: atomic\nnot a profile line at all\n")
	var out, errOut strings.Builder
	err := run([]string{
		"-profile", path,
		"-floor", write(t, "coverage-floor.txt", "0\n"),
	}, &out, &errOut)
	if err == nil {
		t.Fatal("a malformed profile was accepted")
	}
	if !strings.Contains(err.Error(), path) {
		t.Fatalf("the refusal %q does not name the file", err)
	}
}

func TestAnUnexpectedArgumentIsRefused(t *testing.T) {
	var out, errOut strings.Builder
	if err := run([]string{"coverage.out"}, &out, &errOut); err == nil {
		t.Fatal("a positional argument was accepted; the profile is named by a flag")
	}
}

// The floor this repository actually holds is read by the same code the gate
// runs, so a floor file edited into a shape the gate cannot parse is caught by
// the default suite rather than on the gate.
func TestTheFloorThisRepositoryHoldsIsReadable(t *testing.T) {
	var out, errOut strings.Builder
	err := run([]string{
		"-profile", write(t, "coverage.out", profile),
		"-floor", filepath.Join("..", "..", "coverage-floor.txt"),
	}, &out, &errOut)
	// The tree's floor is above 50%, so this run is refused. What is being
	// checked is that it was refused for being below the floor rather than for
	// a floor file nothing could parse.
	if err != nil && !strings.Contains(err.Error(), "below the floor") {
		t.Fatalf("the floor this repository holds could not be read: %v", err)
	}
}
