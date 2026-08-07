// Package auth turns what an identity provider said about a user into a
// principal a source system's permissions can be compared against.
//
// The two rarely agree on how to name a person. A provider issues a stable
// subject identifier and some claims. A file server knows a security
// identifier. A groupware system knows an account name. A mail store knows an
// address. The same human is all four, and the mapping between them is the
// place where a mistake grants access rather than denying it.
//
// So the direction of failure is chosen here rather than left to whatever the
// call site does. A mapping that cannot be resolved means no access to that
// source. Never a fallback to a broader principal, never a guess from a
// matching display name, and never a guess from a matching address the source
// did not itself assert. docs/principal.md is the argument; this package is
// the shape.
package auth

import (
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/iderex/kanzlei/internal/authz"
)

// A SourceID names a source system this project can be pointed at.
type SourceID string

// Claims is what the identity provider handed over for this session.
//
// DisplayName and Email are here because a provider issues them and something
// above this package will want to show them. Nothing in this file may map with
// them, and TestNothingMapsFromADisplayNameOrAnAddress reads the source to
// hold that: a mapping inferred from either is the failure this package exists
// to refuse, and the way it arrives is somebody writing one line in a function
// that already has the claims in hand.
type Claims struct {
	// Subject is the provider's stable subject identifier. It is the only
	// field a mapping may be keyed on.
	Subject string
	// Groups are the group claims as the provider issued them.
	Groups []string
	// DisplayName is for showing to a person.
	DisplayName string
	// Email is for showing to a person, and for nothing else.
	Email string
}

// AssertedBy says where a per-source identity came from.
type AssertedBy string

const (
	// AssertedByConfiguration is an operator writing the mapping down.
	AssertedByConfiguration AssertedBy = "configuration"
	// AssertedBySource is the source system answering for itself, which is the
	// only other admissible origin.
	AssertedBySource AssertedBy = "source"
)

// An Assertion records how one per-source identity was established, so that a
// consumer can tell an operator's claim from a source's own answer.
type Assertion struct {
	By AssertedBy
	// Source is which source asserted it, set when By is AssertedBySource and
	// empty otherwise.
	Source SourceID
}

// A SourceIdentity is how one source system names this user, and how that came
// to be known.
type SourceIdentity struct {
	Source    SourceID
	Value     string
	Assertion Assertion
}

// A Principal is who is asking, in every naming scheme this session has
// resolved.
//
// The per-source identities are unexported and reached through IdentityIn, so
// that a caller cannot read the map, find nothing, and carry on with the
// subject identifier as though it were a source identity. Finding nothing has
// to be a branch somebody wrote.
type Principal struct {
	// Subject is the provider's stable subject identifier.
	Subject string
	// Groups are the groups resolved for this session.
	Groups []string
	// GroupsResolvedAt is when that resolution happened. Resolution is once
	// per session, so this is the age of every group decision taken from this
	// principal.
	GroupsResolvedAt time.Time

	identities map[SourceID]SourceIdentity
}

// MaxGroupAge is how old a session's resolved group membership may be before
// it has to be resolved again.
//
// Fifteen minutes is the number, and it is a bound on how long a group removed
// at the provider goes on being honoured by a session that is already open. It
// is not the whole of that exposure: the point-of-use recheck in
// docs/decisions/0003-permission-model.md is what closes the rest, and a
// session that is never used again is never re-resolved because nothing asks.
//
// docs/principal.md carries the argument. The done-when of #16 asks for it to
// be in the decision record of #15 instead, and that record does not carry it
// today; the note in #16 says so.
const MaxGroupAge = 15 * time.Minute

// ErrNoSubject is a set of claims with no stable subject identifier.
//
// It is refused rather than defaulted. A principal with an empty subject
// matches an entry whose value is also empty, and half-populated records are
// how such an entry arrives.
var ErrNoSubject = errors.New("claims carry no subject identifier")

// A MappingEntry is one operator-written line: this subject is that user, in
// that source.
type MappingEntry struct {
	Subject string
	Source  SourceID
	Value   string
}

// A Mapping is the operator's explicit configuration, subject by source.
type Mapping struct {
	entries map[string]map[SourceID]string
}

// NewMapping builds a mapping from configured entries, refusing one that is
// not fully written down.
//
// Every field is required. An entry missing any of them is a line somebody
// half wrote, and the reading that completes it from somewhere else is the
// inference this package refuses. A subject configured twice for one source is
// refused rather than resolved by order, because a configuration file whose
// meaning depends on line order is one an operator cannot review.
func NewMapping(entries []MappingEntry) (Mapping, error) {
	built := map[string]map[SourceID]string{}
	for i, entry := range entries {
		switch {
		case entry.Subject == "":
			return Mapping{}, fmt.Errorf("mapping entry %d names no subject", i)
		case entry.Source == "":
			return Mapping{}, fmt.Errorf("mapping entry %d for subject %q names no source", i, entry.Subject)
		case entry.Value == "":
			return Mapping{}, fmt.Errorf("mapping entry %d for subject %q in source %q has no identifier", i, entry.Subject, entry.Source)
		}
		if _, seen := built[entry.Subject][entry.Source]; seen {
			return Mapping{}, fmt.Errorf("mapping entry %d maps subject %q into source %q twice", i, entry.Subject, entry.Source)
		}
		if built[entry.Subject] == nil {
			built[entry.Subject] = map[SourceID]string{}
		}
		built[entry.Subject][entry.Source] = entry.Value
	}
	return Mapping{entries: built}, nil
}

// Resolve turns claims into a principal, applying the configured mapping.
//
// resolvedAt is passed in rather than read from a clock, so that the age of a
// session's group membership is a fact the caller states and a test can set.
//
// Only claims.Subject is read to decide a mapping. The other claims are copied
// through untouched.
func Resolve(claims Claims, mapping Mapping, resolvedAt time.Time) (Principal, error) {
	if claims.Subject == "" {
		return Principal{}, ErrNoSubject
	}

	principal := Principal{
		Subject:          claims.Subject,
		Groups:           slices.Clone(claims.Groups),
		GroupsResolvedAt: resolvedAt,
		identities:       map[SourceID]SourceIdentity{},
	}
	for source, value := range mapping.entries[claims.Subject] {
		principal.identities[source] = SourceIdentity{
			Source:    source,
			Value:     value,
			Assertion: Assertion{By: AssertedByConfiguration},
		}
	}
	return principal, nil
}

// WithSourceAssertion returns a copy of p carrying an identity the source
// system asserted for itself.
//
// This is the only other way an identity arrives. The source is answering a
// question about its own user, which is a fact rather than a guess, and the
// assertion records that it came from there so a consumer can tell the two
// apart later. An assertion with an empty value is refused: a source that
// answered with nothing has not asserted anything.
func (p Principal) WithSourceAssertion(source SourceID, value string) (Principal, error) {
	if source == "" {
		return Principal{}, errors.New("a source assertion names no source")
	}
	if value == "" {
		return Principal{}, fmt.Errorf("source %q asserted an empty identifier for subject %q", source, p.Subject)
	}

	copied := p
	copied.Groups = slices.Clone(p.Groups)
	copied.identities = map[SourceID]SourceIdentity{}
	for id, identity := range p.identities {
		copied.identities[id] = identity
	}
	copied.identities[source] = SourceIdentity{
		Source:    source,
		Value:     value,
		Assertion: Assertion{By: AssertedBySource, Source: source},
	}
	return copied, nil
}

// IdentityIn reports how source names this user, and whether it names them at
// all.
//
// The second result is the whole point. A user with no mapping into a source
// is a user that source has never heard of, and the only safe reading of that
// is no access rather than a fallback to something broader.
func (p Principal) IdentityIn(source SourceID) (SourceIdentity, bool) {
	identity, mapped := p.identities[source]
	return identity, mapped
}

// Sources lists the sources this principal is mapped into, in no order.
func (p Principal) Sources() []SourceID {
	sources := make([]SourceID, 0, len(p.identities))
	for source := range p.identities {
		sources = append(sources, source)
	}
	slices.Sort(sources)
	return sources
}

// GroupsAreStale reports whether the group membership resolved for this
// session is older than MaxGroupAge at the given moment.
//
// It is a question rather than an enforcement, because what to do about a
// stale session belongs to whoever holds the session, which is #31. What this
// package owes is that the age is carried at all.
func (p Principal) GroupsAreStale(now time.Time) bool {
	return now.Sub(p.GroupsResolvedAt) > MaxGroupAge
}

// ForSource reduces this principal to what an entry in that source's
// permissions can name, and reports whether the reduction was possible.
//
// False means the user has no identity in that source, and a caller that gets
// false has no business evaluating anything against that source's documents.
// The zero authz.Principal returned alongside it is not a usable fallback: it
// matches nothing, so a caller that ignores the second result gets refusals
// rather than a broader principal. That is deliberate, and
// TestIgnoringTheSecondResultStillRefuses holds it.
//
// The groups pass through as the provider issued them, which is correct only
// for a source that names groups the way the provider does. Mapping a group
// claim into a source's own group identifiers, and refusing a claim that maps
// into none, is #30. Until that lands, a source whose groups are named
// differently sees group entries that match nothing, which errs towards
// refusal rather than towards access.
func (p Principal) ForSource(source SourceID) (authz.Principal, bool) {
	identity, mapped := p.IdentityIn(source)
	if !mapped {
		return authz.Principal{}, false
	}
	return authz.Principal{
		Subject: identity.Value,
		Groups:  slices.Clone(p.Groups),
	}, true
}
