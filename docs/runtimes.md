# The engines this project has been run against

This is a record of observations rather than a description of engines. It grows
by measurement: something is run, what it did is written down here with the
command that produced the statement, and nothing is entered because it is
expected.

Three issues write into it and they write different things. #73 records a
difference the contract suite found between one engine and another. #75 records
what the process-manager adapter adds over the compatible one. #77 records the
engine and model versions this project has actually been run against. The
shape below is what all three add to, decided here because the first of them
arrived first and an accreted document is one nobody can read.

## What is in it today

Nothing has been observed. One adapter is registered in this tree and it is the
fake, which needs no model and is not an engine:

    go test ./internal/runtime/contract -run TestThisTreeMatchesTheRegister -v
    adapters_test.go:49: 1 adapter(s) registered, every one of them handed to this suite by a test in its own package

The register fails closed in both directions, so that line is a reading of the
tree rather than a list somebody keeps up to date. A run of the suite against
the fake is evidence about this project's own code and about nothing else,
which is what [fake-runtime.md](fake-runtime.md) says at more length.

That sentence is the state of this file and not a placeholder for one somebody
forgot to fill in. A table of engines written before any of them was reached
would be a table of expectations, and it would be read as a table of results.

## How an observation is written

One section per engine, and one entry per difference. An entry carries four
things and is refused by a reader without them.

- What was observed, written as behaviour rather than as a conclusion.
- The engine and the version it was observed on, exactly as the engine reports
  it rather than as the operator installed it.
- The command that produced the statement, so the reading can be repeated.
- What the adapter does about it, or the issue that decides what it should do.

An observation is never edited into a general claim about an engine. Two
versions of one engine are two entries, because the reason this file exists is
that a new version changes behaviour quietly.

## What the contract suite can and cannot see

The suite in `internal/runtime/contract` is what produces most of what will go
here, so the bound on what it sees is the bound on what this file can say.

It drives an adapter through the interface and reads what comes back. It judges
behaviour at that boundary and nothing past it: it does not see a request leave
a machine, it does not read an engine's own logs, and it cannot tell an answer
that is wrong from one that is right.

Two of its cases are worth naming here because a reader of an entry needs to
know which question was asked. An over-long prompt is proved refused by the
adapter producing no chunk and returning the over-limit failure, which is a
statement about the adapter rather than about what an engine would have done
with the prompt. And a cancelled generation is proved to have stopped by the
goroutines it was using going away, which says the process stopped working and
says nothing about whether the engine at the other end did.

## The conditions an adapter has to be able to arrange

A case runs only where its condition can be arranged, and an adapter that
cannot arrange one declares that against the reason. The suite prints every
case it did not run, so a partial run cannot be read as a full one.

That is worth knowing before the first real engine is measured. The fake
arranges all four conditions, so the run in the default gate is complete. An
adapter for a real engine may not be able to manufacture a malformed stream on
demand, and the entry for that engine will then be a suite that ran nine cases
rather than ten.

## What is not here

No latency or throughput figure. #77 measures those against a real model and
says where they go, and a number produced against the fake would be a
measurement of a digest.

No recommendation. An operator choosing between engines reads what each one did
and decides; a document that ranked them would be carrying an opinion this
project has not measured.
