package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// checkout builds a git checkout in a temporary directory holding the files
// given and adds them to the index. Nothing is committed: git ls-files reads
// the index, and a commit would need an identity and a signature this case has
// no business arranging.
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

const whole = "# A title\n" +
	"\n" +
	"Prose.\n" +
	"\n" +
	"    gh api repos/iderex/kanzlei/rulesets/20487686 \\\n" +
	"      --jq '{enforcement, required: [.rules[].type]}'\n"

func TestATreeWhoseCommandsCloseIsGreenAndSaysWhatItRead(t *testing.T) {
	dir := checkout(t, map[string]string{
		"README.md":      whole,
		"docs/layout.md": "# Another title\n\nProse and no command at all.\n",
	})

	var out, errOut strings.Builder
	if err := run([]string{"-root", dir}, &out, &errOut); err != nil {
		t.Fatalf("run: %v (%s)", err, errOut.String())
	}
	// The reach is on the green line, both halves of it. A run that read one
	// command and a run that read none are otherwise the same sentence.
	for _, want := range []string{"1 command(s)", "2 tracked document(s)", "passed over"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("the pass line does not say what it read, wanted %q: %q", want, out.String())
		}
	}
}

// The bite the contributor guide asks to be shown by running rather than
// asserted, and it is the one-character mistake somebody actually makes: the
// closing quote deleted along with the argument that followed it.
func TestACommandThatDoesNotCloseIsRefusedWithItsLineAndItsRule(t *testing.T) {
	broken := strings.Replace(whole, "[.rules[].type]}'\n", "[.rules[].type]}\n", 1)
	if broken == whole {
		t.Fatal("the case did not change the document")
	}
	dir := checkout(t, map[string]string{"docs/layout.md": broken})

	var out, errOut strings.Builder
	err := run([]string{"-root", dir}, &out, &errOut)
	if err == nil {
		t.Fatal("a command whose quote never closes passed")
	}
	for _, want := range []string{"docs/layout.md:5", "unbalanced-quote"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("the finding does not carry %q: %q", want, out.String())
		}
	}
	if !strings.Contains(err.Error(), "1 command(s) that do not close") {
		t.Fatalf("the exit does not say how many: %v", err)
	}

	// The same document with the character put back is green, which is what
	// says the refusal was about the quote and not about the document.
	dir = checkout(t, map[string]string{"docs/layout.md": whole})
	out.Reset()
	errOut.Reset()
	if err := run([]string{"-root", dir}, &out, &errOut); err != nil {
		t.Fatalf("the repaired document was refused: %v (%s)", err, out.String())
	}
}

// A file git does not hold is not the tree. A command in an untracked draft
// would otherwise be judged here and nowhere a reviewer can see it.
func TestAnUntrackedDocumentIsNotRead(t *testing.T) {
	dir := checkout(t, map[string]string{"docs/layout.md": whole})
	draft := "# A title\n\nProse.\n\n    gh api repos/iderex/kanzlei \\\n      --jq '.license\n"
	if err := os.WriteFile(filepath.Join(dir, "docs", "draft.md"), []byte(draft), 0o600); err != nil {
		t.Fatalf("write the draft: %v", err)
	}

	var out, errOut strings.Builder
	if err := run([]string{"-root", dir}, &out, &errOut); err != nil {
		t.Fatalf("an untracked draft was read: %v (%s)", err, out.String())
	}
	if !strings.Contains(out.String(), "1 tracked document(s)") {
		t.Fatalf("the pass line counts the draft: %q", out.String())
	}
}

func TestATreeWithNoDocumentIsNotAGreenRun(t *testing.T) {
	dir := checkout(t, map[string]string{"internal/authz/authz.go": "package authz\n"})

	var out, errOut strings.Builder
	err := run([]string{"-root", dir}, &out, &errOut)
	if err == nil {
		t.Fatalf("a tree holding no document passed: %q", out.String())
	}
	if !strings.Contains(err.Error(), "tracks no document") {
		t.Fatalf("the refusal does not say what was absent: %v", err)
	}
}

func TestADirectoryThatIsNotACheckoutIsRefused(t *testing.T) {
	var out, errOut strings.Builder
	if err := run([]string{"-root", t.TempDir()}, &out, &errOut); err == nil {
		t.Fatal("a directory git does not hold passed")
	}
}

func TestAnUnexpectedArgumentIsRefused(t *testing.T) {
	var out, errOut strings.Builder
	if err := run([]string{"docs/layout.md"}, &out, &errOut); err == nil {
		t.Fatal("an argument that names nothing this command reads was accepted")
	}
}
