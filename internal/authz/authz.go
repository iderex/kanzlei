// Package authz resolves a permission set against a principal.
//
// It is three rules and each one of them has been got wrong somewhere before.
// A deny entry beats an allow entry whatever their order. A principal that
// matches nothing gets nothing. A term the evaluator does not recognise is a
// deny and not a skip, because skipping a restriction it cannot read is how a
// restriction stops restricting.
//
// The third is the one that matters as this project grows. A connector written
// a year from now will emit a construct this code has never seen, and the only
// safe reading of a restriction nobody can parse is that it restricts.
//
// docs/permissions.md states the three rules and names the test that holds
// each. docs/decisions/0003-permission-model.md is where the model this
// package resolves for was decided.
//
// What this package does not do: it takes no decision about who a principal
// is, it reads no source system, and it holds no route by which any caller
// gets a different answer from the one the entries say. There is no
// privileged caller here, and #18 is the issue that says why that absence is
// the design rather than an omission.
package authz

import (
	"fmt"
	"slices"
)

// An Effect is what an entry does when its term matches.
type Effect string

const (
	// Allow lets the principal see the document, unless something else denies.
	Allow Effect = "allow"
	// Deny refuses it, whatever else the set says.
	Deny Effect = "deny"
)

// A TermType is what an entry names: the kind of thing its value identifies.
//
// The complete set of them is the key set of matchers below, and a type absent
// from there is unrecognised however plausible it looks.
type TermType string

const (
	// TermUser names one principal by the stable subject identifier the
	// identity provider issued.
	TermUser TermType = "user"
	// TermGroup names a group, matched against the groups resolved for the
	// session.
	TermGroup TermType = "group"
)

// matchers is the complete set of term types this evaluator understands, and
// the only place a type is declared understood.
//
// Recognition and matching are read from the same entry deliberately. A list of
// recognised types sitting beside a switch that matches them is two lists, and
// the day they disagree a term is recognised by the first and matched by
// nothing in the second. A deny entry of that type would then be read, found not
// to match anybody, and skipped, which is the exact failure the unrecognised
// rule exists to prevent, wearing the disguise of a recognised term.
var matchers = map[TermType]func(Principal, string) bool{
	TermUser: func(p Principal, value string) bool {
		return value != "" && value == p.Subject
	},
	TermGroup: func(p Principal, value string) bool {
		return value != "" && slices.Contains(p.Groups, value)
	},
}

// A Term is one side of an entry: what it names, and which name.
type Term struct {
	Type  TermType
	Value string
}

func (t Term) String() string {
	return fmt.Sprintf("%s:%s", t.Type, t.Value)
}

// An Entry is one line of a permission set.
type Entry struct {
	Effect Effect
	Term   Term
}

// A Set is the permission entries carried with one document.
//
// Order carries no meaning. It is a set in the sense that matters here: the
// same entries in any order resolve to the same decision, which is what
// deny-over-allow being independent of order means in practice.
type Set []Entry

// A Principal is who is asking, reduced to what an entry can name.
//
// This is not the principal of #16, which additionally carries the per-source
// identifiers and which source asserted each one. That type does not exist yet.
// When it does, it is mapped into this one at the call site rather than
// imported here, so that this package keeps taking its decision from the
// entries alone.
type Principal struct {
	// Subject is the identity provider's stable subject identifier.
	Subject string
	// Groups are the groups resolved for this session.
	Groups []string
}

// A Reason is why a decision came out the way it did, in a form something
// other than a human can read.
//
// It is a fixed vocabulary rather than a sentence because the audit trail is
// meant to be queried. #40 is the issue that records these; nothing in this
// tree records anything today, and this type is the half of that which can
// exist before it.
type Reason string

const (
	// ReasonEmptySet is a document whose permission set holds no entries.
	ReasonEmptySet Reason = "empty-permission-set"
	// ReasonUnrecognisedTerm is an entry naming a term type this evaluator
	// cannot read.
	ReasonUnrecognisedTerm Reason = "unrecognised-term"
	// ReasonUnrecognisedEffect is an entry whose effect is neither allow nor
	// deny.
	ReasonUnrecognisedEffect Reason = "unrecognised-effect"
	// ReasonDeniedByEntry is a deny entry that matched.
	ReasonDeniedByEntry Reason = "denied-by-entry"
	// ReasonMatchedNothing is a set that named this principal nowhere.
	ReasonMatchedNothing Reason = "matched-nothing"
	// ReasonAllowedByEntry is an allow entry that matched with nothing denying.
	ReasonAllowedByEntry Reason = "allowed-by-entry"
)

// A Decision is the answer, with why it was reached.
//
// The zero value refuses. That is not a convenience: a Decision that arrived
// from a path nobody wrote, or from a struct somebody built and forgot to
// fill, has to be a refusal, and TestTheZeroDecisionRefuses is what holds it.
type Decision struct {
	// Allowed is the answer. False refuses.
	Allowed bool
	// Reason is why, from the fixed vocabulary above.
	Reason Reason
	// Detail names the entry or the term the reason is about, where naming one
	// is meaningful. It never carries document content.
	Detail string
}

// Evaluate resolves set against p.
//
// It returns no error, and no function in this package that returns a Decision
// returns one. That is the shape rather than an accident: an evaluator whose
// signature can hand back an allow beside an error has a caller who will read
// the first and drop the second, and the allow will be the one that survives.
// Everything that could have been an error here is a refusal with a reason
// instead. TestNoDecisionIsReturnedBesideAnError walks the source and refuses
// the other shape.
func Evaluate(p Principal, set Set) Decision {
	if len(set) == 0 {
		return deny(ReasonEmptySet, "")
	}

	// First pass: can every entry be read at all. An entry this evaluator
	// cannot parse is a restriction it cannot apply, and the document is
	// refused before any matching is attempted rather than after. Doing it
	// first is what makes the rule hold for an unreadable entry that names
	// somebody else: the question of who it names is never reached.
	//
	// The matcher comes out of the same lookup that decides recognition, and
	// the passes below read the result of that lookup rather than repeating
	// it. An entry that was not recognised therefore has no representation for
	// them to examine, so there is no arrangement of this function in which one
	// of them reaches a term type the other rejected.
	resolved := make([]resolvedEntry, 0, len(set))
	for _, entry := range set {
		match, known := matchers[entry.Term.Type]
		if !known {
			return deny(ReasonUnrecognisedTerm, entry.Term.String())
		}
		if entry.Effect != Allow && entry.Effect != Deny {
			return deny(ReasonUnrecognisedEffect, string(entry.Effect))
		}
		resolved = append(resolved, resolvedEntry{
			effect:  entry.Effect,
			term:    entry.Term,
			matched: match(p, entry.Term.Value),
		})
	}

	// Second pass: deny wins. Every deny is examined before any allow is
	// looked at, which is what makes the result independent of the order the
	// entries arrived in.
	for _, entry := range resolved {
		if entry.effect == Deny && entry.matched {
			return deny(ReasonDeniedByEntry, entry.term.String())
		}
	}

	// Third pass: an allow that matched, with nothing denying.
	for _, entry := range resolved {
		if entry.effect == Allow && entry.matched {
			return allow(entry.term.String())
		}
	}

	// Named nowhere. This is the default and it is a refusal.
	return deny(ReasonMatchedNothing, "")
}

// A resolvedEntry is one entry after it has been read: its effect, what it
// names, and whether it named the principal who is asking.
//
// It exists so that recognising an entry and matching it are one step. The
// shape it replaces looked the entry's matcher up once to decide whether the
// type was known and again to run it, which is two readings of one map that a
// later edit can put out of step, and the second reading of an entry the first
// had rejected is a nil call rather than a refusal.
type resolvedEntry struct {
	effect  Effect
	term    Term
	matched bool
}

// allow is the one place in this package that builds a permitting Decision.
//
// One site rather than several, so that the question "what can let a document
// through" has one answer a reader can hold in their head, and so that the
// source walk in the test has one thing to count.
func allow(detail string) Decision {
	return Decision{Allowed: true, Reason: ReasonAllowedByEntry, Detail: detail}
}

// deny builds a refusing Decision with the reason it was refused for.
func deny(reason Reason, detail string) Decision {
	return Decision{Allowed: false, Reason: reason, Detail: detail}
}
