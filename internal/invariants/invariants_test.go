package invariants_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iderex/kanzlei/internal/invariants"
)

// The rule set the cases below are read against. It is written here rather than
// read from invariants.txt on purpose: a case that judged against this
// repository's real declarations would prove the state of the tree on the day
// it ran and not the operator, which is the distinction the import rules make
// in their own suite.
const rules = `
invariant: no-route-outside-the-router
shape: call-only-in
names: Handle HandleFunc
paths: internal/server
reason: a route registered outside the routing table is an entry the table does not show

invariant: no-compiled-in-credential
shape: named-string-literal
names: password secret
reason: a credential written into the source ships in every binary

invariant: needsreal-case-states-its-requirement
shape: constrained-test-calls
constraint: needsreal
names: Start
reason: a case that does not reach its suite gate states no requirement

invariant: no-administrative-right-from-the-environment
shape: literal-argument
names: Getenv LookupEnv
words: admin superuser
reason: an environment variable granting an administrative right is a way in that no group mapping shows
`

func declared(t *testing.T) []invariants.Invariant {
	t.Helper()
	inv, err := invariants.Parse("invariants.txt", []byte(rules))
	if err != nil {
		t.Fatalf("parse the rule set: %v", err)
	}
	return inv
}

func check(t *testing.T, filename, src string) []invariants.Violation {
	t.Helper()
	found, err := invariants.Check(declared(t), filename, []byte(src))
	if err != nil {
		t.Fatalf("check %s: %v", filename, err)
	}
	return found
}

func onlyViolation(t *testing.T, found []invariants.Violation, want string) invariants.Violation {
	t.Helper()
	if len(found) != 1 {
		t.Fatalf("want exactly one violation of %s, got %v", want, found)
	}
	if found[0].Invariant != want {
		t.Fatalf("want %s, got %s", want, found[0].Invariant)
	}
	if strings.TrimSpace(found[0].Reason) == "" {
		t.Fatal("the violation carries no reason, so the report cannot say why")
	}
	return found[0]
}

// The near miss for the router invariant. It is the mistake somebody makes
// rather than an invented one: a package that has its own routes registers them
// on the mux it was handed, which reads as tidy and puts an entry into the
// service that the routing table does not list.
const routeInASecondPackage = `package api

import "net/http"

func Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /ask", ask)
}

func ask(http.ResponseWriter, *http.Request) {}
`

func TestARouteRegisteredOutsideTheRouterIsRefused(t *testing.T) {
	found := check(t, "internal/api/routes.go", routeInASecondPackage)
	v := onlyViolation(t, found, "no-route-outside-the-router")
	if v.Line != 6 {
		t.Fatalf("want the line the call is on, got %d", v.Line)
	}
}

// The same bytes inside the package the rule allows. This is what stops the
// invariant being a ban on a function name: the routing table is where routes
// are registered, and the rule is about everywhere else.
func TestTheSameRegistrationInsideTheRouterIsNotRefused(t *testing.T) {
	if found := check(t, "internal/server/routes.go", routeInASecondPackage); len(found) != 0 {
		t.Fatalf("want nothing refused inside the router, got %v", found)
	}
}

func TestAPackageBelowTheRouterIsAlsoAllowed(t *testing.T) {
	if found := check(t, "internal/server/middleware/routes.go", routeInASecondPackage); len(found) != 0 {
		t.Fatalf("want a package under the router allowed, got %v", found)
	}
}

// The near miss for the credential invariant, and it is one character from the
// version that passes: a bootstrap value left empty while the mechanism was
// being written, and filled in the day somebody wanted the first run to work
// without reading the guide.
const bootstrapWithAValue = `package auth

const bootstrapPassword = "changeme"
`

const bootstrapWithNone = `package auth

const bootstrapPassword = ""
`

func TestACompiledInCredentialIsRefused(t *testing.T) {
	found := check(t, "internal/auth/bootstrap.go", bootstrapWithAValue)
	onlyViolation(t, found, "no-compiled-in-credential")
}

func TestTheSameNameHoldingNothingIsNotRefused(t *testing.T) {
	if found := check(t, "internal/auth/bootstrap.go", bootstrapWithNone); len(found) != 0 {
		t.Fatalf("want an empty placeholder left alone, got %v", found)
	}
}

// The three other shapes the same mistake is written in. A field set on a
// struct, a value in a composite literal, and a plain assignment all reach the
// binary identically, and a rule that read only declarations would refuse the
// first spelling and admit the other three.
func TestTheOtherSpellingsOfTheSameMistakeAreRefused(t *testing.T) {
	for name, src := range map[string]string{
		"an assignment": `package auth

func configure(c *Config) { c.Password = "changeme" }
`,
		"a composite literal": `package auth

var c = Config{Password: "changeme"}
`,
		"a package variable": `package auth

var clientSecret = "s3cr3t"
`,
	} {
		t.Run(name, func(t *testing.T) {
			onlyViolation(t, check(t, "internal/auth/config.go", src), "no-compiled-in-credential")
		})
	}
}

// A name that carries none of the declared words is left alone whatever it
// holds. The check reads the name and never the value, and its own limit is
// written where the rule is rather than being discovered from a green run.
func TestAValueUnderAnotherNameIsNotRead(t *testing.T) {
	src := `package auth

var s = "changeme"
`
	if found := check(t, "internal/auth/config.go", src); len(found) != 0 {
		t.Fatalf("want a name outside the vocabulary left alone, got %v", found)
	}
}

// The near miss for the environment invariant. It is the first-run convenience
// rather than an invented mistake: the bootstrap needs an administrative group
// before a mapping exists, reading one from the environment makes a deployment
// work on the first try, and the variable then grants that right for as long as
// it is set. The name is one word from the version that passes, and the word is
// the whole rule.
const administrativeGroupFromTheEnvironment = `package auth

import "os"

func bootstrapGroup() string {
	if group := os.Getenv("KANZLEI_ADMIN_GROUP"); group != "" {
		return group
	}
	return ""
}
`

func TestAnAdministrativeRightReadFromTheEnvironmentIsRefused(t *testing.T) {
	found := check(t, "internal/auth/bootstrap.go", administrativeGroupFromTheEnvironment)
	v := onlyViolation(t, found, "no-administrative-right-from-the-environment")
	if v.Line != 6 {
		t.Fatalf("want the line the name is written on, got %d", v.Line)
	}
	if !strings.Contains(v.Detail, "KANZLEI_ADMIN_GROUP") {
		t.Fatalf("want the variable named, got %q", v.Detail)
	}
}

// The same call reading a variable that grants nothing. This is what keeps the
// invariant from being a ban on reading the environment at all: which variable
// is read is the rule, and where it is read is not.
func TestAnEnvironmentVariableThatGrantsNothingIsNotRefused(t *testing.T) {
	src := strings.Replace(administrativeGroupFromTheEnvironment, "KANZLEI_ADMIN_GROUP", "KANZLEI_ADDR", 1)
	if found := check(t, "internal/auth/bootstrap.go", src); len(found) != 0 {
		t.Fatalf("want an ordinary setting left alone, got %v", found)
	}
}

// The other spelling of the same lookup, and the one somebody reaches for when
// they want to know whether the variable was set rather than what it holds. The
// name sits in the same argument and the rule reads every argument, so neither
// spelling is the one that gets through.
func TestTheOtherSpellingOfTheSameLookupIsRefused(t *testing.T) {
	src := `package auth

import "os"

func bootstrapGroup() (string, bool) { return os.LookupEnv("KANZLEI_SUPERUSER") }
`
	onlyViolation(t, check(t, "internal/auth/bootstrap.go", src), "no-administrative-right-from-the-environment")
}

// The word is matched however the variable is spelled, because the two
// conventions a deployment writes it in are the same variable.
func TestTheWordIsFoundWhateverTheCase(t *testing.T) {
	src := `package auth

import "os"

var group = os.Getenv("kanzlei_Admin")
`
	onlyViolation(t, check(t, "internal/auth/config.go", src), "no-administrative-right-from-the-environment")
}

// The bound the record states, proved rather than asserted. The rule reads the
// literal in the call, so a name held in a constant and passed in reaches the
// same variable and is invisible here. A green run says nothing about that
// spelling, which is why the record says so where the rule is.
func TestANameHeldInAConstantIsNotRead(t *testing.T) {
	src := `package auth

import "os"

const adminGroupVariable = "KANZLEI_ADMIN_GROUP"

var group = os.Getenv(adminGroupVariable)
`
	if found := check(t, "internal/auth/config.go", src); len(found) != 0 {
		t.Fatalf("want the declared bound to hold, got %v", found)
	}
}

// A call under one of the declared names that is given no string literal at
// all. The harness in test/needs-real-hardware-or-services reads the
// environment this way, one name at a time out of a slice, and refusing it
// would be refusing the roster that reports what a run turned away.
func TestALookupWithNoLiteralIsNotRefused(t *testing.T) {
	src := `package needsreal

import "os"

func value(name string) string { return os.Getenv(name) }
`
	if found := check(t, "test/needs-real-hardware-or-services/harness.go", src); len(found) != 0 {
		t.Fatalf("want a lookup with no literal left alone, got %v", found)
	}
}

// The near miss for the constrained suite invariant: a second case added beside
// a first, copied from it, with the one line that reaches the gate dropped. It
// compiles, it runs when somebody asks for the tag, and it fails inside an
// assertion instead of refusing with the requirement named.
const caseWithoutItsGate = `//go:build needsreal

package needsreal

import "testing"

var suite = &Suite{Name: "a model runtime"}

func TestTheModelAnswers(t *testing.T) {
	suite.Start(t)
	answer(t)
}

func TestTheModelStreams(t *testing.T) {
	answer(t)
}

func answer(*testing.T) {}
`

func TestAConstrainedCaseThatReachesNoGateIsRefused(t *testing.T) {
	found := check(t, "test/needs-real-hardware-or-services/model_test.go", caseWithoutItsGate)
	v := onlyViolation(t, found, "needsreal-case-states-its-requirement")
	if !strings.Contains(v.Detail, "TestTheModelStreams") {
		t.Fatalf("want the case that went without its gate named, got %q", v.Detail)
	}
}

// The same file with the one line back. The invariant is about the case that
// went without the gate and not about the file it sits in.
func TestTheSameFileWithTheGateBackIsNotRefused(t *testing.T) {
	src := strings.Replace(caseWithoutItsGate,
		"func TestTheModelStreams(t *testing.T) {\n\tanswer(t)",
		"func TestTheModelStreams(t *testing.T) {\n\tsuite.Start(t)\n\tanswer(t)", 1)
	if found := check(t, "test/needs-real-hardware-or-services/model_test.go", src); len(found) != 0 {
		t.Fatalf("want nothing refused once the gate is reached, got %v", found)
	}
}

// TestMain is the file's entry point rather than a case. It is where the roster
// of suites nobody asked for is printed, so a rule that required it to reach a
// gate would refuse the one function that reports the suites that never did.
func TestTheEntryPointIsNotACase(t *testing.T) {
	src := `//go:build needsreal

package needsreal

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) { os.Exit(m.Run()) }
`
	if found := check(t, "test/needs-real-hardware-or-services/main_test.go", src); len(found) != 0 {
		t.Fatalf("want the entry point left alone, got %v", found)
	}
}

// The constraint is parsed rather than matched as text, so a tag that is one
// term of a longer expression is found.
func TestACaseUnderALongerConstraintIsStillRead(t *testing.T) {
	src := `//go:build needsreal && !windows

package needsreal

import "testing"

func TestTheModelAnswers(t *testing.T) {}
`
	onlyViolation(t, check(t, "test/needs-real-hardware-or-services/model_test.go", src), "needsreal-case-states-its-requirement")
}

// A case in an unconstrained file is the default suite and is none of this
// rule's business. Without this the invariant would refuse every test in the
// repository.
func TestAnUnconstrainedCaseIsNotRead(t *testing.T) {
	src := `package server

import "testing"

func TestTheRouteAnswers(t *testing.T) {}
`
	if found := check(t, "internal/server/server_test.go", src); len(found) != 0 {
		t.Fatalf("want the default suite left alone, got %v", found)
	}
}

// The two constraints that are the reason the expression is evaluated rather
// than matched as text.
//
// A file constrained out of the needsreal build runs in the default suite,
// where the gate this rule requires does not exist, so reading it would refuse
// a case for not calling something that is not there. A file carrying a
// platform is in the default build on that platform and is the same case one
// step further out.
func TestAFileConstrainedOutOfThatBuildIsNotRead(t *testing.T) {
	for name, tag := range map[string]string{
		"the tag negated": "!needsreal",
		"a platform":      "!windows",
		"another tag":     "scanfixture",
	} {
		t.Run(name, func(t *testing.T) {
			src := "//go:build " + tag + `

package needsreal

import "testing"

func TestTheModelAnswers(t *testing.T) {}
`
			if found := check(t, "test/needs-real-hardware-or-services/model_test.go", src); len(found) != 0 {
				t.Fatalf("want a file outside that build left alone, got %v", found)
			}
		})
	}
}

// A function carrying the name and not the parameter is not a case the test
// binary would run, whichever of the two it is.
func TestAFunctionThatIsNotACaseIsNotRead(t *testing.T) {
	src := `//go:build needsreal

package needsreal

import "testing"

func Testing(t *testing.T) {}

func TestingTheModel(t *testing.T) {}

func TestTheModelUnderLoad(b *testing.B) {}

func TestTheModelWithTwo(t *testing.T, name string) {}
`
	if found := check(t, "test/needs-real-hardware-or-services/model_test.go", src); len(found) != 0 {
		t.Fatalf("want a function that is not a case left alone, got %v", found)
	}
}

// What the credential rule does with a value that is not a string with
// something in it. A number under one of the declared names is not a
// credential, and a literal with nothing between its quotes is a placeholder
// however it is written.
func TestAValueThisCannotReadAsAStringIsNotJudged(t *testing.T) {
	for name, src := range map[string]string{
		"a number": `package auth

const passwordAttempts = 3
`,
		"an empty raw literal": "package auth\n\nconst password = ``\n",
	} {
		t.Run(name, func(t *testing.T) {
			if found := check(t, "internal/auth/config.go", src); len(found) != 0 {
				t.Fatalf("want it left alone, got %v", found)
			}
		})
	}
}

// What the declaration file refuses when it is read, before any tree is looked
// at. A record that looks like a rule and is read by nothing is worse than an
// absent one, which is the argument .editorconfig already carries here for a
// property nothing implements.
func TestTheDeclarationFileRefusesWhatNoOperatorImplements(t *testing.T) {
	for name, src := range map[string]string{
		"a shape nothing implements": `invariant: no-panic-in-a-handler
shape: no-panics
reason: a panic in a handler is an outage
`,
		"a key nothing reads": `invariant: no-route-outside-the-router
shape: call-only-in
names: Handle
paths: internal/server
severity: high
reason: a route outside the table is an entry it does not show
`,
		"no reason": `invariant: no-route-outside-the-router
shape: call-only-in
names: Handle
paths: internal/server
`,
		"no shape": `invariant: no-route-outside-the-router
names: Handle
reason: a route outside the table is an entry it does not show
`,
		"a call with nowhere it is allowed": `invariant: no-route-outside-the-router
shape: call-only-in
names: Handle
reason: a route outside the table is an entry it does not show
`,
		"a credential rule naming no identifier": `invariant: no-compiled-in-credential
shape: named-string-literal
reason: a credential written into the source ships in every binary
`,
		"a constrained rule naming no constraint": `invariant: needsreal-case-states-its-requirement
shape: constrained-test-calls
names: Start
reason: a case that does not reach its gate states no requirement
`,
		"an argument rule naming no call": `invariant: no-administrative-right-from-the-environment
shape: literal-argument
words: admin
reason: an environment variable granting an administrative right is a way in
`,
		"an argument rule naming no word": `invariant: no-administrative-right-from-the-environment
shape: literal-argument
names: Getenv
reason: an environment variable granting an administrative right is a way in
`,
		"a name declared twice": `invariant: no-route-outside-the-router
shape: call-only-in
names: Handle
paths: internal/server
reason: a route outside the table is an entry it does not show

invariant: no-route-outside-the-router
shape: call-only-in
names: Handle
paths: cmd/kanzlei
reason: the same name a second time
`,
		"a value before any invariant": `shape: call-only-in
`,
		"a line that is not a key and a value": `invariant: no-route-outside-the-router
this is prose
`,
		"nothing at all": "# only a comment\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := invariants.Parse("invariants.txt", []byte(src)); err == nil {
				t.Fatal("want the declaration refused when it is read, got no error")
			}
		})
	}
}

// A tree that cannot be read is reported rather than passed over. A walk that
// stopped quietly would print the green line, and a green line here is read as
// the invariants holding across the tree.
func TestATreeThatCannotBeReadIsAnError(t *testing.T) {
	t.Run("a root that is not there", func(t *testing.T) {
		if _, _, err := invariants.CheckTree(declared(t), filepath.Join(t.TempDir(), "absent")); err == nil {
			t.Fatal("want a missing root reported, got no error")
		}
	})
	t.Run("a file that does not parse", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "broken.go"), []byte("package"), 0o600); err != nil {
			t.Fatalf("write the fixture: %v", err)
		}
		if _, _, err := invariants.CheckTree(declared(t), dir); err == nil {
			t.Fatal("want a file that does not parse reported, got no error")
		}
	})
}

func TestSourceThatDoesNotParseIsAnError(t *testing.T) {
	if _, err := invariants.Check(declared(t), "internal/auth/broken.go", []byte("package")); err == nil {
		t.Fatal("want a parse failure reported, got no error")
	}
}

func TestAViolationPrintsItsPositionAndItsInvariant(t *testing.T) {
	v := check(t, "internal/api/routes.go", routeInASecondPackage)[0]
	line := v.String()
	for _, want := range []string{"internal/api/routes.go", ":6:", "no-route-outside-the-router"} {
		if !strings.Contains(line, want) {
			t.Fatalf("want %q in %q", want, line)
		}
	}
}
