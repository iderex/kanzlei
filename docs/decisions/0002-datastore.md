# 0002 Datastore: permission facts and vectors in one place

Status: accepted

## Context

Where the permission facts live decides whether the promise in #15 can be kept.
If document permissions sit in one system and vectors sit in another, every
search is a join across two stores that can disagree, and the interval in which
they disagree is an interval in which somebody sees a document they may not see.
That is the exact failure this project exists to not have.

The question is therefore not which store searches vectors best. It is whether
the permission predicate and the nearest neighbour search can be one query, in
one transaction, over one consistent snapshot.

## Decision

One relational datastore holds documents, chunks, permissions, principals,
conversations and vectors. Vector search is performed by an extension inside
that same database, so the permission predicate and the nearest neighbour search
are evaluated together rather than composed afterwards.

The datastore is PostgreSQL. The minimum version is 16. The vector extension is
pgvector, minimum 0.8. The tags that exist upstream are printed by

    gh api repos/pgvector/pgvector/tags --jq '[.[].name][0]'

which returned `v0.8.6` when this record was written. The version floors here are
set by this record; the binding statement is the one the migration mechanism
enforces once #94 lands, and it is not enforced by anything in the tree today.

### Why they are not separated

Three reasons, in the order they matter.

A filter applied inside the query cannot be skipped by a caller who forgot it.
A filter applied after a search is a step, and a step is something a new
retrieval route can omit. #62 makes the predicate part of the query for exactly
this reason, and that is only available if the predicate and the vectors are
reachable from the same query planner.

A single transaction gives one snapshot. Revocation is the case that decides it:
#52 has to propagate a revocation faster than the next full sync, and #68 has to
prove the filter holds under revocation in the middle of a query. Both of those
are statements about a consistent view, and two stores have no shared view to
make a statement about.

A single store has no synchronisation lag to measure, so there is no lag to get
wrong. Two stores need a reconciliation job, and that job becomes a second thing
that can be behind, silently, in the direction that discloses.

## Rejected alternative

A dedicated vector database, with permissions in the relational store and
vectors beside it.

What it would have bought is real: better recall at scale, more index types,
approximate search tuned by people who do only that, and horizontal scaling that
a single relational instance does not offer.

What it costs is the join above, and with it the property the product is.

The measurement that reverses this decision is in #67, which fixes the retrieval
quality method and the corpus it is measured on. Reverse this record if measured
recall at a stated corpus size falls below what the answer contract in #80
needs, and reverse it by moving the vectors out rather than by moving the
permissions out. The permission facts stay where the authorisation decision is
taken, whatever else moves.

## What happens to #15 if a future change separates them

Written down here so the cost is on the record before anybody pays it.

#15 promises that a document filtered out at index time is also re-authorised at
the point of use, and that no retrieval route can reach the index without the
filter. Separating the stores does not remove the promise, it changes what
backs it. The predicate stops being part of the query and becomes a step
applied to the result of somebody else's query, which means:

- Every retrieval route acquires a way to be wrong that the compiler cannot see,
  and the conformance suite in #25 becomes the only thing standing between a new
  route and a disclosure.
- The window between a permission change landing in the relational store and
  landing in the vector store becomes a real interval, with a real duration, and
  that duration has to be measured, published, and stated in the answer under
  #66 whenever it is not zero.
- The fail-closed rule in #19 has to be extended: unreachable now has a second
  meaning, because the vector store can be reachable while the permission store
  is not, and a search that succeeds against a stale filter is worse than one
  that fails.

Any change that separates them owes those three, and owes an update to this
record saying which of them it did.

## Schema migrations

Migrations exist from the first table rather than from the first schema change.
The mechanism is delivered by #94, which requires that an upgrade needs no
manual step, and this record is what #94 names as its reason for existing from
the beginning: a store holding permission facts cannot acquire a migration
mechanism later, because the first schema change without one is the one that
runs by hand on a production instance holding somebody's document permissions.

There is no migration mechanism in the tree today. This paragraph names where it
comes from and does not claim it is present.

## What this record does not deliver

The suite does not yet run against a real datastore in a throwaway instance.
There is no suite, because there is no module (#2), and there is no throwaway
instance mechanism, because the condition in #7 has not been established. Both
are prerequisites of that step rather than parts of this decision, and the step
is unmet at the time of writing. It is not partially met and it is not met in
a different form.
