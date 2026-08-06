# 0009 Runtime: one wire contract, adapters behind it, no bundled engine

Status: accepted

## Context

This project needs a language model and will not contain one. That is the
decision. The reasoning is written down because the opposite is what most
projects of this shape do without deciding it.

## Decision

No inference engine is bundled. Every model call leaves this process over a
network contract.

### Three reasons

Shipping an engine means shipping what an engine needs. Accelerator drivers, a
build matrix per hardware family, and model weights measured in gigabytes, none
of which this project has anything to say about. The container image in #116
would stop being a thing an operator can read the contents of.

It is a security surface in a language this project is not. The means record,
`docs/decisions/0001-means.md`, permits a second language where the work is not
this project's to own, and refuses it wherever a permission decision is taken.
An inference engine linked into this process would be the largest body of
memory-unsafe code in the tree, sitting inside the process that holds the
authorisation boundary, and reachable with bytes taken from somebody else's
document store.

It fixes a choice that changes every few months, in the one component whose
behaviour is not this project's contribution. #70 is where that is argued in
full. An engine chosen at build time is an engine an operator is stuck with
until this project cuts a release.

### The cost, stated rather than implied

An operator has to run something else before this is useful. A first-run
experience that begins by telling somebody to go and install another service
loses people, and it loses them at exactly the moment they were evaluating.
That cost is real and it is carried by #95, which has to say what is wrong on
the first run in terms an operator can act on, and by #117, which has to publish
an example that stands the whole thing up beside an identity provider and a
runtime rather than describing it.

## The wire contract

The contract is the OpenAI-compatible HTTP shape.

It is chosen for one reason, and the reason is not that it is well designed. The
local engines an operator would already be running speak it, so one adapter
covers several of them, and a fourth engine is usually configuration rather than
code. What each project says about itself:

    for r in ggml-org/llama.cpp ollama/ollama vllm-project/vllm; do
      gh api repos/$r/readme --jq '.content' | base64 -d | grep -ic 'openai'
    done
    1
    2
    1

That command shows the interface is named in each project's own readme. It does
not show that the three agree with each other, and they do not.

The compatible shape is a convention and not a specification. There is no
document that all of these engines conform to and no suite that says whether one
of them does. They differ at the edges, and the edges are where this project
lives: how a streamed response terminates, what happens to a request cancelled
mid-generation, whether a token count comes back at all, what an unknown
parameter does, and what an error looks like when the model is loading rather
than missing.

That is why the compatible shape is not used directly. Adapters sit behind an
internal interface, defined by #71, and the differences between engines are
pinned by the contract suite in #73. An engine's quirk that is not in that suite
is a quirk this project has not met yet, rather than one it has handled.

## Engines targeted at the first release

Two adapters cover three engines.

The compatible-server adapter, #74, targets a local server speaking the
compatible contract directly. `ggml-org/llama.cpp` and `vllm-project/vllm` are
both reached this way, and they are the two ends of the hardware range an
operator is likely to have: one that runs on a machine with no accelerator at
all, and one built for a machine that has several.

The process-manager adapter, #75, targets `ollama/ollama`, which speaks the
compatible contract and also manages model files, pulls and lifecycle. #75 is
where what it adds beyond the compatible shape is decided, because an adapter
that only used the compatible half would be #74 under a second name.

The licences of all three, since an operator will be asked about them:

    for r in ggml-org/llama.cpp ollama/ollama vllm-project/vllm; do
      gh api repos/$r --jq '.license.spdx_id'
    done
    MIT
    MIT
    Apache-2.0

None of their code is linked here in any case. They are reached over the wire,
which is the whole point of this record.

## The failure modes a remote engine introduces

An engine in the process fails when the process fails. An engine over a network
fails on its own, and each way has to have an answer decided here rather than in
whichever adapter meets it first.

Unreachable. The request fails and the user is told the assistant is
unavailable, naming that the model runtime could not be reached rather than
producing an empty answer. It is not retried against a different engine, because
a silent fallback to another model is a different answer presented as the same
one.

Slow. Every call carries a deadline from configuration, and the deadline is
enforced by cancelling the request rather than by abandoning it, so a generation
nobody is waiting for stops costing the engine work. #76 holds cancellation.
A slow engine is visible to the operator as a metric rather than only to the
user as a wait, which is #91.

Wrong shape. A response that does not match the contract is a failure and not
something to salvage. An adapter parses strictly and refuses, because a
half-understood response is how a truncated answer gets presented as a complete
one. The refusal names the adapter and the field, so an operator can tell an
engine's quirk from a defect here, and each such quirk that is worth supporting
becomes a case in #73.

A model that is not the one configured. This is the quiet one. An engine can
serve a different model than the name suggests, and nothing in the response has
to admit it. The adapter reads the model identifier the engine reports and
compares it with the configured one; a mismatch is refused at startup and
recorded, rather than being discovered later in an answer whose quality nobody
can explain. The identifier travels with the answer, so #65 can cite what
produced what and #41 can record it without recording the content.

## What would reverse this

A bundled engine would have to be worth all four of the costs above at once, and
the bar is stated so a future argument has something to clear.

It would have to remove the second service from the operator's first run without
adding a hardware matrix this project maintains. It would have to be reachable
by the same means the rest of the tree is written in, or be small enough that
its boundary is auditable in an afternoon. It would have to be stable enough
that pinning it does not mean shipping last year's model quality. And it would
have to leave the authorisation boundary outside its address space.

Nothing available meets that today. If something does, this record is replaced
rather than amended, because bundling changes what the artefact is.

The narrower reversal is easier and more likely. If the compatible shape stops
being what the engines an operator runs actually speak, the wire contract
changes and the adapter interface in #71 is what absorbs it. That is a change to
one layer rather than to this decision.

## What this record does not decide

The operator's choice of model is theirs. This project takes no position on
which model is good, and #70 is where that is stated and defended rather than
here.

This record puts every model call behind a network contract. It does not decide
whether the address at the other end may belong to somebody the operator does
not run, which is a separate question with its own costs, and it is open. #124
holds it. Nothing in this record should be read as answering it in either
direction, and an adapter is not a permission.
