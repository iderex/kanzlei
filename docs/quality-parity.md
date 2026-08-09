# Quality parity

The standard this project is held to is not invented here. It is the gate that
stands in front of the default branch of the public repository
`iderex/jellyfin-plugin-sso`, adapted to a service in a different language.

This document maps that gate onto this project, entry by entry, and then names
the set proposed as merge conditions here.

It does not list the checks this repository has. That list drifts against the
thing it describes, and the drift is invisible until somebody follows it and
meets a red check it does not mention. What exists is printed:

    gh api repos/iderex/kanzlei/actions/workflows --jq '.workflows[] | "\(.name)\t\(.path)\t\(.state)"'

The target's required set below is quoted from a command rather than from
memory, because it is evidence about another repository and it moves.

## Reading the target rather than remembering it

    gh api repos/iderex/jellyfin-plugin-sso/rulesets --jq '.[] | "\(.id) \(.name)"'
    18802863 Protect main and 5.0

    gh api repos/iderex/jellyfin-plugin-sso/rulesets/18802863 \
      --jq '{enforcement, bypass:.bypass_actors, required:[.rules[].parameters.required_status_checks[]?.context]}'
    {"bypass":[],"enforcement":"active","required":["build","ABI floor build","Package (JPRM) / Build package","Package (JPRM) / Generate SBOM","CodeQL","Analyze (csharp)","DCO sign-off","Deterministic PR-hygiene checks","Enforce greppable invariants","Reject Trojan Source Unicode","Audit workflows (zizmor)","prettier","dependency-review"]}

    gh api repos/iderex/jellyfin-plugin-sso/actions/workflows --jq '.workflows[] | "\(.name)\t\(.path)\t\(.state)"'

Parity is not thirteen check names copied across. Some of them are about a
plugin binary and a plugin catalogue and have no counterpart in a service. Some
have a counterpart under a different name because the language differs. And this
project needs gates that one does not, because it takes untrusted document bytes
from an organisation's own file stores, holds an authorisation boundary, and
puts text from those documents into a language model.

## The target's required set, mapped

Each entry is matched, replaced, or not applicable, with the reason in one line
and the issue that delivers it here.

### build

Replaced. The counterpart is the default suite, compiled and run on every pull
request under a stable name, which is what a build check is for in a language
where compilation is part of the test command. Delivered by #6, on the harness
from #5.

### ABI floor build

Replaced. A plugin proves it still builds against the oldest host API it claims;
a service proves it still builds against the minimum toolchain and runs against
the minimum datastore version it claims, which are the numbers in
`docs/decisions/0001-means.md` and `docs/decisions/0002-datastore.md`. Delivered
by #2 for the toolchain pin and #6 for the run.

### Package (JPRM) / Build package

Not applicable. It builds a plugin package for a plugin catalogue, and this
project ships neither. The nearest thing here is a container image an operator
runs, which is a different artefact with a different failure mode, and it is
#116.

### Package (JPRM) / Generate SBOM

Replaced. The obligation is the same, a machine-readable statement of what is
inside the artefact, and the artefact differs. Delivered by #102.

### CodeQL

Replaced. Code scanning is the practice; the analyser is chosen for the
language. Delivered by #106.

### Analyze (csharp)

Replaced, and it is the job name of the entry above rather than a second
practice. The target names one job per language and this project has one
language, so the counterpart is a single job rather than a matrix entry. It is
published as `Code scanning`, which is the string the proposed set below holds
it by, and `.github/workflows/codeql.yml` is the authority for it.

### DCO sign-off

Matched. The same check, under the same job name, already runs here. It is in
the tree rather than owed, so no issue delivers it; `CONTRIBUTING.md` and `DCO`
explain what it certifies, which is #11.

### Deterministic PR-hygiene checks

Replaced. The target's check reasons about the pull request rather than about
the code: that the body carries an issue reference, that every commit subject
carries one, and that a version bump also touches the changelog. #14 delivers
templates, which prompt rather than refuse, so the refusal is its own thing.

The first two legs are delivered by #127, under the check name `Pull request
hygiene`. The decision lives in `internal/prhygiene` rather than in the workflow
file, so the shapes that look like a reference and are not have cases in front
of them.

The third leg is not delivered and is not claimed. This project has no version
number and no changelog, so there is nothing for that leg to read. #118 is where
a version number acquires a meaning, and the leg belongs with it rather than
here.

The target's job name carries the word deterministic because it separates a
deterministic tier from a heuristic one. The counterpart here has a refusing
tier and a warning tier, and the warning tier is the size, which never blocks.

### Enforce greppable invariants

Replaced. Same practice, a lint over the tree for invariants that are cheaper to
grep than to type-check. Delivered by #110.

### Reject Trojan Source Unicode

Matched. The same check, under the same job name, already runs here.

### Audit workflows (zizmor)

Matched. The same check, under the same job name, already runs here.

### prettier

Replaced. The practice is one formatter over everything the repository holds,
enforced rather than requested. The tool differs because the languages differ.
Delivered by #3.

### dependency-review

Matched. The same check, under the same job name, already runs here, failing on
any known advisory in a newly introduced or upgraded dependency.

## The target's non-required practices, mapped

These run there without being merge conditions. The mapping is the same three
kinds.

Mutation testing. Replaced, by #109, which is where coverage stops being the
measurement.

Fuzzing. Replaced, by #108, and it is also an addition below, because what this
project fuzzes is not what that project fuzzes.

A second static analyser. Replaced, by #107, on the same reasoning that two
analysers with different lenses find different things.

An end-to-end login harness. Replaced, by #35, which proves the sign-on flow
without a browser and without a display, so it runs under the condition in #7
rather than beside it.

A documentation lint. Replaced, by #113.

Supply-chain scoring. Matched. The same workflow already runs here.

The publication, manifest and nightly workflows. Not applicable. They publish a
plugin to a catalogue. The counterparts are the release route in #118 and the
image in #116, and they are release machinery rather than merge conditions in
either repository.

## Additions this project needs beyond the target

Each of these exists because of something this project does that the target does
not.

The authorisation conformance suite, #25. Every retrieval-shaped route is
checked against every user's expected visible set, including what reached the
model's context and not only what reached the client. The target has no
authorisation boundary of this kind to prove.

The adversarial suite, #26. The leaks this project is most likely to have,
written as tests before they are found in the field.

The headless, unelevated and offline conformance gate, #114. The condition is
established by #7; this proves it still holds on every pull request, which is
the only way an intention like that survives.

The import boundary, #111. Architecture rules turned into tests, so a retrieval
package cannot acquire a path around the permission filter by an import nobody
noticed.

The extraction fuzz gate, #108. This project parses formats strangers control,
taken from an organisation's own file stores, which is the surface where a
memory-safety or resource-exhaustion defect turns into an incident.

The filter under concurrency and under revocation mid-query, #68. A filter that
holds in a quiet test and not in a race is a filter that does not hold.

The connector contract suite, #49, and the runtime contract suite, #73. Both
exist because this project defines interfaces that somebody else implements, and
an interface without a conformance suite is a suggestion.

## Which of these are merge conditions here

The split is not by importance. It is by whether a red result means the change
is wrong.

A check is a merge condition when it is deterministic, fast enough that waiting
for it is reasonable, and red only when the change is at fault. A check whose
red result means the world moved, or that takes long enough that people learn to
merge around it, is advisory: it is run, it is read, and it does not hold the
branch.

That is why supply-chain scoring is advisory here even though it is valuable. It
scores the repository rather than the change, and its triggers are `push` to the
default branch, a schedule, and a branch protection event, with no
`pull_request` trigger at all, so there is no result on a pull request to
require. Mutation testing and fuzzing are advisory for the second reason: both
are long, and a fuzzer that finds something today could have found it last week,
which is a fact about the fuzzer's luck rather than about the change.

## The set proposed as required

This is a proposal. Changing the required set is a maintainer action, and this
document does not perform it.

The current state, read rather than described:

    gh api repos/iderex/kanzlei/rulesets --jq '.[] | "\(.id) \(.name) \(.enforcement)"'
    20487686 gate active

    gh api repos/iderex/kanzlei/rulesets/20487686 \
      --jq '{enforcement, bypass:.bypass_actors, rules:[.rules[].type], required:[.rules[].parameters.required_status_checks[]?.context]}'
    {"bypass":[],"enforcement":"active","required":[],"rules":["deletion","non_fast_forward","pull_request"]}

So today the branch refuses deletion, refuses a non-fast-forward, and requires a
pull request, and no check is a merge condition. Every check in front of this
branch is advisory by configuration, whatever any document says about it.

### Proposed now, from checks that exist and run on pull requests

`DCO sign-off`. Without it a commit lands with no assertion from its author that
they had the right to submit it, and the assertion cannot be added afterwards by
anybody else.

`Reject Trojan Source Unicode`. Without it source can be made to render
differently from how it executes, which defeats review itself rather than any
one reviewer.

`Reject nondeterministic text`. Without it a tracked file's bytes depend on the
clone that wrote it, and a fixture whose bytes are the thing under test is
rewritten on the way into the index.

`Audit workflows (zizmor)`. Without it a change to a workflow can widen its own
permissions, take an unpinned action, or interpolate untrusted input into a
shell, and workflow files are the one place a change can grant itself rights.

`dependency-review`. Without it a dependency carrying a known advisory lands and
nothing says so until somebody looks.

`Tests`. Without it a change lands that does not compile or does not pass, which
in a language where compilation is part of the test command is the whole of what
a build check is for. It carries a second thing under the same name: the suite
runs with its route out removed, so a red result also means a test acquired a
dependency on the network, and that is always the change's fault and never the
world's. There is no second job and no second string a protection rule could
name, because the condition binds the run rather than the job and a second job
would have to run the whole suite again to be about anything. What the condition
still does not reach is in #114, which stays open on it.

`Code scanning`. Without it a defect class the compiler does not see lands and
nothing reads the tree for it. It also carries the one proof route in this
repository for a gate whose analyser runs on the hosting service rather than in
the suite, which is the manual input in `.github/workflows/codeql.yml` that
compiles `internal/scanfixture` into the analysis and is expected to be refused.

### Proposed as they land

`#25`, the authorisation conformance suite, is in the required set. It is the
one check whose red result means the product's central claim is false for some
user, and an advisory gate on that claim is not a gate. The counter-argument is
runtime: a suite over every user and every query grows, and a required check
that takes too long gets worked around. The answer is to keep it in the required
set and treat its runtime as a defect in the suite when it becomes one, rather
than to move it out of the set the day it gets slow.

### Proposed to stay advisory

Supply-chain scoring, for the reason above: it produces no pull request result
to require.

Mutation testing, #109, and fuzzing, #108. Both are long-running and both go red
for reasons that are not the change in front of them. They are read, and a
finding from either becomes an issue rather than a blocked merge.

The documentation lint, #113, until it has been run long enough to know its
false-positive rate on prose. A formatter that is wrong about prose teaches
people to ignore it, and a check people ignore is worse than one that does not
exist.
