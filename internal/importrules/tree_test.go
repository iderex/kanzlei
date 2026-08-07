package importrules_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/iderex/kanzlei/internal/importrules"
)

// TestThisTreeMatchesTheImportRules is the case that judges this repository
// rather than a fixture.
//
// It proves the state of the tree on the day it runs and it does not prove the
// guard. What proves the guard is the fixtures beside it, each of which puts a
// violation in front of the same functions and requires the refusal. A run over
// a tree that happens to obey the rules would look identical with every rule
// deleted.
//
// It is in the default suite rather than behind a tag or a schedule, because a
// forbidden import is cheapest to remove in the minute it is written.
func TestThisTreeMatchesTheImportRules(t *testing.T) {
	root := filepath.Join("..", "..")

	module, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatalf("the module file could not be read: %v", err)
	}
	path, err := importrules.ModulePath(module)
	if err != nil {
		t.Fatalf("the module file names no module: %v", err)
	}

	declaration, err := os.ReadFile(filepath.Join(root, "import-rules.txt"))
	if err != nil {
		t.Fatalf("the rule file could not be read: %v", err)
	}
	rules, err := importrules.Parse("import-rules.txt", declaration)
	if err != nil {
		t.Fatalf("the rule file was refused: %v", err)
	}

	graph, err := importrules.Load(root, path)
	if err != nil {
		t.Fatalf("the tree could not be read: %v", err)
	}

	for _, finding := range importrules.Check(rules, graph) {
		t.Errorf("import-rules.txt: %v", finding)
	}
	t.Logf("%d package(s) in the tree, %d line(s) in import-rules.txt", len(graph.Packages), len(rules.Declared()))
}
