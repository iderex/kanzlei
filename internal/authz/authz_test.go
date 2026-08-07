package authz

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

// alice is the principal most cases below ask about. She is in one group, and
// the group is named for what it is rather than for a role, because a role name
// in a fixture invites a reader to assume the evaluator knows what a role is.
var alice = Principal{Subject: "sub-alice", Groups: []string{"legal-team"}}

func user(value string) Term  { return Term{Type: TermUser, Value: value} }
func group(value string) Term { return Term{Type: TermGroup, Value: value} }

func allowEntry(t Term) Entry { return Entry{Effect: Allow, Term: t} }
func denyEntry(t Term) Entry  { return Entry{Effect: Deny, Term: t} }

// TestDenyBeatsAllowWhateverTheOrder holds the first of the three rules in
// docs/permissions.md.
//
// Each case is stated twice, with the entries reversed, because an evaluator
// that returns the first match resolves one order correctly and the other one
// wrongly, and a table that only ever writes deny first would pass for that
// evaluator.
func TestDenyBeatsAllowWhateverTheOrder(t *testing.T) {
	cases := []struct {
		name string
		set  Set
	}{
		{
			name: "the deny naming her is written first",
			set:  Set{denyEntry(user("sub-alice")), allowEntry(group("legal-team"))},
		},
		{
			name: "the deny naming her is written last",
			set:  Set{allowEntry(group("legal-team")), denyEntry(user("sub-alice"))},
		},
		{
			name: "the deny is on the group and the allow on her",
			set:  Set{denyEntry(group("legal-team")), allowEntry(user("sub-alice"))},
		},
		{
			name: "the same, reversed",
			set:  Set{allowEntry(user("sub-alice")), denyEntry(group("legal-team"))},
		},
		{
			name: "two allows around one deny",
			set: Set{
				allowEntry(user("sub-alice")),
				denyEntry(group("legal-team")),
				allowEntry(group("legal-team")),
			},
		},
		{
			name: "the deny is buried under allows that do not name her",
			set: Set{
				allowEntry(user("sub-bob")),
				allowEntry(group("finance")),
				allowEntry(user("sub-alice")),
				denyEntry(user("sub-alice")),
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Evaluate(alice, c.set)
			if got.Allowed {
				t.Fatalf("a deny entry matching the principal did not win: %+v", got)
			}
			if got.Reason != ReasonDeniedByEntry {
				t.Fatalf("reason = %q, want %q", got.Reason, ReasonDeniedByEntry)
			}
		})
	}
}

// TestAnAllowWithNothingDenyingIsTheOnlyWayThrough is the other half of the
// first rule. Without it the table above passes for an evaluator that refuses
// everything, which proves nothing about deny winning.
func TestAnAllowWithNothingDenyingIsTheOnlyWayThrough(t *testing.T) {
	cases := []struct {
		name string
		set  Set
	}{
		{name: "named directly", set: Set{allowEntry(user("sub-alice"))}},
		{name: "named by group", set: Set{allowEntry(group("legal-team"))}},
		{
			name: "denied entries that name somebody else",
			set: Set{
				denyEntry(user("sub-bob")),
				denyEntry(group("finance")),
				allowEntry(group("legal-team")),
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Evaluate(alice, c.set)
			if !got.Allowed {
				t.Fatalf("an allow with nothing denying was refused: %+v", got)
			}
			if got.Reason != ReasonAllowedByEntry {
				t.Fatalf("reason = %q, want %q", got.Reason, ReasonAllowedByEntry)
			}
		})
	}
}

// TestMatchingNothingIsDenied holds the second of the three rules.
func TestMatchingNothingIsDenied(t *testing.T) {
	set := Set{
		allowEntry(user("sub-bob")),
		allowEntry(group("finance")),
		denyEntry(user("sub-carol")),
	}

	got := Evaluate(alice, set)
	if got.Allowed {
		t.Fatalf("a principal the set never names was allowed: %+v", got)
	}
	if got.Reason != ReasonMatchedNothing {
		t.Fatalf("reason = %q, want %q", got.Reason, ReasonMatchedNothing)
	}
}

// TestAnEmptySetDeniesEveryone is the second rule at its edge, and it is the
// case an evaluator written around "is there a deny" gets wrong.
//
// The last principal in the table is the one the issue asks about. It carries
// every group a deployment is likely to call privileged, and it is refused for
// the same reason as the empty principal beside it: this package has no notion
// of a privileged caller at all. TestNoIdentifierNamesAPrivilegedRoute is what
// holds that absence against a later edit, and docs/permissions.md says where
// such a route would have to live instead.
func TestAnEmptySetDeniesEveryone(t *testing.T) {
	cases := []struct {
		name      string
		principal Principal
	}{
		{name: "an ordinary user", principal: alice},
		{name: "a principal with nothing resolved", principal: Principal{}},
		{
			name: "a principal carrying every privileged-looking group",
			principal: Principal{
				Subject: "sub-operator",
				Groups: []string{
					"domain-admins", "superusers", "wheel", "sudo",
					"kanzlei-operators", "everyone",
				},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Evaluate(c.principal, Set{})
			if got.Allowed {
				t.Fatalf("an empty permission set allowed somebody: %+v", got)
			}
			if got.Reason != ReasonEmptySet {
				t.Fatalf("reason = %q, want %q", got.Reason, ReasonEmptySet)
			}
		})
	}
}

// TestAnUnrecognisedTermDenies holds the third rule.
func TestAnUnrecognisedTermDenies(t *testing.T) {
	cases := []struct {
		name string
		set  Set
	}{
		{
			name: "a term type from a connector written later",
			set:  Set{Entry{Effect: Allow, Term: Term{Type: "device-posture", Value: "managed"}}},
		},
		{
			name: "an empty term type",
			set:  Set{Entry{Effect: Allow, Term: Term{Type: "", Value: "anything"}}},
		},
		{
			name: "a plausible near-spelling of a type that does exist",
			set:  Set{Entry{Effect: Allow, Term: Term{Type: "users", Value: "sub-alice"}}},
		},
		{
			name: "unrecognised alongside an allow that does match",
			set: Set{
				allowEntry(user("sub-alice")),
				Entry{Effect: Allow, Term: Term{Type: "device-posture", Value: "managed"}},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Evaluate(alice, c.set)
			if got.Allowed {
				t.Fatalf("an entry the evaluator cannot read was skipped rather than refused: %+v", got)
			}
			if got.Reason != ReasonUnrecognisedTerm {
				t.Fatalf("reason = %q, want %q", got.Reason, ReasonUnrecognisedTerm)
			}
		})
	}
}

// TestAnUnrecognisedEffectDenies is the same rule on the other field of an
// entry. An effect that is neither allow nor deny is as unreadable as a term
// type nobody declared, and an evaluator that tests only for Deny treats it as
// an allow.
func TestAnUnrecognisedEffectDenies(t *testing.T) {
	set := Set{Entry{Effect: "audit-only", Term: user("sub-alice")}}

	got := Evaluate(alice, set)
	if got.Allowed {
		t.Fatalf("an entry with an unreadable effect was allowed: %+v", got)
	}
	if got.Reason != ReasonUnrecognisedEffect {
		t.Fatalf("reason = %q, want %q", got.Reason, ReasonUnrecognisedEffect)
	}
}

// TestTheNearMissOnRecognition is the fixture the issue asks for, and it is the
// one that proves the third rule is load bearing rather than incidentally
// satisfied.
//
// Both sets hold a deny entry that names something the principal is not, beside
// an allow that names her directly. They differ in exactly one field: the term
// type of that deny entry. In the first it is a type this evaluator has never
// heard of, in the second it is a recognised type whose value matches nobody in
// this session.
//
// The deny is therefore inert in both readings. What decides the answer is
// whether the evaluator was willing to read past an entry it could not parse,
// and the two results have to differ or the rule does nothing.
func TestTheNearMissOnRecognition(t *testing.T) {
	unrecognised := Set{
		Entry{Effect: Deny, Term: Term{Type: "device-posture", Value: "unmanaged"}},
		allowEntry(user("sub-alice")),
	}
	recognised := Set{
		Entry{Effect: Deny, Term: Term{Type: TermGroup, Value: "unmanaged"}},
		allowEntry(user("sub-alice")),
	}

	if got := Evaluate(alice, unrecognised); got.Allowed {
		t.Fatalf("an unreadable deny entry was skipped and the allow beside it won: %+v", got)
	} else if got.Reason != ReasonUnrecognisedTerm {
		t.Fatalf("reason = %q, want %q", got.Reason, ReasonUnrecognisedTerm)
	}

	if got := Evaluate(alice, recognised); !got.Allowed {
		t.Fatalf("with the term type recognised and matching nobody, the allow should win: %+v", got)
	}
}

// TestTheZeroDecisionRefuses holds the shape of the answer rather than the
// evaluation. A Decision that arrived from a path nobody wrote has to refuse.
func TestTheZeroDecisionRefuses(t *testing.T) {
	var d Decision
	if d.Allowed {
		t.Fatal("the zero Decision permits; it has to refuse")
	}
}

// TestEveryRecognisedTermTypeHasAMatcher is the check that the constants and
// the table cannot drift apart. A type declared here and absent from matchers
// would be unrecognised, so every set carrying it would be refused, which is
// safe but is a silent outage rather than a decision.
func TestEveryRecognisedTermTypeHasAMatcher(t *testing.T) {
	for _, declared := range []TermType{TermUser, TermGroup} {
		if _, ok := matchers[declared]; !ok {
			t.Fatalf("term type %q is declared as a constant and has no matcher", declared)
		}
	}
}

// TestAMatcherNeverMatchesAnEmptyValue covers the entry that names nothing. An
// allow whose value is empty must not match a principal whose subject is also
// empty, which is the shape a half-populated record arrives in.
func TestAMatcherNeverMatchesAnEmptyValue(t *testing.T) {
	empty := Principal{}

	for _, term := range []Term{{Type: TermUser}, {Type: TermGroup}} {
		if matchers[term.Type](empty, term.Value) {
			t.Fatalf("term %v matched a principal with nothing resolved", term)
		}
	}

	got := Evaluate(empty, Set{allowEntry(Term{Type: TermUser})})
	if got.Allowed {
		t.Fatalf("an entry naming nobody allowed a principal who is nobody: %+v", got)
	}
}

// --- what the source is allowed to look like ---
//
// The three tests below read this package's own source rather than running it.
// They hold properties a behavioural test cannot reach: that no signature here
// can hand back an allow beside an error, that there is one place a permitting
// Decision is built, and that no identifier names a privileged route around the
// entries. Each one is proved to bite by a fixture that it refuses.

// sourceViolation is one thing found in a parsed file that this package's own
// rules refuse.
type sourceViolation struct {
	Line int
	What string
}

const (
	violationErrorBesideDecision  = "a function returning a Decision also returns an error"
	violationDecisionNotBuiltHere = "a Decision is returned without going through allow or deny"
	violationPermitOutsideAllow   = "a permitting Decision is built outside allow"
)

// constructors are the two functions permitted to build a Decision from a
// literal. Every other function returning one returns through a call to these.
var constructors = map[string]bool{"allow": true, "deny": true}

// inspectSource reports what src does that this package's rules refuse.
//
// filename is used for the reported position and for nothing else, so the
// fixtures below stay in source rather than being written to disk. That is the
// same shape internal/sourcecheck uses and for the same reason.
func inspectSource(t *testing.T, filename string, src []byte) []sourceViolation {
	t.Helper()

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, src, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse %s: %v", filename, err)
	}

	var found []sourceViolation
	report := func(pos token.Pos, what string) {
		found = append(found, sourceViolation{Line: fset.Position(pos).Line, What: what})
	}

	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}

		returnsDecision, returnsError := false, false
		if fn.Type.Results != nil {
			for _, result := range fn.Type.Results.List {
				switch name := typeName(result.Type); name {
				case "Decision":
					returnsDecision = true
				case "error":
					returnsError = true
				}
			}
		}
		if returnsDecision && returnsError {
			report(fn.Pos(), violationErrorBesideDecision)
		}

		ast.Inspect(fn.Body, func(node ast.Node) bool {
			if literal, ok := node.(*ast.CompositeLit); ok {
				if typeName(literal.Type) == "Decision" && permits(literal) && fn.Name.Name != "allow" {
					report(literal.Pos(), violationPermitOutsideAllow)
				}
			}
			if returnsDecision && !constructors[fn.Name.Name] {
				if ret, ok := node.(*ast.ReturnStmt); ok && !isConstructorCall(ret) {
					report(ret.Pos(), violationDecisionNotBuiltHere)
				}
			}
			return true
		})
	}
	return found
}

// typeName is the identifier of a type expression, for the simple forms a
// signature in this package uses. Anything else answers with the empty string,
// which matches nothing the rules are about.
func typeName(expr ast.Expr) string {
	if ident, ok := expr.(*ast.Ident); ok {
		return ident.Name
	}
	return ""
}

// permits reports whether a Decision literal sets Allowed to anything other
// than the literal false. An expression this cannot read counts as permitting,
// because a rule that fails open on a shape it does not understand is the same
// defect this package is about.
func permits(literal *ast.CompositeLit) bool {
	for i, element := range literal.Elts {
		value := element
		if keyed, ok := element.(*ast.KeyValueExpr); ok {
			if typeName(keyed.Key) != "Allowed" {
				continue
			}
			value = keyed.Value
		} else if i != 0 {
			// Positional, and Allowed is the first field of Decision.
			continue
		}
		return typeName(value) != "false"
	}
	// No Allowed field at all is the zero value, which refuses.
	return false
}

// isConstructorCall reports whether a return statement hands back exactly one
// value and that value is a call to allow or to deny.
func isConstructorCall(ret *ast.ReturnStmt) bool {
	if len(ret.Results) != 1 {
		return false
	}
	call, ok := ret.Results[0].(*ast.CallExpr)
	if !ok {
		return false
	}
	return constructors[typeName(call.Fun)]
}

// thisPackagesSource is the file the three source tests judge. It is read from
// disk rather than embedded so that the test judges what is committed.
func thisPackagesSource(t *testing.T) []byte {
	t.Helper()
	src, err := os.ReadFile("authz.go")
	if err != nil {
		t.Fatalf("read authz.go: %v", err)
	}
	return src
}

// TestNoDecisionIsReturnedBesideAnError is the walk the issue asks for: every
// return of the evaluator, read from the source rather than exercised.
//
// It holds three things at once. No signature here returns a Decision beside an
// error, so there is no call site that can read an allow and drop the reason it
// was not one. No function other than the two constructors builds a Decision
// from a literal. And a permitting literal appears in exactly one function.
func TestNoDecisionIsReturnedBesideAnError(t *testing.T) {
	found := inspectSource(t, "authz.go", thisPackagesSource(t))
	for _, violation := range found {
		t.Errorf("authz.go:%d: %s", violation.Line, violation.What)
	}
}

// TestTheSourceRulesRefuseWhatTheyName is the proof that the test above bites.
// Each fixture is this package's shape with one thing changed, and each one is
// a mistake somebody makes rather than a mistake nobody would.
func TestTheSourceRulesRefuseWhatTheyName(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "the evaluator grows an error result",
			src: `package authz
func Evaluate(p Principal, set Set) (Decision, error) {
	return allow(""), nil
}`,
			want: violationErrorBesideDecision,
		},
		{
			name: "a permitting Decision is built inline",
			src: `package authz
func Evaluate(p Principal, set Set) Decision {
	return Decision{Allowed: true}
}`,
			want: violationPermitOutsideAllow,
		},
		{
			name: "a permitting Decision is built inline, positionally",
			src: `package authz
func Evaluate(p Principal, set Set) Decision {
	return Decision{true, ReasonAllowedByEntry, ""}
}`,
			want: violationPermitOutsideAllow,
		},
		{
			name: "a Decision is returned from a variable the reader has to chase",
			src: `package authz
func Evaluate(p Principal, set Set) Decision {
	d := deny(ReasonEmptySet, "")
	return d
}`,
			want: violationDecisionNotBuiltHere,
		},
		{
			name: "Allowed is set from something the rule cannot read",
			src: `package authz
func Evaluate(p Principal, set Set) Decision {
	return Decision{Allowed: everythingIsFine}
}`,
			want: violationPermitOutsideAllow,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			found := inspectSource(t, "fixture.go", []byte(c.src))
			if len(found) == 0 {
				t.Fatalf("the fixture was accepted; %s is not refused", c.want)
			}
			var saw bool
			for _, violation := range found {
				if violation.What == c.want {
					saw = true
				}
			}
			if !saw {
				t.Fatalf("found %v, none of them %q", found, c.want)
			}
		})
	}
}

// TestTheSourceRulesAcceptThisPackagesShape is the other half of the proof. A
// rule that refuses everything refuses the fixtures above for a reason that has
// nothing to do with what it names.
func TestTheSourceRulesAcceptThisPackagesShape(t *testing.T) {
	src := `package authz
func Evaluate(p Principal, set Set) Decision {
	if len(set) == 0 {
		return deny(ReasonEmptySet, "")
	}
	return allow("")
}

func allow(detail string) Decision {
	return Decision{Allowed: true, Reason: ReasonAllowedByEntry, Detail: detail}
}

func deny(reason Reason, detail string) Decision {
	return Decision{Allowed: false, Reason: reason, Detail: detail}
}`

	if found := inspectSource(t, "fixture.go", []byte(src)); len(found) != 0 {
		t.Fatalf("the shape this package actually has was refused: %v", found)
	}
}

// privilegedWords are the names a route around the entries arrives under. The
// list is what has to be kept current and nothing derives it, which is this
// check's known limit: a bypass named something else walks past.
var privilegedWords = []string{"admin", "superuser", "sudo", "bypass", "override", "godmode"}

// TestNoIdentifierNamesAPrivilegedRoute holds the second half of the empty-set
// rule. An empty permission set denying everyone is only meaningful while there
// is no other door, and the way a door gets added is that somebody writes a
// field or a branch named for the person they want to let through.
//
// It reads identifiers and not comments deliberately, so that the absence can
// still be explained in prose in the file it is an absence in.
func TestNoIdentifierNamesAPrivilegedRoute(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "authz.go", thisPackagesSource(t), parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse authz.go: %v", err)
	}

	ast.Inspect(file, func(node ast.Node) bool {
		ident, ok := node.(*ast.Ident)
		if !ok {
			return true
		}
		lowered := strings.ToLower(ident.Name)
		for _, word := range privilegedWords {
			if strings.Contains(lowered, word) {
				t.Errorf("authz.go:%d: identifier %q names a privileged route; there is no such route in this package and adding one is a separate issue with its own audit record",
					fset.Position(ident.Pos()).Line, ident.Name)
			}
		}
		return true
	})
}
