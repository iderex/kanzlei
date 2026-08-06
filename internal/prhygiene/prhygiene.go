// Package prhygiene decides whether a pull request is linked to the work it
// belongs to.
//
// CONTRIBUTING.md says every change starts as an issue, and nothing read a
// pull request to see whether one was named. A change that closes nothing and
// references nothing is found again a year later by somebody trying to work out
// why a line exists, and the link is the cheapest thing to require at the moment
// it is free to add.
//
// The decision is here rather than in a shell block in the workflow for the
// reason docs/decisions/0001-means.md gives: it reads text somebody else wrote,
// it has near misses, and a rule with near misses belongs where fixtures can be
// put in front of it. The workflow gathers the inputs and this package judges
// them.
//
// Judge is a function of its input and of nothing else. It reads no clock, no
// network and no tree, so the same head judged twice gives the same verdict,
// which is the third thing #127 asks for.
package prhygiene

import (
	"fmt"
	"strings"
)

// A Commit is what the check needs to know about one commit: how many parents
// it has, and the first line of its message.
//
// Parents is here because a merge commit is the one subject that legitimately
// carries no issue. Merging the default branch forward is required of a branch
// that has been open while something else landed, and the merge is not a change
// with an issue of its own.
type Commit struct {
	Parents int
	Subject string
}

// A Request is one pull request, as much of it as this check reasons about.
//
// Changed is lines added plus lines removed. A negative value means the size
// was not measured on this route, which is reported rather than treated as
// zero: a size nobody measured and a small change look the same to a reader
// otherwise.
type Request struct {
	Body    string
	Commits []Commit
	Changed int
}

// A Report is the verdict, in three registers that are not interchangeable.
//
// Refusals turn the check red. Warnings never do. Skipped says what the check
// did not judge and why, because a check that silently passed over something
// reads afterwards as a check that looked at it and found nothing.
type Report struct {
	Refusals []string
	Warnings []string
	Skipped  []string
}

// Refused reports whether anything turns the check red.
func (r Report) Refused() bool { return len(r.Refusals) > 0 }

// Lines is the whole report as the check prints it, in a fixed order.
func (r Report) Lines() []string {
	var out []string
	for _, s := range r.Skipped {
		out = append(out, "skipped: "+s)
	}
	for _, w := range r.Warnings {
		out = append(out, "warning: "+w)
	}
	for _, f := range r.Refusals {
		out = append(out, "refused: "+f)
	}
	if !r.Refused() {
		out = append(out, "the pull request names the work it belongs to")
	}
	return out
}

// sizeWarning is where the size warning starts, in lines added plus removed.
//
// It is inherited rather than measured here, and it never blocks. A bulk
// rename, a document migration and a first implementation all pass it
// legitimately, and a cap that refused them would teach people to cut a change
// into pieces that are individually unreviewable, which is the opposite of what
// a size limit is for.
const sizeWarning = 400

// Judge reads a pull request and says what is wrong with how it is linked.
func Judge(request Request) Report {
	var report Report

	if !HasReference(request.Body) {
		report.Refusals = append(report.Refusals, "the body names no issue; write the issue number as #<number> where the body says which issue this closes")
	}

	merges := 0
	for _, commit := range request.Commits {
		if commit.Parents > 1 {
			merges++
			continue
		}
		if HasReference(commit.Subject) {
			continue
		}
		report.Refusals = append(report.Refusals, fmt.Sprintf("the commit subject %q names no issue", commit.Subject))
	}
	if merges > 0 {
		report.Skipped = append(report.Skipped, fmt.Sprintf("%d merge commit(s), whose subject is written by git rather than by an author; merging the default branch forward is not a change with an issue of its own", merges))
	}
	if len(request.Commits) == 0 {
		report.Refusals = append(report.Refusals, "the range holds no commit; a pull request that changes nothing is not a change")
	}

	switch {
	case request.Changed < 0:
		report.Skipped = append(report.Skipped, "the size, which was not measured on this route")
	case request.Changed > sizeWarning:
		report.Warnings = append(report.Warnings, fmt.Sprintf("%d changed line(s), above %d. This never refuses a change: a bulk rename, a document migration or a first implementation passes it legitimately. It is here so that a change nobody planned to be this large is noticed by its author first", request.Changed, sizeWarning))
	}

	return report
}

// HasReference reports whether text names an issue.
//
// What counts is a hash followed by a number, which is what the tracker itself
// links. The rules around it are what stop the shapes that look like a
// reference and are not:
//
//   - the character before the hash is not a letter or a digit, so a word with
//     a hash inside it is not a reference
//   - the digits do not start with a zero, because no issue is numbered that way
//     and a leading zero is somebody writing a date or a version
//   - the character after the digits is not a letter or a digit, so an
//     identifier that begins with a number is not read as one
//
// It reads the text as text. A reference inside a fenced block, a quotation or
// a struck-through line counts, and one written as a bare link to the tracker
// does not. Both are stated in CONTRIBUTING.md rather than guessed at here.
func HasReference(text string) bool {
	for i := 0; i < len(text); i++ {
		if text[i] != '#' {
			continue
		}
		if i > 0 && isLetterOrDigit(text[i-1]) {
			continue
		}
		digits := 0
		for i+1+digits < len(text) && isDigit(text[i+1+digits]) {
			digits++
		}
		if digits == 0 || text[i+1] == '0' {
			continue
		}
		after := i + 1 + digits
		if after < len(text) && isLetterOrDigit(text[after]) {
			continue
		}
		return true
	}
	return false
}

func isDigit(b byte) bool { return b >= '0' && b <= '9' }

func isLetterOrDigit(b byte) bool {
	return isDigit(b) || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

// ParseLog reads the commit list the workflow hands over.
//
// One line per commit, the parent hashes and then a tab and the subject, which
// is what `git log --format=%p%x09%s` writes. The parents are counted rather
// than kept: what this check needs from them is whether the commit is a merge.
//
// A subject holding a tab keeps it, because the split is on the first one. A
// line with no tab at all is refused rather than read as a subject with no
// parents, since that shape is a format that changed under the check.
func ParseLog(text string) ([]Commit, error) {
	var commits []Commit
	for number, line := range strings.Split(text, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parents, subject, ok := strings.Cut(line, "\t")
		if !ok {
			return nil, fmt.Errorf("line %d of the commit list holds no tab: %q", number+1, line)
		}
		commits = append(commits, Commit{
			Parents: len(strings.Fields(parents)),
			Subject: strings.TrimSpace(subject),
		})
	}
	return commits, nil
}
