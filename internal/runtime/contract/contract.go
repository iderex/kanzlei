// Package contract is the suite every model runtime adapter has to pass.
//
// The wire shape the local engines speak is a convention, and they differ at
// its edges: how a stream ends, what happens to an over-long prompt, whether a
// cancellation is noticed, which failure is reported as which. Those
// differences are the reason internal/runtime exists, and this package is
// where each one is pinned so that a new engine version cannot change
// behaviour quietly. #73 is the issue this holds.
//
// It is a suite handed an adapter rather than a set of cases written per
// adapter. A case written twice is a case that diverges, and the divergence is
// invisible because both copies are green.
//
// What it can see, and what it cannot, stated here rather than left to be
// discovered. It drives an adapter through the interface and reads what comes
// back, so it judges behaviour at that boundary and nothing beyond it: it
// cannot see a request leave a machine, it cannot read an engine's own logs,
// and it cannot tell an answer that is wrong from one that is right. Every
// case below says in its own message what its failure means.
//
// docs/runtimes.md is where a difference this suite finds is written down,
// with the engine and version it was observed on.
package contract

import (
	"context"
	"errors"
	"fmt"
	goruntime "runtime"
	"strings"
	"testing"
	"time"

	"github.com/iderex/kanzlei/internal/runtime"
)

// A Condition is a situation an adapter has to be put in before a case can be
// run against it.
//
// It is asked for rather than arranged, because how an engine is made
// unreachable is the adapter's own business: the fake is told to be, and a
// real adapter is pointed at an address that does not answer. A suite that
// arranged the condition itself would be a suite that knows which engine it
// has, which is the thing internal/runtime exists to stop.
type Condition string

const (
	// Healthy is an engine that behaves. Every adapter has to be able to
	// produce one, because an adapter that cannot is not an adapter.
	Healthy Condition = "healthy"
	// Unreachable is an engine that does not answer at all.
	Unreachable Condition = "unreachable"
	// Refusing is an engine that answers and declines.
	Refusing Condition = "refusing"
	// Malformed is an engine whose stream stops part way with nothing saying
	// it had. It is the condition a caller is most likely to mistake for a
	// complete answer, because everything that arrived was well formed.
	Malformed Condition = "malformed"
)

// Conditions is the declared set. A Cannot entry naming anything else is
// refused as a typo rather than passed over, because a condition nobody can
// spell is a case that silently never runs.
var Conditions = []Condition{Healthy, Unreachable, Refusing, Malformed}

// timeout bounds every call this suite makes, so an adapter that never returns
// fails as a case rather than as a suite that hangs until the harness kills
// it.
const timeout = 30 * time.Second

// A Subject is one adapter offered to the suite.
type Subject struct {
	// Name is the adapter's own name, which is what a failure message names.
	Name string
	// Declared is what the engine under test says about itself. The suite
	// compares the adapter's answer against it, so a subject that declared
	// something the engine does not report fails here rather than later.
	Declared runtime.Capabilities
	// Under returns an adapter with its engine in the named condition. It is
	// called once per case, so a case cannot be affected by what a previous
	// one did to the engine.
	Under func(t *testing.T, condition Condition) runtime.Runtime
	// Cannot names a condition this adapter cannot be put in, against the
	// reason it cannot. It exists because an adapter for a real engine may not
	// be able to manufacture every failure on demand, and the alternative to
	// declaring that is a case quietly written to pass.
	//
	// It fails closed in three directions. Healthy may not appear in it. A
	// condition outside the declared set may not appear in it. And a reason of
	// one word may not, which is the rule internal/sourcecheck already holds
	// for a suppression: a reason nobody can read is the same as none.
	Cannot map[Condition]string
}

// A reporter is what a case reports to.
//
// testing.T satisfies it. The indirection is what lets the proofs beside this
// file run the suite against a deliberately broken adapter and read the
// failure, instead of asserting that a guard bites by describing it.
type reporter interface {
	Helper()
	Logf(format string, args ...any)
	Errorf(format string, args ...any)
	Fatalf(format string, args ...any)
}

// A caseDef is one thing the suite asks of an adapter.
type caseDef struct {
	// name is what the subtest is called.
	name string
	// condition is the state the engine has to be in for it.
	condition Condition
	// embeddings says the case is only meaningful where the declaration says
	// the engine produces them.
	embeddings bool
	// run is the case.
	run func(r reporter, s Subject, rt runtime.Runtime)
}

// Run drives one adapter through the whole suite.
//
// It is what a new adapter's pull request shows: the adapter, handed to this,
// green. A case that did not run is printed rather than passed over, because a
// run that covered part of the set and one that covered all of it are
// otherwise the same green tick.
func Run(t *testing.T, subject Subject) {
	t.Helper()
	if err := subject.validate(); err != nil {
		t.Fatalf("this subject cannot be handed to the contract suite: %v", err)
	}

	ran, declined, unreached := 0, 0, 0
	for _, c := range cases {
		if reason, cannot := subject.Cannot[c.condition]; cannot {
			declined++
			t.Logf("%s: not run, because %s cannot be put in the %q condition: %s", c.name, subject.Name, c.condition, reason)
			continue
		}
		if c.embeddings && !subject.Declared.Embeddings {
			unreached++
			t.Logf("%s: not run, because %s declares no embeddings", c.name, subject.Name)
			continue
		}
		ran++
		t.Run(c.name, func(t *testing.T) {
			rt := subject.Under(t, c.condition)
			if rt == nil {
				t.Fatalf("%s produced no adapter for the %q condition, and a nil adapter is not a case that passed", subject.Name, c.condition)
			}
			execute(t, c, subject, rt)
		})
	}

	t.Logf("contract: %s, %d case(s) run, %d not arrangeable, %d outside the declaration", subject.Name, ran, declined, unreached)
	t.Logf("A case that did not run proved nothing about %s.", subject.Name)
}

// stop is what a case's Fatalf raises against a reporter that is not a
// testing.T, so a fatal case ends where testing.T would have ended it.
var stop = errors.New("the case ended")

// execute runs one case and absorbs the fatal a reporter raises.
//
// recover returns nil while testing.T is unwinding its own Fatalf, so the
// clause below leaves that path alone and re-raises anything that is neither.
func execute(r reporter, c caseDef, s Subject, rt runtime.Runtime) {
	defer func() {
		if raised := recover(); raised != nil && raised != stop {
			panic(raised)
		}
	}()
	c.run(r, s, rt)
}

// validate refuses a subject the suite cannot judge.
func (s Subject) validate() error {
	if strings.TrimSpace(s.Name) == "" {
		return errors.New("the subject has no name, and a failure that names no adapter cannot be placed")
	}
	if s.Under == nil {
		return errors.New("the subject supplies no adapter")
	}
	if err := s.Declared.Validate(); err != nil {
		return fmt.Errorf("the subject declares what the contract already refuses: %w", err)
	}
	for condition, reason := range s.Cannot {
		if condition == Healthy {
			return errors.New("the subject declares it cannot be healthy, and an adapter that cannot reach a working engine is not one this suite can say anything about")
		}
		if !known(condition) {
			return fmt.Errorf("the subject declares it cannot arrange %q, which is not a condition this suite has, so the case it was meant to excuse would run anyway", condition)
		}
		if len(strings.Fields(reason)) < 2 {
			return fmt.Errorf("the subject excuses the %q condition with %q, and a reason of one word is the same as none", condition, reason)
		}
	}
	return nil
}

// known says whether a condition is one the suite declares.
func known(condition Condition) bool {
	for _, declared := range Conditions {
		if condition == declared {
			return true
		}
	}
	return false
}

// ask is the request every generation case starts from: two turns, a bound the
// contract requires, and nothing an engine could refuse for its content.
func ask() runtime.GenerateRequest {
	return runtime.GenerateRequest{
		Messages: []runtime.Message{
			{Role: runtime.RoleSystem, Text: "Answer from the passages given to you."},
			{Role: runtime.RoleUser, Text: "What does the retention section say?"},
		},
		MaxOutputTokens: 64,
	}
}

// discard is a sink for a case that is about the failure rather than about the
// text.
func discard(string) error { return nil }

// named refuses an error that does not carry which adapter produced it and
// which of the four kinds it is.
func named(r reporter, err error, kind error, adapter string) {
	r.Helper()
	if !errors.Is(err, kind) {
		r.Fatalf("%s answered with %v, and the contract declares this failure as %v", adapter, err, kind)
		panic(stop)
	}
	var failure *runtime.Error
	if !errors.As(err, &failure) {
		r.Fatalf("%s answered with %v, which is not a runtime.Error, and runtime.Fail is the only way one is built so that an error leaving an adapter cannot be missing its kind", adapter, err)
		panic(stop)
	}
	if failure.Adapter == "" {
		r.Fatalf("%s produced a failure naming no adapter, and an operator reading it cannot tell an engine's quirk from a defect here", adapter)
		panic(stop)
	}
}

// cases is the whole suite, in the order a reader of a failing run wants them:
// what the engine says about itself, then what it does, then how it fails.
var cases = []caseDef{
	{name: "the-declaration-is-the-configured-one", condition: Healthy, run: declarationIsTheConfiguredOne},
	{name: "a-stream-ends-cleanly", condition: Healthy, run: streamEndsCleanly},
	{name: "the-sink-is-consulted-while-the-answer-is-produced", condition: Healthy, run: sinkIsConsultedWhileProducing},
	{name: "a-cancellation-mid-stream-leaves-nothing-behind", condition: Healthy, run: cancellationLeavesNothingBehind},
	{name: "an-over-long-prompt-is-refused-before-anything-is-produced", condition: Healthy, run: overLongPromptIsRefused},
	{name: "an-answer-stopped-at-the-bound-says-so", condition: Healthy, run: boundIsReported},
	{name: "an-embedding-has-the-declared-dimension", condition: Healthy, embeddings: true, run: embeddingHasDeclaredDimension},
	{name: "an-engine-that-does-not-answer-is-unreachable", condition: Unreachable, run: unreachableIsReported},
	{name: "an-engine-that-declines-is-refused", condition: Refusing, run: refusalIsReported},
	{name: "a-stream-that-stops-part-way-is-malformed", condition: Malformed, run: malformedIsReported},
}

// declarationIsTheConfiguredOne holds the half of #71 that only an adapter can
// hold: a declaration is asked of the engine rather than read from a
// configuration file, so an engine serving a different model than the one this
// deployment was configured for is visible instead of quiet.
func declarationIsTheConfiguredOne(r reporter, s Subject, rt runtime.Runtime) {
	r.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	got, err := rt.Capabilities(ctx)
	if err != nil {
		r.Fatalf("%s could not report its capabilities: %v", s.Name, err)
		panic(stop)
	}
	if err := got.Validate(); err != nil {
		r.Fatalf("%s reported a declaration the contract refuses: %v", s.Name, err)
		panic(stop)
	}
	if got != s.Declared {
		r.Fatalf("%s reports %+v and was configured for %+v; an engine serving something other than what it was asked for is the failure docs/decisions/0009-runtime.md names", s.Name, got, s.Declared)
		panic(stop)
	}
}

// streamEndsCleanly is the ordinary generation, and what it pins is the end
// rather than the text. An answer that arrived and a result saying why it
// stopped are two different facts, and #66 reads the second one.
func streamEndsCleanly(r reporter, s Subject, rt runtime.Runtime) {
	r.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	chunks, bytes := 0, 0
	result, err := rt.Generate(ctx, ask(), func(chunk string) error {
		chunks++
		bytes += len(chunk)
		return nil
	})
	if err != nil {
		r.Fatalf("%s failed an ordinary generation: %v", s.Name, err)
		panic(stop)
	}
	if chunks == 0 || bytes == 0 {
		r.Fatalf("%s produced %d chunk(s) and %d byte(s), so nothing was generated and nothing said so", s.Name, chunks, bytes)
		panic(stop)
	}
	if result.Finish != runtime.FinishComplete {
		r.Fatalf("%s ended an unbounded answer as %q rather than %q, and a caller cannot tell a complete answer from a stopped one by reading the text", s.Name, result.Finish, runtime.FinishComplete)
		panic(stop)
	}
	if result.Model == "" || result.ModelVersion == "" {
		r.Fatalf("%s produced an answer attributed to model %q version %q, and #65 has to cite what produced what", s.Name, result.Model, result.ModelVersion)
		panic(stop)
	}
	if result.OutputTokens <= 0 {
		r.Fatalf("%s produced text and reported %d output token(s), so the cost of an answer cannot be read from the result", s.Name, result.OutputTokens)
		panic(stop)
	}
}

// sinkIsConsultedWhileProducing is what separates streaming from an answer
// assembled and then handed over in pieces. A caller whose reader has gone
// away stops paying an engine to keep producing only if the sink is read as it
// goes.
func sinkIsConsultedWhileProducing(r reporter, s Subject, rt runtime.Runtime) {
	r.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	gone := errors.New("the reader has gone away")
	chunks := 0
	_, err := rt.Generate(ctx, ask(), func(string) error {
		chunks++
		return gone
	})
	if !errors.Is(err, gone) {
		r.Fatalf("%s answered a sink that refused the first chunk with %v, and the contract returns the sink's own error", s.Name, err)
		panic(stop)
	}
	if chunks != 1 {
		r.Fatalf("%s called a sink that refused its first chunk %d time(s), so the answer was produced before the sink was consulted", s.Name, chunks)
		panic(stop)
	}
}

// cancellationLeavesNothingBehind is #76's half that an adapter owns. A
// handler that returned is not the same as a generation that stopped, and the
// goroutine count is the only part of the second one a suite can read from
// outside.
func cancellationLeavesNothingBehind(r reporter, s Subject, rt runtime.Runtime) {
	r.Helper()
	before := quiesce()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	chunks := 0
	result, err := rt.Generate(ctx, ask(), func(string) error {
		chunks++
		cancel()
		return nil
	})
	if chunks == 0 {
		r.Fatalf("%s produced nothing before the cancellation, so this case cancelled a stream that had not started", s.Name)
		panic(stop)
	}
	if !errors.Is(err, context.Canceled) {
		r.Fatalf("%s answered a cancelled generation with %v, and a truncated answer handed back with no error is stored as a complete one", s.Name, err)
		panic(stop)
	}
	if result.Finish != runtime.FinishCancelled {
		r.Fatalf("%s ended a cancelled generation as %q rather than %q", s.Name, result.Finish, runtime.FinishCancelled)
		panic(stop)
	}
	settle(r, s.Name, before)
}

// settleWithin is how long a cancelled generation has to stop working.
//
// It is a variable rather than a constant so that the proof beside this file
// can shorten it. A case proving that a leaked goroutine is caught has to wait
// out this whole period to see the refusal, and five seconds of every default
// run spent proving one guard is how a suite becomes one people run less
// often.
var settleWithin = 5 * time.Second

// quiesce waits for the goroutine count to stop moving, and reports it.
//
// The baseline this case compares against has to be taken when nothing else is
// going away, because a count read while a previous case's timers are still
// unwinding is too high and a leak then hides underneath it. Two readings that
// agree is the cheapest test for that, and a count that never settles is
// reported as it stands rather than waited on forever.
func quiesce() int {
	deadline := time.Now().Add(settleWithin)
	last := goruntime.NumGoroutine()
	for time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
		now := goruntime.NumGoroutine()
		if now == last {
			return now
		}
		last = now
	}
	return last
}

// settle waits for the goroutines a cancelled generation was using to go away,
// and refuses the ones that do not.
//
// It waits rather than reading once, because a goroutine returning is not
// instantaneous and a case that read the count immediately would fail on
// timing rather than on a leak.
func settle(r reporter, name string, before int) {
	r.Helper()
	deadline := time.Now().Add(settleWithin)
	for {
		after := goruntime.NumGoroutine()
		if after <= before {
			return
		}
		if time.Now().After(deadline) {
			r.Fatalf("%d goroutine(s) were running before the cancelled generation and %d still are, so %s stopped answering and did not stop working", before, after, name)
			panic(stop)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// overLongPromptIsRefused is the first half of #73's sixth condition. An
// adapter that forwards a prompt longer than the engine declared gets back an
// answer built from part of it, and nothing downstream can tell that from a
// complete one.
func overLongPromptIsRefused(r reporter, s Subject, rt runtime.Runtime) {
	r.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	request := ask()
	request.Messages = append(request.Messages, runtime.Message{
		Role: runtime.RoleUser,
		Text: strings.TrimSpace(strings.Repeat("retention ", s.Declared.ContextLength+1)),
	})

	chunks := 0
	_, err := rt.Generate(ctx, request, func(string) error {
		chunks++
		return nil
	})
	named(r, err, runtime.ErrOverLimit, s.Name)
	if chunks != 0 {
		r.Fatalf("%s produced %d chunk(s) from a prompt longer than the %d tokens it declares, so the prompt was sent and something was truncated", s.Name, chunks, s.Declared.ContextLength)
		panic(stop)
	}
}

// boundIsReported is the second half of #73's sixth condition, and it is the
// only case here that catches an adapter which works. An answer that stopped
// at MaxOutputTokens reported as one the engine ended on its own is a
// truncated answer presented as a complete one.
func boundIsReported(r reporter, s Subject, rt runtime.Runtime) {
	r.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	request := ask()
	request.MaxOutputTokens = 1

	result, err := rt.Generate(ctx, request, discard)
	if err != nil {
		r.Fatalf("%s failed a generation bounded at one token: %v", s.Name, err)
		panic(stop)
	}
	if result.Finish != runtime.FinishOutputLimit {
		r.Fatalf("%s stopped at a bound of one token and reported %q rather than %q, so a truncated answer reaches #66 as a complete one", s.Name, result.Finish, runtime.FinishOutputLimit)
		panic(stop)
	}
}

// embeddingHasDeclaredDimension is what an index is built against. A vector of
// a length nobody checked is found on the day the index refuses it, by which
// time a corpus has been embedded.
func embeddingHasDeclaredDimension(r reporter, s Subject, rt runtime.Runtime) {
	r.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	texts := []string{"the retention section", "an unrelated passage"}
	result, err := rt.Embed(ctx, runtime.EmbedRequest{Texts: texts})
	if err != nil {
		r.Fatalf("%s could not embed %d text(s): %v", s.Name, len(texts), err)
		panic(stop)
	}
	if len(result.Vectors) != len(texts) {
		r.Fatalf("%s was given %d text(s) and returned %d vector(s), and a batch whose order cannot be relied on cannot be written to an index", s.Name, len(texts), len(result.Vectors))
		panic(stop)
	}
	for i, vector := range result.Vectors {
		if len(vector) != s.Declared.EmbeddingDimension {
			r.Fatalf("%s returned vector %d with %d component(s) and declares %d", s.Name, i, len(vector), s.Declared.EmbeddingDimension)
			panic(stop)
		}
	}
	if result.Model == "" || result.ModelVersion == "" {
		r.Fatalf("%s produced vectors attributed to model %q version %q, and #61 cannot tell a stale vector from a current one without both", s.Name, result.Model, result.ModelVersion)
		panic(stop)
	}
}

// unreachableIsReported is the engine that did not answer. The request did not
// happen, which is what lets a caller retry it.
func unreachableIsReported(r reporter, s Subject, rt runtime.Runtime) {
	r.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	_, err := rt.Generate(ctx, ask(), discard)
	named(r, err, runtime.ErrUnreachable, s.Name)
}

// refusalIsReported is the engine that answered and declined. The request
// happened and was turned away, and an operator reading #91 needs that told
// apart from an engine that is down.
func refusalIsReported(r reporter, s Subject, rt runtime.Runtime) {
	r.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	_, err := rt.Generate(ctx, ask(), discard)
	named(r, err, runtime.ErrRefused, s.Name)
}

// malformedIsReported is the stream that stopped with nothing saying it had. A
// half-understood response salvaged into an answer is how a truncated answer
// is presented as a complete one.
func malformedIsReported(r reporter, s Subject, rt runtime.Runtime) {
	r.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	_, err := rt.Generate(ctx, ask(), discard)
	named(r, err, runtime.ErrMalformed, s.Name)
}
