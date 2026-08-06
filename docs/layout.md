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

`internal/` holds everything the binary is made of. It is `internal` on purpose:
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

`.github/` holds what the hosting service reads and nothing this project runs
itself. `.github/workflows/` holds the checks, one file per published check, and
`.github/ISSUE_TEMPLATE/` holds the forms that route a report.

## Inside `internal/`

One package per thing the project has to be able to do, named after the thing
rather than after its layer. A package called `util`, `common` or `helpers` is a
package nobody can describe, and the first import cycle in this tree will come
from one.

`internal/build` reports which version this binary is and which commit it was
built from. It imports nothing from this module and never will, so anything may
import it.

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
