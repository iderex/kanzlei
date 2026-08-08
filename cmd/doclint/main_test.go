package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// register is the smallest planned declaration this command can read. It is
// written out here rather than copied from the root file so a case does not
// depend on what that file happens to declare today.
const register = `internal/authz:
planned internal/store #9
internal/store:
`

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

func TestATreeWhoseDocumentsResolvePassesAndSaysWhatItRead(t *testing.T) {
	dir := checkout(t, map[string]string{
		"import-rules.txt":        register,
		"internal/authz/authz.go": "package authz\n",
		"README.md":               "The evaluator is `internal/authz/authz.go`.\n",
		"docs/layout.md":          "See [the readme](../README.md).\n",
	})

	var out, errOut strings.Builder
	if err := run([]string{"-root", dir}, &out, &errOut); err != nil {
		t.Fatalf("run: %v (%s)", err, errOut.String())
	}
	if !strings.Contains(out.String(), "2 path reference(s)") || !strings.Contains(out.String(), "2 tracked document(s)") {
		t.Fatalf("the pass line does not say what it read: %q", out.String())
	}
}

// The bite the contributor guide asks to be shown by running rather than
// asserted: a document naming a path the tree does not have, and the check has
// to be red and say where.
func TestADocumentNamingAMissingPathIsRefusedWithItsPathAndItsLine(t *testing.T) {
	dir := checkout(t, map[string]string{
		"import-rules.txt": register,
		"docs/layout.md":   "fine\nThe evaluator is `internal/authz/authz.go`.\nfine\n",
	})

	var out, errOut strings.Builder
	err := run([]string{"-root", dir}, &out, &errOut)
	if err == nil {
		t.Fatal("a document naming a path that is not there passed")
	}
	if !strings.Contains(out.String(), "docs/layout.md:2") {
		t.Fatalf("the finding does not carry the path and the line: %q", out.String())
	}
	if !strings.Contains(err.Error(), "1 unresolved reference(s)") {
		t.Fatalf("the exit does not say how many: %v", err)
	}
}

func TestAPlannedPackageIsReadFromTheRegisterRatherThanFromASecondList(t *testing.T) {
	files := map[string]string{
		"import-rules.txt": register,
		// A package that is really there, so that internal is a directory this
		// tree has. Without one, a span naming internal/store is not read as a
		// path at all and the case below would pass for the wrong reason.
		"internal/authz/authz.go": "package authz\n",
		"docs/layout.md":          "The datastore is `internal/store/`.\n",
	}
	dir := checkout(t, files)

	var out, errOut strings.Builder
	if err := run([]string{"-root", dir}, &out, &errOut); err != nil {
		t.Fatalf("a planned package was refused: %v (%s)", err, out.String())
	}

	// The same document against a register that no longer plans it is red,
	// which is what says the pass above came from the register.
	files["import-rules.txt"] = "internal/authz:\n"
	dir = checkout(t, files)
	out.Reset()
	errOut.Reset()
	if err := run([]string{"-root", dir}, &out, &errOut); err == nil {
		t.Fatalf("an undeclared package passed: %q", out.String())
	}
}

func TestAMissingRegisterIsARefusalRatherThanAnEmptyOne(t *testing.T) {
	dir := checkout(t, map[string]string{
		"docs/layout.md": "The datastore is `internal/store/`.\n",
	})

	var out, errOut strings.Builder
	err := run([]string{"-root", dir}, &out, &errOut)
	if err == nil {
		t.Fatal("a tree with no register passed")
	}
	if !strings.Contains(err.Error(), "read the planned register") {
		t.Fatalf("the refusal does not name what was missing: %v", err)
	}
}

func TestATreeWithNoDocumentIsNotAGreenRun(t *testing.T) {
	dir := checkout(t, map[string]string{
		"import-rules.txt":        register,
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

func TestAnUnexpectedArgumentIsRefused(t *testing.T) {
	var out, errOut strings.Builder
	if err := run([]string{"docs/layout.md"}, &out, &errOut); err == nil {
		t.Fatal("an argument that names nothing this command reads was accepted")
	}
}
