package main

import (
	"bytes"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const cleanSarif = `{"runs":[{"tool":{"driver":{"name":"CodeQL"}},"results":[]}]}`

const highSarif = `{"runs":[{"tool":{"driver":{"name":"CodeQL"},"extensions":[{"rules":[
  {"id":"go/path-injection","properties":{"security-severity":"7.5"}}]}]},
  "results":[{"ruleId":"go/path-injection","message":{"text":"user input reaches a file path"},
  "locations":[{"physicalLocation":{"artifactLocation":{"uri":"internal/scanfixture/pathinjection.go"},
  "region":{"startLine":30}}}]}]}]}`

func write(t *testing.T, name, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}

func TestACleanRunPassesAndSaysSo(t *testing.T) {
	sarif := write(t, "results.sarif", cleanSarif)
	floor := write(t, "scan-floor.txt", "# the argument\n0.0\n")

	var stdout, stderr bytes.Buffer
	if err := run([]string{"-sarif", sarif, "-floor", floor}, &stdout, &stderr); err != nil {
		t.Fatalf("run: %v (stderr %q)", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "no findings") {
		t.Errorf("stdout = %q, want it to say what was judged", stdout.String())
	}
}

func TestAFindingAtTheFloorRefusesTheRunAndNamesIt(t *testing.T) {
	sarif := write(t, "results.sarif", highSarif)
	floor := write(t, "scan-floor.txt", "0.0\n")

	var stdout, stderr bytes.Buffer
	err := run([]string{"-sarif", sarif, "-floor", floor}, &stdout, &stderr)
	if err == nil {
		t.Fatal("run accepted a tree carrying a finding at the floor")
	}
	if !strings.Contains(stderr.String(), "internal/scanfixture/pathinjection.go:30") {
		t.Errorf("stderr = %q, want it to name where the finding is", stderr.String())
	}
	if !strings.Contains(stderr.String(), "go/path-injection") {
		t.Errorf("stderr = %q, want it to name the rule", stderr.String())
	}
}

func TestTheSameFindingPassesUnderAHigherFloor(t *testing.T) {
	// The near miss: the same document, judged against a floor above it. This
	// is what proves the number in the file is what decides and not the
	// presence of a result.
	sarif := write(t, "results.sarif", highSarif)
	floor := write(t, "scan-floor.txt", "9.0\n")

	var stdout, stderr bytes.Buffer
	if err := run([]string{"-sarif", sarif, "-floor", floor}, &stdout, &stderr); err != nil {
		t.Fatalf("run: %v (stderr %q)", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "none at or above") {
		t.Errorf("stdout = %q, want it to say the finding was under the floor", stdout.String())
	}
}

func TestAMissingSarifIsARefusalRatherThanACleanTree(t *testing.T) {
	floor := write(t, "scan-floor.txt", "0.0\n")
	var stdout, stderr bytes.Buffer
	err := run([]string{"-sarif", filepath.Join(t.TempDir(), "absent.sarif"), "-floor", floor}, &stdout, &stderr)
	if err == nil {
		t.Fatal("run passed with no sarif to read, and an analysis that never wrote one must not clear the gate")
	}
	if !strings.Contains(err.Error(), "read the sarif") {
		t.Errorf("err = %v, want it to say the sarif could not be read", err)
	}
}

func TestTheProgramRefusesToJudgeAFileItWasNotPointedAt(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := run(nil, &stdout, &stderr); err == nil {
		t.Fatal("run accepted an empty -sarif")
	}
}

func TestBadInputIsRefused(t *testing.T) {
	cases := []struct {
		name  string
		sarif string
		floor string
		want  string
	}{
		{name: "a sarif that is not json", sarif: "{", floor: "0.0\n", want: "parse the sarif"},
		{name: "a sarif with no runs", sarif: `{"runs":[]}`, floor: "0.0\n", want: "no runs"},
		{name: "a floor that is not a number", sarif: cleanSarif, floor: "high\n", want: "not a number"},
		{name: "a floor file holding nothing", sarif: cleanSarif, floor: "# only a comment\n", want: "holds no number"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sarif := write(t, "results.sarif", c.sarif)
			floor := write(t, "scan-floor.txt", c.floor)
			var stdout, stderr bytes.Buffer
			err := run([]string{"-sarif", sarif, "-floor", floor}, &stdout, &stderr)
			if err == nil {
				t.Fatalf("run accepted %s", c.name)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("err = %v, want it to mention %q", err, c.want)
			}
		})
	}
}

func TestAMissingFloorFileIsARefusal(t *testing.T) {
	sarif := write(t, "results.sarif", cleanSarif)
	var stdout, stderr bytes.Buffer
	err := run([]string{"-sarif", sarif, "-floor", filepath.Join(t.TempDir(), "absent.txt")}, &stdout, &stderr)
	if err == nil {
		t.Fatal("run passed with no floor to judge against")
	}
	if !strings.Contains(err.Error(), "read the floor") {
		t.Errorf("err = %v, want it to say the floor could not be read", err)
	}
}

func TestAnUnexpectedArgumentIsRefused(t *testing.T) {
	sarif := write(t, "results.sarif", cleanSarif)
	var stdout, stderr bytes.Buffer
	if err := run([]string{"-sarif", sarif, "leftover"}, &stdout, &stderr); err == nil {
		t.Fatal("run accepted an argument it does not take")
	}
}

func TestHelpIsNotAFailure(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := run([]string{"-h"}, &stdout, &stderr)
	if !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("run(-h) = %v, want flag.ErrHelp so main can exit cleanly", err)
	}
}
