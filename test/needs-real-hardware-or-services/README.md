# needs-real-hardware-or-services

The suites that cannot be proved without the real thing: a model runtime with
weights loaded on real hardware, an identity provider issuing real tokens, a
source system with real permissions on real files.

The directory is named after what it needs rather than after what it does. A
directory called `integration` that half the contributors cannot run reads as
coverage this project has, and it is not. What is in here is coverage that
exists on a machine that has the thing, and nowhere else.

## What it proves, and what it does not

It proves that this project works against a real one of something. It proves
nothing about the parts of the project that are exercised by the default suite,
and it is not a second place to put a test that could have run there. A case
that can be written against a fake belongs beside the code, not here.

A run of this harness is evidence about the machine it ran on. It says nothing
about a machine that has a different provider, a different model or a different
source system.

## Running it

    go test -tags needsreal ./test/...

The build constraint is what keeps it out of the default run: `go test ./...`
compiles this package, finds no test files in it, and says so. Nothing about the
default run has to remember to exclude anything.

## What has to be present

Per suite, because that is the only honest granularity. A suite declares its own
requirements and turns its cases away when one is missing, naming the missing
one.

| Suite | What it needs |
| --- | --- |
| `harness` | Nothing. It exercises the precondition mechanism and the roster that the rest of this directory is built on. |

The table has one row because the harness has one suite. #35 attaches the
sign-on flow against a real provider, #59 the source system with real
permissions, and #77 the model runtime on real hardware, and each adds its row
here when it does.

## Reading a run

The last thing a run prints is the roster: every suite the binary knows about
and what happened to it.

    needs-real-hardware-or-services: 1 suite(s)
      harness: ran
    A suite reported as not asked for proved nothing in this run.

Three outcomes. `ran` means the suite's requirements were met and its cases were
selected. `refused` means a case was selected and the suite turned it away,
with the missing requirement printed underneath. `not asked for` means the run
selected nothing from that suite, which is what `-run` produces and what a
partial run looks like.

The roster is printed because a skip is quiet. A run where every suite was
refused exits zero and prints `ok`, exactly like a run where every suite passed,
and the roster is the only thing between those two readings.

## Adding a suite

Declare it at package level with what it needs, register it, and call `Start` at
the top of every case:

    var identity = &Suite{
        Name:  "identity-provider",
        Why:   "a token this project did not mint",
        Needs: []Requirement{EnvSet("KANZLEI_TEST_OIDC_ISSUER", "the issuer URL of a running provider")},
    }

    func init() { Register(identity) }

    func TestSomethingAgainstARealProvider(t *testing.T) {
        identity.Start(t)
        ...
    }

`Start` is what makes the refusal happen before the case body rather than
somewhere inside an assertion. A case that skips the call fails on a connection
error instead, and whoever reads that failure starts by suspecting the code
under test.

Register in `init` rather than lazily. A suite that registers itself the first
time one of its cases runs is invisible in the roster on exactly the run where
none of them did, which is the run the roster is for.
