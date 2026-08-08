package auth

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/iderex/kanzlei/internal/authz"
)

// resolvedAt is the moment every case below resolves at. A fixed value rather
// than time.Now, so that nothing here is a different test tomorrow.
var resolvedAt = time.Date(2026, 3, 4, 9, 0, 0, 0, time.UTC)

// alice is the same person in three naming schemes, which is the situation
// this package exists for.
var alice = Claims{
	Subject:     "sub-alice",
	Groups:      []string{"legal-team"},
	DisplayName: "A. Muster",
	Email:       "a.muster@example.invalid",
}

const (
	fileServer = SourceID("file-server")
	mailStore  = SourceID("mail-store")
	groupware  = SourceID("groupware")
)

func mustMap(t *testing.T, entries ...MappingEntry) Mapping {
	t.Helper()
	mapping, err := NewMapping(entries)
	if err != nil {
		t.Fatalf("NewMapping: %v", err)
	}
	return mapping
}

func mustResolve(t *testing.T, claims Claims, mapping Mapping) Principal {
	t.Helper()
	principal, err := Resolve(claims, mapping, resolvedAt)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	return principal
}

// TestAPrincipalCarriesEveryNameThisSessionResolved is the first line of the
// done-when: the subject, the groups, and the per-source identifiers with
// where each one came from.
func TestAPrincipalCarriesEveryNameThisSessionResolved(t *testing.T) {
	mapping := mustMap(t,
		MappingEntry{Subject: "sub-alice", Source: fileServer, Value: "S-1-5-21-1004"},
		MappingEntry{Subject: "sub-alice", Source: mailStore, Value: "amuster"},
		MappingEntry{Subject: "sub-bob", Source: fileServer, Value: "S-1-5-21-1007"},
	)
	principal := mustResolve(t, alice, mapping)

	if principal.Subject != "sub-alice" {
		t.Fatalf("Subject = %q, want the provider's subject identifier", principal.Subject)
	}
	if !slices.Equal(principal.Groups, []string{"legal-team"}) {
		t.Fatalf("Groups = %v, want the groups the provider issued", principal.Groups)
	}
	if !principal.GroupsResolvedAt.Equal(resolvedAt) {
		t.Fatalf("GroupsResolvedAt = %v, want %v", principal.GroupsResolvedAt, resolvedAt)
	}
	if got := principal.Sources(); !slices.Equal(got, []SourceID{fileServer, mailStore}) {
		t.Fatalf("Sources() = %v, want the two this subject is configured into", got)
	}

	identity, mapped := principal.IdentityIn(fileServer)
	if !mapped {
		t.Fatal("the configured identity in the file server was not resolved")
	}
	if identity.Value != "S-1-5-21-1004" {
		t.Fatalf("Value = %q, want the configured identifier", identity.Value)
	}
	if identity.Assertion.By != AssertedByConfiguration {
		t.Fatalf("Assertion.By = %q, want %q", identity.Assertion.By, AssertedByConfiguration)
	}
}

// TestNoMappingIntoASourceIsNoAccessToIt is the third line of the done-when,
// asserted rather than assumed.
//
// It is checked at both layers that exist. IdentityIn reports that the source
// has never heard of this user, and ForSource refuses to produce anything to
// evaluate against that source's documents.
func TestNoMappingIntoASourceIsNoAccessToIt(t *testing.T) {
	mapping := mustMap(t, MappingEntry{Subject: "sub-alice", Source: fileServer, Value: "S-1-5-21-1004"})
	principal := mustResolve(t, alice, mapping)

	if _, mapped := principal.IdentityIn(groupware); mapped {
		t.Fatal("a source this subject was never mapped into reported an identity")
	}
	if _, usable := principal.ForSource(groupware); usable {
		t.Fatal("ForSource produced a principal for a source the user is not mapped into")
	}
	if slices.Contains(principal.Sources(), groupware) {
		t.Fatal("Sources() lists a source the user is not mapped into")
	}
}

// TestIgnoringTheSecondResultStillRefuses is the case where somebody writes
// the call wrongly.
//
// A caller that drops the boolean is left holding the zero authz.Principal.
// That has to be useless rather than broad: it names nobody, so every entry
// that could have matched does not, and the document is refused for having
// named nobody rather than allowed for having named everybody.
func TestIgnoringTheSecondResultStillRefuses(t *testing.T) {
	mapping := mustMap(t, MappingEntry{Subject: "sub-alice", Source: fileServer, Value: "S-1-5-21-1004"})
	principal := mustResolve(t, alice, mapping)

	forSource, _ := principal.ForSource(groupware) // the mistake this case is about, made on purpose

	sets := map[string]authz.Set{
		"allowed to a user":  {{Effect: authz.Allow, Term: authz.Term{Type: authz.TermUser, Value: "S-1-5-21-1004"}}},
		"allowed to a group": {{Effect: authz.Allow, Term: authz.Term{Type: authz.TermGroup, Value: "legal-team"}}},
		"allowed to nobody":  {},
	}
	for name, set := range sets {
		if decision := authz.Evaluate(forSource, set); decision.Allowed {
			t.Fatalf("%s: a dropped mapping failure produced a principal that was allowed: %+v", name, decision)
		}
	}
}

// TestChangingOnlyTheMappingChangesWhatIsPermitted is the last line of the
// done-when, at the layer this tree has.
//
// The claims are identical, the documents are identical, and the only thing
// that differs between the two runs is one line of mapping configuration. If
// the permitted set did not move, the mapping would be decoration.
//
// What it measures is which documents the evaluator admits, not which
// documents a retrieval route returns, because no retrieval route exists yet.
// #62 is where that layer arrives, and the note in #16 says so.
func TestChangingOnlyTheMappingChangesWhatIsPermitted(t *testing.T) {
	documents := map[string]authz.Set{
		"the case file": {{Effect: authz.Allow, Term: authz.Term{Type: authz.TermUser, Value: "S-1-5-21-1004"}}},
		"the archive":   {{Effect: authz.Allow, Term: authz.Term{Type: authz.TermUser, Value: "S-1-5-21-1007"}}},
	}

	permitted := func(t *testing.T, mapping Mapping) []string {
		t.Helper()
		principal := mustResolve(t, alice, mapping)
		forSource, usable := principal.ForSource(fileServer)
		if !usable {
			return nil
		}
		var visible []string
		for name, set := range documents {
			if authz.Evaluate(forSource, set).Allowed {
				visible = append(visible, name)
			}
		}
		slices.Sort(visible)
		return visible
	}

	asAlice := permitted(t, mustMap(t, MappingEntry{Subject: "sub-alice", Source: fileServer, Value: "S-1-5-21-1004"}))
	if !slices.Equal(asAlice, []string{"the case file"}) {
		t.Fatalf("with the first mapping the permitted set is %v, want the case file", asAlice)
	}

	asBob := permitted(t, mustMap(t, MappingEntry{Subject: "sub-alice", Source: fileServer, Value: "S-1-5-21-1007"}))
	if !slices.Equal(asBob, []string{"the archive"}) {
		t.Fatalf("with the identifier changed the permitted set is %v, want the archive", asBob)
	}

	none := permitted(t, mustMap(t))
	if len(none) != 0 {
		t.Fatalf("with the mapping removed the permitted set is %v, want nothing", none)
	}
}

// TestNewMappingRefusesAHalfWrittenEntry holds the second line of the
// done-when from the configuration side. Every field is required, because the
// reading that fills in a missing one from somewhere else is the inference
// this package refuses.
func TestNewMappingRefusesAHalfWrittenEntry(t *testing.T) {
	cases := []struct {
		name    string
		entries []MappingEntry
	}{
		{name: "no subject", entries: []MappingEntry{{Source: fileServer, Value: "S-1-5-21-1004"}}},
		{name: "no source", entries: []MappingEntry{{Subject: "sub-alice", Value: "S-1-5-21-1004"}}},
		{name: "no identifier", entries: []MappingEntry{{Subject: "sub-alice", Source: fileServer}}},
		{
			name: "the same subject mapped into one source twice",
			entries: []MappingEntry{
				{Subject: "sub-alice", Source: fileServer, Value: "S-1-5-21-1004"},
				{Subject: "sub-alice", Source: fileServer, Value: "S-1-5-21-1007"},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := NewMapping(c.entries); err == nil {
				t.Fatal("the mapping was accepted; it has to be refused")
			}
		})
	}
}

// TestResolveRefusesClaimsWithNoSubject covers the half-populated resolvedAt. An
// empty subject would match a mapping keyed on an empty subject, and an empty
// principal is exactly what a broken sign-on hands over.
func TestResolveRefusesClaimsWithNoSubject(t *testing.T) {
	_, err := Resolve(Claims{Groups: []string{"legal-team"}}, Mapping{}, resolvedAt)
	if !errors.Is(err, ErrNoSubject) {
		t.Fatalf("err = %v, want %v", err, ErrNoSubject)
	}
}

// TestASourceAssertionIsRecordedAsOne holds the other admissible origin. A
// source answering about its own user is a fact, and it is tagged as coming
// from the source so a consumer can tell it from an operator's configuration.
func TestASourceAssertionIsRecordedAsOne(t *testing.T) {
	principal := mustResolve(t, alice, mustMap(t))

	asserted, err := principal.WithSourceAssertion(groupware, "amuster@groupware")
	if err != nil {
		t.Fatalf("WithSourceAssertion: %v", err)
	}
	identity, mapped := asserted.IdentityIn(groupware)
	if !mapped {
		t.Fatal("the asserted identity was not carried")
	}
	if identity.Assertion.By != AssertedBySource || identity.Assertion.Source != groupware {
		t.Fatalf("Assertion = %+v, want it recorded as asserted by %q", identity.Assertion, groupware)
	}

	if _, mapped := principal.IdentityIn(groupware); mapped {
		t.Fatal("the assertion changed the principal it was called on; it has to return a copy")
	}

	for _, empty := range []struct {
		source SourceID
		value  string
	}{{groupware, ""}, {"", "amuster@groupware"}} {
		if _, err := principal.WithSourceAssertion(empty.source, empty.value); err == nil {
			t.Fatalf("an assertion of (%q, %q) was accepted; an empty half asserts nothing", empty.source, empty.value)
		}
	}
}

// TestGroupsCarryTheirAge holds the fourth line of the done-when as far as
// this package can. Membership is resolved once, at a stated moment, and the
// principal can say when that stops being current.
func TestGroupsCarryTheirAge(t *testing.T) {
	principal := mustResolve(t, alice, mustMap(t))

	if principal.GroupsAreStale(resolvedAt.Add(MaxGroupAge)) {
		t.Fatal("membership exactly at the maximum age was called stale")
	}
	if !principal.GroupsAreStale(resolvedAt.Add(MaxGroupAge + time.Second)) {
		t.Fatal("membership past the maximum age was not called stale")
	}
}

// TestResolveDoesNotAliasTheCallersGroups covers the aliasing mistake. A
// principal sharing the claims' backing array is a principal whose groups
// change when somebody else edits theirs.
func TestResolveDoesNotAliasTheCallersGroups(t *testing.T) {
	claims := Claims{Subject: "sub-alice", Groups: []string{"legal-team"}}
	principal := mustResolve(t, claims, mustMap(t))

	claims.Groups[0] = "domain-admins"

	if !slices.Equal(principal.Groups, []string{"legal-team"}) {
		t.Fatalf("Groups = %v; editing the caller's slice changed the principal", principal.Groups)
	}
}

// --- what the source is allowed to look like ---

// inferenceFields are the claims a mapping may never be inferred from. Two
// users with the same display name is an ordinary state of affairs in a large
// directory, and an address is a thing an administrator can set to any value
// in either system.
var inferenceFields = []string{"DisplayName", "Email"}

// mappingFunctions are the functions that decide or carry a mapping. A read of
// an inference field inside any of them is what this refuses.
var mappingFunctions = []string{"NewMapping", "Resolve", "WithSourceAssertion", "IdentityIn", "ForSource"}

// inferenceReads reports every place in src where one of the mapping functions
// reads a field a mapping may not be inferred from.
//
// filename is used for the reported position and nothing else, so the fixtures
// below stay in source rather than being written to disk.
func inferenceReads(t *testing.T, filename string, src []byte) []string {
	t.Helper()

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, src, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse %s: %v", filename, err)
	}

	var found []string
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil || !slices.Contains(mappingFunctions, fn.Name.Name) {
			continue
		}
		ast.Inspect(fn.Body, func(node ast.Node) bool {
			selector, ok := node.(*ast.SelectorExpr)
			if ok && slices.Contains(inferenceFields, selector.Sel.Name) {
				found = append(found, fmt.Sprintf("%s reads %s at line %d",
					fn.Name.Name, selector.Sel.Name, fset.Position(selector.Pos()).Line))
			}
			return true
		})
	}
	return found
}

func thisPackagesSource(t *testing.T) []byte {
	t.Helper()
	src, err := os.ReadFile("principal.go")
	if err != nil {
		t.Fatalf("read principal.go: %v", err)
	}
	return src
}

// TestNothingMapsFromADisplayNameOrAnAddress is the second line of the
// done-when held as a refusal rather than as an absence.
//
// Claims carries a display name and an address because a provider issues them
// and something above this package will show them. The failure is one line
// added to a function that already has the claims in hand, matching on either,
// and it looks entirely reasonable in a diff. This reads the source instead.
func TestNothingMapsFromADisplayNameOrAnAddress(t *testing.T) {
	for _, read := range inferenceReads(t, "principal.go", thisPackagesSource(t)) {
		t.Errorf("principal.go: %s; a mapping is explicit configuration or a source's own assertion, never a guess from a claim two people can share", read)
	}
}

// TestTheInferenceRuleRefusesWhatItNames proves the test above bites, with the
// two shapes somebody actually writes.
func TestTheInferenceRuleRefusesWhatItNames(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{
			name: "the mapping falls back to matching addresses",
			src: `package auth
func Resolve(claims Claims, mapping Mapping, resolvedAt time.Time) (Principal, error) {
	for source, candidate := range mapping.entries[claims.Subject] {
		_ = source
		_ = candidate
	}
	if len(mapping.entries[claims.Subject]) == 0 {
		return byAddress(claims.Email)
	}
	return Principal{}, nil
}`,
		},
		{
			name: "a display name is used to pick between two candidates",
			src: `package auth
func ForSource(p Principal, source SourceID) (authz.Principal, bool) {
	return authz.Principal{}, false
}
func Resolve(claims Claims, mapping Mapping, resolvedAt time.Time) (Principal, error) {
	if candidate.Name == claims.DisplayName {
		return Principal{Subject: candidate.ID}, nil
	}
	return Principal{}, nil
}`,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if found := inferenceReads(t, "fixture.go", []byte(c.src)); len(found) == 0 {
				t.Fatal("the fixture was accepted; the rule does not refuse an inferred mapping")
			}
		})
	}
}

// TestTheInferenceRuleLeavesTheRestAlone is the other half. A rule that
// refuses every read of those fields anywhere would refuse the code that shows
// a name to a person, which is not what it is for.
func TestTheInferenceRuleLeavesTheRestAlone(t *testing.T) {
	src := `package auth
func greeting(claims Claims) string {
	return "Hello, " + claims.DisplayName + " <" + claims.Email + ">"
}
func Resolve(claims Claims, mapping Mapping, resolvedAt time.Time) (Principal, error) {
	return Principal{Subject: claims.Subject}, nil
}`

	if found := inferenceReads(t, "fixture.go", []byte(src)); len(found) != 0 {
		t.Fatalf("a read outside the mapping functions was refused: %v", found)
	}
}

// TestTheInferenceRuleNamesTheFunctionsThatExist is what stops the rule
// quietly covering nothing. A function renamed in principal.go and not renamed
// in mappingFunctions leaves the walk looking for something that is not there,
// and it would report a clean file for the same reason an empty file is clean.
func TestTheInferenceRuleNamesTheFunctionsThatExist(t *testing.T) {
	src := string(thisPackagesSource(t))
	for _, name := range mappingFunctions {
		if !strings.Contains(src, "func "+name+"(") && !strings.Contains(src, ") "+name+"(") {
			t.Errorf("mappingFunctions names %q and principal.go declares no such function", name)
		}
	}
}
