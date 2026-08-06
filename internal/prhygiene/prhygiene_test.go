package prhygiene_test

import (
	"strings"
	"testing"

	"github.com/iderex/kanzlei/internal/prhygiene"
)

// acceptable is the pull request the near misses are one edit away from. Every
// case below starts here and changes one thing, which is what makes the
// refusals evidence about that one thing.
func acceptable() prhygiene.Request {
	return prhygiene.Request{
		Body: "## Issue\n\nCloses #127\n\n## What changed\n\nThe check reads the request.\n",
		Commits: []prhygiene.Commit{
			{Parents: 1, Subject: "Add the pull request hygiene gate (#127)"},
		},
		Changed: 120,
	}
}

func TestAnAcceptablePullRequestIsNotRefused(t *testing.T) {
	report := prhygiene.Judge(acceptable())
	if report.Refused() {
		t.Fatalf("an acceptable pull request was refused: %v", report.Refusals)
	}
	if len(report.Warnings) != 0 || len(report.Skipped) != 0 {
		t.Fatalf("it also carried %v and %v", report.Warnings, report.Skipped)
	}
	if got := strings.Join(report.Lines(), "\n"); !strings.Contains(got, "names the work it belongs to") {
		t.Fatalf("the report reads %q and does not say it passed", got)
	}
}

// The near miss this check exists for, and it is exactly one edit from the case
// above: the same body with the number taken off the reference, which is how
// the template ships and how a body left unfilled reads.
func TestABodyThatDiffersOnlyInTheReferenceIsRefused(t *testing.T) {
	request := acceptable()
	request.Body = strings.Replace(request.Body, "Closes #127", "Closes #", 1)

	report := prhygiene.Judge(request)
	if !report.Refused() {
		t.Fatal("a body naming no issue was accepted")
	}
	if len(report.Refusals) != 1 {
		t.Fatalf("want the one refusal, got %v", report.Refusals)
	}
	if !strings.Contains(report.Refusals[0], "the body names no issue") {
		t.Fatalf("the refusal reads %q and does not say what is missing", report.Refusals[0])
	}
}

func TestACommitSubjectWithNoReferenceIsRefusedAndQuoted(t *testing.T) {
	request := acceptable()
	request.Commits = append(request.Commits, prhygiene.Commit{Parents: 1, Subject: "Fix the thing"})

	report := prhygiene.Judge(request)
	if len(report.Refusals) != 1 {
		t.Fatalf("want the one refusal, got %v", report.Refusals)
	}
	if !strings.Contains(report.Refusals[0], `"Fix the thing"`) {
		t.Fatalf("the refusal reads %q and does not quote the subject", report.Refusals[0])
	}
}

// A merge commit is the one subject written by git rather than by an author,
// and merging the default branch forward is required of a branch that has been
// open while something else landed. It is skipped, and the skip is named.
func TestAMergeCommitIsSkippedByNameRatherThanSilently(t *testing.T) {
	request := acceptable()
	request.Commits = append(request.Commits, prhygiene.Commit{Parents: 2, Subject: "Merge branch 'main' into ci/pr-hygiene"})

	report := prhygiene.Judge(request)
	if report.Refused() {
		t.Fatalf("a forward merge was refused: %v", report.Refusals)
	}
	if len(report.Skipped) != 1 || !strings.Contains(report.Skipped[0], "1 merge commit(s)") {
		t.Fatalf("the skip is %v, and a check that passes over something silently reads afterwards as one that looked", report.Skipped)
	}
}

func TestAPullRequestWithNoCommitsIsRefused(t *testing.T) {
	request := acceptable()
	request.Commits = nil

	report := prhygiene.Judge(request)
	if len(report.Refusals) != 1 || !strings.Contains(report.Refusals[0], "holds no commit") {
		t.Fatalf("want the empty range refused, got %v", report.Refusals)
	}
}

// The size tier never refuses. A bulk rename, a document migration and a first
// implementation all pass 400 lines legitimately, and a cap that refused them
// would teach people to cut a change into pieces nobody can review alone.
func TestTheSizeWarningWarnsAndNeverRefuses(t *testing.T) {
	request := acceptable()
	request.Changed = 401

	report := prhygiene.Judge(request)
	if report.Refused() {
		t.Fatalf("a large change was refused: %v", report.Refusals)
	}
	if len(report.Warnings) != 1 || !strings.Contains(report.Warnings[0], "401 changed line(s), above 400") {
		t.Fatalf("want the size warning with both numbers, got %v", report.Warnings)
	}
	if !strings.Contains(report.Warnings[0], "never refuses") {
		t.Fatalf("the warning reads %q and does not say that it never blocks", report.Warnings[0])
	}
}

func TestTheSizeExactlyAtTheWarningIsNotWarnedAbout(t *testing.T) {
	request := acceptable()
	request.Changed = 400

	if warnings := prhygiene.Judge(request).Warnings; len(warnings) != 0 {
		t.Fatalf("the boundary was warned about: %v", warnings)
	}
}

func TestASizeNobodyMeasuredIsSaidRatherThanReadAsZero(t *testing.T) {
	request := acceptable()
	request.Changed = -1

	report := prhygiene.Judge(request)
	if len(report.Skipped) != 1 || !strings.Contains(report.Skipped[0], "not measured on this route") {
		t.Fatalf("want the unmeasured size named, got %v", report.Skipped)
	}
	if len(report.Warnings) != 0 {
		t.Fatalf("an unmeasured size produced a warning about its own size: %v", report.Warnings)
	}
}

// The third thing #127 asks for: the same head gives the same verdict. Judge
// reads no clock, no network and no tree, so this is a case about the shape of
// the function rather than about a flake somebody saw.
func TestTheSameRequestJudgedTwiceGivesTheSameReport(t *testing.T) {
	request := acceptable()
	request.Body = "no reference here"
	request.Commits = append(request.Commits, prhygiene.Commit{Parents: 1, Subject: "Another subject"})
	request.Changed = 900

	first := strings.Join(prhygiene.Judge(request).Lines(), "\n")
	second := strings.Join(prhygiene.Judge(request).Lines(), "\n")
	if first != second {
		t.Fatalf("two runs over one request differ:\n%s\n----\n%s", first, second)
	}
}

func TestAReportPrintsSkipsThenWarningsThenRefusals(t *testing.T) {
	report := prhygiene.Judge(prhygiene.Request{
		Body:    "nothing named here",
		Commits: []prhygiene.Commit{{Parents: 2, Subject: "Merge branch 'main'"}},
		Changed: 900,
	})
	lines := report.Lines()
	if len(lines) != 3 {
		t.Fatalf("want one line per finding, got %v", lines)
	}
	if !strings.HasPrefix(lines[0], "skipped: ") || !strings.HasPrefix(lines[1], "warning: ") || !strings.HasPrefix(lines[2], "refused: ") {
		t.Fatalf("the registers are not kept apart: %v", lines)
	}
}

// What counts as a reference, and the shapes that look like one. Each is a
// character or two away from the one above it.
func TestWhatCountsAsAReference(t *testing.T) {
	carries := []string{
		"Closes #127",
		"#1",
		"(#127)",
		"see #127.",
		"a body\nwith the reference\non its own line #42\n",
		"#127, and #128",
	}
	for _, text := range carries {
		if !prhygiene.HasReference(text) {
			t.Errorf("%q was read as naming no issue", text)
		}
	}

	does := []string{
		"",
		"Closes #",
		"Closes #.",
		"#0",
		"#012",
		"#12a",
		"issue#12",
		"v2#12",
		"# a markdown heading",
		"the hash is at the end #",
		"https://github.com/iderex/kanzlei/issues/127",
	}
	for _, text := range does {
		if prhygiene.HasReference(text) {
			t.Errorf("%q was read as naming an issue", text)
		}
	}
}

func TestTheCommitListIsReadAsGitWritesIt(t *testing.T) {
	commits, err := prhygiene.ParseLog("abc123\tOne subject (#1)\nabc123 def456\tMerge branch 'main'\n\n\tA root commit (#2)\n")
	if err != nil {
		t.Fatalf("the commit list was refused: %v", err)
	}
	if len(commits) != 3 {
		t.Fatalf("want three commits, got %v", commits)
	}
	if commits[0].Parents != 1 || commits[0].Subject != "One subject (#1)" {
		t.Errorf("the first commit is %+v", commits[0])
	}
	if commits[1].Parents != 2 {
		t.Errorf("a merge commit was read with %d parent(s)", commits[1].Parents)
	}
	if commits[2].Parents != 0 || commits[2].Subject != "A root commit (#2)" {
		t.Errorf("the root commit is %+v", commits[2])
	}
}

func TestASubjectHoldingATabKeepsIt(t *testing.T) {
	commits, err := prhygiene.ParseLog("abc123\tA subject\twith a tab (#1)\n")
	if err != nil {
		t.Fatalf("the commit list was refused: %v", err)
	}
	if len(commits) != 1 || commits[0].Subject != "A subject\twith a tab (#1)" {
		t.Fatalf("the subject is %q", commits[0].Subject)
	}
}

// A line with no tab is a format that changed under the check. Reading it as a
// subject with no parents would turn every commit into a merge candidate and
// judge nothing.
func TestALineWithNoTabIsRefusedRatherThanGuessedAt(t *testing.T) {
	_, err := prhygiene.ParseLog("abc123 One subject (#1)\n")
	if err == nil {
		t.Fatal("a line in an unknown format was read as a commit")
	}
	if !strings.Contains(err.Error(), "line 1") {
		t.Fatalf("the failure says %q and does not name the line", err)
	}
}

func TestAnEmptyCommitListIsNotAnError(t *testing.T) {
	commits, err := prhygiene.ParseLog("\n\n")
	if err != nil {
		t.Fatalf("an empty list was refused: %v", err)
	}
	if len(commits) != 0 {
		t.Fatalf("an empty list produced %v", commits)
	}
}
