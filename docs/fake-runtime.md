# The fake runtime

`internal/runtime/fake` is a model engine that answers without a model. It
exists because the default suite runs with no accelerator and no outbound
network, which is the condition [decisions/0001-means.md](decisions/0001-means.md)
argues for and #7 enforces, and because almost everything in this project
reaches a runtime somewhere.

This document says what a run against it proves and what it does not. That
sentence is the reason the document exists rather than a note in the package,
because the thing being guarded against is a number from a fake-runtime run
being quoted somewhere else as a fact about a model.

## What a result from it is evidence about

A run against the fake is evidence about the code that called it. It says
nothing about any model, about the quality of an answer, about how an engine
behaves under load, or about whether a real engine honours the contract the
adapters are written to.

The mirror of it is already written in this repository from the other
direction: a run of the harness under `test/needs-real-hardware-or-services/`
is evidence about the machine it ran on. Between them the two cover what this
project can say, and neither covers what the other does.

Three specific readings are wrong and each is easy to fall into.

- A ranking that comes out right against the fake is not a ranking that comes
  out right. The vectors carry lexical overlap and no meaning, so a case that
  passes has shown the query reached the index with the filter applied, not
  that the order is useful to anybody.
- A latency measured against the fake is a measurement of this process. There
  is no engine, no network and no accelerator in it.
- A token count from the fake is not a token count. It counts words, which is
  what makes it stable, and no tokeniser agrees with it.

## What it is deterministic about

The same request produces the same bytes on every machine and on every run.
Nothing here is seeded from a clock, from a random source or from a map
iteration, so a case that fails fails for what the code did.

An answer is derived from a digest of the messages and arrives one word per
chunk. Its first word is `fake`, so an answer that turns up in a log, a fixture
or an issue is identifiable as one without anybody having to remember which run
produced it.

A vector is built by hashing each word of a text to one component and to a
sign, and scaling the result to unit length. Two texts sharing most of their
words come back close together and two sharing none come back near orthogonal,
which is enough structure for a retrieval case to be about the query rather
than about the plumbing.

The bound of that structure is the whole of what the fake knows. Two sentences
that mean the same thing in different words are as far apart as two sentences
about nothing in common, because the only thing being compared is the words. A
real embedding model is close to the opposite, and a retrieval result that
depends on the difference is a result this fake cannot produce.

## The ways it can be told to misbehave

Every one of these is off by default, and each is one field set when the fake
is built rather than a callback that decides later, so a case states the
failure it is about where a reader of the case will be standing.

- Unreachable, which fails before anything happens.
- Refused, which is the engine answering and declining.
- Hanging, which returns nothing until the context ends and is how a timeout is
  written.
- A stream that delivers some chunks and then fails, which is the shape a
  caller is most likely to store as a complete answer.
- A request over the limit, without one having to be built.
- A vector with the wrong number of components, with the declaration left
  saying the right one.
- An engine serving a model identifier other than the configured one, reported
  the same way in its declaration, in a generation and in a batch of vectors.

The last two are the quiet failures
[decisions/0009-runtime.md](decisions/0009-runtime.md) names: nothing in an
answer admits either of them, and a caller comparing only one of the three
places the identifier appears is satisfied by an engine serving something else.

A misbehaviour that is not one is refused when the fake is built. Asking for
the wrong dimension and naming the declared one, or asking for another model
and naming the configured one, are both a case that thinks it arranged a wrong
answer and did not, and a case that passes because nothing went wrong is worse
than one that fails.

## Where it is stricter than an engine, deliberately

A compatible server accepts a generation with no bound on its output and a
message whose role it has never heard of. This refuses both, because the
contract declares the bound required and declares the set of roles closed, and
a fake that accepted what the contract forbids hides a caller's defect until an
adapter meets a real engine.

It also returns no result at all when a stream fails part way or when the sink
refuses a chunk. There is no finish reason for either, and a result carrying a
reason that did not happen is what a caller reads as a complete answer.

A generation the context ended is the one case that comes back with both: a
result saying it was cancelled, and the context's own error. That was this
fake's choice and not the contract's until the suite landed, and it is the
contract's now. `cancellationLeavesNothingBehind` in
`internal/runtime/contract` fails a subject that answers a cancelled generation
without the error and one that ends it under any other finish reason, so every
adapter is held to it and an adapter written to match this fake is matching the
suite rather than one implementation's habit.

## Nothing that ships may reach it

`import-rules.txt` declares the package test-only, and `internal/importrules`
refuses a shipped file that imports it, naming the issue in the refusal. A
binary that reached this would answer from a digest of the question, and the
answer would look exactly like one an engine produced.

## What is not here

The fake emits no tool call. The contract carries no shape for one: an engine
declares whether it supports tools and there is nowhere in a request or a
result for a call to be. What a tool may be is #82 and has not been decided, so
a shape invented here would be a second decision that #82 then has to accept or
rename.

This section said the fake had not been run against a contract suite because
there was none. There is one, and it runs: `internal/runtime/fake` hands this
fake to `contract.Run` under each of the suite's conditions, in the default
gate, which is #73's fourth done-condition rather than something this document
arranged.

What that adds to the paragraph above is narrower than it sounds and the
narrowness is the point. This fake is the only implementation of the contract
in the tree, so passing the suite says the fake and the suite agree, and says
nothing whatever about an engine. A green run here is still not evidence about
a model, and it is not evidence that the suite is complete either: a case
nobody wrote is a quirk this project has not met yet. What it does buy is that
the cases in this package are no longer the only thing holding the fake to
anything, and that the first real adapter meets the suite rather than
discovering it.
