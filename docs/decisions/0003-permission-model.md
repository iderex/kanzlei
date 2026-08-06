# 0003 Permission model: filter at index time, re-authorise at the point of use

Status: accepted

## Context

Retrieval must never surface a document the asking user may not see. Everything
else in this project is an assistant. This is the reason it can be pointed at
real material.

There are two honest ways to do it and each fails in a different direction.

Filtering against permissions captured at index time is cheap. One query, one
datastore, no dependency on a source system being reachable while a user waits.
It fails on time. Between the moment access is withdrawn in the source and the
moment the next synchronisation notices, the index holds a permission that is no
longer true, and during that interval the system hands a document to somebody
who has lost the right to it. No synchronisation interval makes that interval
zero.

Authorising every candidate against the source of truth at query time has no
such interval. It fails on latency and on load. A search returning fifty
candidates makes fifty calls into a system that was not built for that, and it
stops working entirely the moment the source is unreachable, which is when a
search matters most.

## Decision

Both, in layers, with the layers doing different jobs.

The index-time permission set is a filter and a prune. It is applied inside the
search query rather than around it (#62), over the record defined in #17, and it
is never the last word.

Every document that is about to be shown to a user or placed in a model's
context is re-authorised immediately before that happens, against the freshest
authority available. A document that fails that check is dropped.

The prune exists to make the recheck affordable. The recheck exists because the
prune can be stale. Neither layer is sufficient and neither is decoration.

### What each layer alone would fail at

The prune alone fails on withdrawn access, in the interval between the source
changing and the synchronisation noticing. That interval is measured rather than
assumed, and #51 is where it is measured.

The recheck alone fails on latency and on availability. Every candidate becomes
a call into somebody else's system, the answer gets slower in proportion to how
much was found, and an unreachable source turns every search into an error
rather than into a smaller answer.

## The recheck is not configurable

Stated as a rule rather than as a preference: there is no configuration setting
that disables the point-of-use recheck, and none is added later.

The reason is not that a setting would be misused by a careless operator. It is
that a setting that turns it off is a setting that will be on in a deployment
somebody later describes as secure, in a document nobody re-reads, and the
system will look identical from the outside while the central claim is false.
A property that can be switched off is not a property of the system. It is a
property of one deployment's configuration file.

What is configurable is what the recheck costs: how long it may take, how many
candidates it runs against, and how its results are cached and for how long. Any
of those set badly makes the system slower or the prune wider. None of them
makes the recheck skippable.

## The freshest authority, per connector class

The recheck asks the freshest authority available. What that means depends on
what the source can answer, which is what the fidelity declaration in #20
carries.

A source with a permissions interface that answers who may read a named
document. The authority is that interface, called at the point of use. This is
the class the model is designed for, and a connector in it declares full
fidelity.

A source that expresses permissions by container inheritance and marks only the
exceptions. The authority is the same interface, asked about the document and
about the containers above it, and the answer is resolved with deny winning
(#18). A connector in this class is full fidelity only if it can read the
exceptions as well as the inheritance; if it can read one and not the other it
is not, and it says so.

A source that can only answer by acting as the user. The authority exists but
calling it at the point of use means impersonation on the request path, which is
a larger grant than this project should hold. A connector in this class declares
partial fidelity, and the recheck falls back to the index-time set with the
answer marked incomplete under #19 rather than pretending the check was made.

A filesystem tree (#50). The authority is the operating system's own access
check, evaluated as the mapped principal. It is cheap, local and exact, which is
why this connector is first: it is the one that can be tested honestly with no
service present.

A source that cannot provide any authority at the point of use. The recheck
cannot be performed, so the document is dropped and the answer says the search
was incomplete, naming the source. That is #19, and it is the same behaviour as
an authority that is simply unreachable, because from the user's side the two
are the same fact.

## The cost

Answers are slower by the cost of the recheck. The recheck runs against the
candidate set rather than against the corpus, so the cost scales with how much
was found rather than with how much exists, and the prune is what keeps that
number small. That is the prune's job and it is the reason it stays.

The source system carries traffic it did not carry before. An organisation's
document store was sized for people opening documents, not for a search engine
asking about fifty of them at once. Rate limits, caching windows and batch
interfaces are how a connector manages that, and each connector states what it
does.

An unreachable source degrades the product rather than merely degrading
freshness. A system built only on the prune would keep answering from a stale
index and say nothing. This one drops what it cannot re-authorise and says the
search was incomplete, which is a worse experience and an honest one.

That cost is accepted. The alternative is a system whose central claim is true
most of the time, and a claim that is true most of the time is not a claim an
organisation can put in front of a regulator.

## What later issues have to enforce

Each line of this model is a property somebody has to make refusable. This list
is what stops the model being prose with no owner.

Deny wins, and unknown means deny, wherever entries are resolved. #18.

The permission record holds allow entries, deny entries, the asserting source,
its revision stamp and the time it was read, and cannot be omitted for a
document in the index. #17.

Every connector declares its fidelity, and a consumer can tell a complete
permission set from a partial one. #20.

Permissions reach the chunk and the vector, or the chunk and the vector are not
stored. #21.

Conversation history is re-authorised before it is used again, because a
document that was visible when it entered a conversation is not necessarily
visible now. #22.

The absence of a result does not disclose that a document exists. #23.

An unperformable recheck drops the document, and the answer carries a
machine-readable marker naming the unreachable source and not the dropped
documents. #19.

The predicate is inside the search query rather than applied to its result. #62.

Reranking does not leak what the filter removed. #64.

Every retrieval-shaped route is proved against every user's expected visible
set, including what reached the model's context. #25.

The leaks this project is most likely to have are written as tests before they
are found in the field. #26.

The filter holds under concurrency and under revocation in the middle of a
query. #68.

The revocation propagates faster than the next full sync, and the lag is
measured rather than described. #51, #52.

## Relationship to the datastore

`docs/decisions/0002-datastore.md` keeps the permission facts and the vectors in
one store so the prune can be part of the query rather than a step around it.
That record states what a future separation would owe. This one is the reason
the debt would be worth counting: the prune is the affordability of the recheck,
and a prune that has to be composed after somebody else's search is a prune a
new route can forget.
