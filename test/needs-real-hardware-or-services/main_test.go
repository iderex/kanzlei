//go:build needsreal

package needsreal

import "testing"

// TestMain prints the roster after the cases have run. It is here rather than
// in each suite's file because a Go test binary holds one of these, and the
// roster is a fact about the whole binary rather than about any one suite.
func TestMain(m *testing.M) { Run(m) }

// harness is the suite that proves the shape of this directory works. It needs
// nothing, which is what lets it run everywhere and is why it is the suite that
// exists from the start: the mechanism is exercised on every run rather than
// only on a machine that has a model, an identity provider or a source system.
var harness = &Suite{
	Name:  "harness",
	Needs: nil,
	Why:   "the precondition mechanism and the roster this directory is built on",
}

func init() { Register(harness) }
