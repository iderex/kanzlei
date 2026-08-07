# How a permission set is resolved

`docs/decisions/0003-permission-model.md` decides that a document is filtered
at index time and re-authorised at the point of use. This document is the
narrower thing underneath both of those: given a permission set and a
principal, what the answer is.

## The rule, in three sentences

**A deny entry beats an allow entry, whatever order they are written in.**
Held by `TestDenyBeatsAllowWhateverTheOrder` in `internal/authz`, which writes
every case twice with the entries reversed, because an evaluator that returns
the first match resolves one order correctly and the other one wrongly.

**A principal that matches nothing gets nothing, and a set with no entries
matches nobody.** Held by `TestMatchingNothingIsDenied` and
`TestAnEmptySetDeniesEveryone`.

**A term the evaluator does not recognise is a deny and not a skip.** Held by
`TestAnUnrecognisedTermDenies`, and proved to be load bearing by
`TestTheNearMissOnRecognition`.

## Why the third sentence is the one that matters

The first two are the rules everybody writes down. The third is the one that
decides what happens on the day this project grows past what it can currently
read.

A connector written a year from now will emit a permission construct this
evaluator has never seen: a device condition, a time window, a classification
label, a construct from a source system nobody here has looked at yet. An
evaluator that skips what it cannot parse treats that construct as though it
were not there. If the construct was a restriction, the restriction stops
restricting, and it does so silently, on exactly the documents somebody
bothered to restrict.

So an entry this evaluator cannot read refuses the document, and it refuses it
before asking who the entry names. The order is deliberate. An unreadable entry
naming somebody else is still an unreadable entry, and an evaluator that
checked who it named first would have to understand it to find that out.

`TestTheNearMissOnRecognition` is the fixture that proves this is doing work
rather than being satisfied by accident. Two permission sets differ in exactly
one field, the term type of a deny entry that names something the asking
principal is not. In both readings that deny is inert. The unrecognised one
refuses the document and the recognised one lets the allow beside it through,
and if that pair of results ever converges the rule has stopped existing.

## There is no administrative route through this evaluator

An empty permission set refuses everybody, and that includes whoever the
deployment considers privileged. `internal/authz` has no field for it, no
branch for it and no identifier naming one;
`TestNoIdentifierNamesAPrivilegedRoute` reads the package's own source and
refuses one being added. The list of names it looks for is a list somebody
maintains, so a route named something nobody thought of walks past it. That is
the check's limit and it is written into the test beside it.

This absence is the design. A privileged read of a document somebody was not
granted is a real operational need and it is a different act from an ordinary
retrieval: it wants its own route, its own configuration, and an audit record
that says an override happened rather than a record that says access was
granted. Folding it in here would make those two indistinguishable in every
record downstream. Nothing in this repository provides such a route today, and
this document is not the place that would decide to.

## What this evaluator does not decide

It does not decide who the principal is. `Principal` here carries the subject
identifier the provider issued and the groups resolved for the session, which
is what an entry can name. The fuller principal, with the per-source
identifiers and which source asserted each one, is #16, and it is mapped into
this one at the call site rather than imported here.

It does not decide whether the permission set it was handed is complete. A
connector that can only partially resolve a source's permissions says so, which
is #20, and the consequence of a partial answer is #19 rather than a different
result from this function.

It does not resolve container inheritance. A source expressing permissions by
inheritance with local exceptions is flattened by its connector into the
entries this evaluator sees, and deny winning is what makes that flattening
safe to do.

It records nothing. Every refusal carries a `Reason` from a fixed vocabulary so
that what happened can be queried rather than grepped, and #40 is where those
are written to the audit trail. Nothing in this tree writes an audit record
today, so the reason is currently produced and dropped by every caller.

## The known limits

The evaluator is pure: same set, same principal, same answer, no clock and no
network. That is what makes it testable and it is also what makes it only one
layer of the model in `0003`, not the whole of it. Everything about staleness,
about a source that has changed since the set was captured, and about the
recheck at the point of use lives elsewhere.

`internal/authz` today resolves two term types, a user and a group. Nested
groups are flattened before they get here, or the connector declares them
unresolved under #20, and this evaluator never sees the difference. That
placement is deliberate: an evaluator that resolved nesting would need to reach
a source system, and a function that can reach a source system is a function
that can fail open when the source is unreachable.
