package auth

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"
)

// A GroupName is a group as this application knows it, after mapping.
//
// It is a distinct type from the raw claim value on purpose. The two are
// different things that both happen to be strings, and the step between them
// is the whole subject of this file.
type GroupName string

// A Token is the claim set as the provider issued it.
//
// It is read only through GroupValues, and only at the claim the policy names.
type Token map[string]any

// A ClaimShape is how a provider delivers group membership.
//
// Which one a deployment is on is configuration. Guessing it, by looking for a
// claim called groups and falling back to roles and then to memberOf, is how a
// deployment silently resolves nothing and every user quietly loses access, or
// resolves the wrong claim and quietly gains it.
type ClaimShape string

const (
	// ShapeNames is a claim holding group names as text.
	ShapeNames ClaimShape = "names"
	// ShapeIdentifiers is a claim holding opaque identifiers, which is what a
	// provider issues when a group can be renamed without becoming a different
	// group.
	ShapeIdentifiers ClaimShape = "identifiers"
	// ShapeProviderCall is a provider that puts membership behind a second
	// call rather than in the token.
	ShapeProviderCall ClaimShape = "provider-call"
)

// A GroupPolicy is the whole of what a deployment configures about groups.
type GroupPolicy struct {
	// Claim is which claim carries the groups. Required for the two token
	// shapes, and refused for ShapeProviderCall, where the answer does not
	// come from the token at all.
	Claim string
	// Shape is how to read it.
	Shape ClaimShape
	// Mapping is claim value to application group. It is the whole rule: a
	// value absent from it has no mapping, and there is no fallback that
	// treats an unmapped raw value as a group name.
	Mapping map[string]GroupName
	// MaxGroups is how many values one session may carry.
	MaxGroups int
	// Administrative are the application groups carrying administrative rights
	// in this application.
	//
	// They are application groups, which is to say the output of Mapping and
	// never a raw claim value, so a user who can influence what the provider
	// puts in a claim still cannot reach this list without an operator having
	// written the mapping entry that leads to it.
	//
	// This grants nothing over a document. internal/authz has no
	// administrative branch, this change adds none, and a document is reached
	// through its permission set or not at all. Nothing in this tree consumes
	// this field yet; #34 is where an administrative route first exists.
	Administrative []GroupName
}

// ErrUnmappedClaim is a group claim value the policy has no mapping for.
//
// It fails the sign-on rather than being dropped. A dropped value is a
// permission change nobody asked for and it is invisible: the user signs in,
// sees less than they should, and reports it as a search that found nothing.
// An operator who meets this error has a configuration problem with a name.
var ErrUnmappedClaim = errors.New("group claim value has no mapping")

// ErrTooManyGroups is a session carrying more group values than the policy
// allows.
//
// It fails the sign-on rather than truncating, because a truncated list is a
// silent permission change in the other direction, and which values survived
// depends on the order the provider happened to send them in.
var ErrTooManyGroups = errors.New("session carries more group values than the policy allows")

// ErrGroupsAlreadySet is a caller handing NewSession claims that already carry
// groups.
//
// Under a policy the groups come from the policy. Accepting a pre-filled list
// would give this package two sources for one fact, and the one that wins
// would be whichever the code happened to read last.
var ErrGroupsAlreadySet = errors.New("claims arrived with groups already set")

// A GroupSource answers group membership for a subject, for a provider that
// keeps it out of the token.
//
// It is an interface because the implementation is a call into a provider,
// which is #28, and because the alternative is a test suite that needs one.
type GroupSource interface {
	GroupsFor(ctx context.Context, subject string) ([]string, error)
}

// Validate refuses a policy that could not be applied as written.
func (p GroupPolicy) Validate() error {
	switch p.Shape {
	case ShapeNames, ShapeIdentifiers:
		if p.Claim == "" {
			return fmt.Errorf("shape %q names no claim to read groups from", p.Shape)
		}
	case ShapeProviderCall:
		if p.Claim != "" {
			return fmt.Errorf("shape %q names claim %q, and membership under this shape does not come from the token", p.Shape, p.Claim)
		}
	default:
		return fmt.Errorf("group claim shape %q is not one this application reads", p.Shape)
	}
	if p.MaxGroups <= 0 {
		return errors.New("the policy sets no positive bound on how many group values a session may carry")
	}
	for _, administrative := range p.Administrative {
		if !p.produces(administrative) {
			return fmt.Errorf("administrative group %q is produced by no mapping entry, so nothing can ever reach it", administrative)
		}
	}
	return nil
}

// produces reports whether any mapping entry results in this application
// group. An administrative group nothing maps to is a configuration that reads
// as a grant and is one, which is worse than an error.
func (p GroupPolicy) produces(group GroupName) bool {
	for _, mapped := range p.Mapping {
		if mapped == group {
			return true
		}
	}
	return false
}

// GroupValues reads the group values out of a token, at the claim the policy
// names and nowhere else.
//
// There is no list of likely claim names here and no fallback to a second
// claim. A deployment states which claim carries groups; a deployment that
// states the wrong one gets no groups and an operator who can see why, which
// is the failure that can be fixed. Guessing produces the failure that cannot:
// a claim that happened to match, resolving to memberships nobody configured.
//
// A claim that is absent is an empty list rather than an error, because a user
// in no groups is an ordinary user. A claim present in a shape this cannot
// read is refused, because that is a provider or a configuration this
// deployment has not been set up for, and reading half of it would be a guess.
func GroupValues(token Token, policy GroupPolicy) ([]string, error) {
	if err := policy.Validate(); err != nil {
		return nil, fmt.Errorf("group policy: %w", err)
	}
	if policy.Shape == ShapeProviderCall {
		return nil, fmt.Errorf("shape %q takes its groups from a call, not from the token", policy.Shape)
	}

	raw, present := token[policy.Claim]
	if !present {
		return nil, nil
	}

	switch values := raw.(type) {
	case []string:
		return slices.Clone(values), nil
	case []any:
		out := make([]string, 0, len(values))
		for i, value := range values {
			text, ok := value.(string)
			if !ok {
				return nil, fmt.Errorf("claim %q holds %T at position %d, and this application reads group values as text", policy.Claim, value, i)
			}
			out = append(out, text)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("claim %q holds %T, and this application reads a list of group values", policy.Claim, raw)
	}
}

// ResolveGroups maps raw values into application groups, refusing rather than
// repairing.
//
// The bound is checked before the mapping, so a session carrying a thousand
// values is refused for the reason it is refused rather than for whichever
// unmapped value happened to come first.
//
// The result is sorted and deduplicated. Two claim values mapping to one
// application group is an ordinary configuration and it must not make a
// session look different from one that named the group once.
func ResolveGroups(raw []string, policy GroupPolicy) ([]GroupName, error) {
	if err := policy.Validate(); err != nil {
		return nil, fmt.Errorf("group policy: %w", err)
	}
	if len(raw) > policy.MaxGroups {
		return nil, fmt.Errorf("%w: %d supplied, %d allowed", ErrTooManyGroups, len(raw), policy.MaxGroups)
	}

	groups := make([]GroupName, 0, len(raw))
	for _, value := range raw {
		mapped, known := policy.Mapping[value]
		if !known {
			return nil, fmt.Errorf("%w: %q, read under claim shape %q", ErrUnmappedClaim, value, policy.Shape)
		}
		if !slices.Contains(groups, mapped) {
			groups = append(groups, mapped)
		}
	}
	slices.Sort(groups)
	return groups, nil
}

// A Session is one sign-on with its groups resolved once.
//
// It exists so that once per session is a property of a type rather than a
// convention. The groups are resolved when the session is built and there is
// no method here that resolves them again, so a second call into a provider is
// not something a later caller can make on the request path by accident.
type Session struct {
	// Principal is who this session is, in every naming scheme it resolved.
	Principal Principal
	// Groups are the application groups, which are what Principal.Groups
	// carries as text.
	Groups []GroupName
	// Administrative is whether any of them is a group the policy names as
	// administrative.
	Administrative bool
}

// NewSession resolves this session's groups once and builds the principal.
//
// token is read only at the claim the policy names, and only for the two token
// shapes. source is used only for ShapeProviderCall and is called exactly
// once, which is what binds the answer to the session rather than to a
// request.
//
// claims must arrive with no groups. Under a policy the groups come from the
// policy, and accepting a pre-filled list would give this package two sources
// for one fact.
func NewSession(ctx context.Context, claims Claims, token Token, policy GroupPolicy, source GroupSource, mapping Mapping, resolvedAt time.Time) (Session, error) {
	if len(claims.Groups) != 0 {
		return Session{}, ErrGroupsAlreadySet
	}

	raw, err := rawGroups(ctx, claims.Subject, token, policy, source)
	if err != nil {
		return Session{}, err
	}
	groups, err := ResolveGroups(raw, policy)
	if err != nil {
		return Session{}, err
	}

	claims.Groups = asStrings(groups)
	principal, err := Resolve(claims, mapping, resolvedAt)
	if err != nil {
		return Session{}, err
	}

	return Session{
		Principal:      principal,
		Groups:         groups,
		Administrative: administrative(groups, policy),
	}, nil
}

// rawGroups gets the values the provider supplied, from wherever this
// deployment's provider puts them.
func rawGroups(ctx context.Context, subject string, token Token, policy GroupPolicy, source GroupSource) ([]string, error) {
	if policy.Shape != ShapeProviderCall {
		return GroupValues(token, policy)
	}
	if source == nil {
		return nil, fmt.Errorf("claim shape %q needs a group source and none was supplied", policy.Shape)
	}
	raw, err := source.GroupsFor(ctx, subject)
	if err != nil {
		return nil, fmt.Errorf("resolving groups for subject %q: %w", subject, err)
	}
	return raw, nil
}

// administrative reports whether any of this session's application groups is
// one the policy names as administrative.
//
// It reads mapped groups and nothing else. No claim value reaches it, which is
// what stops an administrative right coming from something the user might
// influence, and TestNoAdministrativeRightIsReadFromAClaim reads the source to
// hold that.
func administrative(groups []GroupName, policy GroupPolicy) bool {
	for _, group := range groups {
		if slices.Contains(policy.Administrative, group) {
			return true
		}
	}
	return false
}

func asStrings(groups []GroupName) []string {
	out := make([]string, 0, len(groups))
	for _, group := range groups {
		out = append(out, string(group))
	}
	return out
}
