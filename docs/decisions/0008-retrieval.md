# 0008 Retrieval: the filter is inside the query, not around it

Status: accepted

## Context

Retrieval is where the permission model in
[0003-permission-model.md](0003-permission-model.md) stops being a decision and
becomes a query. This record fixes the shape of that query, because the obvious
way to build it is the wrong one and it is wrong in a way that passes every
test somebody is likely to write.

The obvious way is three steps. Search the index for the passages nearest the
question, take the best ones, then drop the ones the asking user may not see.
Every part of that is easy, the filter is a few lines, and a suite written by
the person who built it will be green.

What it destroys is the answer, and only for some users. A search asks the index
for the ten nearest passages. Those ten are the ten nearest in the whole corpus,
chosen before anybody looked at who was asking. A user permitted a tenth of the
corpus loses nine of them to the filter and is answered from one passage, while
nine perfectly good permitted passages sat just below the cut and were never
considered. The system returns an answer, the answer carries a citation, nothing
errors, and no log records that anything went wrong.

The damage lands hardest on exactly the users with the least access, which in
the organisations this project is for means the ones whose access was restricted
on purpose. It is also invisible from inside: the person who built the system
has broad access, and for them post-filtering removes nothing.

## Decision

The permission predicate is part of the search. It is evaluated by the datastore
inside the same statement as the similarity search, over one snapshot, so that
the passages the search returns are the nearest passages among those the asking
principal may see rather than the nearest passages with a filter run over them
afterwards.

The top ten are the top ten of what the user may see. That sentence is the whole
of this record and everything below it is consequence.

[0002-datastore.md](0002-datastore.md) is what makes it available. Permission
facts and vectors live in one store so that the predicate and the nearest
neighbour search are reachable from one query planner, and that record already
states what a future separation of the two would owe.

The predicate is applied over the permission record in #17, resolved under the
rules in #18, and it is a prune rather than the last word. What the last word is
is in the section below.

### Every leg of the search carries the predicate

Retrieval here is hybrid. A vector leg finds what a synonym would have found, a
lexical leg finds the part number and the case reference the model never saw,
and the two are combined by a stated method, which is #63.

The predicate applies to both legs, in the same way, inside each leg's own
query. It is not applied to the combination.

The reason is that a candidate set is a union. A leg without the predicate is a
route from a question to the index with the filter switched off, and a document
that entered the candidate set through that leg is in the set. Filtering the
union afterwards is post-filtering again, one layer up, with the same recall
collapse and the same invisibility, and it arrives by a route that looks like a
small change: a lexical index added beside the vector one, written by somebody
reading the lexical documentation rather than this record.

Reranking is the same rule from the other end. A reranker orders a candidate
set, and it only ever sees what the legs already permitted, which is #64.

### Where the recheck sits, and why it is not a substitute for this

[0003-permission-model.md](0003-permission-model.md) puts a second layer under
the first: every document about to be shown to a user or placed in a model's
context is re-authorised immediately before that happens, against the freshest
authority available, and a document that fails is dropped.

That layer sits after this one and it does a different job. The predicate is
about which passages the search considers. The recheck is about whether a
permission captured at index time is still true at the moment of use. The
predicate cannot answer the second question, because the index is as fresh as
the last synchronisation. The recheck cannot answer the first.

The direction that matters here is that the recheck only ever removes. It is
handed the candidate set the search produced and it drops from it. It has no way
to reach back into the corpus for the nine passages a post-filter cut, because
by the time it runs those passages are not in the set and nothing records that
they were considered. A system that post-filtered and then rechecked would pass
every authorisation test in this repository and would still answer badly for
every restricted user, which is why this record exists separately from the one
that describes the two layers.

The recheck is also what makes the prune safe to be a prune. Neither layer is
sufficient and neither is decoration, and that argument is in the permission
model record rather than restated here.

### What would show post-filtering damage if it ever came back

Two things, and they answer different questions.

The measurement is #67. It fixes an evaluation set and reports recall at a
stated cut-off and a rank-sensitive measure, and its third line is the one this
record names: the same numbers are reported for a restricted principal as well
as for an unrestricted one. Post-filtering shows up there as a gap between the
two that widens as the principal's access narrows. An unrestricted principal
loses nothing to a post-filter, so a measurement taken only against one is a
measurement that cannot see this defect at all.

#67 is a comparison instrument and not a gate, for the reason that issue gives:
a threshold on a quality number is a threshold that gets tuned against. So the
measurement is what makes the damage visible and it is not what refuses it.

What refuses it is #62, whose fifth line is the near-miss: moving the predicate
out of the query and into a filter over the results makes a test red, and the
test says why in its failure message. That is the guard. #67 is how anybody
would notice if the guard were removed along with the property.

### What an indexed predicate is expected to cost

Stated so that the number can be argued with before it is measured rather than
after.

At a corpus of one million chunks, with a principal permitted one per cent of
them, the expectation this record commits to is that the search examines a
number of rows proportional to the permitted set and the requested cut-off
rather than to the corpus, and that the added cost over the same search with no
predicate stays within the same order of magnitude. What would break that is the
planner choosing to scan: a predicate that cannot be satisfied from an index
turns the least-privileged user's search into a walk of the whole corpus, which
is the same user the post-filter would have answered badly, failing this time by
being slow rather than by being wrong.

That is an estimate and this repository has measured nothing. There is no
datastore package, no client and no instance:

    git ls-tree -r --name-only origin/main -- internal/store | wc -l
    0

run at `26e8114a0c8e5c0027201c5445169369b0530fa1`. No query has been planned, no
corpus of that size exists, and the paragraph above is reasoning about a shape
rather than a report of a run.

Two measurements replace it, and they replace different halves. #62's second
line asserts the query plan against a corpus large enough for the plan to be
meaningful, which is what turns the expectation about scanning into a fact about
this project's own query. #91 is where the cost becomes visible in a running
deployment rather than in a suite. When either lands, the paragraph above is
rewritten to carry the number and the command, and the estimate is deleted
rather than left beside the measurement.

## What later issues have to enforce

This record is a shape. Each line of it becomes somebody's refusal.

The predicate is one statement with the similarity search, read out of the
generated statement by a test rather than trusted, and no exported function in
`internal/retrieval` executes a search without a principal. #62.

`internal/store` is reachable only through `internal/retrieval`, so there is no
second route to the index with the filter left off. That is already declared in
`import-rules.txt` and refused by `internal/importrules`, and #62 is where the
packages arrive to be governed by it.

The lexical leg carries the same predicate applied the same way, and the
combination method is stated with the reason it was chosen. #63.

Reranking sees only what the legs permitted, and any score normalisation is
computed over the permitted set alone. #64.

Where the search rested on less than it should, the answer says so, naming the
reason and not the documents. #66.

The filter holds while permissions change under it, and the window between the
predicate and the recheck is proved rather than assumed. #68.

Every retrieval-shaped route is proved against every user's expected visible
set, including what reached the model's context. #25.

## What this record does not settle

Chunking is #59, embedding generation and its versioning are #60, and
re-embedding without a half-old index is #61. All three decide what is in the
index and none of them changes where the filter sits, so this record does not
speak about them.

Which datastore, and why the permission facts and the vectors are not separated,
is [0002-datastore.md](0002-datastore.md). The two layers and what each one
fails at is [0003-permission-model.md](0003-permission-model.md). This record
cites both and restates neither.

Nothing in this record is implemented. There is no retrieval package and no
store:

    git ls-tree -r -d --name-only origin/main -- internal/
    internal
    internal/audit
    internal/auth
    internal/authz
    internal/build
    internal/coverfloor
    internal/doclint
    internal/importrules
    internal/prhygiene
    internal/scanfixture
    internal/scanfloor
    internal/server
    internal/sourcecheck
    internal/testreach
    internal/treefmt

at the same commit. What this record delivers is the decision and the reason,
so that the query in #62 is written against an argument rather than against
whoever is at the keyboard.
