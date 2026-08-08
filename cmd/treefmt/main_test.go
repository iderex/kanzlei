package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// rules is the smallest rule set that still exercises both a repair and a
// report, written out here so a case does not depend on what the root file
// happens to say today.
const rules = `root = true

[*]
end_of_line = lf
insert_final_newline = true
trim_trailing_whitespace = true
indent_style = space
indent_size = 4
`

// checkout builds a git checkout in a temporary directory holding the files
// given, adds them to the index, and returns its path. Nothing is committed:
// git ls-files reads the index, and a commit would need an identity and a
// signature this case has no business arranging.
func checkout(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	git(t, dir, "init", "-q")
	for name, content := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatalf("mkdir for %s: %v", name, err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	git(t, dir, "add", "-A")
	return dir
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, out)
	}
}

func TestATreeThatMatchesTheRuleSetPassesAndSaysWhatItRead(t *testing.T) {
	dir := checkout(t, map[string]string{
		".editorconfig": rules,
		"README.md":     "one\ntwo\n",
		"main.go":       "package main\n\nfunc main() {}\n",
	})

	var out, errOut strings.Builder
	if err := run([]string{"-root", dir}, &out, &errOut); err != nil {
		t.Fatalf("run: %v (%s)", err, errOut.String())
	}
	if !strings.Contains(out.String(), "3 tracked file(s)") {
		t.Fatalf("the pass line does not say what it read: %q", out.String())
	}
}

// The bite the issue asks to be shown by running rather than asserted: a
// deliberately misformatted file, and the check has to be red and say where.
func TestAMisformattedFileIsRefusedWithItsPathAndItsLine(t *testing.T) {
	dir := checkout(t, map[string]string{
		".editorconfig":  rules,
		"docs/layout.md": "fine\nthis line ends in spaces   \nfine\n",
	})

	var out, errOut strings.Builder
	err := run([]string{"-root", dir}, &out, &errOut)
	if err == nil {
		t.Fatal("a misformatted file passed")
	}
	if !strings.Contains(out.String(), "docs/layout.md:2: trim_trailing_whitespace") {
		t.Fatalf("the output does not name the path, the line and the rule: %q", out.String())
	}
	if !strings.Contains(err.Error(), "1 departure(s)") {
		t.Fatalf("the summary is %q", err.Error())
	}
}

func TestWriteFixesWhatTheCheckReportedAndTheCheckIsThenGreen(t *testing.T) {
	dir := checkout(t, map[string]string{
		".editorconfig":  rules,
		"docs/layout.md": "trailing   \nno final newline",
	})

	var out, errOut strings.Builder
	if err := run([]string{"-root", dir, "-write"}, &out, &errOut); err != nil {
		t.Fatalf("write: %v (%s)", err, errOut.String())
	}
	if !strings.Contains(out.String(), "1 file(s) rewritten") {
		t.Fatalf("the write line is %q", out.String())
	}

	got, err := os.ReadFile(filepath.Join(dir, "docs", "layout.md"))
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(got) != "trailing\nno final newline\n" {
		t.Fatalf("the file now holds %q", got)
	}

	out.Reset()
	if err := run([]string{"-root", dir}, &out, &errOut); err != nil {
		t.Fatalf("the check is still red after -write: %v", err)
	}
}

func TestAFileGitDoesNotTrackIsNotJudged(t *testing.T) {
	// A build output or a scratch file is not the tree, and reporting one as a
	// defect in the tree is how a gate teaches people to ignore it.
	dir := checkout(t, map[string]string{
		".editorconfig": rules,
		"README.md":     "one\n",
	})
	if err := os.WriteFile(filepath.Join(dir, "scratch.md"), []byte("trailing   \n"), 0o600); err != nil {
		t.Fatalf("write the untracked file: %v", err)
	}

	var out, errOut strings.Builder
	if err := run([]string{"-root", dir}, &out, &errOut); err != nil {
		t.Fatalf("an untracked file made the check red: %v", err)
	}
}

func TestAMissingRuleSetIsRefusedRatherThanReadAsNoRules(t *testing.T) {
	// Deleting .editorconfig would otherwise turn every rule off and leave the
	// gate green, which is the one way this check can be removed in silence.
	dir := checkout(t, map[string]string{"README.md": "one\n"})

	var out, errOut strings.Builder
	err := run([]string{"-root", dir}, &out, &errOut)
	if err == nil {
		t.Fatal("a tree with no rule set passed")
	}
	if !strings.Contains(err.Error(), "read the rule set") {
		t.Fatalf("the error is %q", err.Error())
	}
}

func TestARuleSetThatDoesNotDeclareRootIsRefused(t *testing.T) {
	// Without root = true an .editorconfig in a parent directory is laid under
	// this one, so what the gate enforces would depend on where the checkout
	// happens to sit.
	dir := checkout(t, map[string]string{
		".editorconfig": "[*]\ncharset = utf-8\n",
		"README.md":     "one\n",
	})

	var out, errOut strings.Builder
	err := run([]string{"-root", dir}, &out, &errOut)
	if err == nil {
		t.Fatal("a rule set with no root declaration passed")
	}
	if !strings.Contains(err.Error(), "root = true") {
		t.Fatalf("the error is %q", err.Error())
	}
}

func TestARuleSetThatDoesNotParseIsRefusedWithItsLine(t *testing.T) {
	dir := checkout(t, map[string]string{
		".editorconfig": "root = true\n\n[*]\nmax_line_length = 80\n",
		"README.md":     "one\n",
	})

	var out, errOut strings.Builder
	err := run([]string{"-root", dir}, &out, &errOut)
	if err == nil {
		t.Fatal("an unreadable rule set passed")
	}
	if !strings.Contains(err.Error(), ".editorconfig:4") {
		t.Fatalf("the error does not name the line: %q", err.Error())
	}
}

func TestADirectoryThatIsNotACheckoutIsRefusedRatherThanReportedClean(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".editorconfig"), []byte(rules), 0o600); err != nil {
		t.Fatalf("write the rule set: %v", err)
	}

	var out, errOut strings.Builder
	err := run([]string{"-root", dir}, &out, &errOut)
	if err == nil {
		t.Fatal("a directory git knows nothing about passed")
	}
	if !strings.Contains(err.Error(), "git") {
		t.Fatalf("the error is %q", err.Error())
	}
}

func TestAnUnexpectedArgumentIsRefused(t *testing.T) {
	var out, errOut strings.Builder
	if err := run([]string{"suspiciously-like-a-path"}, &out, &errOut); err == nil {
		t.Fatal("a positional argument was accepted, so a caller could think it named the file to check")
	}
}

func TestAnUnknownFlagIsRefused(t *testing.T) {
	var out, errOut strings.Builder
	if err := run([]string{"-fix"}, &out, &errOut); err == nil {
		t.Fatal("an unknown flag was accepted")
	}
}
