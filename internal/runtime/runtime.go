// Package runtime declares the contract every model engine is reached
// through, and the declaration an engine has to make about itself before it is
// used.
//
// docs/decisions/0009-runtime.md is where the shape of this was decided: no
// engine is bundled, every model call leaves this process over a network
// contract, and the differences between engines are absorbed by adapters
// behind an interface rather than by callers that know which engine they have.
// This package is that interface. The record deliberately leaves the shape to
// it, so the argument for each part is here rather than there.
//
// Three operations and no more, because the fourth is the one that carries an
// engine's quirk into the callers. TestTheContractIsThreeOperations refuses a
// fourth.
//
// Nothing here speaks to an engine. There is no client, no address, no
// timeout and no retry: this package declares what an adapter has to be able
// to do and what it has to say about itself, and the adapters are #74 and #75.
// A package that both declared the contract and implemented one engine would
// be a contract shaped like that engine.
//
// What #71 asks for and this package does not answer, stated rather than left
// to be discovered. The declaration is read at startup and refused against a
// configuration in #88, and there is no configuration surface yet, so Refuse
// below is written and nothing calls it. Assembling a prompt inside the
// declared context length is #80. Adapters proved to honour a cancelled
// context are #73, #74 and #75. Those three are what turn this from a
// declaration into a refusal, and until they land a capability declaration is
// a struct somebody fills in.
package runtime

import (
	"context"
	"errors"
	"fmt"
)

// A Role is who a message in a generation request came from.
//
// The set is closed. A role an engine understands and this list does not is a
// role no adapter may pass through, because a message whose role the rest of
// this project cannot read is a message it cannot decide anything about.
type Role string

const (
	// RoleSystem is the instruction this project gives the model.
	RoleSystem Role = "system"
	// RoleUser is what the person asked.
	RoleUser Role = "user"
	// RoleAssistant is what the model produced earlier in the same
	// conversation.
	RoleAssistant Role = "assistant"
)

// Roles is the declared set.
var Roles = []Role{RoleSystem, RoleUser, RoleAssistant}

// A Message is one turn handed to an engine.
//
// Text and nothing else. A retrieved passage reaches an engine as the content
// of a message assembled by #80, and it is data there rather than instruction,
// which is #81's rule and not this package's to hold.
type Message struct {
	// Role is who it came from.
	Role Role
	// Text is the content of the turn.
	Text string
}

// A GenerateRequest is one generation.
//
// What is absent from it is the part worth reading. There is no model name: an
// adapter is configured for one model and reports which in Capabilities, so a
// caller cannot silently ask for a different one and get an answer that looks
// the same. There is no temperature or sampling setting, because the first
// release takes no position on those and a field nothing reads is a field
// somebody assumes is honoured. There is no tool list: whether an engine
// supports tools is declared in Capabilities, and what a tool may be is
// bounded by #82, which has not decided it.
type GenerateRequest struct {
	// Messages is the assembled conversation, oldest first.
	Messages []Message
	// MaxOutputTokens bounds what the engine may produce. It is required: an
	// unbounded generation is one an operator cannot cost and a user cannot
	// cancel by waiting.
	MaxOutputTokens int
}

// A Sink receives generated text as it arrives.
//
// Streaming is the interface rather than an option on it, because a caller
// that wants the whole answer can accumulate and a caller that needs the first
// token cannot un-buffer one. An error returned from a Sink stops the
// generation and is returned by Generate, so a caller whose reader has gone
// away stops paying an engine to keep producing.
type Sink func(chunk string) error

// A FinishReason is why a generation stopped.
type FinishReason string

const (
	// FinishComplete is the engine stopping on its own.
	FinishComplete FinishReason = "complete"
	// FinishOutputLimit is MaxOutputTokens reached. It is not an error: the
	// caller set the bound. It is distinct from FinishComplete because an
	// answer that stopped at a bound is a truncated answer and #66 has to be
	// able to say so.
	FinishOutputLimit FinishReason = "output-limit"
	// FinishCancelled is the context ending before the engine did.
	FinishCancelled FinishReason = "cancelled"
)

// FinishReasons is the declared set.
var FinishReasons = []FinishReason{FinishComplete, FinishOutputLimit, FinishCancelled}

// A GenerateResult is what a generation produced besides its text.
//
// The model identifier and version are on the result rather than only in
// Capabilities because they travel with the answer: #65 cites what produced
// what and #41 records it without recording the content. An engine that served
// a different model than the one configured is the quiet failure
// docs/decisions/0009-runtime.md names, and a result that carries no
// identifier cannot show it happened.
type GenerateResult struct {
	// Model is the identifier the engine reported for this generation.
	Model string
	// ModelVersion is the engine's version stamp for that model.
	ModelVersion string
	// InputTokens is what the request cost.
	InputTokens int
	// OutputTokens is what the answer cost.
	OutputTokens int
	// Finish is why it stopped.
	Finish FinishReason
}

// An EmbedRequest is one batch of texts to embed.
type EmbedRequest struct {
	// Texts are the passages, in the order the vectors come back in.
	Texts []string
}

// An EmbedResult is the vectors and what produced them.
//
// Model and ModelVersion are here for the same reason they are on a
// generation, and for one more: #61 re-embeds when the model changes, and a
// vector whose producing model is not recorded is a vector nobody can tell is
// stale.
type EmbedResult struct {
	// Vectors are one per text, in the request's order.
	Vectors [][]float32
	// Model is the identifier the engine reported.
	Model string
	// ModelVersion is the engine's version stamp for that model.
	ModelVersion string
	// InputTokens is what the batch cost.
	InputTokens int
}

// Capabilities is what an engine says it can do.
//
// It is asked for rather than configured. A capability set written into a
// configuration file is a claim about an engine made by whoever wrote the
// file, and it stays true in the file after the engine has changed.
type Capabilities struct {
	// Model is the identifier the engine reports for the model it is serving.
	Model string
	// ModelVersion is the engine's version stamp for that model. An engine
	// serving a model it cannot version is an engine whose answers cannot be
	// attributed, so this is required rather than best effort.
	ModelVersion string
	// ContextLength is how many tokens the engine will accept in one request.
	ContextLength int
	// Tools is whether the engine supports tool calls.
	Tools bool
	// Embeddings is whether the engine will produce embeddings.
	Embeddings bool
	// EmbeddingDimension is how many components a vector has. It is zero when
	// and only when Embeddings is false.
	EmbeddingDimension int
}

// A Requirement is what a deployment needs an engine to be able to do.
//
// It is the other half of Refuse. Nothing constructs one yet; #88 is the
// configuration surface that will, and this type is what it constructs.
type Requirement struct {
	// Model is the model identifier this deployment was configured for. Empty
	// accepts whatever the engine reports, which is a deployment that has
	// chosen not to notice a model being swapped underneath it.
	Model string
	// MinimumContextLength is the shortest context this deployment's prompts
	// fit in. Zero asks for nothing.
	MinimumContextLength int
	// Tools is whether this deployment needs tool calls.
	Tools bool
	// Embeddings is whether this deployment needs embeddings.
	Embeddings bool
	// EmbeddingDimension is the dimension the index was built at. Zero accepts
	// whatever the engine produces, which is only right before anything has
	// been indexed.
	EmbeddingDimension int
}

// Validate refuses a capability declaration that cannot be compared against
// anything.
//
// It refuses rather than filling in. A zero context length read as "unknown,
// carry on" is the field nobody set being read as permission to send any
// prompt at all, and the failure arrives later as an answer the engine
// truncated rather than as a refusal here.
func (c Capabilities) Validate() error {
	if c.Model == "" {
		return errors.New("the engine reported no model identifier, so no answer it produces can be attributed")
	}
	if c.ModelVersion == "" {
		return errors.New("the engine reported no version for its model, so a model changing underneath this deployment is invisible")
	}
	if c.ContextLength <= 0 {
		return fmt.Errorf("the engine reported a context length of %d, and a length that is not positive cannot bound a prompt", c.ContextLength)
	}
	if c.Embeddings && c.EmbeddingDimension <= 0 {
		return fmt.Errorf("the engine declares embeddings and a dimension of %d, and a vector store cannot be built against a dimension that is not positive", c.EmbeddingDimension)
	}
	if !c.Embeddings && c.EmbeddingDimension != 0 {
		return fmt.Errorf("the engine declares no embeddings and a dimension of %d, and the two cannot both be true", c.EmbeddingDimension)
	}
	return nil
}

// Refuse decides whether an engine can do what a deployment needs, and names
// both sides when it cannot.
//
// Both sides, because the operator reading the message has to know which end
// to change. A message saying only that embeddings are unsupported leaves them
// looking for the setting that turns them on, and a message naming the engine
// and the requirement together says which of the two was wrong.
//
// A declaration that does not Validate is refused before it is compared: an
// engine that could not describe itself has not said it can do anything.
func (c Capabilities) Refuse(r Requirement) error {
	if err := c.Validate(); err != nil {
		return fmt.Errorf("the engine's capability declaration is not usable: %w", err)
	}
	if r.Model != "" && r.Model != c.Model {
		return fmt.Errorf("this deployment is configured for model %q and the engine reports %q", r.Model, c.Model)
	}
	if r.MinimumContextLength > c.ContextLength {
		return fmt.Errorf("this deployment needs a context of at least %d tokens and the engine declares %d", r.MinimumContextLength, c.ContextLength)
	}
	if r.Tools && !c.Tools {
		return fmt.Errorf("this deployment needs tool calls and the engine serving %q declares none", c.Model)
	}
	if r.Embeddings && !c.Embeddings {
		return fmt.Errorf("this deployment needs embeddings and the engine serving %q declares none", c.Model)
	}
	if r.Embeddings && r.EmbeddingDimension != 0 && r.EmbeddingDimension != c.EmbeddingDimension {
		return fmt.Errorf("this deployment's index was built at %d dimensions and the engine serving %q produces %d", r.EmbeddingDimension, c.Model, c.EmbeddingDimension)
	}
	return nil
}

// The four ways a call into an engine fails.
//
// They are four rather than one because the callers behave differently for
// each: #76 cancels and retries nothing, #66 says the answer was built from
// less than everything, and an operator reading #91 needs an engine that is
// down told apart from one that is refusing. A single error type would put
// that decision in whoever wrote the string comparison.
var (
	// ErrUnreachable is the engine not answering: no connection, no response,
	// or a deadline reached before one arrived. The request did not happen.
	ErrUnreachable = errors.New("the model runtime could not be reached")
	// ErrRefused is the engine answering and declining: an authentication
	// failure, a rate limit, or a model it will not serve. The request
	// happened and was turned away.
	ErrRefused = errors.New("the model runtime refused the request")
	// ErrOverLimit is the request not fitting what the engine declared: more
	// input than its context length, or the engine saying so itself. It is
	// separate from ErrRefused because it is this project's own defect rather
	// than the operator's, and #80 is where the prompt is meant to be kept
	// inside the bound.
	ErrOverLimit = errors.New("the request is larger than the model runtime accepts")
	// ErrMalformed is the engine answering with something the adapter cannot
	// read. It is a refusal rather than a salvage, because a half-understood
	// response is how a truncated answer is presented as a complete one.
	ErrMalformed = errors.New("the model runtime answered with something the adapter could not read")
)

// An Error is one failure, carrying which adapter produced it and what it was
// about.
//
// The adapter is named because docs/decisions/0009-runtime.md asks for it: an
// operator has to be able to tell an engine's quirk from a defect here, and an
// error that names neither the adapter nor the field leaves them reading this
// project's source to find out which.
type Error struct {
	// Kind is one of the four sentinels above.
	Kind error
	// Adapter is which adapter produced it.
	Adapter string
	// Field is what the failure was about: the response field that could not
	// be read, the limit that was exceeded, the address that did not answer.
	// It is a name and never a value, so an error carrying it does not carry
	// document content or a credential outward.
	Field string
	// Err is what happened underneath, where there was something.
	Err error
}

// Fail builds an Error. It is the only way one is made, so an error leaving an
// adapter cannot be missing its kind.
func Fail(kind error, adapter, field string, err error) *Error {
	return &Error{Kind: kind, Adapter: adapter, Field: field, Err: err}
}

// Error renders the failure with everything a reader needs to place it.
func (e *Error) Error() string {
	message := fmt.Sprintf("%s: %s", e.Adapter, e.Kind)
	if e.Field != "" {
		message += fmt.Sprintf(" (%s)", e.Field)
	}
	if e.Err != nil {
		message += ": " + e.Err.Error()
	}
	return message
}

// Is reports the kind, so a caller writes errors.Is against the sentinel
// rather than comparing strings.
func (e *Error) Is(target error) bool { return target == e.Kind }

// Unwrap gives up what happened underneath, which is what a log needs and what
// a decision must not read.
func (e *Error) Unwrap() error { return e.Err }

// A Runtime is one configured model engine, reached over the wire.
//
// Every operation takes a context first, and it carries cancellation and a
// deadline. That is the whole of what this interface can say about it: whether
// an adapter honours a cancelled context is a property of the adapter, and
// #73's suite is what proves each one does rather than this declaration.
type Runtime interface {
	// Capabilities is what the engine says it can do. It is asked at startup
	// and may be asked again; an adapter that cached an answer forever would
	// hide a model being swapped underneath it.
	Capabilities(ctx context.Context) (Capabilities, error)

	// Generate produces an answer, handing each chunk to the sink as it
	// arrives. It returns when the engine has finished, when the sink returns
	// an error, or when the context ends.
	Generate(ctx context.Context, req GenerateRequest, sink Sink) (GenerateResult, error)

	// Embed produces one vector per text. It is a batch because an index is
	// built in batches and a per-text interface makes the round trips the cost.
	Embed(ctx context.Context, req EmbedRequest) (EmbedResult, error)
}
