package contract

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/iderex/kanzlei/internal/runtime"
)

// declared is the engine every case below is written against. Each near miss
// is the correct adapter with one thing moved, so a case that goes red goes
// red for the thing that moved.
var declared = runtime.Capabilities{
	Model:              "a-model",
	ModelVersion:       "2026-08-01",
	ContextLength:      64,
	Tools:              false,
	Embeddings:         true,
	EmbeddingDimension: 8,
}

// words is what a correct adapter generates. Four, so that a bound of one
// truncates and an unbounded request does not.
var words = []string{"alpha", "beta", "gamma", "delta"}

// A stub is an adapter written out rather than mocked, because every near miss
// below is a field of it set to something an adapter would do wrong.
//
// The zero value is an adapter that behaves. That is deliberate: a proof that
// a guard bites is only worth reading if the same adapter with the flaw
// removed is green.
type stub struct {
	// model is what the engine reports for itself, when that is to be
	// something other than what it was configured for.
	model string
	// nameless reports a declaration with no model identifier, which is a
	// declaration the contract itself refuses.
	nameless bool
	// ignoreContext carries on generating after the context has ended.
	ignoreContext bool
	// ignoreLimit answers a prompt longer than the declared context instead of
	// refusing it.
	ignoreLimit bool
	// hideBound reports an answer cut at MaxOutputTokens as one the engine
	// ended on its own.
	hideBound bool
	// bufferFirst produces the whole answer before consulting the sink.
	bufferFirst bool
	// silent produces no text and says nothing about it.
	silent bool
	// unattributed produces an answer naming no model, which is an answer
	// nothing downstream can cite.
	unattributed bool
	// leak keeps a goroutine working for that long after a cancelled
	// generation has returned, which is the accelerator a cancellation was
	// meant to free.
	leak time.Duration
	// vectors is how many vectors come back for a batch, when that is to be
	// the wrong count.
	vectors int
	// dimension is the number of components a vector comes back with, when
	// that is to be the wrong one.
	dimension int
	// failure is what every operation fails with, when it is to fail.
	failure error
}

func (s stub) Capabilities(context.Context) (runtime.Capabilities, error) {
	if s.failure != nil {
		return runtime.Capabilities{}, s.failure
	}
	reported := declared
	if s.model != "" {
		reported.Model = s.model
	}
	if s.nameless {
		reported.Model = ""
	}
	return reported, nil
}

func (s stub) Generate(ctx context.Context, req runtime.GenerateRequest, sink runtime.Sink) (runtime.GenerateResult, error) {
	if s.failure != nil {
		return runtime.GenerateResult{}, s.failure
	}

	input := 0
	for _, message := range req.Messages {
		input += len(strings.Fields(message.Text))
	}
	if input > declared.ContextLength && !s.ignoreLimit {
		return runtime.GenerateResult{}, runtime.Fail(runtime.ErrOverLimit, "stub", "Messages", errors.New("the request is longer than the engine declares"))
	}

	produced := words
	finish := runtime.FinishComplete
	if len(produced) > req.MaxOutputTokens {
		produced = produced[:req.MaxOutputTokens]
		finish = runtime.FinishOutputLimit
		if s.hideBound {
			finish = runtime.FinishComplete
		}
	}
	if s.silent {
		produced = nil
	}

	result := runtime.GenerateResult{Model: declared.Model, ModelVersion: declared.ModelVersion, InputTokens: input}
	if s.unattributed {
		result.Model, result.ModelVersion = "", ""
	}
	var refused error
	for _, word := range produced {
		if err := ctx.Err(); err != nil && !s.ignoreContext {
			result.Finish = runtime.FinishCancelled
			if s.leak > 0 {
				go time.Sleep(s.leak)
			}
			return result, err
		}
		if err := sink(word); err != nil {
			if !s.bufferFirst {
				return runtime.GenerateResult{}, err
			}
			refused = err
		}
		result.OutputTokens++
	}
	if refused != nil {
		return runtime.GenerateResult{}, refused
	}
	result.Finish = finish
	return result, nil
}

func (s stub) Embed(_ context.Context, req runtime.EmbedRequest) (runtime.EmbedResult, error) {
	if s.failure != nil {
		return runtime.EmbedResult{}, s.failure
	}
	dimension := declared.EmbeddingDimension
	if s.dimension != 0 {
		dimension = s.dimension
	}
	result := runtime.EmbedResult{Model: declared.Model, ModelVersion: declared.ModelVersion}
	if s.unattributed {
		result.Model, result.ModelVersion = "", ""
	}
	count := len(req.Texts)
	if s.vectors != 0 {
		count = s.vectors
	}
	for range count {
		result.Vectors = append(result.Vectors, make([]float32, dimension))
	}
	return result, nil
}

// subject wraps one adapter as a subject that behaves in every condition,
// which is what the near misses need: the case under proof is the only thing
// that can fail.
func subject(adapter runtime.Runtime) Subject {
	return Subject{
		Name:     "stub",
		Declared: declared,
		Under: func(_ *testing.T, condition Condition) runtime.Runtime {
			switch condition {
			case Unreachable:
				return stub{failure: runtime.Fail(runtime.ErrUnreachable, "stub", "address", nil)}
			case Refusing:
				return stub{failure: runtime.Fail(runtime.ErrRefused, "stub", "request", nil)}
			case Malformed:
				return stub{failure: runtime.Fail(runtime.ErrMalformed, "stub", "chunk", nil)}
			default:
				return adapter
			}
		},
	}
}

// A recorder is a reporter that keeps what a case said instead of failing the
// run, so a near miss can be proved to go red by reading the message it
// produced.
type recorder struct {
	failures []string
	logs     []string
}

func (r *recorder) Helper() {}

func (r *recorder) Logf(format string, args ...any) {
	r.logs = append(r.logs, fmt.Sprintf(format, args...))
}

func (r *recorder) Errorf(format string, args ...any) {
	r.failures = append(r.failures, fmt.Sprintf(format, args...))
}

func (r *recorder) Fatalf(format string, args ...any) {
	r.failures = append(r.failures, fmt.Sprintf(format, args...))
}

// only runs one named case against one adapter and reports what it said.
func only(t *testing.T, name string, adapter runtime.Runtime) []string {
	t.Helper()
	s := subject(adapter)
	for _, c := range cases {
		if c.name != name {
			continue
		}
		r := &recorder{}
		execute(r, c, s, s.Under(t, c.condition))
		return r.failures
	}
	t.Fatalf("there is no case called %q in the suite, so this proof is written against a name that moved", name)
	return nil
}

// TestTheSuitePassesAnAdapterThatBehaves is the other half of every proof
// below. Without it a suite that failed everything would look like a suite
// whose guards all bite.
func TestTheSuitePassesAnAdapterThatBehaves(t *testing.T) {
	Run(t, subject(stub{}))
}

// TestEveryCaseRefusesTheAdapterItIsFor runs each case against an adapter with
// exactly the defect that case exists to catch, and requires the case to
// refuse it. Removing the defect is the green run above.
func TestEveryCaseRefusesTheAdapterItIsFor(t *testing.T) {
	// The leak below is only visible once the wait for it has run out, and
	// five seconds of the default run spent proving one guard is how a suite
	// becomes one people run less often.
	patient := settleWithin
	settleWithin = 100 * time.Millisecond
	defer func() { settleWithin = patient }()

	for _, near := range []struct {
		name    string
		adapter stub
		defect  string
	}{
		{"the-declaration-is-the-configured-one", stub{model: "another-model"}, "an engine serving a model other than the configured one"},
		{"the-declaration-is-the-configured-one", stub{nameless: true}, "a declaration with no model identifier on it"},
		{"the-declaration-is-the-configured-one", stub{failure: errors.New("no answer")}, "an engine that cannot say what it is"},
		{"a-stream-ends-cleanly", stub{silent: true}, "a generation that produced nothing and said nothing about it"},
		{"a-stream-ends-cleanly", stub{unattributed: true}, "an answer attributed to no model"},
		{"a-stream-ends-cleanly", stub{failure: errors.New("no answer")}, "an ordinary generation that failed"},
		{"the-sink-is-consulted-while-the-answer-is-produced", stub{bufferFirst: true}, "an answer produced before the sink was consulted"},
		{"a-cancellation-mid-stream-leaves-nothing-behind", stub{ignoreContext: true}, "a generation that carried on after the context ended"},
		{"a-cancellation-mid-stream-leaves-nothing-behind", stub{leak: 2 * time.Second}, "a generation that stopped answering and did not stop working"},
		{"an-over-long-prompt-is-refused-before-anything-is-produced", stub{ignoreLimit: true}, "a prompt longer than the declared context answered instead of refused"},
		{"an-answer-stopped-at-the-bound-says-so", stub{hideBound: true}, "an answer cut at the bound reported as one the engine ended"},
		{"an-answer-stopped-at-the-bound-says-so", stub{failure: errors.New("no answer")}, "a bounded generation that failed"},
		{"an-embedding-has-the-declared-dimension", stub{dimension: declared.EmbeddingDimension + 1}, "a vector of a length the index was not built for"},
		{"an-embedding-has-the-declared-dimension", stub{vectors: 1}, "a batch answered with fewer vectors than it had texts"},
		{"an-embedding-has-the-declared-dimension", stub{unattributed: true}, "vectors attributed to no model"},
		{"an-embedding-has-the-declared-dimension", stub{failure: errors.New("no answer")}, "an embedding batch that failed"},
	} {
		t.Run(near.defect, func(t *testing.T) {
			if failures := only(t, near.name, near.adapter); len(failures) == 0 {
				t.Fatalf("%q passed %s, so the case does not refuse what it is for", near.name, near.defect)
			}
			if failures := only(t, near.name, stub{}); len(failures) != 0 {
				t.Fatalf("%q refused an adapter that behaves: %v", near.name, failures)
			}
		})
	}
}

// TestEveryConditionRefusesTheAdapterItIsFor covers the three cases that are
// about how an adapter fails rather than about what it produces. Each is run
// against an engine that fails in the way the other two are for, so a case
// that accepted any failure at all would go green here.
func TestEveryConditionRefusesTheAdapterItIsFor(t *testing.T) {
	kinds := map[string]error{
		"an-engine-that-does-not-answer-is-unreachable": runtime.ErrUnreachable,
		"an-engine-that-declines-is-refused":            runtime.ErrRefused,
		"a-stream-that-stops-part-way-is-malformed":     runtime.ErrMalformed,
	}
	for name, kind := range kinds {
		for other, wrong := range kinds {
			if other == name {
				continue
			}
			t.Run(name+"/"+other, func(t *testing.T) {
				if failures := failing(t, name, runtime.Fail(wrong, "stub", "request", nil)); len(failures) == 0 {
					t.Fatalf("an engine that failed with %v passed the case asking for %v, so the two are not told apart", wrong, kind)
				}
			})
		}
	}
}

// TestAFailureTheContractCannotPlaceIsRefused is the shape an adapter acquires
// by building an error itself instead of through runtime.Fail. All three come
// back to a caller as something, and none of the three can be sorted into one
// of the four behaviours the kinds exist for.
func TestAFailureTheContractCannotPlaceIsRefused(t *testing.T) {
	for _, c := range []struct {
		name    string
		failure error
	}{
		{"an error carrying no kind at all", errors.New("something went wrong")},
		{"the sentinel returned rather than built through Fail", fmt.Errorf("dial: %w", runtime.ErrUnreachable)},
		{"an error naming no adapter", &runtime.Error{Kind: runtime.ErrUnreachable}},
	} {
		t.Run(c.name, func(t *testing.T) {
			if failures := failing(t, "an-engine-that-does-not-answer-is-unreachable", c.failure); len(failures) == 0 {
				t.Fatalf("the suite accepted %s", c.name)
			}
		})
	}

	if failures := failing(t, "an-engine-that-does-not-answer-is-unreachable", runtime.Fail(runtime.ErrUnreachable, "stub", "address", nil)); len(failures) != 0 {
		t.Fatalf("the same case refused a failure built the way the contract asks for: %v", failures)
	}
}

// failing runs one named case against an engine that fails with exactly that
// error, whatever condition the case asks for.
func failing(t *testing.T, name string, failure error) []string {
	t.Helper()
	broken := stub{failure: failure}
	s := subject(broken)
	s.Under = func(*testing.T, Condition) runtime.Runtime { return broken }

	r := &recorder{}
	for _, definition := range cases {
		if definition.name == name {
			execute(r, definition, s, broken)
		}
	}
	return r.failures
}

// TestAPanicThatIsNotACaseEndingIsNotSwallowed holds the one thing the
// reporter indirection could quietly break. A case that ends itself is
// absorbed; a defect in the suite is not, because a suite that swallowed its
// own panics would report a green run over a case that never finished.
func TestAPanicThatIsNotACaseEndingIsNotSwallowed(t *testing.T) {
	defect := errors.New("a defect inside the suite")
	defer func() {
		raised := recover()
		if raised != defect {
			t.Fatalf("the suite absorbed %v, and a panic that is not a case ending is a defect the run has to show", raised)
		}
	}()

	execute(&recorder{}, caseDef{
		name:      "a-case-that-is-broken",
		condition: Healthy,
		run:       func(reporter, Subject, runtime.Runtime) { panic(defect) },
	}, subject(stub{}), stub{})
}

// TestASubjectTheSuiteCannotJudgeIsRefused holds the three directions Cannot
// fails closed in, and the two ways a subject arrives unusable. Each is a
// thing somebody writes while making a case go green.
func TestASubjectTheSuiteCannotJudgeIsRefused(t *testing.T) {
	usable := subject(stub{})
	for _, c := range []struct {
		name    string
		subject Subject
		want    string
	}{
		{"no name", Subject{Declared: declared, Under: usable.Under}, "no name"},
		{"no adapter", Subject{Name: "stub", Declared: declared}, "supplies no adapter"},
		{"a declaration the contract refuses", Subject{Name: "stub", Under: usable.Under}, "already refuses"},
		{"cannot be healthy", withCannot(usable, Healthy, "the engine is never up"), "cannot be healthy"},
		{"a condition that is not one", withCannot(usable, Condition("unreachble"), "the address always answers"), "not a condition this suite has"},
		{"a reason of one word", withCannot(usable, Unreachable, "no"), "one word is the same as none"},
	} {
		t.Run(c.name, func(t *testing.T) {
			err := c.subject.validate()
			if err == nil {
				t.Fatalf("the suite accepted a subject with %s", c.name)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("the refusal was %q and does not say %q, so a reader is not told what to change", err, c.want)
			}
		})
	}

	if err := withCannot(usable, Unreachable, "this engine has no address that can be made to stop answering").validate(); err != nil {
		t.Fatalf("the suite refused a subject that declared a condition it cannot arrange, with a reason: %v", err)
	}
}

// withCannot is one subject with one excuse on it.
func withCannot(s Subject, condition Condition, reason string) Subject {
	s.Cannot = map[Condition]string{condition: reason}
	return s
}

// TestADeclinedConditionIsNotRunAndIsPrinted is the accounting. A suite that
// ran nothing and a suite that ran everything are otherwise the same green
// tick, which is the failure the harness under test/ was built against.
func TestADeclinedConditionIsNotRunAndIsPrinted(t *testing.T) {
	reached := map[Condition]bool{}
	s := subject(stub{})
	s.Cannot = map[Condition]string{Malformed: "this engine cannot be made to stop mid-stream"}
	inner := s.Under
	s.Under = func(t *testing.T, condition Condition) runtime.Runtime {
		reached[condition] = true
		return inner(t, condition)
	}

	Run(t, s)

	if reached[Malformed] {
		t.Fatal("a condition the subject declared it cannot arrange was arranged anyway")
	}
	if !reached[Healthy] || !reached[Unreachable] || !reached[Refusing] {
		t.Fatalf("one excuse stopped more than the case it was for: %v", reached)
	}
}
