package fake

import (
	"context"
	"errors"
	"math"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/iderex/kanzlei/internal/runtime"
)

// declared is an engine this fake can be. Every case below is this one with a
// single field moved, so a case that fails fails for what it moved.
var declared = runtime.Capabilities{
	Model:              "a-fake-model",
	ModelVersion:       "2026-08-01",
	ContextLength:      64,
	Tools:              false,
	Embeddings:         true,
	EmbeddingDimension: 768,
}

// ask is the request most cases send, small enough to fit the context above.
var ask = runtime.GenerateRequest{
	Messages: []runtime.Message{
		{Role: runtime.RoleSystem, Text: "answer from the passages and nothing else"},
		{Role: runtime.RoleUser, Text: "which group may read the minutes"},
	},
	MaxOutputTokens: 32,
}

// behaving builds a fake that does nothing wrong, which is what a case about
// something other than misbehaviour wants.
func behaving(t *testing.T) *Runtime {
	t.Helper()
	engine, err := New(declared, Behaviour{})
	if err != nil {
		t.Fatalf("New refused a declaration the contract accepts: %v", err)
	}
	return engine
}

// collect runs a generation and gives back everything the sink saw, joined the
// way a caller accumulating an answer would.
func collect(t *testing.T, engine *Runtime, ctx context.Context, req runtime.GenerateRequest) (string, []string, runtime.GenerateResult, error) {
	t.Helper()
	var chunks []string
	result, err := engine.Generate(ctx, req, func(chunk string) error {
		chunks = append(chunks, chunk)
		return nil
	})
	return strings.Join(chunks, ""), chunks, result, err
}

// This is the structural half of #72's first line, and it is here rather than
// inside a case because the failure it is for is the contract gaining an
// operation this fake does not have. That failure has to arrive as a build
// error in this package rather than as a case somebody can skip.
var _ runtime.Runtime = (*Runtime)(nil)

// TestTheFakeIsUsableAsTheContract is the other half of it. Satisfying an
// interface is a compile-time fact and says nothing about whether the three
// operations answer when they are reached through it, which is how every
// caller in this project will reach them.
func TestTheFakeIsUsableAsTheContract(t *testing.T) {
	var engine runtime.Runtime = behaving(t)

	if _, err := engine.Capabilities(t.Context()); err != nil {
		t.Errorf("Capabilities failed through the contract: %v", err)
	}
	if _, err := engine.Generate(t.Context(), ask, func(string) error { return nil }); err != nil {
		t.Errorf("Generate failed through the contract: %v", err)
	}
	if _, err := engine.Embed(t.Context(), runtime.EmbedRequest{Texts: []string{"a passage"}}); err != nil {
		t.Errorf("Embed failed through the contract: %v", err)
	}
}

// TestTheSameRequestProducesTheSameAnswer is why this package exists. A test
// using a real engine measures the engine; one using this measures the code.
func TestTheSameRequestProducesTheSameAnswer(t *testing.T) {
	engine := behaving(t)

	first, _, firstResult, err := collect(t, engine, t.Context(), ask)
	if err != nil {
		t.Fatalf("a generation that should have succeeded failed: %v", err)
	}
	second, _, secondResult, err := collect(t, engine, t.Context(), ask)
	if err != nil {
		t.Fatalf("the second generation failed: %v", err)
	}
	if first != second {
		t.Fatalf("the same request produced %q and then %q", first, second)
	}
	if firstResult != secondResult {
		t.Fatalf("the same request produced %+v and then %+v", firstResult, secondResult)
	}

	other := ask
	other.Messages = append(append([]runtime.Message{}, ask.Messages[:1]...), runtime.Message{Role: runtime.RoleUser, Text: "which group may read the accounts"})
	third, _, _, err := collect(t, engine, t.Context(), other)
	if err != nil {
		t.Fatalf("a generation for a different question failed: %v", err)
	}
	if third == first {
		t.Fatalf("two different questions produced the same answer %q, so the answer is not derived from the request", first)
	}
}

// TestAnAnswerSaysWhereItCameFrom holds the property that stops a fake result
// being quoted as a real one. Every answer opens with the marker, so one that
// turns up in a log, a fixture or an issue is identifiable without anybody
// having to remember which run produced it.
func TestAnAnswerSaysWhereItCameFrom(t *testing.T) {
	engine := behaving(t)

	joined, chunks, _, err := collect(t, engine, t.Context(), ask)
	if err != nil {
		t.Fatalf("a generation that should have succeeded failed: %v", err)
	}
	if len(chunks) == 0 {
		t.Fatal("nothing was streamed")
	}
	if chunks[0] != marker {
		t.Errorf("the first chunk is %q and every answer is meant to open with %q", chunks[0], marker)
	}
	if !strings.HasPrefix(joined, marker+" ") {
		t.Errorf("the accumulated answer is %q, which does not read as one word followed by the rest", joined)
	}
}

// TestOneChunkIsOneTokenAndTheCountSaysSo ties the streamed shape to the
// number the result reports, because a caller sizing anything off
// OutputTokens is entitled to have it mean what arrived.
func TestOneChunkIsOneTokenAndTheCountSaysSo(t *testing.T) {
	engine := behaving(t)

	joined, chunks, result, err := collect(t, engine, t.Context(), ask)
	if err != nil {
		t.Fatalf("a generation that should have succeeded failed: %v", err)
	}
	if result.OutputTokens != len(chunks) {
		t.Errorf("the result reports %d output token(s) and %d chunk(s) arrived", result.OutputTokens, len(chunks))
	}
	if got := len(strings.Fields(joined)); got != result.OutputTokens {
		t.Errorf("the answer holds %d word(s) and the result reports %d output token(s)", got, result.OutputTokens)
	}
	if result.Finish != runtime.FinishComplete {
		t.Errorf("an answer that ran to its end finished as %q", result.Finish)
	}
	if result.InputTokens != 13 {
		t.Errorf("the request holds 13 words and the result reports %d input token(s)", result.InputTokens)
	}
}

// TestAnAnswerCutAtTheBoundSaysItWasCut is #66's problem in miniature. A
// truncated answer and a complete one read identically, so the only thing that
// can tell them apart is the finish reason.
func TestAnAnswerCutAtTheBoundSaysItWasCut(t *testing.T) {
	engine := behaving(t)

	bounded := ask
	bounded.MaxOutputTokens = 2

	_, chunks, result, err := collect(t, engine, t.Context(), bounded)
	if err != nil {
		t.Fatalf("a bounded generation failed: %v", err)
	}
	if len(chunks) != 2 {
		t.Errorf("a bound of 2 produced %d chunk(s)", len(chunks))
	}
	if result.Finish != runtime.FinishOutputLimit {
		t.Errorf("an answer cut at its bound finished as %q rather than %q", result.Finish, runtime.FinishOutputLimit)
	}
}

// TestTheDeclarationThisFakeCannotMakeIsRefused is the near-miss table for
// New. Two of these are the case worth the table: a misbehaviour set to the
// value that is already declared is a test that thinks it arranged a wrong
// answer and did not, and it passes for a reason nobody wrote down.
func TestTheDeclarationThisFakeCannotMakeIsRefused(t *testing.T) {
	noVersion := declared
	noVersion.ModelVersion = ""

	cases := []struct {
		name      string
		declared  runtime.Capabilities
		behaviour Behaviour
	}{
		{"a declaration the contract itself refuses", noVersion, Behaviour{}},
		{"a stream failing after a negative number of chunks", declared, Behaviour{ChunksBeforeFailure: -1}},
		{"a stream failing after more chunks than an answer holds", declared, Behaviour{ChunksBeforeFailure: answerWords}},
		{"a vector with a negative number of components", declared, Behaviour{Dimension: -1}},
		{"the wrong dimension set to the declared one", declared, Behaviour{Dimension: declared.EmbeddingDimension}},
		{"the other model set to the declared one", declared, Behaviour{Model: declared.Model}},
	}
	for _, one := range cases {
		t.Run(one.name, func(t *testing.T) {
			engine, err := New(one.declared, one.behaviour)
			if err == nil {
				t.Fatalf("New built a fake for %s", one.name)
			}
			if engine != nil {
				t.Errorf("New returned an engine beside its refusal")
			}
		})
	}
}

// TestEachMisbehaviourIsTheOneAskedFor holds #72's second line for the four
// that are a failure kind. The other two are a wrong answer rather than a
// failure and have their own cases below.
func TestEachMisbehaviourIsTheOneAskedFor(t *testing.T) {
	cases := []struct {
		name      string
		behaviour Behaviour
		want      error
	}{
		{"unreachable", Behaviour{Unreachable: true}, runtime.ErrUnreachable},
		{"refused", Behaviour{Refuse: true}, runtime.ErrRefused},
		{"over the limit", Behaviour{OverLimit: true}, runtime.ErrOverLimit},
		{"a stream that stops part way", Behaviour{ChunksBeforeFailure: 2}, runtime.ErrMalformed},
	}
	for _, one := range cases {
		t.Run(one.name, func(t *testing.T) {
			engine, err := New(declared, one.behaviour)
			if err != nil {
				t.Fatalf("New refused a misbehaviour it is meant to carry: %v", err)
			}
			_, _, _, err = collect(t, engine, t.Context(), ask)
			if !errors.Is(err, one.want) {
				t.Fatalf("the generation failed with %v and the behaviour asked for %v", err, one.want)
			}
			var failure *runtime.Error
			if !errors.As(err, &failure) {
				t.Fatalf("the failure is %T rather than one an adapter produces", err)
			}
			if failure.Adapter != adapter {
				t.Errorf("the failure names the adapter %q", failure.Adapter)
			}
		})
	}
}

// TestAStreamThatStopsPartWayDeliveredWhatItSaidItDid is the half of the
// partial stream that the table above cannot see. The chunks arrive, they are
// well formed, and then nothing does, which is why a caller mistakes it for a
// complete answer.
func TestAStreamThatStopsPartWayDeliveredWhatItSaidItDid(t *testing.T) {
	engine, err := New(declared, Behaviour{ChunksBeforeFailure: 2})
	if err != nil {
		t.Fatalf("New refused a partial stream: %v", err)
	}

	_, chunks, result, err := collect(t, engine, t.Context(), ask)
	if !errors.Is(err, runtime.ErrMalformed) {
		t.Fatalf("a stream that stopped part way failed with %v", err)
	}
	if len(chunks) != 2 {
		t.Errorf("the stream was to stop after 2 chunk(s) and %d arrived", len(chunks))
	}
	if (result != runtime.GenerateResult{}) {
		t.Errorf("a failed stream came back with the result %+v, and a caller reading a finish reason there reads a partial answer as a whole one", result)
	}
}

// TestAnEngineServingAnotherModelSaysSoEverywhereItIsAsked is the quiet
// failure docs/decisions/0009-runtime.md names. The identifier has to be wrong
// in all three places or a caller comparing only one of them is satisfied.
func TestAnEngineServingAnotherModelSaysSoEverywhereItIsAsked(t *testing.T) {
	engine, err := New(declared, Behaviour{Model: "some-other-model"})
	if err != nil {
		t.Fatalf("New refused an engine serving another model: %v", err)
	}

	capabilities, err := engine.Capabilities(t.Context())
	if err != nil {
		t.Fatalf("Capabilities failed: %v", err)
	}
	if capabilities.Model != "some-other-model" {
		t.Errorf("the engine declares %q and was told to serve %q", capabilities.Model, "some-other-model")
	}

	_, _, result, err := collect(t, engine, t.Context(), ask)
	if err != nil {
		t.Fatalf("the generation failed: %v", err)
	}
	if result.Model != "some-other-model" {
		t.Errorf("the generation was attributed to %q", result.Model)
	}

	embedded, err := engine.Embed(t.Context(), runtime.EmbedRequest{Texts: []string{"a passage"}})
	if err != nil {
		t.Fatalf("Embed failed: %v", err)
	}
	if embedded.Model != "some-other-model" {
		t.Errorf("the vectors were attributed to %q", embedded.Model)
	}
}

// TestAnEngineReturningTheWrongDimensionReturnsIt is the failure #60 versions
// vectors against. The declaration is untouched, which is the point: the
// engine goes on saying 768 and hands back something else.
func TestAnEngineReturningTheWrongDimensionReturnsIt(t *testing.T) {
	engine, err := New(declared, Behaviour{Dimension: 512})
	if err != nil {
		t.Fatalf("New refused a wrong dimension: %v", err)
	}

	capabilities, err := engine.Capabilities(t.Context())
	if err != nil {
		t.Fatalf("Capabilities failed: %v", err)
	}
	if capabilities.EmbeddingDimension != declared.EmbeddingDimension {
		t.Errorf("the declaration moved to %d, and an engine that admits the mismatch is not the failure this is for", capabilities.EmbeddingDimension)
	}

	embedded, err := engine.Embed(t.Context(), runtime.EmbedRequest{Texts: []string{"a passage"}})
	if err != nil {
		t.Fatalf("Embed failed: %v", err)
	}
	if got := len(embedded.Vectors[0]); got != 512 {
		t.Errorf("the vector has %d components and the engine was told to return 512", got)
	}
}

// TestATimeoutIsTheContextEndingRatherThanAnAnswer covers the misbehaviour
// that cannot be a return value, because a call that never comes back is the
// thing under test.
func TestATimeoutIsTheContextEndingRatherThanAnAnswer(t *testing.T) {
	engine, err := New(declared, Behaviour{Hang: true})
	if err != nil {
		t.Fatalf("New refused a hanging engine: %v", err)
	}

	// The clock is read before the deadline is set, not after. Whatever passes
	// between the two lines is time the call really did wait, and a
	// measurement that starts after the deadline cannot see it: on a loaded
	// runner this case reported a wait of 17.6ms against a deadline of 20ms
	// and failed while the engine had behaved.
	started := time.Now()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()

	_, _, _, err = collect(t, engine, ctx, ask)
	if !errors.Is(err, runtime.ErrUnreachable) {
		t.Fatalf("a deadline reached before an answer failed with %v", err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("the failure does not carry the deadline underneath it: %v", err)
	}
	if elapsed := time.Since(started); elapsed < 20*time.Millisecond {
		t.Errorf("the call came back after %v, which is before the deadline it was meant to wait for", elapsed)
	}
}

// TestACancelledGenerationSaysSoAndTheEngineSawIt is the assertion #76 cannot
// make against a real engine. A handler that returns on a disconnected client
// while the engine keeps generating passes every test that only watches the
// handler.
func TestACancelledGenerationSaysSoAndTheEngineSawIt(t *testing.T) {
	engine := behaving(t)

	ctx, cancel := context.WithCancel(t.Context())
	var chunks []string
	result, err := engine.Generate(ctx, ask, func(chunk string) error {
		chunks = append(chunks, chunk)
		cancel()
		return nil
	})

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("a cancelled generation came back with %v", err)
	}
	if result.Finish != runtime.FinishCancelled {
		t.Errorf("a cancelled generation finished as %q rather than %q", result.Finish, runtime.FinishCancelled)
	}
	if len(chunks) != 1 {
		t.Errorf("the cancellation arrived after 1 chunk and %d were streamed", len(chunks))
	}
	if engine.Cancellations() != 1 {
		t.Errorf("the engine recorded %d cancellation(s), so a test asserting the engine noticed has nothing to read", engine.Cancellations())
	}
}

// TestASinkThatFailsStopsTheGeneration holds the contract's own sentence about
// a caller whose reader has gone away.
func TestASinkThatFailsStopsTheGeneration(t *testing.T) {
	engine := behaving(t)

	gone := errors.New("the reader has gone away")
	seen := 0
	result, err := engine.Generate(t.Context(), ask, func(string) error {
		seen++
		return gone
	})
	if !errors.Is(err, gone) {
		t.Fatalf("the sink's error came back as %v", err)
	}
	if seen != 1 {
		t.Errorf("the sink was called %d time(s) after refusing the first chunk", seen)
	}
	if (result != runtime.GenerateResult{}) {
		t.Errorf("a generation the sink stopped came back with the result %+v", result)
	}
}

// TestARequestTheContractForbidsIsRefused is where this fake is deliberately
// stricter than an engine. Each of these is a caller's defect that a
// compatible server would answer anyway, and answering it is how the defect
// reaches a real deployment unnoticed.
func TestARequestTheContractForbidsIsRefused(t *testing.T) {
	unbounded := ask
	unbounded.MaxOutputTokens = 0

	strange := ask
	strange.Messages = []runtime.Message{{Role: runtime.Role("tool"), Text: "a role the contract does not declare"}}

	cases := []struct {
		name  string
		req   runtime.GenerateRequest
		field string
	}{
		{"no messages", runtime.GenerateRequest{MaxOutputTokens: 8}, "Messages"},
		{"a role outside the declared set", strange, "Role"},
		{"no bound on the output", unbounded, "MaxOutputTokens"},
	}
	for _, one := range cases {
		t.Run(one.name, func(t *testing.T) {
			engine := behaving(t)
			_, _, _, err := collect(t, engine, t.Context(), one.req)
			if !errors.Is(err, runtime.ErrRefused) {
				t.Fatalf("the request came back with %v", err)
			}
			var failure *runtime.Error
			if !errors.As(err, &failure) {
				t.Fatalf("the failure is %T rather than one an adapter produces", err)
			}
			if failure.Field != one.field {
				t.Errorf("the refusal names the field %q and the defect is in %q", failure.Field, one.field)
			}
		})
	}
}

// TestARequestLargerThanTheDeclaredContextIsRefused is the ordinary way an
// over-limit failure arrives, as against the behaviour that forces one.
func TestARequestLargerThanTheDeclaredContextIsRefused(t *testing.T) {
	engine := behaving(t)

	long := ask
	long.Messages = []runtime.Message{{Role: runtime.RoleUser, Text: strings.Repeat("word ", declared.ContextLength+1)}}

	_, _, _, err := collect(t, engine, t.Context(), long)
	if !errors.Is(err, runtime.ErrOverLimit) {
		t.Fatalf("a request longer than the declared context came back with %v", err)
	}
}

// TestTextsSharingWordsComeBackCloserThanTextsSharingNone is the second half
// of #72's first line, and it is the line every later retrieval result rests
// on. A fake whose vectors carried no structure would let a ranking test pass
// while measuring nothing but the plumbing.
func TestTextsSharingWordsComeBackCloserThanTextsSharingNone(t *testing.T) {
	engine := behaving(t)

	embedded, err := engine.Embed(t.Context(), runtime.EmbedRequest{Texts: []string{
		"the minutes of the supervisory board meeting",
		"the minutes of the supervisory board session",
		"quarterly invoices from an unrelated supplier",
	}})
	if err != nil {
		t.Fatalf("Embed failed: %v", err)
	}
	if len(embedded.Vectors) != 3 {
		t.Fatalf("3 texts produced %d vector(s)", len(embedded.Vectors))
	}

	near := dot(embedded.Vectors[0], embedded.Vectors[1])
	far := dot(embedded.Vectors[0], embedded.Vectors[2])
	if !(near > far) {
		t.Errorf("two texts sharing most of their words scored %v and two sharing none scored %v", near, far)
	}
	if near < 0.5 {
		t.Errorf("two texts differing in one word scored %v, which is too little structure for a retrieval case to be about the query", near)
	}
	if math.Abs(float64(far)) > 0.25 {
		t.Errorf("two texts sharing no words scored %v, which is more structure than the hashing is entitled to", far)
	}
}

// TestWordsMoveAComponentInBothDirections is what holds the sign in the
// hashing, and it is written as an assertion about a vector rather than about
// a similarity because a similarity does not hold it. At the dimension a
// declaration is likely to name, two words rarely land on one component, so
// removing the sign leaves every comparison in this package where it was. The
// case that goes red is this one.
func TestWordsMoveAComponentInBothDirections(t *testing.T) {
	crowded := vector("the minutes of the supervisory board meeting", 8)

	up, down := false, false
	for _, component := range crowded {
		if component > 0 {
			up = true
		}
		if component < 0 {
			down = true
		}
	}
	if !up || !down {
		t.Errorf("the vector is %v; a hashing that only ever adds makes two words landing on one component reinforce each other", crowded)
	}
}

// TestEveryVectorHasUnitLength keeps a cosine a dot product, and holds the two
// ways a direction disappears: a text with no words at all, and words that
// cancel each other on one component.
func TestEveryVectorHasUnitLength(t *testing.T) {
	engine := behaving(t)

	embedded, err := engine.Embed(t.Context(), runtime.EmbedRequest{Texts: []string{
		"a passage of ordinary length",
		"",
		"   ",
		strings.Repeat("repeated ", 40),
	}})
	if err != nil {
		t.Fatalf("Embed failed: %v", err)
	}
	for i, vector := range embedded.Vectors {
		length := math.Sqrt(float64(dot(vector, vector)))
		if math.Abs(length-1) > 1e-5 {
			t.Errorf("vector %d has length %v", i, length)
		}
	}
}

// TestEmbeddingTheSameTextTwiceGivesTheSameVector is the embedding half of the
// determinism this package exists for.
func TestEmbeddingTheSameTextTwiceGivesTheSameVector(t *testing.T) {
	engine := behaving(t)

	req := runtime.EmbedRequest{Texts: []string{"the minutes of the supervisory board meeting"}}
	first, err := engine.Embed(t.Context(), req)
	if err != nil {
		t.Fatalf("Embed failed: %v", err)
	}
	second, err := engine.Embed(t.Context(), req)
	if err != nil {
		t.Fatalf("Embed failed: %v", err)
	}
	if dot(first.Vectors[0], second.Vectors[0]) < 1-1e-6 {
		t.Error("the same text embedded twice came back as two directions")
	}
	if first.InputTokens != second.InputTokens {
		t.Errorf("the same batch cost %d and then %d", first.InputTokens, second.InputTokens)
	}
}

// TestAnEngineThatDoesNotEmbedRefusesTo covers the declaration a deployment
// reads before it builds an index, and the empty batch beside it.
func TestAnEngineThatDoesNotEmbedRefusesTo(t *testing.T) {
	without := declared
	without.Embeddings = false
	without.EmbeddingDimension = 0

	engine, err := New(without, Behaviour{})
	if err != nil {
		t.Fatalf("New refused an engine that does not embed: %v", err)
	}
	if _, err := engine.Embed(t.Context(), runtime.EmbedRequest{Texts: []string{"a passage"}}); !errors.Is(err, runtime.ErrRefused) {
		t.Errorf("an engine declaring no embeddings answered with %v", err)
	}

	embedding := behaving(t)
	if _, err := embedding.Embed(t.Context(), runtime.EmbedRequest{}); !errors.Is(err, runtime.ErrRefused) {
		t.Errorf("a batch with no texts answered with %v", err)
	}
}

// TestOneFakeServesManyCallersAtOnce is what the concurrency proofs in #68 and
// the cancellation work in #76 need from it, and it is run under the race
// detector by the command the gate runs.
func TestOneFakeServesManyCallersAtOnce(t *testing.T) {
	engine := behaving(t)

	expected, _, _, err := collect(t, engine, t.Context(), ask)
	if err != nil {
		t.Fatalf("the first generation failed: %v", err)
	}

	var wait sync.WaitGroup
	answers := make([]string, 8)
	for i := range answers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			var built []string
			if _, err := engine.Generate(t.Context(), ask, func(chunk string) error {
				built = append(built, chunk)
				return nil
			}); err != nil {
				t.Errorf("a concurrent generation failed: %v", err)
				return
			}
			answers[i] = strings.Join(built, "")
		}()
	}
	wait.Wait()

	for i, got := range answers {
		if got != expected {
			t.Errorf("caller %d got %q and the answer for that request is %q", i, got, expected)
		}
	}
	if engine.Cancellations() != 0 {
		t.Errorf("nothing was cancelled and the engine counted %d", engine.Cancellations())
	}
}

// TestAnOperationOnAnEndedContextDoesNothing holds the cheapest half of the
// contract's sentence about cancellation: a call made with a context that has
// already ended does not reach the engine at all.
func TestAnOperationOnAnEndedContextDoesNothing(t *testing.T) {
	engine := behaving(t)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := engine.Capabilities(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("Capabilities answered an ended context with %v", err)
	}
	if _, err := engine.Embed(ctx, runtime.EmbedRequest{Texts: []string{"a passage"}}); !errors.Is(err, context.Canceled) {
		t.Errorf("Embed answered an ended context with %v", err)
	}
	streamed := 0
	if _, err := engine.Generate(ctx, ask, func(string) error {
		streamed++
		return nil
	}); !errors.Is(err, context.Canceled) {
		t.Errorf("Generate answered an ended context with %v", err)
	}
	if streamed != 0 {
		t.Errorf("%d chunk(s) were produced for a caller that had already gone", streamed)
	}
}

// dot is the similarity these vectors are compared with. It is a plain dot
// product because every vector this package produces has unit length, which
// the case above holds.
func dot(a, b []float32) float32 {
	if len(a) != len(b) {
		return 0
	}
	total := float32(0)
	for i := range a {
		total += a[i] * b[i]
	}
	return total
}
