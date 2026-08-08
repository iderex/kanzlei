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

func TestATreeWhoseDocumentsHoldTheShapePassesAndSaysWhatItRead(t *testing.T) {
	dir := checkout(t, map[string]string{
		"README.md":      "# A title\n\nProse.\n\n## A section\n\n- an item\n",
		"docs/layout.md": "# Another title\n\nProse.\n",
	})

	var out, errOut strings.Builder
	if err := run([]string{"-root", dir}, &out, &errOut); err != nil {
		t.Fatalf("run: %v (%s)", err, errOut.String())
	}
	for _, want := range []string{"2 tracked document(s)", "3 heading(s)"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("the pass line does not say what it read, wanted %q: %q", want, out.String())
		}
	}
}

// The bite the contributor guide asks to be shown by running rather than
// asserted. One character is the mistake somebody actually makes: an asterisk
// where the rest of the tree writes a hyphen.
func TestADocumentOutsideTheShapeIsRefusedWithItsLineAndItsRule(t *testing.T) {
	dir := checkout(t, map[string]string{
		"docs/layout.md": "# A title\n\nProse.\n\n* an item\n",
	})

	var out, errOut strings.Builder
	err := run([]string{"-root", dir}, &out, &errOut)
	if err == nil {
		t.Fatal("a document written outside the shape passed")
	}
	for _, want := range []string{"docs/layout.md:5", "list-marker"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("the finding does not carry %q: %q", want, out.String())
		}
	}
	if !strings.Contains(err.Error(), "1 departure(s)") {
		t.Fatalf("the exit does not say how many: %v", err)
	}

	// The same document with the character put back is green, which is what
	// says the refusal was about the marker and not about the document.
	dir = checkout(t, map[string]string{
		"docs/layout.md": "# A title\n\nProse.\n\n- an item\n",
	})
	out.Reset()
	errOut.Reset()
	if err := run([]string{"-root", dir}, &out, &errOut); err != nil {
		t.Fatalf("the repaired document was refused: %v (%s)", err, out.String())
	}
}

// A file git does not hold is not the tree. A departure in an untracked draft
// would otherwise be reported here and nowhere a reviewer can see it.
func TestAnUntrackedDocumentIsNotRead(t *testing.T) {
	dir := checkout(t, map[string]string{
		"docs/layout.md": "# A title\n\nProse.\n",
	})
	path := filepath.Join(dir, "docs", "draft.md")
	if err := os.WriteFile(path, []byte("# A title\n\n* an item\n"), 0o600); err != nil {
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
	dir := checkout(t, map[string]string{
		"internal/authz/authz.go": "package authz\n",
	})

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
