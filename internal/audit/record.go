// Package audit declares the one record shape every event in this project is
// written as.
//
// One shape rather than six, because an auditor querying six shapes will query
// three of them and conclude from the answer. The fields are here so that what
// a person was able to see, and whether the filter was applied at all, are
// questions a query answers rather than questions somebody answers by reading.
//
// Nothing in this package writes anything. It declares the record, refuses one
// that could not be queried as intended, and generates docs/audit.md from that
// declaration. Where the records go, how they are made tamper-evident and how
// long they are kept are #39, #43 and #44.
package audit

import (
	"errors"
	"fmt"
	"slices"
	"time"
)

// Version is the schema this code writes.
//
// It is on every record because #45 replays records older than the code
// reading them. A reader that meets a version it does not know refuses rather
// than reading the fields it recognises and assuming the rest were absent,
// which is how a record written by a later version is silently downgraded into
// evidence of something that did not happen.
const Version = 1

// A Class is what kind of event a record is about.
//
// The complete set is here and a record carrying anything else is refused.
// Adding one is an edit to this list, which is a diff with a commit message,
// rather than a string appearing at a call site.
type Class string

const (
	// ClassAuthorisation is one decision about one principal and one object.
	ClassAuthorisation Class = "authorisation"
	// ClassSignOn is a session being established or refused.
	ClassSignOn Class = "sign-on"
	// ClassSessionEnd is a session ending, for whatever reason.
	ClassSessionEnd Class = "session-end"
	// ClassConfiguration is a change to what this deployment is configured to
	// do.
	ClassConfiguration Class = "configuration"
)

// Classes is the declared set, in the order docs/audit.md lists them.
//
// The order is fixed rather than derived from a map, because the document is
// compared byte for byte against what this declaration generates and a map
// iteration would make that comparison fail at random.
var Classes = []Class{ClassAuthorisation, ClassSignOn, ClassSessionEnd, ClassConfiguration}

// A Reason is why a decision came out as it did.
//
// It is a code from this set and never a sentence. A free-text reason is a
// field an auditor cannot query, and it is the field into which somebody
// eventually writes the name of the document.
//
// The first six are the vocabulary internal/authz already refuses with, and
// TestTheAuthorisationReasonsMatchTheEvaluator holds the two lists together.
type Reason string

const (
	// ReasonEmptySet is a document whose permission set holds no entries.
	ReasonEmptySet Reason = "empty-permission-set"
	// ReasonUnrecognisedTerm is an entry naming a term type the evaluator
	// cannot read, which is a defect signal rather than a normal refusal.
	ReasonUnrecognisedTerm Reason = "unrecognised-term"
	// ReasonUnrecognisedEffect is an entry whose effect is neither allow nor
	// deny.
	ReasonUnrecognisedEffect Reason = "unrecognised-effect"
	// ReasonDeniedByEntry is a deny entry that matched the principal.
	ReasonDeniedByEntry Reason = "denied-by-entry"
	// ReasonMatchedNothing is a permission set that named the principal
	// nowhere.
	ReasonMatchedNothing Reason = "matched-nothing"
	// ReasonAllowedByEntry is an allow entry that matched with nothing
	// denying.
	ReasonAllowedByEntry Reason = "allowed-by-entry"

	// ReasonAuthorityUnreachable is a recheck that could not be performed
	// because the source could not be asked. It is deliberately distinct from
	// every ordinary refusal above: it is the event #19 counts, and reading it
	// as an ordinary denial hides an outage inside normal traffic.
	ReasonAuthorityUnreachable Reason = "authority-unreachable"
	// ReasonGroupClaimUnmapped is a sign-on refused because a group claim
	// value had no mapping.
	ReasonGroupClaimUnmapped Reason = "group-claim-unmapped"
	// ReasonTooManyGroups is a sign-on refused for carrying more group values
	// than the policy allows.
	ReasonTooManyGroups Reason = "too-many-groups"
	// ReasonSubjectUnmapped is a principal with no identity in the source the
	// object belongs to.
	ReasonSubjectUnmapped Reason = "subject-unmapped"
)

// Reasons is the declared set, in the order docs/audit.md lists them.
var Reasons = []Reason{
	ReasonEmptySet, ReasonUnrecognisedTerm, ReasonUnrecognisedEffect,
	ReasonDeniedByEntry, ReasonMatchedNothing, ReasonAllowedByEntry,
	ReasonAuthorityUnreachable, ReasonGroupClaimUnmapped, ReasonTooManyGroups,
	ReasonSubjectUnmapped,
}

// An Outcome is what was decided.
type Outcome string

const (
	// OutcomeAllowed is access granted.
	OutcomeAllowed Outcome = "allowed"
	// OutcomeRefused is access refused.
	OutcomeRefused Outcome = "refused"
)

// Outcomes is the declared set, in the order docs/audit.md lists them.
var Outcomes = []Outcome{OutcomeAllowed, OutcomeRefused}

// A DetailKey names one piece of structured detail a record may carry.
//
// Detail is keyed rather than free because the alternative is a sentence, and
// a sentence is where document content ends up. Every key declared here names
// something that identifies or classifies; none of them names anything read
// out of a document, and TestNoDetailKeyCarriesContent is what holds that
// against a key added later.
type DetailKey string

const (
	// DetailTerm is the permission entry a decision was about, as the
	// evaluator's own term rendering.
	DetailTerm DetailKey = "term"
	// DetailSource is the source system an object belongs to.
	DetailSource DetailKey = "source"
	// DetailClaim is the name of a claim, never its value.
	DetailClaim DetailKey = "claim"
	// DetailCount is a number, as text.
	DetailCount DetailKey = "count"
	// DetailLayer is which layer took the decision, the index-time filter or
	// the point-of-use recheck.
	DetailLayer DetailKey = "layer"
)

// DetailKeys is the declared set, in the order docs/audit.md lists them.
var DetailKeys = []DetailKey{DetailTerm, DetailSource, DetailClaim, DetailCount, DetailLayer}

// A Layer is which of the two layers in
// docs/decisions/0003-permission-model.md took a decision.
type Layer string

const (
	// LayerIndexFilter is the permission predicate inside the search query.
	LayerIndexFilter Layer = "index-filter"
	// LayerPointOfUse is the recheck immediately before a document is shown or
	// placed in a model's context.
	LayerPointOfUse Layer = "point-of-use"
)

// Layers is the declared set, in the order docs/audit.md lists them.
var Layers = []Layer{LayerIndexFilter, LayerPointOfUse}

// maxDetailValue is how long one detail value may be.
//
// It is a bound rather than a preference. A field with no bound is a field a
// paragraph fits in, and a paragraph is how document content arrives in a
// record that declared it would carry none.
const maxDetailValue = 256

// An Object is the thing acted on.
type Object struct {
	// Source is the source system it came from.
	Source string
	// ID is its identifier in that source. It is an identifier and never a
	// title, a path fragment or an extract.
	ID string
}

// A Principal is who acted, as a record identifies them.
type Principal struct {
	// Subject is the provider's stable subject identifier.
	Subject string
	// Session is which session they were in.
	Session string
}

// A Record is one event.
//
// Every field is a declared type rather than a bare string, apart from the
// three that carry identifiers supplied from outside this process. That is not
// tidiness: a bare string field is a field somebody writes a sentence into, and
// TestNoFieldTakesFreeText reads this declaration and refuses a new one.
type Record struct {
	// Version is the schema this record was written under.
	Version int
	// ID identifies this record.
	ID string
	// At is when the event happened, from the clock stated in docs/audit.md.
	At time.Time
	// Class is what kind of event it was.
	Class Class
	// Principal is who acted.
	Principal Principal
	// SourceAddress is where the request came from.
	SourceAddress string
	// Object is what was acted on.
	Object Object
	// Outcome is what was decided.
	Outcome Outcome
	// Reason is why, as a code.
	Reason Reason
	// Detail is structured detail under declared keys.
	Detail map[DetailKey]string
	// Correlation ties together everything that happened while answering one
	// request.
	Correlation string
}

// ErrUnknownVersion is a record written under a schema this code does not
// know.
var ErrUnknownVersion = errors.New("record version is not one this code knows")

// Validate refuses a record that could not be queried as this package
// promises.
//
// It refuses rather than repairing. A record repaired on the way in is a
// record that says something nobody chose, and it is indistinguishable
// afterwards from one that was right.
func (r Record) Validate() error {
	if r.Version != Version {
		return fmt.Errorf("%w: %d, this code writes %d", ErrUnknownVersion, r.Version, Version)
	}
	if r.ID == "" {
		return errors.New("record has no identifier")
	}
	if r.At.IsZero() {
		return errors.New("record has no time")
	}
	if !slices.Contains(Classes, r.Class) {
		return fmt.Errorf("event class %q is not declared", r.Class)
	}
	if r.Correlation == "" {
		return errors.New("record carries no correlation identifier, so it cannot be gathered with the rest of its request")
	}
	if r.Outcome != "" && !slices.Contains(Outcomes, r.Outcome) {
		return fmt.Errorf("outcome %q is not declared", r.Outcome)
	}
	if r.Reason != "" && !slices.Contains(Reasons, r.Reason) {
		return fmt.Errorf("reason %q is not declared", r.Reason)
	}
	if r.Class == ClassAuthorisation {
		if r.Outcome == "" {
			return errors.New("an authorisation record states no outcome")
		}
		if r.Reason == "" {
			return errors.New("an authorisation record states no reason")
		}
	}
	for key, value := range r.Detail {
		if !slices.Contains(DetailKeys, key) {
			return fmt.Errorf("detail key %q is not declared", key)
		}
		if len(value) > maxDetailValue {
			return fmt.Errorf("detail %q is %d bytes, and no detail value may exceed %d", key, len(value), maxDetailValue)
		}
	}
	if layer, carried := r.Detail[DetailLayer]; carried && !slices.Contains(Layers, Layer(layer)) {
		return fmt.Errorf("layer %q is not declared", layer)
	}
	return nil
}
