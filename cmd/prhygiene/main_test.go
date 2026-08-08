package main

import (
	"bytes"
	"strings"
	"testing"
)

// environment is the fixture form of the process environment: a map, so a case
// says what the check was given rather than changing the environment of the
// test binary and every case after it.
func environment(pairs map[string]string) func(string) string {
	return func(name string) string { return pairs[name] }
}

const oneGoodCommit = "abc123\tAdd the pull request hygiene gate (#127)\n"

func TestAnAcceptableRequestPassesAndSaysSo(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := run([]string{"-changed", "120"}, strings.NewReader(oneGoodCommit), &stdout, &stderr,
		environment(map[string]string{"PR_BODY": "Closes #127"}))
	if err != nil {
		t.Fatalf("an acceptable request was refused: %v", err)
	}
	if !strings.Contains(stdout.String(), "names the work it belongs to") {
		t.Fatalf("the output is %q", stdout.String())
	}
}

func TestABodyNamingNoIssueExitsNonZero(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := run([]string{"-changed", "120"}, strings.NewReader(oneGoodCommit), &stdout, &stderr,
		environment(map[string]string{"PR_BODY": "Closes #"}))
	if err == nil {
		t.Fatal("a body naming no issue passed")
	}
	if !strings.Contains(err.Error(), "1 refusal(s)") {
		t.Fatalf("the failure says %q", err)
	}
	if !strings.Contains(stdout.String(), "refused: the body names no issue") {
		t.Fatalf("the refusal was not printed where a contributor reads it: %q", stdout.String())
	}
}

// The body is read from the environment and never from the command line. A
// pull request body is written by whoever opened the request, and text somebody
// else wrote does not belong in an argument list.
func TestTheBodyIsNotTakenFromAnArgument(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := run([]string{"Closes #127"}, strings.NewReader(oneGoodCommit), &stdout, &stderr,
		environment(nil))
	if err == nil || !strings.Contains(err.Error(), "unexpected argument") {
		t.Fatalf("a body passed as an argument gave %v", err)
	}
}

func TestAnUnreadableCommitListIsReported(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := run(nil, strings.NewReader("abc123 no tab here\n"), &stdout, &stderr,
		environment(map[string]string{"PR_BODY": "Closes #127"}))
	if err == nil || !strings.Contains(err.Error(), "holds no tab") {
		t.Fatalf("a commit list in an unknown format gave %v", err)
	}
}

func TestASizeThatWasNotMeasuredIsSaidRatherThanAssumed(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := run(nil, strings.NewReader(oneGoodCommit), &stdout, &stderr,
		environment(map[string]string{"PR_BODY": "Closes #127"}))
	if err != nil {
		t.Fatalf("an acceptable request was refused: %v", err)
	}
	if !strings.Contains(stdout.String(), "skipped: the size, which was not measured") {
		t.Fatalf("the default said nothing about the size it did not have: %q", stdout.String())
	}
}

func TestAnUnknownFlagIsRefused(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := run([]string{"-nonsense"}, strings.NewReader(""), &stdout, &stderr, environment(nil)); err == nil {
		t.Fatal("an unknown flag was accepted")
	}
}
