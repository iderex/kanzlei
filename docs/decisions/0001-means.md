# 0001 Means: language, runtime and toolchain

Status: accepted

## Context

The repository holds a readme, a notice and a set of workflow guards. It holds
no product code, and every other issue in the plan assumes a language without
saying which one. This record says which one, and says enough about why that a
reader can disagree on the merits.

Nothing below this project forces the answer. Inference runs outside this
process behind a wire contract (#69), so no model runtime dictates the host
language. What the work actually is: an HTTP surface, an identity flow, an
authorisation decision taken on every request, a job that walks somebody else's
source system, and a query against a datastore. It is orchestration and policy.

## Decision

Go, one module, one binary, one toolchain.

The minimum version is Go 1.26. The toolchain is the one the Go distribution
ships: `go build`, `go vet`, `go test`, the race detector and the coverage
profiler, with no extra installation step. The exact version is pinned in `go.mod`
by #2, and that pin is the authority for what the gate runs; the number here is
the floor below which the module will not build.

`go version` prints what a reader has locally. The pinned version prints from
the module file.

## The four questions

The order below is the order the questions are asked in, and each is answered
for this artefact rather than carried over from another one.

### Can the means carry a property a machine can refuse, a proof that runs, and a claim that cites the command behind it

Yes, and this is the part that decides it.

A refusable property needs a place where a violation is turned away by code
rather than by prose. In Go the authorisation decision is a value that a
retrieval call cannot be given by accident: an unexported field, a constructor
that takes a principal, and a compile error where somebody forgets. That is a
machine refusing a violation before any test runs.

A proof that runs needs a test command that is present wherever the source is.
`go test ./...` is that command, and the race detector and the coverage profile
come from the same run rather than from a second apparatus somebody has to
install.

A claim that cites a command needs commands a reader can run against a checkout
and get the same answer. The Go toolchain's verbs are stable and their output is
stable enough to quote, which is what makes evidence in an issue reproducible by
somebody who was not there.

### Is anything outside this repository forcing it

No. This is a free choice, and it is worth saying so plainly, because a forced
means and a chosen means are held to different standards.

Two surfaces are genuinely forced and neither forces Go. The workflow files are
YAML with POSIX shell inside them, because that is the interface the hosted
runner offers. Text extraction from formats strangers control (#54) has its
mature implementations elsewhere, and the boundary in that issue is where that
force is absorbed.

### Does it add a language, a runtime or a dependency the tree does not already carry

Yes, and the cost is named rather than waved past.

Today the tree carries YAML, POSIX shell inside workflow files, and one tool
fetched by the workflow that audits the workflows. What is tracked is printed by

    git ls-tree -r origin/main --name-only

Go is a new language in the tree, and it is the first one the product itself is
written in. What it costs is one toolchain an operator building from source has
to have, and one language a contributor has to read. What it saves is the set of
things that would otherwise be separate installs: a test runner, a race
detector, a coverage tool, an HTTP server, a TLS stack and a cancellation
mechanism are all in the distribution.

The static binary is the part an operator feels. A container image (#116) that
holds one file has a smaller surface to keep patched than one that holds an
interpreter and its package set.

### Is the result testable by the suite that will exist, or does it need a parallel apparatus

By the suite that will exist. The authorisation conformance suite (#25), the
adversarial suite (#26) and the connector contract suite (#49) are all ordinary
tests in the same run, against fakes that need no service (#48, #72). The
condition in #7, that a test needs no display, no administrative rights, no
accelerator and no network, is reachable with a standard library that does its
own HTTP and its own crypto, because there is no service to stand up to exercise
either.

Retrieval and the permission filter live in one process. That is not a
convenience: #15 and #62 both assume there is no second service that can be
reached around the filter, and a means that put retrieval behind its own network
hop would make that assumption false on day one.

## Rejected alternatives

### Rust

What it buys is a stronger type system, no garbage collector, and memory safety
guarantees that hold under more of the codebase than Go's do.

What it costs is a slower loop on a codebase that is mostly IO and policy, where
the borrow checker is paying for a class of defect this project has little of,
and thinner libraries on the identity surface, which is the one surface where a
subtle library defect is a complete authentication bypass.

Reverse it if a memory-unsafe dependency ends up on the request path, or if the
extraction boundary in #54 turns out to need in-process parsing of formats
strangers control rather than a subprocess.

### Python

What it buys is the retrieval and machine learning ecosystem directly, in the
process, with no wire contract in between.

What it costs is a network-facing process in a dynamically typed language, where
the authorisation decision is a convention rather than something the compiler
can refuse to skip, plus a packaging story the operator has to care about, which
is the opposite of what #116 is for.

Reverse it if the embedding and reranking work (#60, #64) turns out to need to
be in-process for a reason a measurement shows, rather than for convenience.

## Where a second language is permitted

Three places, and the permission stops at each boundary rather than spreading.

The workflow files hold POSIX shell, because the runner offers no other
interface at that point. The permission covers the steps of a workflow and does
not extend to enforcement logic: a rule that decides whether the tree is
acceptable belongs in a program the suite can test, not in a shell block nobody
can run a fixture against.

Text extraction (#54) may be a subprocess or a separate service in another
language. The permission stops at the ingestion boundary, and what crosses it is
bytes in and text plus a fidelity statement out. Nothing in the extraction path
takes an authorisation decision.

The model runtime (#69) is another process in whatever language its authors
chose, reached over the wire contract. The permission is the contract itself.
Nothing in this repository links model code, and no adapter may take an
authorisation decision on the other side of that boundary.

The common line under all three: a second language is permitted where it does
work this project does not want to own, and it is refused wherever a permission
decision is taken, because that is where the property has to be refusable.

## File naming

Decision records live in `docs/decisions/` and are named `NNNN-slug.md`, with a
four digit number allocated in order and never reused. This record is 0001.
Every later decision record in this repository follows the same naming.
