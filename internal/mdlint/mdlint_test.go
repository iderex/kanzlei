package mdlint

import (
	"fmt"
	"strings"
	"testing"
)

// clean is a document in the shape this repository writes, small enough to
// read and wide enough to carry one of everything the rules look at: a title,
// a section under it, a fenced block, an indented command block, a hyphenated
// list, and a sentence opening with an issue reference.
//
// Every case below is this document with one thing changed, so a refusal is
// about the change and not about the document. It is written out here rather
// than read from the repository: a case judging against the real tree proves
// the state of the tree on the day it ran and not the rule.
const clean = `# A title

Prose under it.

## A section

` + "```" + `
a fenced block
` + "```" + `

    an indented command block

- a list item
- another one

#113 opens this sentence and is not a heading.
`

func check(t *testing.T, body string) []Finding {
	t.Helper()
	findings, read := Check([]File{{Name: "docs/example.md", Bytes: []byte(body)}})
	if read.Documents != 1 {
		t.Fatalf("want one document read, got %d", read.Documents)
	}
	return findings
}

// The document every other case is derived from passes, which is what makes
// each of those cases a statement about the one line it changed.
func TestTheShapeThisRepositoryWritesPasses(t *testing.T) {
	if findings := check(t, clean); len(findings) != 0 {
		t.Fatalf("the clean document was refused: %v", findings)
	}
}

// A sentence that opens with an issue reference is prose, and seven of this
// repository's own documents write one. A rule taking a leading hash for a
// heading would refuse every one of them, so the case is here rather than
// left to the clean document to carry silently.
func TestASentenceOpeningWithAnIssueReferenceIsNotAHeading(t *testing.T) {
	for _, body := range []string{
		"# A title\n\n#52 has to propagate a revocation faster than the next full sync.\n",
		"# A title\n\n#18.\n",
	} {
		if findings := check(t, body); len(findings) != 0 {
			t.Fatalf("%q was refused: %v", body, findings)
		}
	}
}

// The bite for each rule, shown by running rather than asserted, and shown
// twice: the departure is refused with its line, and the same document with
// the one character put back is green. The second half is what says the
// refusal was about the rule and not about the surrounding document.
func TestEachRuleRefusesItsDepartureAndPassesTheRepair(t *testing.T) {
	for _, c := range departures() {
		t.Run(c.name, func(t *testing.T) {
			body := strings.Replace(clean, c.from, c.to, 1)
			if body == clean {
				t.Fatalf("the case changed nothing: %q is not in the document", c.from)
			}

			var got []string
			for _, f := range check(t, body) {
				got = append(got, fmt.Sprintf("%s:%d", f.Rule, f.Line))
				if !strings.Contains(f.String(), "docs/example.md") {
					t.Fatalf("the finding does not name its document: %s", f)
				}
			}
			if strings.Join(got, " ") != strings.Join(c.want, " ") {
				t.Fatalf("want %v, got %v", c.want, got)
			}

			// The repair. Putting the document back is the same edit in the
			// other direction, and the run has to be green afterwards.
			if findings := check(t, strings.Replace(body, c.to, c.from, 1)); len(findings) != 0 {
				t.Fatalf("the repaired document was refused: %v", findings)
			}
		})
	}
}

// A departure is the clean document with one thing changed, and what the run
// has to say about it, written as the rule and the line it is reported on.
type departure struct {
	name string
	from string
	to   string
	want []string
}

func departures() []departure {
	return []departure{
		{
			name: "a fence opened and closed with tildes",
			from: "```\na fenced block\n```",
			to:   "~~~\na fenced block\n~~~",
			// Both lines, because both are the marker this repository does
			// not write and either one alone desynchronises a reader that
			// tracks the other.
			want: []string{RuleFenceMarker + ":7", RuleFenceMarker + ":9"},
		},
		{
			name: "a fence that is never closed",
			from: "```\na fenced block\n```",
			to:   "```\na fenced block",
			// Reported at the line that opened it rather than at the end of
			// the document, because that is the line somebody has to edit.
			want: []string{RuleUnclosedFence + ":7"},
		},
		{
			name: "a heading written as an underline",
			from: "## A section",
			to:   "A section\n---------",
			want: []string{RuleSetextHeading + ":6"},
		},
		{
			name: "a heading closed with a trailing hash run",
			from: "## A section",
			to:   "## A section ##",
			want: []string{RuleClosedHeading + ":5"},
		},
		{
			name: "a heading two levels below the one above it",
			from: "## A section",
			to:   "### A section",
			want: []string{RuleHeadingLevelSkip + ":5"},
		},
		{
			name: "a heading glued to the paragraph above it",
			from: "Prose under it.\n\n## A section",
			to:   "Prose under it.\n## A section",
			want: []string{RuleHeadingBlankLine + ":4"},
		},
		{
			name: "a list item written with an asterisk",
			from: "- a list item",
			to:   "* a list item",
			want: []string{RuleListMarker + ":13"},
		},
	}
}

// A rule with no case in front of it is a rule nobody can prove bites, and a
// rule that exists only in a case is a finding nobody can act on. This holds
// both directions closed: every rule the package declares is reached by one of
// the departures above, and the departures reach nothing else.
func TestEveryRuleIsReachedByACaseAndTheCasesReachNothingElse(t *testing.T) {
	reached := map[string]bool{}
	for _, c := range departures() {
		for _, f := range check(t, strings.Replace(clean, c.from, c.to, 1)) {
			reached[f.Rule] = true
		}
	}

	for _, rule := range []string{
		RuleFenceMarker,
		RuleUnclosedFence,
		RuleSetextHeading,
		RuleClosedHeading,
		RuleHeadingLevelSkip,
		RuleHeadingBlankLine,
		RuleListMarker,
	} {
		if !reached[rule] {
			t.Errorf("no case reaches %s, so nothing proves it bites", rule)
		}
		delete(reached, rule)
	}
	for rule := range reached {
		t.Errorf("a case reached %s, which is not in the rule set this test knows about", rule)
	}
}

// What the rules deliberately do not reach. Each of these would be a finding
// if the reading were wider, and each is a shape this repository's own
// documents carry.
func TestWhatIsInsideABlockIsNotRead(t *testing.T) {
	// A document showing markdown is showing it. Every departure above,
	// written inside a fence, is the example rather than the defect.
	body := "# A title\n\n```\n" +
		"A section\n---------\n" +
		"## Closed ##\n" +
		"* an asterisk item\n" +
		"```\n"
	if findings := check(t, body); len(findings) != 0 {
		t.Fatalf("a fenced example was read as prose: %v", findings)
	}

	// The same for an indented command block, which is how this repository
	// writes a command.
	indented := "# A title\n\n" +
		"    * not a list item\n" +
		"    ## not a heading\n"
	if findings := check(t, indented); len(findings) != 0 {
		t.Fatalf("an indented block was read as prose: %v", findings)
	}
}

func TestAThematicBreakIsNotASetextHeading(t *testing.T) {
	// A run of hyphens after a blank line is a thematic break, and after a
	// list item it is the list's own punctuation. Neither is a heading.
	for _, body := range []string{
		"# A title\n\nProse.\n\n---\n\nMore prose.\n",
		"# A title\n\n- an item\n---\n",
	} {
		if findings := check(t, body); len(findings) != 0 {
			t.Fatalf("%q was refused: %v", body, findings)
		}
	}
}

func TestAHeadingWhoseTextEndsInAHashIsNotClosed(t *testing.T) {
	if findings := check(t, "# A title\n\n## Writing C#\n\nProse.\n"); len(findings) != 0 {
		t.Fatalf("a heading naming a language was refused: %v", findings)
	}
}

// The first heading sets the baseline rather than being required to be a
// level one. A pull-request template has no title of its own and opens at a
// level two, which is deliberate and not a departure.
func TestTheFirstHeadingSetsTheBaseline(t *testing.T) {
	if findings := check(t, "## Issue\n\nProse.\n\n## Evidence\n\nProse.\n"); len(findings) != 0 {
		t.Fatalf("a document opening at a level two was refused: %v", findings)
	}
}

func TestGoingBackUpSeveralLevelsIsNotASkip(t *testing.T) {
	body := "# A title\n\n## A section\n\n### A subsection\n\n## Another section\n\nProse.\n"
	if findings := check(t, body); len(findings) != 0 {
		t.Fatalf("a document returning to a level two was refused: %v", findings)
	}
}

// A run over more than one document reports each finding against its own file
// and orders the output, so the same tree prints the same run twice.
func TestFindingsCarryTheirFileAndAreOrdered(t *testing.T) {
	findings, read := Check([]File{
		{Name: "docs/second.md", Bytes: []byte("# A title\n\n* an item\n")},
		{Name: "docs/first.md", Bytes: []byte("# A title\n\n+ an item\n")},
	})
	if read.Documents != 2 {
		t.Fatalf("want two documents read, got %d", read.Documents)
	}
	if len(findings) != 2 {
		t.Fatalf("want two findings, got %d: %v", len(findings), findings)
	}
	if findings[0].File != "docs/first.md" || findings[1].File != "docs/second.md" {
		t.Fatalf("the findings are not ordered by file: %v", findings)
	}
	if !strings.Contains(findings[0].String(), `"+"`) {
		t.Fatalf("the finding does not name the marker it refused: %s", findings[0])
	}
}

// An empty document is read and reported as read. It is not a departure, and
// it is not a run that examined nothing.
func TestAnEmptyDocumentIsRead(t *testing.T) {
	findings, read := Check([]File{{Name: "docs/empty.md", Bytes: nil}})
	if len(findings) != 0 {
		t.Fatalf("an empty document was refused: %v", findings)
	}
	if read.Documents != 1 {
		t.Fatalf("want one document read, got %d", read.Documents)
	}
	if read.Headings != 0 {
		t.Fatalf("want no headings, got %d", read.Headings)
	}
}

func TestTheReadingCountsWhatItExamined(t *testing.T) {
	_, read := Check([]File{{Name: "docs/example.md", Bytes: []byte(clean)}})
	if read.Headings != 2 {
		t.Fatalf("want two headings, got %d", read.Headings)
	}
	if read.Lines != strings.Count(strings.TrimSuffix(clean, "\n"), "\n")+1 {
		t.Fatalf("the line count does not match the document: %d", read.Lines)
	}
	// Prose is the subset read as prose, so it is short of the whole document
	// by the fenced and indented lines the rules pass over.
	if read.Prose >= read.Lines {
		t.Fatalf("every line was read as prose, so no block was passed over: %d of %d", read.Prose, read.Lines)
	}
}
