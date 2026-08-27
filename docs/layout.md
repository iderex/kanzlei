# Repository layout

Where things go, and which package may import which. It is written now, while
the tree is small enough that the rules are cheap, because the boundary this
project rests on is easier to keep as a directory boundary than as something
somebody remembers.

This note is the authority for what a directory is for. A directory that is not
explained here is either a mistake or an edit to this file that was skipped.

## Top level

`cmd/` holds one directory per binary, and each holds `package main` and as
little else as possible. `cmd/kanzlei` is the service: it parses flags, builds
what it needs, starts it and stops it. Logic that is worth testing does not live
here, because a `main` package is awkward to reuse and its tests cannot be
imported by anything else.

Not every binary here is part of the service. `cmd/coverfloor` is a check this
repository runs against itself: it reads the coverage profile the default suite
wrote and refuses a run that fell below `coverage-floor.txt`. It is a binary
rather than a shell block for the reason
[decisions/0001-means.md](decisions/0001-means.md) gives, that a rule deciding
whether the tree is acceptable belongs where a fixture can be put in front of
it. `go build ./...` builds it alongside the service; anything that ships the
service names `./cmd/kanzlei` rather than the module.

`cmd/scanfloor` is the second of those, and it is here for the same reason. It
reads the output of the code scanning analysis and refuses a run carrying a
finding at or above the severity in `scan-floor.txt`. The analyser itself
refuses nothing, and the alternative to a program here was a setting outside the
tree that no diff would show changing.
[code-scanning.md](code-scanning.md) is the whole of it.

`internal/` holds everything the binary is made of, and the packages that are
not: the checks this repository runs against itself. It is `internal` on purpose:
nothing outside this module can import any of it, so no part of this tree
becomes somebody's dependency by accident and every package here can be changed
without an argument about who else is using it.

`docs/` holds prose for people building or operating this. `docs/decisions/`
holds the decision records, numbered `NNNN-slug.md` in the order they were
taken, with the number never reused. What a decision record is for and how it is
named is settled in [decisions/0001-means.md](decisions/0001-means.md).

`testdata/` holds test material read from disk rather than written in source.
The Go toolchain ignores any directory with that name, so nothing in it is
built. `testdata/fixtures/` is declared binary in `.gitattributes` and is for
fixtures whose exact bytes are the thing under test.
[testdata/README.md](../testdata/README.md) says which fixtures belong there and
which belong beside the test that reads them.

`test/` holds the suites that are not part of the default run, one directory per
harness, each named after what it needs.
`test/needs-real-hardware-or-services/` is the one that exists: the cases that
cannot be proved without a real model runtime, a real identity provider or a
real source system. A test that can be written against a fake belongs beside the
code in `internal/`, not here.
[the readme in that directory](../test/needs-real-hardware-or-services/README.md)
says what each suite needs and how a run is read.

`.github/` holds what the hosting service reads and nothing this project runs
itself. `.github/workflows/` holds the checks, one file per published check,
`.github/ISSUE_TEMPLATE/` holds the forms that route a report, and
`.github/codeql/` holds the query set the code scanning analysis is configured
with, which is in the tree rather than left to the analyser's default.

## Inside `internal/`

One package per thing the project has to be able to do, named after the thing
rather than after its layer. A package called `util`, `common` or `helpers` is a
package nobody can describe, and the first import cycle in this tree will come
from one.

`internal/build` reports which version this binary is and which commit it was
built from. It imports nothing from this module and never will, so anything may
import it.

The packages that check this tree are the exceptions to the sentence above, and
they are the ones named below rather than a count stated here. None of them is
part of the binary, nothing in the service imports any of them, and each lives
here rather than in a shell block inside a workflow because it decides whether
the tree is acceptable, and that decision is worth having fixtures in front of
it. They are not enumerated anywhere else; this is the note that says what each
directory is for, and it is where they belong.

`internal/sourcecheck` reads this repository's own Go source and refuses what no
analyser can see, which today is a suppression comment carrying no reason.

`internal/coverfloor` reads a coverage profile, reports what share of statements
the run reached, and decides whether that clears the floor the tree holds.
`cmd/coverfloor` is the argument parsing over it and takes no decision of its
own.

`internal/testreach` reads this repository's own test files and refuses a test in
the default run that dials an address which is not a loopback one, resolves a
name, or opens a device, and refuses a marked file that is not under `test/`. It
exists because the condition in #7 cannot be enforced from inside a test
process, and it is the half of that condition which can be read out of the tree.

`internal/importrules` is the one that reads this note.
It decides whether the import graph inside this module is the one
`import-rules.txt` declares, and its own test is what turns the section below
from prose into a rule. It is not part of the binary and nothing in the binary
imports it.

`internal/scanfloor` is another of them, and the same sentence applies. It reads
the SARIF a code scanning run wrote, resolves the severity of each finding
against the rules the document declares, and decides which of them refuse the
run. The direction it decides in is the part worth the fixtures: a finding whose
severity cannot be determined refuses, rather than being skipped or read as
low. `cmd/scanfloor` is the argument parsing over it.

`internal/scanfixture` is not a package at all in the sense the others are. It
holds one deliberately defective file, kept out of every build by
`//go:build scanfixture`, whose only purpose is to make the code scanning gate
go red on demand. Nothing imports it and no binary contains it.
[code-scanning.md](code-scanning.md) says how the proof is run and why a gate
whose analyser runs on the hosting service cannot be proved by the suite.

`internal/server` holds the HTTP surface: the routing table, the listener and
the shutdown. Handlers live here. What a handler calls does not.

`internal/audit` declares the one record shape every event in this project is
written as, and refuses a record that could not be queried as intended. It
writes nothing and stores nothing. [audit.md](audit.md) is generated from that
declaration rather than written by hand, and the test that generates it also
compares it, so the two cannot drift.

`internal/auth` turns what an identity provider said about a user into a
principal that a source system's permissions can be compared against: the
subject identifier, the groups resolved for the session and how old that
resolution is, and the per-source identifiers with how each one was
established. It imports `internal/authz` and nothing else from this module.
[principal.md](principal.md) is where the mapping rules are argued.

`internal/runtime` declares the contract every model engine is reached through:
three operations, the declaration an engine has to make about itself, and the
four ways a call into one fails. It speaks to nothing. There is no address, no
client and no retry here, because a package that both declared the contract and
implemented one engine would be a contract shaped like that engine, and the
adapters are #74 and #75. [decisions/0009-runtime.md](decisions/0009-runtime.md)
is where the decision above it was taken.

`internal/runtime/fake` is an engine that answers without a model, so the
suites that reach a runtime can run under the conditions in #7. Everything it
produces is derived from the request by a digest, and it can be told to
misbehave in each of the ways a real engine does. It is test-only, and nothing
that ships may import it. [fake-runtime.md](fake-runtime.md) is where what a
result from it proves, and what it does not, is written down.

`internal/runtime/contract` is the suite an adapter is handed to, so that the
edges the engines differ at are pinned once rather than written out again per
adapter. It also holds the register of adapters and compares it against the
tree in both directions, which is what makes an adapter that nothing hands to
the suite a red run rather than a gap. It is test-only and it imports the
contract and nothing else. [runtimes.md](runtimes.md) is where a difference it
finds is recorded, with the engine and version it was observed on.

`internal/authz` resolves a permission set against a principal and answers
whether the document may be shown. It is pure: it reads no source system, holds
no clock and takes no decision about who the principal is, so the same set and
the same principal always produce the same answer. It imports nothing from this
module. [permissions.md](permissions.md) states the rule it holds and names the
test behind each sentence of it.

## The import rules

They are the reason this note exists rather than a preference about tidiness.

`import-rules.txt` at the root of the repository is where they are declared, one
line per package, and it is the authority for what is permitted. This section is
the argument for the shape of that file and not a second copy of it: a table
written twice is a table that disagrees with itself, and the file is the half a
check reads.

The declaration is a permission list. Each line is the complete set of packages
inside this module that the package may import, so an edge nobody thought about
is refused rather than allowed. Most of the rules below are therefore carried by
what a line does not say.

**The index is reached through the retrieval package and through nothing else.**
Every read of the index goes through the one package that applies the permission
filter, so there is no second route to the same data with the filter left off.
The whole argument of #15 and #62 is that the filter is inside the query rather
than wrapped around it, and a package that can be imported from two places is a
package where one of those places will eventually skip it.

**No HTTP handler imports the index.** A handler asks the retrieval package a
question and is given an answer already filtered for the principal that asked.
A handler holding an index client is one refactor away from a handler that
queries it directly, and that refactor looks harmless in a diff.

**The model runtime is not reachable from the store package.** Text on its way
to being embedded passes through the packages that ask for an embedding. A store
that could call a runtime itself is a store that can send a document somewhere
on its own, which is the one thing an operator running this on their own
infrastructure is promised does not happen without their say.

**A fake is reachable from a test file and from nothing that ships.** A fake
source and a fake runtime exist so that the contract suites can run under the
conditions in #7. A shipped file that reached one would be a binary answering
from a fixture corpus, and the answer would look exactly like a real one.

Two of the packages those four rules are about are in the tree.
`internal/runtime` arrived under #71 with the empty line the third rule needs,
and `internal/runtime/fake` arrived under #72 carrying the marker the fourth
rule is written as: a line permitting the contract and nothing else, and a
test-only marker that refuses a shipped file reaching it. The rest are declared
in `import-rules.txt` as planned, each carrying the
issue that creates it, so the rule is written before the package rather than
after the first violation. The day such a package lands, its marker is stale and
the check says so rather than passing over it, which is what stops a planned
line from becoming a place to keep a package nobody declared.

The third rule is not yet enforceable in the direction it is written, and saying
so is cheaper than a reader assuming otherwise. It refuses a store that can call
a runtime, and there is no store; what is in the tree is the runtime half, whose
own line permits nothing at all.

Two rules that apply to what is there today:

`internal/build` imports nothing from this module. It is the one package
everything may depend on, and that holds only while it depends on nothing.

`cmd/kanzlei` imports from `internal/`, and nothing under `internal/` imports
`cmd/`. A package that reaches back into a `main` package has made the binary a
dependency of the thing the binary is built from.

## What enforces them

`internal/importrules` reads `import-rules.txt` and the import graph of this
module, and its case over this tree fails on a disagreement between them. The
failure names the package, the import that was refused and the chain a binary
reaches that package by, because a forbidden import usually arrives through a
package that looked like a helper and the reader's question is which one.

    go test ./internal/importrules -count=1

It runs in the default suite rather than behind a tag or on a schedule. A
forbidden import is cheapest to remove in the minute it is written.

The file fails closed in both directions. A package in the tree with no line is
refused, so the declaration cannot quietly fall behind the tree. A line naming a
package that is not in the tree is refused as a typo unless it is marked
planned, and a planned marker for a package that has since arrived is refused as
stale.

What it does not reach is worth knowing before it is trusted. It reads import
declarations out of source, so it sees what a file says and not what a program
does: a package reached through reflection, a linker flag or a plugin is
invisible to it, and so is a directory the walk skips, which is `testdata` and
anything whose name starts with a dot or an underscore. It reads this module
only. It has no opinion about whether a permitted edge is a good idea, which
stays a question for review.

The tree matching the file is a fact about the tree on the day the case ran, and
it is not evidence that the rules bite. What proves that is the fixtures beside
the case, each of which puts one violation in front of the same functions and
requires the refusal.
