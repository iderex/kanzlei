# The audit record

Generated from the declaration in `internal/audit/record.go`. Do not edit
this file by hand; change the declaration and regenerate it:

    go test ./internal/audit -run TestTheDocumentIsGeneratedFromTheDeclaration -update

`TestTheDocumentIsGeneratedFromTheDeclaration` compares the two, so a
declaration that changed without this file being regenerated reds the suite.

Schema version 1.

## Fields

| Field | Type | Meaning |
| --- | --- | --- |
| `Version` | `int` | The schema this record was written under |
| `ID` | `string` | Identifies this record |
| `At` | `time.Time` | When the event happened, from the clock stated in docs/audit.md |
| `Class` | `Class` | What kind of event it was |
| `Principal` | `Principal` | Who acted |
| `SourceAddress` | `string` | Where the request came from |
| `Object` | `Object` | What was acted on |
| `Outcome` | `Outcome` | What was decided |
| `Reason` | `Reason` | Why, as a code |
| `Detail` | `map[DetailKey]string` | Structured detail under declared keys |
| `Correlation` | `string` | Ties together everything that happened while answering one request |

### `Principal`

| Field | Type | Meaning |
| --- | --- | --- |
| `Subject` | `string` | The provider's stable subject identifier |
| `Session` | `string` | Which session they were in |

### `Object`

| Field | Type | Meaning |
| --- | --- | --- |
| `Source` | `string` | The source system it came from |
| `ID` | `string` | Its identifier in that source |

## Event classes

| Value | Meaning |
| --- | --- |
| `authorisation` | One decision about one principal and one object |
| `sign-on` | A session being established or refused |
| `session-end` | A session ending, for whatever reason |
| `configuration` | A change to what this deployment is configured to do |

## Outcomes

| Value | Meaning |
| --- | --- |
| `allowed` | Access granted |
| `refused` | Access refused |

## Reasons

| Value | Meaning |
| --- | --- |
| `empty-permission-set` | A document whose permission set holds no entries |
| `unrecognised-term` | An entry naming a term type the evaluator cannot read, which is a defect signal rather than a normal refusal |
| `unrecognised-effect` | An entry whose effect is neither allow nor deny |
| `denied-by-entry` | A deny entry that matched the principal |
| `matched-nothing` | A permission set that named the principal nowhere |
| `allowed-by-entry` | An allow entry that matched with nothing denying |
| `authority-unreachable` | A recheck that could not be performed because the source could not be asked |
| `group-claim-unmapped` | A sign-on refused because a group claim value had no mapping |
| `too-many-groups` | A sign-on refused for carrying more group values than the policy allows |
| `subject-unmapped` | A principal with no identity in the source the object belongs to |

## Detail keys

| Value | Meaning |
| --- | --- |
| `term` | The permission entry a decision was about, as the evaluator's own term rendering |
| `source` | The source system an object belongs to |
| `claim` | The name of a claim, never its value |
| `count` | A number, as text |
| `layer` | Which layer took the decision, the index-time filter or the point-of-use recheck |

## Layers

| Value | Meaning |
| --- | --- |
| `index-filter` | The permission predicate inside the search query |
| `point-of-use` | The recheck immediately before a document is shown or placed in a model's context |

## What this document does not say

Where records are written, how they are made tamper-evident and how long
they are kept are #39, #43 and #44. Nothing in `internal/audit` writes a
record today, so every field above is a shape rather than a thing an
operator can query yet.
