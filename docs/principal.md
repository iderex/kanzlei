# Who a user is here, and how they map into a source

A permission check compares a principal against a permission. If the principal
is wrong, the check is decorative, and it is decorative in the direction that
grants access rather than the one that refuses it.

The difficulty is that the systems involved do not agree on how to name a
person. An identity provider issues a stable subject identifier. A file server
knows a security identifier. A groupware system knows an account name. A mail
store knows an address. The same human is all four. Everything below is about
what may be used to join them up.

## What a principal holds

The provider's stable subject identifier, the groups resolved for this session,
the moment that resolution happened, and the per-source identifiers resolved
for this user, each carrying how it was established.

The per-source identifiers are reached through `IdentityIn`, which reports
whether the source names this user at all. They are not a map a caller can
read, because a caller that reads the map, finds nothing, and carries on with
the subject identifier has just invented a mapping. Finding nothing has to be a
branch somebody wrote.

## Where a mapping may come from

Two places, and no third.

An operator writes it down. Every field is required: which subject, which
source, which identifier there. A line missing any of them is refused rather
than completed from context, and one subject mapped into one source twice is
refused rather than resolved by whichever line came last.

The source asserts it about its own user. That is a fact from the system that
owns the name, and it is recorded as having come from there, so a consumer can
tell an operator's claim from the source's own answer.

## What a mapping may never come from

A matching display name. A matching mail address the source did not itself
assert. Anything else two people can share, or an administrator can set.

Two users with the same display name is an ordinary state of affairs in a
directory of any size. An address is a value an administrator can write into
either system, so a join on it is a join on a field somebody else controls.
Either one produces a mapping that looks right in every test somebody thinks to
write and is wrong for one person in a way nothing surfaces.

`Claims` carries a display name and an address because a provider issues them
and something above this package will want to show them to a person. So the
rule cannot be enforced by their absence. `TestNothingMapsFromADisplayNameOrAnAddress`
reads `internal/auth/principal.go` and refuses a read of either inside the
functions that decide or carry a mapping, which is where the failure arrives:
one line added to a function that already has the claims in hand, matching on
what looks like the same person.

That check has a limit written beside it. It names the mapping functions
explicitly, so a mapping decided somewhere it does not name is not read, and
`TestTheInferenceRuleNamesTheFunctionsThatExist` refuses the case where a
rename leaves it looking for something that is no longer there.

## No mapping is no access

A user with no identity in a source is a user that source has never heard of.
The answer is no access to that source. Never a fallback to the subject
identifier, never a broader principal, never a guess.

`ForSource` reports that as a second result, and the principal it returns
alongside a false is the zero value, which names nobody. A caller that drops
the second result therefore gets refusals rather than breadth:
`TestIgnoringTheSecondResultStillRefuses` evaluates that zero principal against
a permission set that would allow the real user and against an empty set, and
both refuse.

## The mapping is load bearing

`TestChangingOnlyTheMappingChangesWhatIsPermitted` holds the same claims and
the same two documents across three runs and changes one line of mapping
configuration between them. The permitted set moves each time, and with the
mapping removed it is empty.

What that measures is which documents `internal/authz` admits for the mapped
principal. It is not which documents a retrieval route returns, because there
is no retrieval route in this tree yet. #62 is where that layer arrives.

## Group membership and how old it may be

Membership is resolved once per session, at a stated moment the principal
carries, and `MaxGroupAge` is fifteen minutes.

That number is a bound on one thing: how long a group removed at the provider
goes on being honoured by a session that is already open. It is not the whole
of that exposure. A session nobody uses is never re-resolved, because nothing
asks, and the point-of-use recheck in
`docs/decisions/0003-permission-model.md` is what stands behind the interval
rather than this number.

The done-when of #16 asks for that age to be stated in the decision record of
#15. It is not there. That record is written and accepted and does not mention
a maximum age, so the number lives here for now and the note in #16 records
that the record still owes it.

Nothing enforces the age. `GroupsAreStale` answers the question and no route
asks it, because what to do about a stale session belongs to whoever holds
sessions, which is #31.

## What is not decided here

Group names pass through as the provider issued them. A source that names its
groups differently sees group entries matching nothing, which errs towards
refusal. Mapping a group claim into a source's own groups, and refusing a claim
that maps into none, is #30.

Nested groups are flattened before they reach here, or the connector declares
them unresolved. This package has no way to tell the two apart, and the
fidelity declaration that would carry the difference is #20. Until it exists, a
connector that silently flattens a nesting it could not fully resolve produces
a principal this package cannot distinguish from a correct one.

Nothing here reads a source system, and nothing here holds a clock. The moment
a session resolved at is passed in. Both are what keep this package testable
without a provider present, and both are why it is only the naming half of the
question.
