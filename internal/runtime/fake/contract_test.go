package fake_test

import (
	"testing"

	"github.com/iderex/kanzlei/internal/runtime"
	"github.com/iderex/kanzlei/internal/runtime/contract"
	"github.com/iderex/kanzlei/internal/runtime/fake"
)

// declared is the engine this fake is told to be for the contract run.
//
// The context length is short on purpose. The suite builds an over-long prompt
// from it, and a declaration of the size a real engine reports would make that
// one case allocate a prompt nobody needs to prove the refusal.
var declared = runtime.Capabilities{
	Model:              "fake-model",
	ModelVersion:       "2026-08-27",
	ContextLength:      64,
	Tools:              false,
	Embeddings:         true,
	EmbeddingDimension: 8,
}

// TestTheFakePassesTheContractSuite is the fourth done-condition of #73: the
// suite runs against the fake in the default gate.
//
// It is also what makes the suite worth having before an adapter exists. The
// fake is the only implementation of the contract in this tree, so a case that
// asks for something no implementation can do would be a case discovered by
// whoever writes the first real adapter, at the worst moment for it.
//
// What it proves is bounded and the bound is the fake's own: nothing here is
// evidence about an engine. docs/fake-runtime.md says what a result from this
// is evidence about, and docs/runtimes.md is where an observation against a
// real engine goes.
func TestTheFakePassesTheContractSuite(t *testing.T) {
	contract.Run(t, contract.Subject{
		Name:     "fake",
		Declared: declared,
		Under: func(t *testing.T, condition contract.Condition) runtime.Runtime {
			t.Helper()
			engine, err := fake.New(declared, behaviour(t, condition))
			if err != nil {
				t.Fatalf("the fake refused to be built for the %q condition: %v", condition, err)
			}
			return engine
		},
	})
}

// behaviour is how this fake is put in each of the suite's conditions.
//
// Every one of them is arranged, which is why this subject declares no Cannot
// entry. That is the fake's whole reason for existing: an engine that can be
// told to fail on demand is the only one every case can be run against with no
// hardware and no network.
func behaviour(t *testing.T, condition contract.Condition) fake.Behaviour {
	t.Helper()
	switch condition {
	case contract.Healthy:
		return fake.Behaviour{}
	case contract.Unreachable:
		return fake.Behaviour{Unreachable: true}
	case contract.Refusing:
		return fake.Behaviour{Refuse: true}
	case contract.Malformed:
		// Two chunks and then nothing saying the stream ended, which is the
		// partial answer a caller is most likely to store as a whole one.
		return fake.Behaviour{ChunksBeforeFailure: 2}
	default:
		t.Fatalf("the suite asked for the %q condition and this subject has no answer for it, so a case would run against an engine that behaves and pass for the wrong reason", condition)
		return fake.Behaviour{}
	}
}
