package importrules_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iderex/kanzlei/internal/importrules"
)

const fixtureModule = "example.test/fixture"

// tree writes a fixture checkout and reports its root. The map is path to
// contents, with forward slashes, which is what a reader of the case wants to
// see rather than a sequence of MkdirAll calls.
func tree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for path, contents := range files {
		full := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("the fixture directory could not be made: %v", err)
		}
		if err := os.WriteFile(full, []byte(contents), 0o644); err != nil {
			t.Fatalf("the fixture file could not be written: %v", err)
		}
	}
	return root
}

func load(t *testing.T, root string) *importrules.Graph {
	t.Helper()
	g, err := importrules.Load(root, fixtureModule)
	if err != nil {
		t.Fatalf("the fixture tree could not be read: %v", err)
	}
	return g
}

func TestAShippedImportAndATestImportAreKeptApart(t *testing.T) {
	root := tree(t, map[string]string{
		"internal/server/server.go": `package server

import "example.test/fixture/internal/build"
`,
		"internal/server/server_test.go": `package server_test

import (
	"example.test/fixture/internal/server"
	"example.test/fixture/internal/sources/fake"
)
`,
		"internal/build/build.go":       "package build\n",
		"internal/sources/fake/fake.go": "package fake\n",
	})
	g := load(t, root)

	if got := g.Shipped["internal/server"]; len(got) != 1 || got[0] != "internal/build" {
		t.Fatalf("the shipped imports of the server package are %v", got)
	}
	want := []string{"internal/server", "internal/sources/fake"}
	got := g.Test["internal/server"]
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("the test imports of the server package are %v, want %v", got, want)
	}
}

// The prefix matched is the module path and a slash. Without the slash a second
// module whose path starts with this one's would be read as part of it, and the
// fixture below holds exactly that shape.
func TestAnImportOfAnotherModuleIsNotAnEdgeHere(t *testing.T) {
	root := tree(t, map[string]string{
		"internal/server/server.go": `package server

import (
	"net/http"

	"github.com/somebody/else/pkg"
	"example.test/fixtures/internal/lookalike"
)

var _ = http.StatusOK
`,
	})
	if got := load(t, root).Shipped["internal/server"]; len(got) != 0 {
		t.Fatalf("an import from outside this module was counted as an edge: %v", got)
	}
}

func TestAPackageWithNoImportsIsStillAPackage(t *testing.T) {
	root := tree(t, map[string]string{"internal/build/build.go": "package build\n"})
	g := load(t, root)
	if len(g.Packages) != 1 || g.Packages[0] != "internal/build" {
		t.Fatalf("the packages found are %v", g.Packages)
	}
	if _, present := g.Shipped["internal/build"]; !present {
		t.Fatalf("a package that imports nothing is missing from the graph")
	}
}

func TestADirectoryHoldingOnlyTestsIsAPackage(t *testing.T) {
	root := tree(t, map[string]string{
		"test/harness/harness_test.go": "package harness\n",
	})
	g := load(t, root)
	if len(g.Packages) != 1 || g.Packages[0] != "test/harness" {
		t.Fatalf("the packages found are %v", g.Packages)
	}
	if len(g.Shipped["test/harness"]) != 0 {
		t.Fatalf("a directory of tests reported a shipped import: %v", g.Shipped["test/harness"])
	}
}

// A build constraint hides a file from the toolchain as effectively as deleting
// it, and a file under one is exactly where a forbidden import would sit and
// stay green. The imports are read out of source for this reason.
func TestAFileUnderABuildConstraintIsRead(t *testing.T) {
	root := tree(t, map[string]string{
		"internal/server/tagged.go": `//go:build needsreal

package server

import "example.test/fixture/internal/store"
`,
		"internal/store/store.go": "package store\n",
	})
	if got := load(t, root).Shipped["internal/server"]; len(got) != 1 || got[0] != "internal/store" {
		t.Fatalf("the import behind a build constraint was not read: %v", got)
	}
}

func TestTheDirectoriesABuildNeverLooksAtAreSkipped(t *testing.T) {
	root := tree(t, map[string]string{
		"internal/server/server.go":          "package server\n",
		"testdata/fixture/a.go":              "package fixture\n",
		"internal/server/testdata/b.go":      "package fixture\n",
		".hidden/c.go":                       "package hidden\n",
		"_ignored/d.go":                      "package ignored\n",
		"internal/server/_scratch/e.go":      "package scratch\n",
		"internal/server/testdata/f_test.go": "package fixture\n",
	})
	g := load(t, root)
	if len(g.Packages) != 1 || g.Packages[0] != "internal/server" {
		t.Fatalf("the packages found are %v", g.Packages)
	}
}

func TestAFileThatDoesNotParseIsReportedWithItsPath(t *testing.T) {
	root := tree(t, map[string]string{"internal/server/server.go": "package server\n\nimport (\n"})
	_, err := importrules.Load(root, fixtureModule)
	if err == nil {
		t.Fatal("a file that does not parse was read as a package with no imports")
	}
	if !strings.Contains(err.Error(), "internal/server/server.go") {
		t.Fatalf("the failure says %q and does not name the file", err)
	}
}

func TestAnEmptyModulePathIsRefusedRatherThanMatchingNothing(t *testing.T) {
	_, err := importrules.Load(t.TempDir(), "")
	if err == nil {
		t.Fatal("a graph was built without knowing what this module is called")
	}
}

func TestATreeThatIsNotThereIsReported(t *testing.T) {
	_, err := importrules.Load(filepath.Join(t.TempDir(), "absent"), fixtureModule)
	if err == nil {
		t.Fatal("a tree that does not exist was read as an empty one")
	}
}

func TestTheModulePathComesFromTheModuleFile(t *testing.T) {
	got, err := importrules.ModulePath([]byte("// a comment\n\nmodule example.test/fixture\n\ngo 1.26\n"))
	if err != nil {
		t.Fatalf("the module line was not read: %v", err)
	}
	if got != fixtureModule {
		t.Fatalf("the module path is %q, want %q", got, fixtureModule)
	}
}

func TestAModuleFileWithNoPathIsRefused(t *testing.T) {
	for _, src := range []string{"go 1.26\n", "module \n", "module\n"} {
		if _, err := importrules.ModulePath([]byte(src)); err == nil {
			t.Fatalf("%q was read as a module declaration", src)
		}
	}
}
