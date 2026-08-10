package runtime

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// good is a declaration that describes an engine this project can use. Every
// case below is this one with a single field moved, so a case that fails
// fails for the field it moved and not for something else in the struct.
var good = Capabilities{
	Model:              "a-model",
	ModelVersion:       "2026-08-01",
	ContextLength:      8192,
	Tools:              true,
	Embeddings:         true,
	EmbeddingDimension: 768,
}

// TestTheContractIsThreeOperations holds the first done-condition of #71,
// which is that the interface covers generation with streaming, embeddings and
// a capability declaration and nothing else in the first release.
//
// It is written against the interface rather than against a list in a document
// because the failure it is for is a fourth operation added to serve one
// engine. That operation would compile, every adapter would grow a stub for
// it, and the reason the set was closed would be nowhere in the diff.
func TestTheContractIsThreeOperations(t *testing.T) {
	contract := reflect.TypeOf((*Runtime)(nil)).Elem()

	got := make([]string, 0, contract.NumMethod())
	for i := range contract.NumMethod() {
		got = append(got, contract.Method(i).Name)
	}
	slices.Sort(got)

	want := []string{"Capabilities", "Embed", "Generate"}
	if !slices.Equal(got, want) {
		t.Fatalf("the contract is %v, and #71 declares it as %v; adding one is a change to that issue first", got, want)
	}
}

// TestEveryOperationTakesACancellableContextFirst holds the half of #71's
// fifth condition that a declaration can hold. The other half is that every
// adapter honours it, which is #73's suite over adapters that do not exist.
//
// The near miss is an operation added without a context. It is the shape
// somebody writes when the call they are adding looks cheap, and it is the one
// operation a deadline cannot reach afterwards.
func TestEveryOperationTakesACancellableContextFirst(t *testing.T) {
	contract := reflect.TypeOf((*Runtime)(nil)).Elem()
	ctx := reflect.TypeOf((*context.Context)(nil)).Elem()

	for i := range contract.NumMethod() {
		method := contract.Method(i)
		if method.Type.NumIn() == 0 || method.Type.In(0) != ctx {
			t.Errorf("%s does not take a context first, so no deadline or cancellation reaches it", method.Name)
		}
	}
}

func TestADeclarationThatDescribesAUsableEngineIsNotRefused(t *testing.T) {
	if err := good.Validate(); err != nil {
		t.Fatalf("Validate refused a usable declaration: %v", err)
	}
	noEmbeddings := good
	noEmbeddings.Embeddings = false
	noEmbeddings.EmbeddingDimension = 0
	if err := noEmbeddings.Validate(); err != nil {
		t.Fatalf("Validate refused an engine that does not embed: %v", err)
	}
}

// TestADeclarationThatCannotBeComparedIsRefused is the near-miss table. Each
// case is one field somebody did not fill in, which is how a capability
// declaration actually arrives wrong: an adapter reads a response, the field
// it wanted is absent, and the zero value travels on looking like an answer.
func TestADeclarationThatCannotBeComparedIsRefused(t *testing.T) {
	for _, c := range []struct {
		name  string
		moved func(*Capabilities)
		says  string
	}{
		{"no model identifier", func(c *Capabilities) { c.Model = "" }, "attributed"},
		{"no model version", func(c *Capabilities) { c.ModelVersion = "" }, "invisible"},
		{"context length not set", func(c *Capabilities) { c.ContextLength = 0 }, "bound a prompt"},
		{"context length negative", func(c *Capabilities) { c.ContextLength = -1 }, "bound a prompt"},
		{"embeddings with no dimension", func(c *Capabilities) { c.EmbeddingDimension = 0 }, "dimension of 0"},
		{"a dimension from an engine that does not embed", func(c *Capabilities) {
			c.Embeddings = false
		}, "cannot both be true"},
	} {
		t.Run(c.name, func(t *testing.T) {
			moved := good
			c.moved(&moved)
			err := moved.Validate()
			if err == nil {
				t.Fatalf("Validate accepted a declaration with %s", c.name)
			}
			if !strings.Contains(err.Error(), c.says) {
				t.Errorf("the refusal is %q and does not say why (%q)", err, c.says)
			}
		})
	}
}

func TestAnEngineThatMeetsTheRequirementIsNotRefused(t *testing.T) {
	need := Requirement{
		Model:                "a-model",
		MinimumContextLength: 4096,
		Tools:                true,
		Embeddings:           true,
		EmbeddingDimension:   768,
	}
	if err := good.Refuse(need); err != nil {
		t.Fatalf("an engine that meets the requirement was refused: %v", err)
	}
	if err := good.Refuse(Requirement{}); err != nil {
		t.Fatalf("an engine was refused against a requirement that asks for nothing: %v", err)
	}
}

// TestARefusalNamesBothSides is the condition in #71 that the message names
// what was asked for and what the engine declares. A message naming one of the
// two sends the operator to the wrong end of the deployment.
func TestARefusalNamesBothSides(t *testing.T) {
	for _, c := range []struct {
		name string
		need Requirement
		says []string
	}{
		{"a different model", Requirement{Model: "another-model"}, []string{"another-model", "a-model"}},
		{"a longer context", Requirement{MinimumContextLength: 32768}, []string{"32768", "8192"}},
		{"a different dimension", Requirement{Embeddings: true, EmbeddingDimension: 1536}, []string{"1536", "768"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			err := good.Refuse(c.need)
			if err == nil {
				t.Fatalf("Refuse accepted an engine asked for %s", c.name)
			}
			for _, says := range c.says {
				if !strings.Contains(err.Error(), says) {
					t.Errorf("the refusal is %q and does not name %q", err, says)
				}
			}
		})
	}
}

func TestAnEngineWithoutWhatTheDeploymentNeedsIsRefused(t *testing.T) {
	plain := good
	plain.Tools = false
	plain.Embeddings = false
	plain.EmbeddingDimension = 0

	for _, c := range []struct {
		name string
		need Requirement
		says string
	}{
		{"tools", Requirement{Tools: true}, "tool calls"},
		{"embeddings", Requirement{Embeddings: true}, "embeddings"},
	} {
		t.Run(c.name, func(t *testing.T) {
			err := plain.Refuse(c.need)
			if err == nil {
				t.Fatalf("Refuse accepted an engine that declares no %s", c.name)
			}
			if !strings.Contains(err.Error(), c.says) || !strings.Contains(err.Error(), "a-model") {
				t.Errorf("the refusal is %q and does not name both the need and the engine", err)
			}
		})
	}
}

// TestAnUnusableDeclarationIsRefusedBeforeItIsCompared is the ordering that
// keeps Refuse honest. An engine that could not describe itself has not said
// it can do anything, and comparing a zero context length against a
// requirement of zero would let it through.
func TestAnUnusableDeclarationIsRefusedBeforeItIsCompared(t *testing.T) {
	silent := Capabilities{}
	err := silent.Refuse(Requirement{})
	if err == nil {
		t.Fatal("Refuse accepted an engine that declared nothing about itself")
	}
	if !strings.Contains(err.Error(), "not usable") {
		t.Errorf("the refusal is %q and does not say the declaration was the problem", err)
	}
}

// TestEachKindIsDistinguishableFromEveryOther is #71's sixth condition. The
// four are separate because the callers behave differently for each, and a
// caller can only behave differently if errors.Is tells them apart.
//
// The near miss is a kind that answers true to another's sentinel, which is
// what happens the moment somebody wraps one in another or reuses a sentinel
// because the two failures looked alike from inside the adapter.
func TestEachKindIsDistinguishableFromEveryOther(t *testing.T) {
	kinds := []error{ErrUnreachable, ErrRefused, ErrOverLimit, ErrMalformed}

	// The four are four values before they are four behaviours. One sentinel
	// declared as another is the cheapest way to collapse them, it reads as
	// tidying in a diff, and every case below would pass afterwards because a
	// value is trivially distinguishable from itself.
	for i, kind := range kinds {
		for j, other := range kinds {
			if i != j && kind == other {
				t.Fatalf("kind %d and kind %d are the same value (%v), so no caller can behave differently for them", i, j, kind)
			}
		}
	}

	for _, kind := range kinds {
		failure := Fail(kind, "compatible-server", "context_length", nil)
		if !errors.Is(failure, kind) {
			t.Errorf("a failure built with %v is not that kind", kind)
		}
		for _, other := range kinds {
			if other == kind {
				continue
			}
			if errors.Is(failure, other) {
				t.Errorf("a failure built with %v also reads as %v, so a caller cannot tell them apart", kind, other)
			}
		}
	}
}

func TestAFailureNamesTheAdapterAndTheField(t *testing.T) {
	failure := Fail(ErrMalformed, "process-manager", "choices[0].delta", errors.New("unexpected end of JSON input"))
	message := failure.Error()
	for _, says := range []string{"process-manager", "choices[0].delta", "could not read", "unexpected end of JSON input"} {
		if !strings.Contains(message, says) {
			t.Errorf("the failure reads %q and does not name %q", message, says)
		}
	}
	if !errors.Is(failure, ErrMalformed) {
		t.Error("the failure does not read as malformed")
	}
	if got := errors.Unwrap(failure); got == nil || got.Error() != "unexpected end of JSON input" {
		t.Errorf("unwrapping gave %v rather than what happened underneath", got)
	}
}

func TestAFailureWithNothingUnderneathReadsWithoutIt(t *testing.T) {
	failure := Fail(ErrUnreachable, "compatible-server", "", nil)
	message := failure.Error()
	if strings.Contains(message, "(") || strings.HasSuffix(message, ": ") {
		t.Errorf("the failure reads %q, which carries the punctuation of a field and a cause it does not have", message)
	}
	if errors.Unwrap(failure) != nil {
		t.Error("a failure with nothing underneath unwrapped to something")
	}
}

// TestTheDeclaredSetsAreTheConstants keeps a set variable from falling behind
// the constants beside it. A role or a finish reason declared and left out of
// its set is one every reader of the set believes does not exist.
func TestTheDeclaredSetsAreTheConstants(t *testing.T) {
	if want := []Role{RoleSystem, RoleUser, RoleAssistant}; !slices.Equal(Roles, want) {
		t.Errorf("Roles is %v and the constants are %v", Roles, want)
	}
	if want := []FinishReason{FinishComplete, FinishOutputLimit, FinishCancelled}; !slices.Equal(FinishReasons, want) {
		t.Errorf("FinishReasons is %v and the constants are %v", FinishReasons, want)
	}
}
