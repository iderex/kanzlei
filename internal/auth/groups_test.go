package auth

import (
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"slices"
	"strings"
	"testing"
)

// policy is the deployment every case below is configured as, unless it says
// otherwise: groups arrive as names under a claim this deployment named, three
// of them map, and one of the mapped groups carries administrative rights.
func policy() GroupPolicy {
	return GroupPolicy{
		Claim: "kanzlei_groups",
		Shape: ShapeNames,
		Mapping: map[string]GroupName{
			"Legal Team":     "legal-team",
			"legal-team-old": "legal-team",
			"Operators":      "operators",
		},
		MaxGroups:      4,
		Administrative: []GroupName{"operators"},
	}
}

// countingSource is a provider that answers membership out of band, and counts
// how often it was asked.
type countingSource struct {
	groups []string
	err    error
	calls  int
}

func (s *countingSource) GroupsFor(context.Context, string) ([]string, error) {
	s.calls++
	return s.groups, s.err
}

// TestTheGroupClaimIsNamedRatherThanGuessed holds the first done-when line.
//
// The third case is the one that matters. A token carrying a claim called
// groups, which is the name a guessing implementation would find first, yields
// nothing here, because this deployment said its groups are somewhere else.
func TestTheGroupClaimIsNamedRatherThanGuessed(t *testing.T) {
	cases := []struct {
		name  string
		token Token
		want  []string
	}{
		{
			name:  "the named claim, as a list of strings",
			token: Token{"kanzlei_groups": []string{"Legal Team"}},
			want:  []string{"Legal Team"},
		},
		{
			name:  "the named claim, as it arrives from a decoded token",
			token: Token{"kanzlei_groups": []any{"Legal Team", "Operators"}},
			want:  []string{"Legal Team", "Operators"},
		},
		{
			name:  "a claim called groups, which this deployment did not name",
			token: Token{"groups": []string{"Legal Team"}, "roles": []string{"Operators"}},
			want:  nil,
		},
		{
			name:  "no group claim at all, which is a user in no groups",
			token: Token{"sub": "sub-alice"},
			want:  nil,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := GroupValues(c.token, policy())
			if err != nil {
				t.Fatalf("GroupValues: %v", err)
			}
			if !slices.Equal(got, c.want) {
				t.Fatalf("GroupValues = %v, want %v", got, c.want)
			}
		})
	}
}

// TestAClaimInAShapeThisCannotReadIsRefused covers the provider that puts
// something else at the configured claim. Reading half of it would be a guess
// about what the other half meant.
func TestAClaimInAShapeThisCannotReadIsRefused(t *testing.T) {
	for _, held := range []any{"Legal Team", 42, map[string]any{"legal": true}, []any{"Legal Team", 7}} {
		if _, err := GroupValues(Token{"kanzlei_groups": held}, policy()); err == nil {
			t.Fatalf("a claim holding %T was accepted", held)
		}
	}
}

// TestAnUnmappedClaimValueFailsTheSignOn holds the third done-when line as far
// as this tree reaches. No principal is produced, and the error names the
// value so an operator can fix the configuration.
func TestAnUnmappedClaimValueFailsTheSignOn(t *testing.T) {
	_, err := ResolveGroups([]string{"Legal Team", "Board of Directors"}, policy())
	if !errors.Is(err, ErrUnmappedClaim) {
		t.Fatalf("err = %v, want %v", err, ErrUnmappedClaim)
	}
	if !strings.Contains(err.Error(), "Board of Directors") {
		t.Fatalf("the error does not name the unmapped value: %v", err)
	}

	built, err := NewSession(context.Background(), Claims{Subject: "sub-alice"},
		Token{"kanzlei_groups": []string{"Board of Directors"}}, policy(), nil, Mapping{}, resolvedAt)
	if !errors.Is(err, ErrUnmappedClaim) {
		t.Fatalf("NewSession err = %v, want %v", err, ErrUnmappedClaim)
	}
	if built.Principal.Subject != "" {
		t.Fatalf("a principal was produced for an unmapped claim: %+v", built.Principal)
	}
}

// TestTooManyGroupsFailsRatherThanTruncates holds the fourth done-when line.
//
// The second half is the point. A truncated list is a permission change that
// nothing reports, and which groups survived depends on the order the provider
// sent them in, so the same user can get different access on two sign-ons.
func TestTooManyGroupsFailsRatherThanTruncates(t *testing.T) {
	tight := policy()
	tight.MaxGroups = 2

	raw := []string{"Legal Team", "Operators", "legal-team-old"}
	got, err := ResolveGroups(raw, tight)
	if !errors.Is(err, ErrTooManyGroups) {
		t.Fatalf("err = %v, want %v", err, ErrTooManyGroups)
	}
	if len(got) != 0 {
		t.Fatalf("groups = %v, want nothing; a truncated list is a silent permission change", got)
	}

	if _, err := ResolveGroups(raw[:2], tight); err != nil {
		t.Fatalf("a session exactly at the bound was refused: %v", err)
	}
}

// TestTheBoundIsCheckedBeforeTheMapping is why the two refusals cannot be
// confused. A session that is both too large and carries an unmapped value is
// reported as too large, because that is the one an operator fixes first.
func TestTheBoundIsCheckedBeforeTheMapping(t *testing.T) {
	tight := policy()
	tight.MaxGroups = 1

	_, err := ResolveGroups([]string{"Legal Team", "Board of Directors"}, tight)
	if !errors.Is(err, ErrTooManyGroups) {
		t.Fatalf("err = %v, want %v", err, ErrTooManyGroups)
	}
}

// TestTwoClaimValuesForOneGroupResolveToOne covers the ordinary configuration
// where a renamed group is carried under both names during a migration.
func TestTwoClaimValuesForOneGroupResolveToOne(t *testing.T) {
	got, err := ResolveGroups([]string{"legal-team-old", "Legal Team"}, policy())
	if err != nil {
		t.Fatalf("ResolveGroups: %v", err)
	}
	if !slices.Equal(got, []GroupName{"legal-team"}) {
		t.Fatalf("groups = %v, want one group named once", got)
	}
}

// TestTheThreeClaimShapesProduceTheSamePrincipal holds the sixth done-when
// line: the same person, delivered three different ways, is the same session.
func TestTheThreeClaimShapesProduceTheSamePrincipal(t *testing.T) {
	byName := policy()

	byIdentifier := GroupPolicy{
		Claim: "kanzlei_groups",
		Shape: ShapeIdentifiers,
		Mapping: map[string]GroupName{
			"7f1c-legal": "legal-team",
			"7f1c-ops":   "operators",
		},
		MaxGroups:      4,
		Administrative: []GroupName{"operators"},
	}

	byCall := GroupPolicy{
		Shape:          ShapeProviderCall,
		Mapping:        byName.Mapping,
		MaxGroups:      4,
		Administrative: []GroupName{"operators"},
	}
	source := &countingSource{groups: []string{"Legal Team", "Operators"}}

	claims := Claims{Subject: "sub-alice", DisplayName: "A. Muster"}
	mapping := mustMap(t, MappingEntry{Subject: "sub-alice", Source: fileServer, Value: "S-1-5-21-1004"})

	sessions := map[string]Session{}
	for name, built := range map[string]func() (Session, error){
		"names": func() (Session, error) {
			return NewSession(context.Background(), claims, Token{"kanzlei_groups": []string{"Legal Team", "Operators"}}, byName, nil, mapping, resolvedAt)
		},
		"identifiers": func() (Session, error) {
			return NewSession(context.Background(), claims, Token{"kanzlei_groups": []any{"7f1c-legal", "7f1c-ops"}}, byIdentifier, nil, mapping, resolvedAt)
		},
		"provider call": func() (Session, error) {
			return NewSession(context.Background(), claims, nil, byCall, source, mapping, resolvedAt)
		},
	} {
		built, err := built()
		if err != nil {
			t.Fatalf("%s: NewSession: %v", name, err)
		}
		sessions[name] = built
	}

	want := []GroupName{"legal-team", "operators"}
	for name, got := range sessions {
		if !slices.Equal(got.Groups, want) {
			t.Fatalf("%s: groups = %v, want %v", name, got.Groups, want)
		}
		if !slices.Equal(got.Principal.Groups, asStrings(want)) {
			t.Fatalf("%s: principal groups = %v, want %v", name, got.Principal.Groups, want)
		}
		if got.Principal.Subject != "sub-alice" {
			t.Fatalf("%s: subject = %q", name, got.Principal.Subject)
		}
		if !got.Administrative {
			t.Fatalf("%s: the operators group did not carry administrative rights", name)
		}
	}

	if source.calls != 1 {
		t.Fatalf("the group source was called %d times, want exactly once per session", source.calls)
	}
}

// TestTheProviderCallHappensOncePerSession is the second done-when line stated
// as the property it is. Building two sessions asks twice; a session already
// built never asks again, because there is no method here that would.
func TestTheProviderCallHappensOncePerSession(t *testing.T) {
	byCall := GroupPolicy{Shape: ShapeProviderCall, Mapping: policy().Mapping, MaxGroups: 4}
	source := &countingSource{groups: []string{"Legal Team"}}

	for range 2 {
		if _, err := NewSession(context.Background(), Claims{Subject: "sub-alice"}, nil, byCall, source, Mapping{}, resolvedAt); err != nil {
			t.Fatalf("NewSession: %v", err)
		}
	}
	if source.calls != 2 {
		t.Fatalf("two sessions made %d calls, want one each", source.calls)
	}
}

// TestAProviderCallThatFailsFailsTheSignOn covers the unreachable provider. A
// session with no groups because the call failed is a session that looks
// exactly like a user in no groups, and it must not be built.
func TestAProviderCallThatFailsFailsTheSignOn(t *testing.T) {
	byCall := GroupPolicy{Shape: ShapeProviderCall, Mapping: policy().Mapping, MaxGroups: 4}

	for _, source := range []GroupSource{
		&countingSource{err: errors.New("the provider is unreachable")},
		nil,
	} {
		if _, err := NewSession(context.Background(), Claims{Subject: "sub-alice"}, nil, byCall, source, Mapping{}, resolvedAt); err == nil {
			t.Fatal("a session was built although its groups could not be resolved")
		}
	}
}

// TestClaimsArrivingWithGroupsAreRefused closes the trap in NewSession's own
// signature: two sources for one fact, with whichever the code read last
// winning.
func TestClaimsArrivingWithGroupsAreRefused(t *testing.T) {
	_, err := NewSession(context.Background(), Claims{Subject: "sub-alice", Groups: []string{"operators"}},
		Token{"kanzlei_groups": []string{"Legal Team"}}, policy(), nil, Mapping{}, resolvedAt)
	if !errors.Is(err, ErrGroupsAlreadySet) {
		t.Fatalf("err = %v, want %v", err, ErrGroupsAlreadySet)
	}
}

// TestAdministrativeRightsComeFromAMappedGroup holds the fifth done-when line
// behaviourally.
//
// The second case is the attack: the user influences what the provider puts in
// the claim, and puts the name of the administrative group there directly.
// That value has no mapping, so it does not become a group, and the sign-on
// fails rather than quietly granting nothing or quietly granting everything.
func TestAdministrativeRightsComeFromAMappedGroup(t *testing.T) {
	build := func(t *testing.T, values []string) (Session, error) {
		t.Helper()
		return NewSession(context.Background(), Claims{Subject: "sub-alice"},
			Token{"kanzlei_groups": values}, policy(), nil, Mapping{}, resolvedAt)
	}

	granted, err := build(t, []string{"Operators"})
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	if !granted.Administrative {
		t.Fatal("a user in the mapped administrative group has no administrative rights")
	}

	if _, err := build(t, []string{"operators"}); !errors.Is(err, ErrUnmappedClaim) {
		t.Fatalf("a claim value spelled like the application group was err = %v, want %v", err, ErrUnmappedClaim)
	}

	ordinary, err := build(t, []string{"Legal Team"})
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	if ordinary.Administrative {
		t.Fatal("an ordinary user has administrative rights")
	}
}

// TestAnUnreachableAdministrativeGroupIsRefused covers the configuration that
// reads as a grant and is not one. An administrative group no mapping entry
// produces can never be held by anybody, and an operator reading the file
// would believe otherwise.
func TestAnUnreachableAdministrativeGroupIsRefused(t *testing.T) {
	unreachable := policy()
	unreachable.Administrative = []GroupName{"platform-admins"}

	if err := unreachable.Validate(); err == nil {
		t.Fatal("an administrative group nothing maps to was accepted")
	}
}

// TestAPolicyThatCouldNotBeAppliedIsRefused covers the rest of the
// configuration surface, including the bound nobody set.
func TestAPolicyThatCouldNotBeAppliedIsRefused(t *testing.T) {
	cases := map[string]GroupPolicy{
		"no claim named for a token shape":     {Shape: ShapeNames, MaxGroups: 4},
		"a claim named for the call shape":     {Shape: ShapeProviderCall, Claim: "groups", MaxGroups: 4},
		"a shape this application cannot read": {Shape: "nested", Claim: "groups", MaxGroups: 4},
		"no shape at all":                      {Claim: "groups", MaxGroups: 4},
		"no bound on how many groups":          {Shape: ShapeNames, Claim: "groups"},
		"a negative bound":                     {Shape: ShapeNames, Claim: "groups", MaxGroups: -1},
	}

	for name, bad := range cases {
		t.Run(name, func(t *testing.T) {
			if err := bad.Validate(); err == nil {
				t.Fatal("the policy was accepted")
			}
			if _, err := ResolveGroups(nil, bad); err == nil {
				t.Fatal("ResolveGroups accepted the policy")
			}
		})
	}
}

// --- what the source is allowed to look like ---

// commonClaimNames are the names a guessing implementation reaches for. A
// literal one of these in this file is either a guess or a default, and both
// are the first done-when line being quietly reversed.
var commonClaimNames = []string{"groups", "roles", "memberOf", "member_of", "wids"}

// TestNoClaimNameIsWrittenIntoTheSource holds the first done-when line as a
// refusal rather than as an absence.
//
// The failure it prevents is a default: somebody meets a deployment whose
// operator has not configured the claim, adds a fallback so the sign-on works,
// and every deployment that never configured one silently starts resolving
// whatever that claim happens to hold.
func TestNoClaimNameIsWrittenIntoTheSource(t *testing.T) {
	for _, literal := range literalsIn(t, "groups.go", sourceOf(t, "groups.go")) {
		for _, guess := range commonClaimNames {
			if strings.EqualFold(literal, guess) {
				t.Errorf("groups.go holds the literal %q; the claim carrying groups is named in configuration and nowhere else", literal)
			}
		}
	}
}

// forbiddenInTheRightDecision are the types and fields that carry something a
// user might have influenced. None of them may be reached from the function
// that decides an administrative right.
var forbiddenInTheRightDecision = []string{"Token", "Claims", "Claim"}

// TestNoAdministrativeRightIsReadFromAClaim holds the fifth done-when line as
// a refusal. The decision reads mapped application groups; a read of the token
// or of the claims inside it would be a right coming from something the user
// can influence.
func TestNoAdministrativeRightIsReadFromAClaim(t *testing.T) {
	for _, read := range readsInside(t, "groups.go", sourceOf(t, "groups.go"), "administrative", forbiddenInTheRightDecision) {
		t.Errorf("groups.go: %s; an administrative right comes from a mapped group and never from a claim", read)
	}
}

// TestTheSourceRulesRefuseWhatTheyName proves both of the tests above bite.
func TestTheSourceRulesRefuseWhatTheyName(t *testing.T) {
	guessing := `package auth
func GroupValues(token Token, policy GroupPolicy) ([]string, error) {
	claim := policy.Claim
	if claim == "" {
		claim = "groups"
	}
	return nil, nil
}`
	literals := literalsIn(t, "fixture.go", guessing)
	if len(literals) == 0 {
		t.Fatal("the fixture holds no string literals; the walk is reading nothing")
	}
	var sawGuess bool
	for _, literal := range literals {
		for _, guess := range commonClaimNames {
			if strings.EqualFold(literal, guess) {
				sawGuess = true
			}
		}
	}
	if !sawGuess {
		t.Fatal("a default claim name was accepted")
	}

	rightFromAClaim := `package auth
func administrative(groups []GroupName, policy GroupPolicy, claims Claims) bool {
	if claims.Email == "operator@example.invalid" {
		return true
	}
	return false
}`
	if len(readsInside(t, "fixture.go", rightFromAClaim, "administrative", forbiddenInTheRightDecision)) == 0 {
		t.Fatal("an administrative right read from a claim was accepted")
	}
}

// sourceOf reads a file of this package, so a source rule judges what is
// committed rather than a string in a test.
func sourceOf(t *testing.T, filename string) string {
	t.Helper()
	src, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("read %s: %v", filename, err)
	}
	return string(src)
}

// literalsIn is every string literal in src.
func literalsIn(t *testing.T, filename, src string) []string {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, src, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse %s: %v", filename, err)
	}

	var literals []string
	ast.Inspect(file, func(node ast.Node) bool {
		if lit, ok := node.(*ast.BasicLit); ok && lit.Kind == token.STRING {
			literals = append(literals, strings.Trim(lit.Value, "`\""))
		}
		return true
	})
	return literals
}

// readsInside reports every place inside one function where a forbidden type
// is taken as a parameter or a forbidden field is read, which is how a rule
// about where a decision may look is checked.
func readsInside(t *testing.T, filename, src, fn string, forbidden []string) []string {
	t.Helper()

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, src, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse %s: %v", filename, err)
	}

	var found []string
	for _, decl := range file.Decls {
		declared, ok := decl.(*ast.FuncDecl)
		if !ok || declared.Body == nil || declared.Name.Name != fn {
			continue
		}
		for _, param := range declared.Type.Params.List {
			if slices.Contains(forbidden, typeName(param.Type)) {
				found = append(found, fn+" takes a "+typeName(param.Type))
			}
		}
		ast.Inspect(declared.Body, func(node ast.Node) bool {
			if selector, ok := node.(*ast.SelectorExpr); ok && slices.Contains(forbidden, selector.Sel.Name) {
				found = append(found, fn+" reads "+selector.Sel.Name)
			}
			return true
		})
	}
	return found
}

// typeName is the identifier of a type expression, for the simple forms a
// signature in this package uses.
func typeName(expr ast.Expr) string {
	if ident, ok := expr.(*ast.Ident); ok {
		return ident.Name
	}
	return ""
}
