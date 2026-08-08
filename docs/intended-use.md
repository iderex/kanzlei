# What this project is for, and what it is not for

[NOTICE.md](../NOTICE.md) says the software is developed for lawful use. That is
the floor and it is a general sentence. This document is the specific version,
because a system that searches an organisation's documents on behalf of
individual people has misuses that a general sentence does not reach.

## What it is for

An organisation that already holds documents in systems with access control, and
that wants people to be able to ask questions of those documents without any
individual seeing more than they already could.

The three things it is built to do are to know who is asking, to find only what
that person may see, and to be able to show afterwards what happened. It is
built to run on the operator's own infrastructure so that the documents and the
questions asked of them stay there.

## What it is not built for

**Monitoring individual people.** The audit trail exists to secure the system
and to answer a question about the system: who saw what, which decision was
taken, what was in a context. It is not a record of what employees have been
reading, and using it that way is a use this project is not built for. The
design follows from that. Document content, questions and generated answers are
not in the trail by default, the trail is queried by an auditor rather than
grepped by anyone with file access, and a retention period is set deliberately
rather than left to accumulate. Where a setting exists that would record more,
the documentation states what it turns this system into.

**Making access decisions.** This system reproduces decisions somebody else
already took. It has no view about who should be allowed to read a document and
it never grants anything.

**Deciding anything about a person.** No output of this system is a judgement
about somebody, and nothing here is built to support one. An answer is text
assembled from passages a person could have opened themselves.

**Discovering documents an organisation does not know it holds, on people who
have not been told.** The system finds what the corpus contains, faster than a
person could. That is the point, and it is also the thing that makes an
unconsidered corpus a problem rather than a filing cabinet.

## The permission model reproduces, it does not improve

This is the sentence most worth reading twice.

The permissions this system enforces are the permissions the source systems
already express. A document that half the organisation can open in the source is
a document half the organisation can retrieve here. The model is careful about
not showing more than the source allows. It has no mechanism, and no intention,
for showing less.

So a source with bad permissions produces a searchable index with bad
permissions. What changes is that the material becomes findable. A folder that
was technically readable by everyone but that nobody would have found is
findable the day it is indexed, and the access control that failed to protect it
was already failing before this system existed.

An operator pointing this at a source is therefore making a claim about that
source's permissions. Checking that claim on a small subset first, with two
accounts of genuinely different access, is the check that matters, and the
operator documentation puts it in the order where it costs least.

## Answer quality depends on a model this project does not supply

The model runs behind a network contract and is chosen and operated by whoever
deploys this. What that means for what a user should expect is in
[scope.md](scope.md), which states the position plainly: model quality is out of
scope here, the parts of the system that carry the permission promise do not run
through the model, and a weak model produces a worse answer rather than a leak.

An answer from this system is not authoritative because it came from this
system. The citations are there so that a reader can check it against the
document, and checking it is the intended use of a citation.

## Before deploying this on material about people

In an operator's terms, not a lawyer's. None of this is legal advice and this
document does not claim conformance with anything.

Know which sources are going in, and be able to say what is in them. A source
added because it was easy to connect is a source nobody has read.

Know who can see what in those sources today, and check it rather than assume
it. This system will make that answer visible whether or not it is the answer
anyone expected.

Decide how long the audit records are kept before turning it on, because the
decision is much cheaper before there are records than after.

Decide whether questions and answers are recorded at all, and be able to say why.
That setting is the difference between a system that secures itself and a system
that watches people.

Tell the people whose material it is that it exists, what it records, and who
can query the record. A system that is discovered rather than announced is one
whose first appearance is as a complaint.

Be able to answer a question about one person's data without giving somebody a
database console. That is what #99 builds, and until it is built the answer is
an operator's manual work.

Have somewhere for a person to raise a concern, and somebody whose job it is to
read it.

## What this document is not

It does not repeat the warranty and liability text, which belongs with the
licence and is not restated here. It does not list every law that might apply to
a deployment, because that list depends on the operator and not on this
software. It is also not the deployment note: the technical measures, the
network shape and the residual risks are collected for somebody deciding
whether to deploy under #104, and that document does not exist yet.
