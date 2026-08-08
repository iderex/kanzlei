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

### Naming the issue in the change

The link between a change and the work it belongs to is checked rather than
asked for, under the check name `Pull request hygiene`. Two rules, and they are
the words the check enforces:

The body names an issue. Write the issue number as `#<number>` where the body
says which issue this closes.

Every commit subject names an issue. A merge commit is skipped, because git
writes its subject and merging the default branch forward is not a change with
an issue of its own.

What counts is a hash followed by a number, with no letter or digit on either
side of it, and no leading zero. A number written that way is what the tracker
itself links. A full link to the tracker written out in prose is not read as a
reference, and a reference inside a fenced block or a quotation is read as one:
the check reads the text as text and does not try to decide which part of a
document you meant.

The same check reports how many lines the change touches and never refuses one
for it. A bulk rename, a document migration and a first implementation all pass
any cap legitimately, and a cap that refused them would push a change into
pieces that are individually unreviewable.

Run it before you push, against the range the check will use:

    git log --format=%p%x09%s "$(git merge-base origin/main HEAD)..HEAD" > commits.tsv
    PR_BODY='Closes #123' go run ./cmd/prhygiene -changed 120 < commits.tsv

`.github/workflows/pr-hygiene.yml` is the authority for how the check gathers
those inputs, and `internal/prhygiene` is where the rule itself lives.

## The checks

This document does not list the checks. A list here drifts against the thing it
describes, and the drift is invisible until somebody follows the list and meets
a red check it does not mention. What exists is printed:

    gh api repos/iderex/kanzlei/actions/workflows --jq '.workflows[] | "\(.name)\t\(.path)\t\(.state)"'

and what ran on your pull request is printed by

    gh pr checks <number>

### Reproducing a check locally

Each published check is defined by exactly one file under
`.github/workflows/`. The first command above does not print the check's name.
It prints the workflow's own name, and the name published as a check is the
`name:` of the job inside that file, or the job's identifier where the job
declares none. Those are different words here, so that command maps a check to
its file only once the file has been read. What the tree declares is printed:

    git grep -n '^    name:' -- .github/workflows/

The file is the authority for what the check runs, not this document. Where a
section below gives the command for a check, it names the check that command
reproduces, because the published name is what a protection rule would hold the
branch by and a command with no name beside it cannot be matched to one.

A step written as a `run:` block is a shell command you can run yourself against
a checkout, and running it is the exact reproduction. A step written as `uses:`
calls an action, and some of those talk to a hosted service that has no local
equivalent; those checks are reproduced by the run rather than on your machine,
and the honest thing to do is push and read the result rather than assume.

Where a check needs a tool, the workflow file pins its version. Use that
version locally. A local run with a different version answers a different
question.

### The analysers

The check is published as `Static analysis`. It runs four things, and each is
named here with the command that reproduces it exactly as the check runs it.
This is the one place this document names what a check does rather than
pointing at the file, because an analyser you cannot run locally is one you
meet for the first time on a red gate. `.github/workflows/analysis.yml` is
still the authority, and if these commands and that file disagree, the file is
right.

Install the two that are not in the Go distribution, at the versions the
workflow pins:

    go install github.com/kisielk/errcheck@v1.9.0
    go install golang.org/x/tools/go/analysis/passes/nilness/cmd/nilness@v0.38.0

The standard vet suite, which the toolchain ships:

    go vet ./...
    go vet -tags needsreal ./...

An error that is returned and never looked at, and a type assertion whose
result is ignored:

    errcheck -excludeonly -exclude .errcheck-excludes -asserts ./...
    errcheck -excludeonly -exclude .errcheck-excludes -asserts -tags needsreal ./...

`.errcheck-excludes` is the whole rule set, because `-excludeonly` turns the
tool's own list off. Adding a line to it is how an exclusion is argued for, and
every line carries its reason above it.

A value that is nil where it is used:

    go vet -vettool="$(go env GOPATH)/bin/nilness" ./...
    go vet -tags needsreal -vettool="$(go env GOPATH)/bin/nilness" ./...

A suppression comment with no reason on the same line:

    go test ./internal/sourcecheck -count=1

Each command is run twice because a build constraint hides code from an
analyser as effectively as deleting it, and the harness under `test/` is only
compiled under its tag.

### Switching an analyser off

On the line that switches it off, say why. A comment that names the analyser and
nothing else is refused, and so is one whose reason is a single word:

    //nolint:errcheck // the error is the authorisation decision and is handled below

`internal/sourcecheck` is what refuses the bare one, and it refuses it whether
the suppression is on its own line or trailing the line it applies to. It reads
line comments only, because that is all any of the analysers read.

The same rule covers the suppression no analyser can see: a call whose last
return value is thrown away into the blank identifier.

    allowed, _ := authorise(principal, document)

That is the defect this project can least afford, because the value discarded
there is the authorisation decision. Where the discard is right, the reason goes
on the same line:

    _ = srv.Shutdown(ctx) // cleanup after the case has already reported, where a shutdown error changes no verdict

The check reads the position rather than the type: the last return value is the
error by convention in every Go program, so a blank anywhere else is left alone
and an ordinary destructuring is not caught by this.

### Formatting

The check is published as `Formatting`. Two commands, and the first is the one
that check runs, character for character:

    go run ./cmd/treefmt
    go run ./cmd/treefmt -write

The first reports and writes nothing. It prints every departure as a path, a
line and the rule that was departed from, and exits non-zero. The second puts
the bytes the rule set asks for into the files. Both come out of the same call,
so the mode that checks and the mode that writes cannot disagree about what
formatted means.

`.editorconfig` at the root is the whole rule set, and this document does not
restate it. Editors read that file as a courtesy and `cmd/treefmt` reads it as a
rule, so there is one rule set and not two. Where a rule is off, the line in
that file says why. A property written there that `internal/treefmt` does not
implement is refused when the file is parsed rather than passed over: a rule
nothing applies is worse than no rule, because the tree then looks governed and
is not.

Two things the tool reports and deliberately does not repair. Bytes that do not
decode as UTF-8, because rewriting them would put a guess at an encoding into
the tree. And a space indent on a path the rule set indents with tabs, because
how many spaces stand for one tab is not a thing a formatter gets to decide.

A `.go` file is formatted by `go/format` and by nothing else. A line-based rule
cannot see the inside of a raw string literal, so trimming a trailing space
there would silently edit a program's data.

The scope is every path `git ls-files` reports. A file you have not added is not
the tree, and a build output reported as a defect in the tree is how a gate
teaches people to ignore it.

If it reports a carriage return in a file you did not touch, your working copy
predates `.gitattributes`. `go run ./cmd/treefmt -write` fixes it, and so does a
fresh checkout of that file.

### Documentation references

The check is published as `Documentation` and it holds two rules. This section
is the first of them. Both are named here with the command that reproduces
each, character for character as the check runs it, and they sit under one
published name because a name is what a protection rule holds the branch by
and one gate either passes or does not.

    go run ./cmd/doclint

It refuses a document that names a path this repository does not have. That is
the way a load-bearing document fails here: a file moves, the sentence pointing
at it still reads correctly, and the next person follows it to a path that is
not there. The formatting gate judges bytes and the analysers judge Go, so
without this nothing reads a sentence.

There is no writing mode. Where a document names a path that is not there,
either the document is wrong or the path is missing, and no tool can tell
which.

What counts as naming a path is narrow on purpose, and the narrowness is the
reason the findings are worth fixing rather than arguing with. Two positions
are read. An inline code span, resolved from the repository root, which is how
this document writes `internal/authz` in a sentence. And a link target,
`[text](target)`, resolved against the directory the document sits in, which is
what a markdown reader does with it.

Three things are deliberately not read. A fenced block and an indented block,
because they hold commands, and a command line carries outputs and arguments
rather than paths in this tree: `coverage.out` is written by a command above
and is deliberately untracked, so a check that read command blocks would refuse
this document for being correct. And a span whose first segment is not a
directory this repository already has, which is what keeps `go/format` and
`application/json` out of the findings and also means `go.mod` and `DCO` are
not checked at all.

A path a document may name before it exists is read from `import-rules.txt`,
which already declares each planned package with the issue that creates it.
There is no second list, so the day a planned package lands the stale marker is
reported by the import rules rather than having to be noticed here.

It is a floor under the documents rather than a proof about them. `#113` is
where the rest of the documentation gate is argued, and the parts not built are
named there: external links, the syntax of commands inside code blocks, and
running the examples rather than reading them.

### The document shape

The second rule under the same check. One command:

    go run ./cmd/mdlint

It refuses a document written outside the shape the rest of the tree assumes.
Seven rules, each of which either has bitten something here or is what another
reader of these documents depends on:

A fenced block is written with backticks, and one that is never closed is
refused. Both matter more than they look, because the path check above decides
which lines are prose by tracking fences and treats a backtick run and a tilde
run as the same thing. A document that opens with one and closes with the
other, or that never closes, moves that boundary for every line after it, and
the path check then reads the wrong half of the document without saying so. A
gate that goes quiet is worse than one that goes red.

Every heading is written with hashes rather than as an underline, without a
trailing hash run, and no more than one level below the heading above it. The
first heading in a document sets the baseline rather than having to be a level
one, because `.github/pull_request_template.md` has no title of its own and
opens at a level two deliberately.

A heading has a blank line before and after it. Without the one before, a
heading glued to the paragraph above is not a heading at all in a strict
reader.

An unordered list item is written with a hyphen.

What it does not read is the same boundary the path check draws. A fenced block
and an indented block are passed over, because a document showing markdown is
showing it rather than writing it. A hash run that is not followed by a space
is not a heading, which is not pedantry here: this repository's documents open
sentences with an issue reference, and `#52 has to propagate a revocation` is
prose to every renderer and would be a finding under a looser rule.

There is no writing mode. A heading level skip has more than one legal repair,
and a tool that picked one would be editing the structure of a document rather
than its bytes.

It is a floor from here rather than a repair: every document in the tree
already held all seven when it landed, so what the rules are proven by is
`internal/mdlint`'s own fixtures rather than by anything they found.

## The default suite

The check is published as `Tests`. One command, and it is the command that
check runs, character for character:

    go test -race -covermode=atomic -coverprofile=coverage.out ./...

The race detector is on in that command because it is on in the check. A suite
that passes locally without it and goes red here is a suite run two ways, and
the way nobody runs locally is the one that finds the defect late. It comes with
the toolchain and needs no installation step.

`-covermode=atomic` belongs with `-race` rather than being a separate opinion:
the default counter is not safe to increment from two goroutines, and the
detector is watching.

A test file sits in the same directory as the code it tests, named for that file
with a `_test.go` suffix.

The command above prints coverage per package. The number over the whole module
comes from the profile it wrote:

    go run ./cmd/coverfloor -profile coverage.out -floor coverage-floor.txt

That command is the second half of the check, and it refuses a run that measured
less than the number in `coverage-floor.txt`. The number is in the tree rather
than in the workflow so that lowering it is a diff with a commit message
attached.

**The floor measures reach and not quality.** A statement counts as covered the
moment any test executes it, whether or not anything was asserted about what it
did, so the number can be raised by a test that checks nothing. It is here for
one narrower thing: it makes a package that arrived with no tests visible on the
day it lands rather than a year later. #109 is where the measurement gets teeth,
by asking whether a test notices when the statement it reaches is changed. Do
not read this number as evidence of a well tested tree, and do not quote it as
one.

`coverage.out` is not tracked. `.gitignore` keeps it out, because a profile is a
fact about one run on one machine.

## Tests run headless, unelevated and offline

Every test in the default suite runs with no display server, no administrative
rights, no accelerator and no outbound network, and a test that needs any of
those is marked and excluded from the default run by configuration rather than
by a flag anybody has to remember.

What is marked is printed rather than listed here:

    go test -tags needsreal -list '.*' ./test/...

The marking is a build constraint, `//go:build needsreal`, and the marked files
live under `test/`. Both halves matter: the constraint is what excludes them
from `go test ./...` without anybody remembering a flag, and the directory is
what makes the command above a complete list rather than a partial one. A marked
file anywhere else would be excluded from the default run and absent from the
listing, and `internal/testreach` refuses that.

The condition binds the run and not the job. The check has a route out while it
checks the tree out and installs the toolchain, and that route is taken away
from the user the suite runs as before the suite starts. The step that proves it
is gone runs first, so a rule that failed to install reddens the check instead of
producing a suite that had a route and a log saying it had none.

Inside the process, `internal/testreach` reads the test files and refuses a test
in the default run that dials an address which is not a loopback one, resolves a
name, or opens a device. It runs as part of the default suite, so the refusal
arrives with a file and a line rather than as a connection error somebody reads
as a defect in the code under test.

Where a test dials a loopback address the check cannot read, because the address
came from a listener it started a moment earlier, the reason goes on the same
line, in the shape the analyser suppressions use:

    resp, err := http.Get("http://" + addr + "/livez") // loopback: the address this case's own child process printed

That reason excuses only an address the check could not read. An address written
out in the source and pointing off this machine is refused whatever comment sits
beside it.

What this does not reach: a test that calls a helper which dials, a dial through
an interface value, and a reason comment that is simply wrong. It reads direct
calls through a package selector and nothing else. It is a floor under the
condition rather than a proof of it, and #114 is where the condition is
re-proved later from a run rather than from source.

The default gate does not run `test/needs-real-hardware-or-services/`, because
a check that needs a model, an identity provider or a source system is a check
that goes red for a reason no contributor's change caused. The readme in that
directory says what each suite needs, how to ask for it and how to read a run
that turned one away.

## Sign your work

Every commit carries a `Signed-off-by` trailer naming its author, and the check
published as `DCO sign-off` refuses a pull request where one does not. What
that trailer certifies is in [DCO](DCO), which is the Developer Certificate of
Origin 1.1 verbatim. Read it once before you sign.

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

Documents in this repository are plain prose.

## Text is stored the same way by everyone

`.gitattributes` declares the treatment of every tracked path, starting with an
explicit default rather than an inherited one. Text is stored with LF and
decodes as UTF-8. Without that file the treatment comes from each clone's
`core.autocrlf`, which is a local setting no tree holds and no reviewer can see,
so two contributors store different bytes for the same edit.

The check published as `Reject nondeterministic text` reads the index rather
than the working tree, and refuses a text blob stored with CR bytes, a tracked
path no attribute line covers, and a text blob that is not valid UTF-8. It
reads the index because a checkout applies the conversion whose absence is the
thing being looked for. If it refuses a file you did not think you changed,
your clone predates the attributes file:

    git add --renormalize .

Do not give a text path a `-text` override to get past the check. That is the
one move it cannot see through, and it is the move that puts the bytes back.

## Fixtures whose bytes matter

Some tests are about bytes: a document with an unusual encoding, a filename from
a source system written on another operating system, a permission string that
has to survive a round trip exactly. Normalisation destroys the thing those
tests exist to prove, and it does it silently.

Two rules.

Encode such a fixture in source rather than committing a raw literal. A base64
string in a test file is ASCII, so every tool leaves it alone, and the test
decodes it back to the bytes the case is about. This is the first choice.

Where a fixture is too large to inline, it goes in `testdata/fixtures/`, which
`.gitattributes` declares binary. Nothing under that directory is converted in
either direction and its diff is not rendered as text. A fixture whose bytes are
not the thing under test does not belong there; put it beside the test that
reads it, where a reviewer can read the diff. `testdata/README.md` says the same
thing where somebody adding a file will be standing.
