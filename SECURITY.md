# Security policy

## Report privately, never in a public issue

Report a vulnerability through GitHub's private vulnerability reporting on this
repository:

    https://github.com/iderex/kanzlei/security/advisories/new

That route is open. It is a fact about the repository rather than a promise made
by this file, and the command that reads it is

    gh api repos/iderex/kanzlei/private-vulnerability-reporting --jq .enabled
    true

A vulnerability is never reported in a public issue, a pull request, a
discussion, or anywhere else a stranger can read it. This project exists to keep
a document away from somebody who may not read it, so a public description of how
to reach one is not a report of the failure, it is the failure happening. The
private route puts the report in front of a maintainer without putting it in
front of everyone else at the same time.

If the private route is unreachable for you, open a public issue that says only
that you have something to report and holds no detail at all: no path, no
payload, no version, no reproduction. Wait to be contacted before saying more.

## What is in scope

Everything in this repository is in scope. Three surfaces are named because a
defect in them is worse than a defect elsewhere, and a report about one of them
gets read first.

The permission filter. This project's central claim is that retrieval cannot
return a passage the asking principal may not read, and that the filter runs
inside the query rather than around it. Anything that returns a passage, a
citation, a title, a filename or a count that the principal was not entitled to
belongs here. So does the weaker shape: an answer whose absence discloses that a
document exists.

The identity flow. Token validation, session handling, the mapping from a
provider's claims to a principal, and the bootstrap that creates the first
administrator. A defect here makes every permission decision downstream a
decision about the wrong person.

The ingestion parsers. Everything that takes bytes from a source system and turns
them into text or into a permission record. Those bytes are written by people
this project does not control, and the parser is the place where a hostile
document becomes a hostile input. Retrieved text that is treated as an
instruction rather than as data belongs here too.

Out of scope, and worth saying so you do not spend the effort: the quality of a
model's answer, a finding produced by a scanner and pasted without a working
reproduction, and anything that needs an attacker who already holds
administrative rights on the host.

## What to expect after you report

An acknowledgement within seven days, saying that the report arrived and who is
reading it. If seven days pass with nothing, send a reminder through the same
route.

An assessment after that, saying whether the report is accepted, and if it is,
what is affected.

A fix and an advisory published together. The advisory names the defect, what it
allowed, which versions carry it and what an operator has to do. Neither is
published without the other, so the first public description of a defect arrives
alongside the thing that repairs it.

Credit in the advisory under whatever name you give, or no credit if you prefer
that. Say which when you report.

## Which versions receive a fix

There is no released version yet:

    gh api repos/iderex/kanzlei/releases --jq length
    0

Until the first release, the default branch is the only thing that exists, so it
is the only thing that receives a fix. Nothing here is deployable and no operator
is running it.

From the first release onwards, the most recent release receives fixes. An older
release receives one only where the advisory says so by name. This paragraph is
written to survive that transition without an edit, and it names no version
number for the same reason.

## No bounty

This project pays nothing for a report. There is no bounty programme, no reward
and no payment of any kind, and there is no plan for one. Report because you want
the defect fixed. Anyone spending effort here on the expectation of payment is
spending it on a false assumption, which is why the sentence is in the policy
rather than left to be inferred.
