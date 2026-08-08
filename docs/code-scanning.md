# Code scanning

What the gate is, what it refuses, and how it was proved to refuse anything at
all.

## What it looks for

Defects an analyser finds by following a value through the program. A value
taken from a request that reaches a file path, a database query, a command line
or a template, across function boundaries, is the shape this project cannot
carry: a permission-aware retrieval service whose handler can be walked out of
has no permission model, whatever the permission model says.

That lens is not the one the other gates have. The formatting gate reads shape.
The vet suite, errcheck and nilness each read one property of one function at a
time. None of them follows a value from where it entered the process to where it
was used.

## The language and the query set

Both are named in the tree rather than detected.

`.github/workflows/codeql.yml` names Go. `.github/codeql/config.yml` names
`security-extended`, says why it is that suite rather than the default one or
the one that also carries maintainability queries, and holds the place where an
excluded query would be written with its reason. Nothing is excluded today, and
the empty filter list is there so that an exclusion is a diff rather than an
alert quietly dismissed in a dashboard.

## What refuses the run

`cmd/scanfloor`, against the number in `scan-floor.txt`.

This part is worth being explicit about, because the obvious answer is wrong in
a way that looks fine. The analyser does not refuse anything. It records
findings, they become alerts, and the run is green. Making them block is either
a repository setting, which is a decision living outside the tree where no diff
ever shows it changing, or a program in the tree. This repository already made
that choice once, for the coverage floor, and `docs/decisions/0001-means.md` is
where the rule is argued: a rule that decides whether the tree is acceptable
belongs where a fixture can be put in front of it.

The number is zero, meaning every security finding refuses the run.
`scan-floor.txt` carries the argument for it beside it.

Two properties of the decision are in `internal/scanfloor` rather than in the
number:

A finding at or above the floor refuses the run. At, not above: a finding
sitting exactly on the number is refused, which is the comparison somebody will
one day write the other way round, and there is a test whose only job is that
boundary.

A finding whose severity cannot be determined refuses the run. Not skipped, not
read as low, not counted as zero. An analyser that reports a result under a rule
this tree cannot find a severity for has told us something, and how bad it is
is not known. That direction is the same one
`docs/decisions/0003-permission-model.md` takes for permissions, and it is taken
here for the same reason: the alternative is a gate that stops gating without
anybody noticing.

An analysis that wrote no output at all is a refusal rather than a clean tree,
for the same reason the coverage floor refuses a missing profile.

## How the gate was proved to bite

By running it and watching it go red.

`internal/scanfixture` holds one deliberately defective file: a handler that
joins a name taken from a request onto a document root and opens it, which
`filepath.Join` cleans and does not confine. It is excluded from every build by
`//go:build scanfixture`, so it is in no binary, and the analyser does not see
it either on an ordinary run.

The workflow takes a manual input that adds that tag. With it set, the analyser
compiles the file, reports the finding, and the floor step refuses the run. That
run does not upload its findings, because they describe a file that ships
nowhere and an alert naming a path nobody can find in their checkout is worse
than no alert.

    gh workflow run code-scanning.yml -f include_fixture=true

That route only exists once the workflow is on the default branch, because a
manual trigger is read from there and from nowhere else. So the proof follows
the landing rather than preceding it, and #106 is where the run is recorded: two
runs of the same workflow, one red and one green, differing only in whether the
defective file was compiled, is what stands behind the claim that this gate
refuses anything. Until that record exists, the claim in this section is a claim
and the sentence saying so has not been removed.

The half of the decision that does not need the hosting service is proved in the
suite instead, on fixtures:

    go test ./internal/scanfloor ./cmd/scanfloor -count=1

Those cover the boundary, the unknown severity, the missing file and the
malformed document, each as a case that fails if the comparison is written the
other way round.

## What this gate does not cover

It analyses Go. The workflow files themselves are analysed by a different gate,
and the supply chain by another.

It does not read the alerts that other tools upload into the same dashboard.
The severity floor judges the output of this analysis and nothing else, so an
alert arriving from elsewhere is visible to a reader and is not judged here.

It runs on the hosting service and there is no local reproduction. The
contributor guide's rule for a check of this kind applies: push and read the
result rather than assume it.
