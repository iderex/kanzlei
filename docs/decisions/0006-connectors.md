# 0006 Connectors: one interface, and what a source has to be able to answer

Status: accepted

## Context

A connector is where this project meets somebody else's system. It is also
where the permission model in
[0003-permission-model.md](0003-permission-model.md) either holds or quietly
stops being true, because everything downstream of a connector trusts what the
connector said about who may read a document.

The tempting question is which sources to support first. That is a question
about audience and it is answered elsewhere. The question this record answers
is the one that has to come before it: what a source must be able to answer
before a connector for it may exist at all.

The reason to settle it now, while no connector exists, is that the answer is
uncomfortable. It rules out sources that people will ask for, and a rule that
only appears once somebody has already written the connector is a rule that
gets argued away.

## Decision

A source is supportable when it can answer four questions.

**What documents exist.** An enumeration this project can walk, with a stable
identifier per document that survives a rename and a move. Without a stable
identifier there is nothing for a permission record, a chunk, a citation or a
deletion to be about.

**What each one contains.** Bytes this project can extract text from, or text
directly. What extraction may and may not do is #54, and the boundary it runs
behind is in [0001-means.md](0001-means.md).

**Who may read each one, in terms this project can compare against a
principal.** Not who owns it, not which container it sits in unless containers
are how the source expresses reading rights, and not a display name that
happens to look like a person this project knows. The answer has to reduce to
entries that can be evaluated against the principal in #16 under the rules in
#18.

**What has changed since a stated point.** A change feed, a revision counter, a
modification stamp, or anything else that lets a synchronisation ask for the
difference rather than for everything.

### The third question is not optional

A source that cannot answer the third does not get a connector.

This is the line, and it is worth being blunt about what stands on the other
side of it. A connector that guesses at permissions does not produce a system
that is a bit less accurate. It produces a searchable index over an
organisation's documents with confident-looking access control on top, which is
strictly worse than no index at all: an organisation that has not indexed its
documents knows it has not, and one running this over a guessing connector
believes it has a permission model.

The failure is also invisible from the outside. Every screen looks the same.
The audit trail in #40 records decisions that were taken, and the decisions
were taken correctly against permissions that were made up.

So the refusal is at the interface, in #47, rather than in a reviewer's
judgement about a particular source.

### A partial answer to the third

Some sources answer it for part of what they hold, or answer part of it for
everything. A source that exposes direct grants but not group nesting, or
inheritance but not the local exceptions that override it, has answered the
third question partially and not completely.

That case gets a connector, and it gets one under #20: the connector declares
what it resolved and what it did not, per document, the declaration travels
with the permission record, and the evaluator reads an unresolved construct as
a denial for anyone whose access depends on it. This record does not restate
those rules. It states that partial is a declared state with a conservative
reading, and that a connector author may not close the gap by claiming more
than the source told them.

The distinction that matters here is between not knowing and knowing there is
nothing. A source that says a document has no restrictions has answered the
third question. A source that returns nothing because the connector lacked the
right to ask has not, and the two must not arrive at the evaluator looking the
same.

### A missing answer to the fourth costs freshness

The fourth question decides whether a deployment is usable, not whether it is
correct. A source with no change feed forces a full walk of everything it
holds.

In an operator's terms: with no change feed, the time between somebody losing
access in the source and this system stopping showing them the document is
bounded by how long a full walk takes plus how often it is scheduled. On a
large corpus that is measured in hours. During that interval the promise in
[0003-permission-model.md](0003-permission-model.md) rests entirely on the
point-of-use recheck, which is the layer that has no interval, and on that
source's ability to answer the recheck at all.

That number is measured rather than described. #51 is where it is measured, per
connector, and #52 is the path that beats it for the case that matters most,
which is a revocation. The measured number belongs in the documentation an
operator reads before they deploy rather than in a release note afterwards,
because it is a number they may find unacceptable, and finding it unacceptable
after the corpus is indexed is the expensive order to find it in.

## A connector runs with the least access the four questions need

A connector is given the smallest grant in the source that lets it answer the
four questions, and no more. Read where read is enough. Metadata-only where the
content is fetched under a separate narrower grant. Nothing that can write, for
the reason in the last section.

Some sources cannot express that. Their permissions interface is only available
to an account that can read everything, so the connector holds a credential
that is, in the source's own terms, an administrator. That is sometimes the
only way the third question gets answered, and this record does not refuse it.
It requires the connector to say so in its own documentation, in the connector's
page rather than in a general note, where an operator deciding to install that
connector will read it. The sentence names the grant, names why the source
gives no smaller one, and names what an attacker who obtained the credential
would be able to read.

A connector that needs an all-reading account and does not say so is a defect in
that connector, not a fact about the source.

## The first connector

A filesystem tree, which is #50.

It is first because it is the one that can be proved without asking anybody to
trust a fixture. It needs no external service, so the tests run under the
conditions in #7. Its permissions are real rather than simulated: the authority
is the operating system's own access check evaluated as the mapped principal,
which is exact and local, so the connector contract suite in #49 is run against
a real permission system on the day it is written rather than against a mock
that agrees with the code by construction.

It is also the connector that makes the fidelity declaration in #20 concrete
early, because a filesystem tree on one operating system and the same tree on
another do not express the same things, and the differences show up
immediately rather than being discovered by the second connector author.

Which source the second connector targets is not decided here. It is an open
question in #124, and it is a question about audience rather than about this
interface.

## What this project will not do

**No connector writes back into a source in the first release.** Not a tag, not
a comment, not a marker recording that a document was indexed, not a
last-accessed stamp.

Two reasons, and the second is the one that lasts.

The first is blast radius. A connector holds a credential into a document store
that an organisation depends on. A read-only credential that leaks is a
disclosure. A credential that can write is a disclosure and an integrity
incident, and the second is the one an organisation cannot recover from by
rotating a secret.

The second is that writing back changes what this project is. A system that
only reads can be removed from a deployment and leave the source exactly as it
found it. That property is worth more than any feature a write would buy, and
it is the property that lets an organisation try this against real material
without a rollback plan for their document store.

Adding a write later is a decision that reopens this record rather than a
capability somebody adds inside a connector.

## What later issues have to enforce

The four questions become the interface in #47, which is where a source that
cannot answer the third is refused by the shape of the code rather than by this
document.

The fidelity declaration is #20, and the contract suite that catches a connector
claiming more than it resolved is #49.

The fake source in #48 is what proves a connector-shaped thing can be tested
with no service present, and the filesystem connector in #50 is what proves the
interface fits a real one.

The lag a missing change feed produces is measured in #51 and beaten for
revocations in #52. Deletion is #53, which is a separate promise: that nothing
survives it.

A source that is unreachable, slow or lying is #56. This record says what a
source must be able to answer. It does not assume the source answers honestly.
