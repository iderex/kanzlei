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

`internal/sourcecheck` is the first exception to the sentence above: it is not
part of the binary and nothing imports it. It reads this repository's own Go source and
refuses what no analyser can see, which today is a suppression comment carrying
no reason. It lives here rather than in a shell block inside a workflow because
it decides whether the tree is acceptable, and that decision is worth having
fixtures in front of it.

`internal/coverfloor` is the second exception, for the same reason as the first:
it is not part of the binary and the service imports nothing from it. It reads a
coverage profile, reports what share of statements the run reached, and decides
whether that clears the floor the tree holds. `cmd/coverfloor` is the argument
parsing over it and takes no decision of its own.

`internal/scanfloor` is the third, and the same sentence applies to it. It reads
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

## The import rules

Two of them, and they are the reason this note exists rather than a preference
about tidiness.

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

Neither package exists yet. #62 creates them and this note gains their paths on
the day it does. Until then the rules are written down and there is nothing in
the tree for them to be about.

Two rules that do apply today, because the packages exist:

`internal/build` imports nothing from this module. It is the one package
everything may depend on, and that holds only while it depends on nothing.

`cmd/kanzlei` imports from `internal/`, and nothing under `internal/` imports
`cmd/`. A package that reaches back into a `main` package has made the binary a
dependency of the thing the binary is built from.

## What enforces them

Nothing. No test in this tree reads an import graph, so every rule above is
prose that a reviewer either notices or does not.

#111 is the issue that turns them into tests. Until it lands, a change that
imports the index from a handler will build, pass and merge, and the only thing
standing in front of it is somebody reading the diff.
