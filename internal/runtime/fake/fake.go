// Package fake is a model runtime that answers without a model.
//
// It exists for the condition in #7: the default suite runs with no
// accelerator and no outbound network, and almost everything in this project
// reaches a runtime somewhere. A suite that needed an engine is a suite nobody
// runs, and a suite that reached a real one measures the engine rather than
// the code.
//
// Everything it produces is derived from the request by a digest, so the same
// request produces the same bytes on every machine and on every run. That is
// the property tests here depend on and it is also the whole of what this is:
// nothing in this package has any idea what any of the words mean.
//
// It is deliberately stricter than an engine in two places, because a fake
// that accepted what the contract forbids is a fake that hides a caller's
// defect until an adapter meets a real engine. A request with no output bound
// and a message carrying a role outside the declared set are both refused
// here, and neither would be refused by a compatible server.
//
// docs/fake-runtime.md is where what a result from this proves, and what it
// does not, is written down. Nothing produced here is evidence about a model.
package fake

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/fnv"
	"math"
	"strings"
	"sync"

	"github.com/iderex/kanzlei/internal/runtime"
)

// adapter is the name this fake puts on every failure it produces, so a
// refusal read in a test names where it came from the same way an adapter's
// would.
const adapter = "fake"

// answerWords is how many words an unbounded answer holds, the first of them
// the marker below.
const answerWords = 6

// marker is the first word of every answer. It is here so that a fake answer
// is recognisable as one wherever it turns up: in a log, in a fixture, or
// pasted into an issue as though an engine had produced it.
const marker = "fake"

// A Behaviour is what this engine does wrong, and nothing here is a default.
// The zero value is an engine that behaves, which is what most callers want,
// and each field below is one of the misbehaviours #72 asks for.
//
// They are fields rather than an interface because a test states the failure
// it is about in the construction and then makes an ordinary call. A callback
// that decided per call would put the failure somewhere a reader of the test
// has to go and find.
type Behaviour struct {
	// Unreachable makes every operation fail with runtime.ErrUnreachable
	// before anything happens. The request did not reach an engine.
	Unreachable bool
	// Refuse makes every operation fail with runtime.ErrRefused. The engine
	// answered and declined, which is a different thing for a caller to do
	// something about.
	Refuse bool
	// Hang makes every operation block until the context ends and then fail
	// with runtime.ErrUnreachable, which is what a deadline reached before an
	// answer arrived looks like. It is the timeout: a test sets a deadline or
	// cancels, and nothing here returns until it does.
	Hang bool
	// ChunksBeforeFailure streams that many chunks and then fails with
	// runtime.ErrMalformed. It is the partial stream, and it is the shape a
	// caller is most likely to mistake for a complete answer, because
	// everything it received was well formed.
	ChunksBeforeFailure int
	// OverLimit makes every generation fail with runtime.ErrOverLimit whatever
	// the request holds, so a caller's handling of an oversized request is
	// reachable without building one.
	OverLimit bool
	// Dimension is the number of components a vector comes back with, when the
	// engine is to return the wrong one. Zero is the declared dimension, which
	// is the engine behaving.
	Dimension int
	// Model is the identifier the engine reports for itself, when it is to
	// report one other than the configured one. Empty is the declared
	// identifier. This is the quiet failure docs/decisions/0009-runtime.md
	// names: an engine serving a different model, with nothing in an answer
	// admitting it.
	Model string
}

// A Runtime is one configured fake engine.
//
// It is safe to use from several goroutines, because the concurrency proofs
// this is for drive one runtime from many. Nothing about it changes after New
// except the count of generations the context ended.
type Runtime struct {
	declared  runtime.Capabilities
	behaviour Behaviour

	mu        sync.Mutex
	cancelled int
}

// New builds a fake engine that declares what the capabilities say and
// misbehaves the way the behaviour says.
//
// It refuses a declaration runtime.Capabilities.Validate refuses, so a fake
// cannot describe an engine the contract would already have turned away, and a
// test written against one cannot pass for that reason.
//
// It also refuses a misbehaviour that is not one. A Dimension equal to the
// declared dimension and a Model equal to the declared identifier both read as
// a test that set up a wrong answer and did not, and a test that passes
// because nothing went wrong is worse than one that fails.
func New(declared runtime.Capabilities, behaviour Behaviour) (*Runtime, error) {
	if err := declared.Validate(); err != nil {
		return nil, fmt.Errorf("this fake cannot declare what the contract refuses: %w", err)
	}
	if behaviour.ChunksBeforeFailure < 0 {
		return nil, fmt.Errorf("a stream cannot fail after %d chunks", behaviour.ChunksBeforeFailure)
	}
	if behaviour.ChunksBeforeFailure >= answerWords {
		return nil, fmt.Errorf("the stream is to fail after %d chunks and an answer is %d, so it would finish instead", behaviour.ChunksBeforeFailure, answerWords)
	}
	if behaviour.Dimension < 0 {
		return nil, fmt.Errorf("a vector cannot have %d components", behaviour.Dimension)
	}
	if behaviour.Dimension != 0 && behaviour.Dimension == declared.EmbeddingDimension {
		return nil, fmt.Errorf("the wrong dimension asked for is %d, which is the declared one, so nothing would be wrong", behaviour.Dimension)
	}
	if behaviour.Model != "" && behaviour.Model == declared.Model {
		return nil, fmt.Errorf("the other model asked for is %q, which is the declared one, so nothing would be served differently", behaviour.Model)
	}
	return &Runtime{declared: declared, behaviour: behaviour}, nil
}

// Cancellations is how many generations ended because the context did.
//
// It is here for the half of #76 that is otherwise unprovable. A test that
// asserts the handler returned passes against an implementation that left the
// engine generating, which is the accelerator the cancellation was supposed to
// free. A real engine cannot be asked whether it noticed. This one can.
func (r *Runtime) Cancellations() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.cancelled
}

// Capabilities is what this engine says it can do.
func (r *Runtime) Capabilities(ctx context.Context) (runtime.Capabilities, error) {
	if err := r.reach(ctx); err != nil {
		return runtime.Capabilities{}, err
	}
	declared := r.declared
	declared.Model = r.serving()
	return declared, nil
}

// Generate produces an answer from a digest of the request, one word per
// chunk.
//
// The bound is honoured rather than approximated: an answer longer than
// MaxOutputTokens is cut to it and the result says FinishOutputLimit, because
// #66 has to be able to say an answer stopped at a bound and a caller cannot
// tell that from the text.
//
// A context that ends mid-stream returns what was produced so far, a result
// saying FinishCancelled, and the context's own error. The error is returned
// rather than swallowed: a truncated answer handed back with no error is how a
// partial answer is stored as a complete one, which is the failure #76's
// fourth line is about. Whether every adapter answers this way is #73's suite
// to fix rather than this fake's to decide, and the choice is written down in
// docs/fake-runtime.md so an adapter written against it is written against
// something stated.
//
// A stream that fails part way, and a sink that returns an error, both come
// back with no result at all. There is no finish reason for either, and a
// result carrying a reason that did not happen is exactly what a caller would
// read as a complete answer.
func (r *Runtime) Generate(ctx context.Context, req runtime.GenerateRequest, sink runtime.Sink) (runtime.GenerateResult, error) {
	if err := r.reach(ctx); err != nil {
		return runtime.GenerateResult{}, err
	}
	if len(req.Messages) == 0 {
		return runtime.GenerateResult{}, runtime.Fail(runtime.ErrRefused, adapter, "Messages", errors.New("a generation with no messages asks nothing"))
	}
	for _, message := range req.Messages {
		if !known(message.Role) {
			return runtime.GenerateResult{}, runtime.Fail(runtime.ErrRefused, adapter, "Role", fmt.Errorf("the role %q is outside the set the contract declares", message.Role))
		}
	}
	if req.MaxOutputTokens <= 0 {
		return runtime.GenerateResult{}, runtime.Fail(runtime.ErrRefused, adapter, "MaxOutputTokens", fmt.Errorf("a bound of %d leaves the generation unbounded, and the contract requires one", req.MaxOutputTokens))
	}

	input := 0
	for _, message := range req.Messages {
		input += tokens(message.Text)
	}
	if r.behaviour.OverLimit || input > r.declared.ContextLength {
		return runtime.GenerateResult{}, runtime.Fail(runtime.ErrOverLimit, adapter, "Messages", fmt.Errorf("the request is %d tokens and the engine declares a context of %d", input, r.declared.ContextLength))
	}

	words := answer(req)
	finish := runtime.FinishComplete
	if len(words) > req.MaxOutputTokens {
		words = words[:req.MaxOutputTokens]
		finish = runtime.FinishOutputLimit
	}

	result := runtime.GenerateResult{
		Model:        r.serving(),
		ModelVersion: r.declared.ModelVersion,
		InputTokens:  input,
	}
	for i, word := range words {
		if err := ctx.Err(); err != nil {
			r.mu.Lock()
			r.cancelled++
			r.mu.Unlock()
			result.Finish = runtime.FinishCancelled
			return result, err
		}
		if r.behaviour.ChunksBeforeFailure > 0 && i == r.behaviour.ChunksBeforeFailure {
			return runtime.GenerateResult{}, runtime.Fail(runtime.ErrMalformed, adapter, "chunk", fmt.Errorf("the stream ended after %d chunk(s) with nothing saying it had", i))
		}
		chunk := word
		if i > 0 {
			chunk = " " + word
		}
		if err := sink(chunk); err != nil {
			return runtime.GenerateResult{}, err
		}
		result.OutputTokens++
	}
	result.Finish = finish
	return result, nil
}

// Embed produces one vector per text, in the request's order.
//
// The structure a vector carries is lexical overlap and nothing else: two
// texts sharing words come back close together and two sharing none come back
// near orthogonal. That is enough for a retrieval test to be about the query
// rather than about the plumbing, and it is not enough to be about ranking
// quality. docs/fake-runtime.md is where the bound is written out.
func (r *Runtime) Embed(ctx context.Context, req runtime.EmbedRequest) (runtime.EmbedResult, error) {
	if err := r.reach(ctx); err != nil {
		return runtime.EmbedResult{}, err
	}
	if !r.declared.Embeddings {
		return runtime.EmbedResult{}, runtime.Fail(runtime.ErrRefused, adapter, "Embeddings", fmt.Errorf("the engine serving %q declares no embeddings", r.declared.Model))
	}
	if len(req.Texts) == 0 {
		return runtime.EmbedResult{}, runtime.Fail(runtime.ErrRefused, adapter, "Texts", errors.New("a batch with no texts asks for nothing"))
	}

	dimension := r.declared.EmbeddingDimension
	if r.behaviour.Dimension != 0 {
		dimension = r.behaviour.Dimension
	}

	result := runtime.EmbedResult{
		Vectors:      make([][]float32, 0, len(req.Texts)),
		Model:        r.serving(),
		ModelVersion: r.declared.ModelVersion,
	}
	for _, text := range req.Texts {
		if err := ctx.Err(); err != nil {
			return runtime.EmbedResult{}, err
		}
		result.Vectors = append(result.Vectors, vector(text, dimension))
		result.InputTokens += tokens(text)
	}
	return result, nil
}

// reach is the part of every operation that is about getting to an engine at
// all, so the three of them cannot disagree about what unreachable means.
func (r *Runtime) reach(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if r.behaviour.Hang {
		<-ctx.Done()
		return runtime.Fail(runtime.ErrUnreachable, adapter, "deadline", ctx.Err())
	}
	if r.behaviour.Unreachable {
		return runtime.Fail(runtime.ErrUnreachable, adapter, "address", errors.New("this fake was told to be unreachable"))
	}
	if r.behaviour.Refuse {
		return runtime.Fail(runtime.ErrRefused, adapter, "request", errors.New("this fake was told to refuse"))
	}
	return nil
}

// serving is the model identifier this engine reports, which is the declared
// one unless it was told to serve another.
func (r *Runtime) serving() string {
	if r.behaviour.Model != "" {
		return r.behaviour.Model
	}
	return r.declared.Model
}

// known says whether a role is one the contract declares. The set is closed
// there, so a message carrying anything else is a message the rest of this
// project cannot decide anything about.
func known(role runtime.Role) bool {
	for _, declared := range runtime.Roles {
		if role == declared {
			return true
		}
	}
	return false
}

// tokens is this fake's unit of cost: one whitespace-separated word.
//
// It is not what any engine counts and it is not meant to be. What a test
// needs from a token count is that it moves with the size of the input and
// that the same input gives the same number, and a word count does both
// without pretending to be a tokeniser.
func tokens(text string) int { return len(strings.Fields(text)) }

// answer is the words a request produces, the first of them the marker.
func answer(req runtime.GenerateRequest) []string {
	// The separators are what stop two different conversations digesting to
	// one. Without them a message ending where the next begins is the same
	// bytes as one message holding both, and two requests nobody would call
	// alike would produce the same answer.
	var input []byte
	for _, message := range req.Messages {
		input = append(input, message.Role...)
		input = append(input, 0)
		input = append(input, message.Text...)
		input = append(input, 0x1f)
	}

	digest := fnv.New64a()
	_, _ = digest.Write(input) // hash.Hash documents that Write never returns an error
	seed := digest.Sum64()

	words := make([]string, 0, answerWords)
	words = append(words, marker)
	for i := 1; i < answerWords; i++ {
		words = append(words, fmt.Sprintf("%04x", uint16(expand(seed, i))))
	}
	return words
}

// expand turns one digest into as many as the answer needs, without a
// generator that would have to be seeded and without a package-level value two
// goroutines would share.
func expand(seed uint64, i int) uint64 {
	var input [9]byte
	binary.BigEndian.PutUint64(input[:8], seed)
	input[8] = byte(i)

	h := fnv.New64a()
	_, _ = h.Write(input[:]) // hash.Hash documents that Write never returns an error
	return h.Sum64()
}

// vector is the embedding of one text at one dimension.
//
// Each word is hashed to a component and to a sign, and the vector is scaled
// to unit length so a cosine is a dot product and a long text is not
// automatically far from a short one. Signed, because two different words
// landing on one component reinforce each other if the hashing only ever adds,
// and the similarity that produces is an artefact of the collision rather than
// of the words.
//
// What holds the sign is TestWordsMoveAComponentInBothDirections and not a
// comparison. At the dimension a declaration is likely to name, collisions are
// rare enough that removing the sign moves no similarity this package
// measures, which was checked rather than assumed.
//
// A text with no words is hashed whole, so an empty string still comes back as
// a unit vector rather than as a direction nothing can be compared against.
// The same guard catches the other way a direction disappears: two words that
// land on one component with opposite signs cancel, and a vector of zeroes
// cannot be scaled to unit length at all.
func vector(text string, dimension int) []float32 {
	components := make([]float32, dimension)

	words := strings.Fields(strings.ToLower(text))
	if len(words) == 0 {
		words = []string{strings.ToLower(text)}
	}
	for _, word := range words {
		h := fnv.New32a()
		_, _ = h.Write([]byte(word)) // hash.Hash documents that Write never returns an error
		sum := h.Sum32()

		at := int(sum % uint32(dimension))
		if sum&1 == 0 {
			components[at]++
		} else {
			components[at]--
		}
	}

	length := float64(0)
	for _, component := range components {
		length += float64(component) * float64(component)
	}
	if length == 0 {
		components[0] = 1
		return components
	}
	length = math.Sqrt(length)
	for i := range components {
		components[i] = float32(float64(components[i]) / length)
	}
	return components
}
