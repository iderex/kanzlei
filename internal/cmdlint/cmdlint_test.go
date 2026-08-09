package cmdlint

import (
	"strings"
	"testing"
)

// clean is a document in the shape this repository writes, small enough to
// read and wide enough to carry one of everything the rule looks at: an
// indented block holding a command and the output it produced, a continued
// command, a fenced block that declares nothing, and a sentence opening with
// an issue reference.
//
// Every case below is this document with one thing changed, so a refusal is
// about the change and not about the document. It is written out here rather
// than read from the repository: a case judging against the real tree proves
// the state of the tree on the day it ran and not the rule.
const clean = "# A title\n" +
	"\n" +
	"Prose under it.\n" +
	"\n" +
	"    go run ./cmd/linkcheck\n" +
	"    linkcheck: 0 external link(s) across this repository's documents\n" +
	"\n" +
	"More prose.\n" +
	"\n" +
	"    gh api repos/iderex/kanzlei/rulesets/20487686 \\\n" +
	"      --jq '{enforcement, required: [.rules[].type]}'\n" +
	"\n" +
	"#113 opens this sentence and is prose.\n" +
	"\n" +
	"```\n" +
	"a fenced block that declares nothing\n" +
	"```\n"

func check(t *testing.T, body string) ([]Finding, Reading) {
	t.Helper()
	findings, read := Check([]File{{Name: "docs/example.md", Bytes: []byte(body)}})
	if read.Documents != 1 {
		t.Fatalf("want one document read, got %d", read.Documents)
	}
	return findings, read
}

// refusals is the same call for a case about what was refused rather than
// about what was read. It exists so that no case discards the reading into a
// blank identifier, which is what internal/sourcecheck refuses: the last
// return value is the error by convention, and a rule that made an exception
// for a test file would be a rule with a hole in it.
func refusals(t *testing.T, body string) []Finding {
	t.Helper()
	findings, read := Check([]File{{Name: "docs/example.md", Bytes: []byte(body)}})
	if read.Documents != 1 {
		t.Fatalf("want one document read, got %d", read.Documents)
	}
	return findings
}

// The document every other case is derived from passes, which is what makes
// each of those cases a statement about the one thing it changed.
func TestTheShapeThisRepositoryWritesPasses(t *testing.T) {
	findings, read := check(t, clean)
	if len(findings) != 0 {
		t.Fatalf("the clean document was refused: %v", findings)
	}
	if read.Commands != 1 {
		t.Fatalf("want the one continued command read, got %d", read.Commands)
	}
	if read.Lines != 2 {
		t.Fatalf("want the continued command's two lines read, got %d", read.Lines)
	}
	if read.Passed != 2 {
		t.Fatalf("want the command and its output passed over, got %d", read.Passed)
	}
	if read.Blocks != 2 {
		t.Fatalf("want the two indented blocks counted, got %d", read.Blocks)
	}
}

// The reason the boundary is where it is. A block here holds a command and the
// output it produced with nothing between them saying which is which, and
// output carries apostrophes, unmatched brackets and anything else a program
// printed. Reading every line as shell would refuse a document for a sentence
// a program wrote.
func TestOutputInsideABlockIsNotReadAsACommand(t *testing.T) {
	for _, out := range []string{
		"    doclint: 98 path reference(s) across 20 of this repository's documents",
		"    prhygiene: the body names no issue, so it doesn't link to one",
		"    error: unexpected ) at line 4",
		"    coverage 90.2% clears the floor of 90.1% (1127 of 1249 statements",
	} {
		body := "# A title\n\nProse.\n\n    go run ./cmd/doclint\n" + out + "\n"
		findings, read := check(t, body)
		if len(findings) != 0 {
			t.Fatalf("output was judged as shell: %q gave %v", out, findings)
		}
		if read.Commands != 0 {
			t.Fatalf("output was counted as a command: %q", out)
		}
		if read.Passed != 2 {
			t.Fatalf("want both lines reported as passed over, got %d for %q", read.Passed, out)
		}
	}
}

// The bite, and it is the one-character mistake somebody actually makes: the
// closing quote of a continued command deleted with the argument it ended.
func TestAContinuedCommandWhoseQuoteNeverClosesIsRefused(t *testing.T) {
	broken := strings.Replace(clean,
		"      --jq '{enforcement, required: [.rules[].type]}'\n",
		"      --jq '{enforcement, required: [.rules[].type]}\n", 1)
	if broken == clean {
		t.Fatal("the case did not change the document")
	}

	findings := refusals(t, broken)
	if len(findings) != 1 {
		t.Fatalf("want one finding, got %v", findings)
	}
	if findings[0].Rule != RuleUnbalancedQuote {
		t.Fatalf("want %s, got %s", RuleUnbalancedQuote, findings[0].Rule)
	}
	// The line is the one a reader would start copying from, not the one the
	// walk stopped on.
	if findings[0].Line != 10 {
		t.Fatalf("want the finding at the first line of the command, got %d", findings[0].Line)
	}

	// The same document with the character put back is green, which is what
	// says the refusal was about the quote and not about the document.
	if findings := refusals(t, clean); len(findings) != 0 {
		t.Fatalf("the repaired document was refused: %v", findings)
	}
}

// The truncation this rule is named for: a multi-line command whose last line
// was dropped, leaving the backslash behind.
func TestACommandStillContinuedWhenItsBlockEndsIsRefused(t *testing.T) {
	body := "# A title\n\nProse.\n\n    gh api repos/iderex/kanzlei/rulesets \\\n\nMore prose.\n"
	findings := refusals(t, body)
	if len(findings) != 1 || findings[0].Rule != RuleUnclosedContinuation {
		t.Fatalf("want one %s, got %v", RuleUnclosedContinuation, findings)
	}
	if findings[0].Line != 5 {
		t.Fatalf("want the finding at the line the command begins on, got %d", findings[0].Line)
	}

	// One character apart: the backslash removed makes it a single line in an
	// undeclared block, which is not read at all.
	whole := "# A title\n\nProse.\n\n    gh api repos/iderex/kanzlei/rulesets\n\nMore prose.\n"
	if findings := refusals(t, whole); len(findings) != 0 {
		t.Fatalf("the repaired document was refused: %v", findings)
	}
}

// A document that ends inside its own block is the same failure with nothing
// after it to close the block.
func TestACommandStillContinuedWhenTheDocumentEndsIsRefused(t *testing.T) {
	body := "# A title\n\nProse.\n\n    go run ./cmd/doclint \\\n"
	findings := refusals(t, body)
	if len(findings) != 1 || findings[0].Rule != RuleUnclosedContinuation {
		t.Fatalf("want one %s, got %v", RuleUnclosedContinuation, findings)
	}
}

func TestASubstitutionThatNeverClosesIsRefused(t *testing.T) {
	body := "# A title\n\nProse.\n\n    git log \"$(git merge-base origin/main HEAD..HEAD\" \\\n      --format=%s\n"
	findings := refusals(t, body)
	if len(findings) != 1 || findings[0].Rule != RuleUnclosedSubstitution {
		t.Fatalf("want one %s, got %v", RuleUnclosedSubstitution, findings)
	}

	// The parenthesis put back, one character, and the command is whole.
	whole := "# A title\n\nProse.\n\n    git log \"$(git merge-base origin/main HEAD)..HEAD\" \\\n      --format=%s\n"
	if findings := refusals(t, whole); len(findings) != 0 {
		t.Fatalf("the repaired document was refused: %v", findings)
	}
}

// A closing parenthesis with no opener is ordinary shell, in a case arm and in
// a subshell written another way, so only the unclosed direction is refused.
func TestAClosingParenthesisWithNoOpenerIsNotRefused(t *testing.T) {
	body := "# A title\n\nProse.\n\n    case $x in a) echo one ;; esac \\\n      && echo done\n"
	if findings := refusals(t, body); len(findings) != 0 {
		t.Fatalf("ordinary shell was refused: %v", findings)
	}
}

func TestACommandEndingInAnOperatorWithNothingAfterItIsRefused(t *testing.T) {
	for _, op := range []string{"|", "&&", "||"} {
		body := "# A title\n\nProse.\n\n    git ls-files \\\n      -z " + op + "\n"
		findings := refusals(t, body)
		if len(findings) != 1 || findings[0].Rule != RuleTruncatedPipeline {
			t.Fatalf("want one %s for %q, got %v", RuleTruncatedPipeline, op, findings)
		}
		if !strings.Contains(findings[0].Detail, op) {
			t.Fatalf("the finding does not say which operator: %q", findings[0].Detail)
		}
	}
}

// A single ampersand backgrounds the command and is a complete line, so it is
// not the same shape as a pipeline with nothing after it.
func TestACommandEndingInASingleAmpersandIsNotRefused(t *testing.T) {
	body := "# A title\n\nProse.\n\n    go run ./cmd/kanzlei \\\n      -addr 127.0.0.1:8080 &\n"
	if findings := refusals(t, body); len(findings) != 0 {
		t.Fatalf("a backgrounded command was refused: %v", findings)
	}
}

// This repository's documents open sentences and comments with an issue
// reference, and a hash inside a word is not a comment. A rule that read one
// as a comment would drop the rest of the command and stop seeing what did not
// close in it.
func TestAHashInsideAWordDoesNotOpenAComment(t *testing.T) {
	body := "# A title\n\nProse.\n\n    gh issue view 113 --json body \\\n      --jq '.body' | grep issue#113 | grep -c \"'\"\n"
	if findings := refusals(t, body); len(findings) != 0 {
		t.Fatalf("an issue reference inside a word was read as a comment: %v", findings)
	}

	// A hash that does open one takes the rest of the command with it, so an
	// apostrophe written in the reason beside a command is prose.
	commented := "# A title\n\nProse.\n\n    go test ./... \\\n      -count=1 # the suite's own run, not the gate's\n"
	if findings := refusals(t, commented); len(findings) != 0 {
		t.Fatalf("a reason comment was judged as shell: %v", findings)
	}
}

// Inside single quotes nothing is special. A backslash there is literal text,
// and a rule treating it as an escape would lose the quote that closes.
func TestNothingIsSpecialInsideSingleQuotes(t *testing.T) {
	body := "# A title\n\nProse.\n\n    grep -n 'a\\' -- . \\\n      | wc -l\n"
	if findings := refusals(t, body); len(findings) != 0 {
		t.Fatalf("a literal backslash inside single quotes was read as an escape: %v", findings)
	}
}

// A line ending in two backslashes ends in an escaped backslash and continues
// nothing, so the line after it is output rather than the rest of a command.
func TestAnEscapedBackslashDoesNotContinueTheLine(t *testing.T) {
	body := "# A title\n\nProse.\n\n    grep -c 'a' -- . \\\\\n    the line after it is output and holds an apostrophe: doesn't\n"
	findings, read := check(t, body)
	if len(findings) != 0 {
		t.Fatalf("an escaped backslash was read as a continuation: %v", findings)
	}
	if read.Commands != 0 {
		t.Fatalf("want nothing read as a command, got %d", read.Commands)
	}
}

// A block that says what it holds is the block that claims to be a command,
// and every line in one is read rather than only a continued run.
func TestABlockThatDeclaresAShellHasEveryLineRead(t *testing.T) {
	body := "# A title\n\nProse.\n\n```sh\ngo run ./cmd/doclint\ngh api repos/iderex/kanzlei --jq '.license\n```\n"
	findings, read := check(t, body)
	if len(findings) != 1 || findings[0].Rule != RuleUnbalancedQuote {
		t.Fatalf("want one %s, got %v", RuleUnbalancedQuote, findings)
	}
	if read.Commands != 2 {
		t.Fatalf("want both lines read as commands, got %d", read.Commands)
	}
	if read.Passed != 0 {
		t.Fatalf("want nothing passed over inside a declared block, got %d", read.Passed)
	}
}

// A fence that declares nothing claims nothing, so it is passed over whole. A
// document showing shell that it is not asking anybody to run is the case
// this protects.
func TestAFenceThatDeclaresNothingIsNotRead(t *testing.T) {
	body := "# A title\n\nProse.\n\n```\ngh api repos/iderex/kanzlei --jq '.license\n```\n"
	findings, read := check(t, body)
	if len(findings) != 0 {
		t.Fatalf("an undeclared fence was read: %v", findings)
	}
	if read.Blocks != 0 || read.Commands != 0 || read.Passed != 0 {
		t.Fatalf("an undeclared fence was counted: %+v", read)
	}
}

// A blank line inside an indented block does not end it. Ending it there would
// split one block in two and report a continuation as unclosed across a gap a
// markdown reader does not see.
func TestABlankLineDoesNotEndAnIndentedBlock(t *testing.T) {
	body := "# A title\n\nProse.\n\n    go run ./cmd/doclint \\\n\n      -root .\n\nMore prose.\n"
	findings, read := check(t, body)
	if len(findings) != 0 {
		t.Fatalf("a continuation across a blank line was refused: %v", findings)
	}
	if read.Blocks != 1 {
		t.Fatalf("want one block, got %d", read.Blocks)
	}
}

// A tab is an indent too, because .editorconfig indents some paths with one
// and a rule reading only spaces would pass over those blocks in silence.
func TestATabIndentIsABlock(t *testing.T) {
	body := "# A title\n\nProse.\n\n\tgo run ./cmd/doclint \\\n\t  --jq '.license\n"
	findings := refusals(t, body)
	if len(findings) != 1 || findings[0].Rule != RuleUnbalancedQuote {
		t.Fatalf("want one %s from a tab-indented block, got %v", RuleUnbalancedQuote, findings)
	}
}

// Findings are ordered by file and then by line, so a run's output is stable
// and a diff of two runs is about what changed rather than about map order.
func TestFindingsAreOrderedByFileAndThenByLine(t *testing.T) {
	first := "# A title\n\nProse.\n\n    go run ./a \\\n      --jq '.x\n\nMore.\n\n    go run ./b \\\n      --jq '.y\n"
	findings, read := Check([]File{
		{Name: "docs/second.md", Bytes: []byte(first)},
		{Name: "docs/first.md", Bytes: []byte(first)},
	})
	if read.Documents != 2 {
		t.Fatalf("want both documents read, got %d", read.Documents)
	}
	if len(findings) != 4 {
		t.Fatalf("want four findings, got %v", findings)
	}
	want := []string{"docs/first.md:5", "docs/first.md:10", "docs/second.md:5", "docs/second.md:10"}
	for i, w := range want {
		if !strings.HasPrefix(findings[i].String(), w+":") {
			t.Fatalf("finding %d is %q, want it to start %q", i, findings[i].String(), w)
		}
	}
}

// A run over no document is not a run that found nothing, and the reading is
// what says so.
func TestTheReadingCountsAcrossDocuments(t *testing.T) {
	_, read := Check([]File{
		{Name: "a.md", Bytes: []byte(clean)},
		{Name: "b.md", Bytes: []byte(clean)},
	})
	if read.Documents != 2 || read.Commands != 2 || read.Blocks != 4 {
		t.Fatalf("the reading does not add up across documents: %+v", read)
	}
	if !strings.Contains(read.String(), "2 tracked document(s)") {
		t.Fatalf("the reading does not say what it read: %q", read.String())
	}
}
