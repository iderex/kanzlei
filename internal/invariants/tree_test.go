package invariants_test

import (
	"os"
	"testing"

	"github.com/iderex/kanzlei/internal/invariants"
)

// This case judges the repository against its own declaration file, which is a
// different thing from the cases beside it. Those prove the operators, against
// fixtures written to be refused. This one proves the state of the tree on the
// day it ran, and it would stay green if every operator stopped refusing
// anything.
//
// Both are worth having, and reading this one as a proof of the rules is the
// mistake the distinction exists against.
func TestThisTreeHoldsItsOwnInvariants(t *testing.T) {
	src, err := os.ReadFile("../../invariants.txt")
	if err != nil {
		t.Fatalf("read the declaration file: %v", err)
	}
	declared, err := invariants.Parse("invariants.txt", src)
	if err != nil {
		t.Fatalf("the declaration file this repository ships does not parse: %v", err)
	}

	found, read, err := invariants.CheckTree(declared, "../..")
	if err != nil {
		t.Fatalf("read the tree: %v", err)
	}
	if len(found) > 0 {
		t.Fatalf("this tree violates its own invariants: %v", found)
	}
	if read == 0 {
		// A run that read nothing prints the same result as a run that found
		// nothing, and a wrong root is how the first happens.
		t.Fatal("no Go file was read, so nothing was judged")
	}
}
