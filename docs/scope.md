# Scope: model quality is not what this project improves

The gap between a model an operator can run on their own hardware and the best
hosted one is real, and this project does not close it. That is said here,
early and plainly, because the alternative is that somebody discovers it during
an evaluation, in front of the people they were trying to convince.

## The position

Model quality is out of scope.

The gap is a matter of weights and of the compute used to produce them. Neither
is produced by writing software in this repository. A project that claimed to
close it by better prompting, better chunking or a cleverer pipeline would be
claiming that the difference between model families is an engineering detail at
this layer, and it is not.

## What this project contributes

The layer around the model, which is the part that is missing from the things
an organisation would otherwise deploy.

Knowing who is asking. An identity that came from the organisation's own
provider and resolves to a principal a source system's permissions can be
compared against.

Finding what that person may see, and nothing else. A permission model that is
part of the query rather than a filter around it, re-authorised at the point of
use, with the failure directions chosen deliberately.

Being able to show afterwards what happened. An audit trail that records the
decisions rather than the documents, and that an auditor can query.

None of those three gets better when the model gets better, and none of them
gets worse when the model gets worse. That is the argument for building this
even though the model is somebody else's work.

## What this project does not contribute

It does not make a given model answer better. It does not train, fine-tune or
distil anything. It does not ship weights. It does not rank models, and it does
not tell an operator which one to run.

Where an answer is bad because the model is weak, this project's contribution is
that the answer is traceable: the passages it was built from are cited, and the
completeness of the search is stated, so a reader can see whether the model was
given the right material and made a poor answer out of it or was given the
wrong material.

## The design obligations that follow

Saying quality is out of scope is only honest if the system is built so that the
model can actually be changed. Three obligations, and each is somebody's issue
rather than a sentiment.

**No dependence on one model's quirks.** Nothing in this project may rely on a
particular model's habits of formatting, its tolerance for a particular prompt
shape, or its behaviour when instructed in a particular way. Where a capability
is needed it is declared by the runtime and read at startup rather than assumed,
which is #71, and a configuration asking for something the engine cannot do is
refused with a message naming both.

**Graceful behaviour on a weak model.** A weaker model must produce a worse
answer, never a broken system and never a silent permission failure. The parts
that carry the promise do not run through the model: the permission predicate is
in the query, the recheck happens before anything reaches the context, and the
citation list is assembled from what was actually retrieved rather than from
what the model said it used. A model that ignores its instructions produces a
poor answer with correct citations and a correct completeness statement, which
is a bad answer and not a leak.

**The model is configuration.** Changing engines is a configuration change and a
re-embedding job, not a code change. Everything the model is reached through is
the wire contract in #69 and the interface in #71. What re-embedding costs and
how it is done without a window where the index is half old is #61, and that
cost is the honest price of this obligation rather than a reason to weaken it.

## What to expect from a small local model on a retrieval task

Stated as a claim rather than as a measurement. Nothing in this repository has
measured any of it, and the sentences below are written from what is generally
reported about models in this size range rather than from a run of this system,
which does not yet exist. Read them as the expectation this project is designed
around, and replace them with numbers when there are numbers.

A model small enough to run on a single machine alongside everything else in
this stack is usually adequate at the shape of task this project gives it, which
is answering from passages placed in front of it rather than answering from what
it knows. That task is the easier half of what a large hosted model is good at,
and it is the half this project is built to make possible.

It is usually weaker at the things that are not that task. Synthesising across
many passages at once, holding a long chain of conditions, and noticing that two
retrieved passages contradict each other are where the difference shows first.
Instruction observance is the other one: an instruction to answer only from the
provided passages and to say so when they do not contain the answer is followed
less reliably by a smaller model, and that failure looks like a confident answer
with no citation behind it.

The part of this that this project can measure is retrieval quality, which is
#67: a fixed question set, a stated method, and a number that can be compared
across changes. That measurement is about whether the right passages were found,
which is this project's own work. It is not a measurement of the model, and #67
says so rather than being read as a model benchmark.

## What this project could do about the gap and does not

**Fine-tuning a model for this task.** It would probably help, and it would move
this project from being a layer around any model to being a layer around one
model. The obligation above would be dead the day it landed: the system would be
built around a specific set of weights, an operator could no longer swap the
engine without losing whatever the tuning bought, and this repository would own
a training pipeline, a dataset and the questions about what is in that dataset.
That is a different project.

**Bundling weights.** It would make the first run easier and it would make this
repository a distributor of somebody else's model, with their licence, their
provenance and their update cadence attached to this one. It would also
contradict the position above by making one model the default in the only way
that matters, which is the one nobody changes.

**Quality benchmarks against hosted services.** A table comparing answers from a
local model against a hosted one would be read as this project's claim about
model quality, which is the thing it says is out of scope. The comparison also
does not survive contact with the reader's situation: it is a measurement of two
models on somebody else's questions, and it would be quoted as though it were a
measurement of this system on theirs. Where a number is published here it is
about retrieval, per #67, and it is labelled as such.

## Where this leaves an operator

An operator choosing hardware and a model is making a decision this project does
not make for them and cannot make better. What this project owes them instead is
that the decision is reversible: the engine is configuration, the interface
declares what an engine can do, and the parts of the system that carry the
promise do not change when the engine does.
