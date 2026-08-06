# Contributing

## Before you start: this repository has no licence yet

    gh api repos/iderex/kanzlei --jq '.license == null'

returns `true`. Until a licence file lands there is nothing either side can
point at that says what may be done with a contribution, and the sign-off below
certifies a contribution under "the open source license indicated in the file"
that does not yet exist. #101 is the issue that adds it and it is blocked on a
decision recorded in #124.

You are welcome to open an issue, and to discuss a change, now. A code
contribution is better held until the licence is in the tree.

## No work without an issue

Every change starts as an issue and lands as a pull request. The default branch
takes no direct pushes. What stands behind that is a ruleset rather than this
sentence:

    gh api repos/iderex/kanzlei/rulesets --jq '.[] | select(.name == "gate") | .id'
    gh api repos/iderex/kanzlei/rulesets/20487686 \
      --jq '{enforcement, bypass: .bypass_actors, required: [.rules[].type]}'

An issue says what is wrong, what the evidence is, and what done means. Where
the evidence is a number, it carries the command that produced it.

## The checks

This document does not list the checks. A list here drifts against the thing it
describes, and the drift is invisible until somebody follows the list and meets
a red check it does not mention. What exists is printed:

    gh api repos/iderex/kanzlei/actions/workflows --jq '.workflows[] | "\(.name)\t\(.path)\t\(.state)"'

and what ran on your pull request is printed by

    gh pr checks <number>

### Reproducing a check locally

Each published check is defined by exactly one file under
`.github/workflows/`, and the command above maps the check name to that file.
The file is the authority for what the check runs, not this document.

A step written as a `run:` block is a shell command you can run yourself against
a checkout, and running it is the exact reproduction. A step written as `uses:`
calls an action, and some of those talk to a hosted service that has no local
equivalent; those checks are reproduced by the run rather than on your machine,
and the honest thing to do is push and read the result rather than assume.

Where a check needs a tool, the workflow file pins its version. Use that
version locally. A local run with a different version answers a different
question.

## Tests run headless, unelevated and offline

Every test in the default suite runs with no display server, no administrative
rights, no accelerator and no outbound network, and a test that needs any of
those is marked and excluded from the default run by configuration rather than
by a flag anybody has to remember.

There is no command here that lists the marked tests, and there is no default
suite yet. Both come from #7, which establishes the condition and the marking
mechanism, and #8, which is the separate harness for the tests that genuinely
need a real service or real hardware. Until #7 lands, the sentence above is the
rule and nothing enforces it.

## Sign your work

Every commit carries a `Signed-off-by` trailer naming its author, and the DCO
check refuses a pull request where one does not. What that trailer certifies is
in [DCO](DCO), which is the Developer Certificate of Origin 1.1 verbatim. Read
it once before you sign.

`git commit -s` adds the trailer from your configured name and address:

    git commit -s -m "Your message"

The check compares the trailer against the commit author exactly, so
`Signed-off-by: Name <address>` has to match the name and address the commit was
authored with. If you have already committed without it:

    git rebase --signoff <base>

adds it to every commit in the range.

## What a good change looks like

One topic. A pull request that changes two unrelated things has a description
that describes one of them, and a reviewer who reads half of it.

A stated failure it prevents. The commit message and the pull request body say
what changed and what goes wrong without it. Where a correction is being made,
they also say what was wrong and how it was found.

A claim that carries its command. Where the body asserts a fact about the tree,
the state of a check or a number, it carries the command that produced it, run
against the reference the reader will have rather than against your working
copy. Where a claim cannot be backed by a command, write it as a claim and say
so.

A guard that is proven to bite. A check that refuses something ships with the
demonstration that removing what it refuses makes it red, shown by running it
rather than asserted.

Anything a change did not do stays written down as not done. A line saying a
step was skipped is more useful than a line implying it was not needed.

## Style

English, in every tracked file. No attribution of authorship to a tool in
anything tracked.

Documents in this repository are plain prose. Fixtures whose bytes matter get
their own treatment, and where that rule lands it is stated here.
