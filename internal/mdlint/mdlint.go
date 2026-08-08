// Package mdlint holds the shape rules this repository's documents are
// written to, and refuses a document that departs from one.
//
// It is the second half of the documentation gate. internal/doclint asks
// whether a sentence points at something that is there; this package asks
// whether the document around that sentence is written in the one shape every
// reader of it assumes. The two are separate rules because they fail
// differently: a dead path is wrong on the day it is written, and a shape
// departure is wrong on the day somebody else's tool reads it.
//
// Three of the rules below exist because this repository's own checks are
// among those readers. internal/doclint decides which lines are prose by
// tracking fences, and it toggles on a backtick run and a tilde run alike
// without matching the two. A document that opens with one marker and closes
// with the other, or that never closes at all, moves the boundary between
// prose and command block for every line after it, and the path check then
// reads the wrong half of the document in silence. A gate going quiet is worse
// than a gate going red, so the shape those readers assume is refused here
// rather than left to hold by habit.
//
// What this package does not do is as much of its definition as what it does.
// It does not render markdown, it takes no position on line length, wording or
// heading text, and it reads no path. It is a floor under the documents rather
// than a proof about them, and #113 is where the rest of the documentation
// gate is argued.
package mdlint

import (
	"fmt"
	"sort"
	"strings"
)

// A File is one document to read, named by its path from the repository root
// so a finding can be followed back to it.
type File struct {
	Name  string
	Bytes []byte
}

// The rules, named as they are printed. A finding names its rule so the reader
// can find the paragraph above that says what the rule is for, rather than
// having to infer the rule from the one line that broke it.
const (
	// RuleFenceMarker refuses a fenced block opened or closed with tildes.
	// Backticks and tildes are two spellings of one thing, and the reader
	// this matters for is internal/doclint, which treats a run of either as
	// the same fence and so loses track of the boundary the moment a document
	// carries both.
	RuleFenceMarker = "fence-marker"

	// RuleUnclosedFence refuses a fenced block that the document never
	// closes. Everything after it stops being prose: it renders as one long
	// code block, and every rule that reads prose, this package's own and the
	// path check in internal/doclint, goes silent over the remainder without
	// saying so.
	RuleUnclosedFence = "unclosed-fence"

	// RuleSetextHeading refuses a heading written as an underline. It is a
	// second way to spell a heading, invisible to anything scanning for a
	// hash, and the dashed spelling is one blank line away from being a
	// thematic break instead, which is a different element with the same
	// bytes.
	RuleSetextHeading = "setext-heading"

	// RuleClosedHeading refuses the trailing hash run in "## Heading ##". It
	// renders identically to the open form, so the two are one heading style
	// spelled two ways, and the difference shows up only in a grep.
	RuleClosedHeading = "closed-heading"

	// RuleHeadingLevelSkip refuses a heading more than one level below the
	// heading before it. A jump reads as a subsection of a section that was
	// never opened, and it is what a document acquires when a middle heading
	// is deleted and the ones under it are left where they were.
	RuleHeadingLevelSkip = "heading-level-skip"

	// RuleHeadingBlankLine refuses a heading with no blank line before or
	// after it. Without the line before, a heading glued to the paragraph
	// above is not a heading at all in a strict reader; it is a paragraph
	// beginning with hashes.
	RuleHeadingBlankLine = "heading-blank-line"

	// RuleListMarker refuses an unordered list item written with an asterisk
	// or a plus. One marker, so a list is a list wherever it is read from.
	RuleListMarker = "list-marker"
)

// A Finding is one departure, at the line it was read on.
type Finding struct {
	File   string
	Line   int
	Rule   string
	Detail string
}

func (f Finding) String() string {
	return fmt.Sprintf("%s:%d: %s: %s", f.File, f.Line, f.Rule, f.Detail)
}

// A Reading is what a run examined. It is reported on the green route as well
// as the red one: a gate that says only that it found nothing cannot be told
// apart from one that read nothing, and this rule reads prose lines rather
// than every byte, so the numbers are what make its reach legible.
type Reading struct {
	Documents int
	Lines     int
	Prose     int
	Headings  int
}

// Check reads every document and reports every departure, ordered by file and
// then by line so a run's output is stable.
func Check(files []File) ([]Finding, Reading) {
	var findings []Finding
	var read Reading
	for _, f := range files {
		fs, r := checkFile(f)
		findings = append(findings, fs...)
		read.Documents++
		read.Lines += r.Lines
		read.Prose += r.Prose
		read.Headings += r.Headings
	}
	sort.SliceStable(findings, func(i, j int) bool {
		if findings[i].File != findings[j].File {
			return findings[i].File < findings[j].File
		}
		return findings[i].Line < findings[j].Line
	})
	return findings, read
}

// checkFile reads one document.
//
// The order the lines are classified in is the rule set's own boundary. A
// fence line is decided first, because it is what says whether the lines after
// it are prose at all. Inside a fence nothing is read, since a document
// showing markdown is showing it rather than writing it, and this package
// refused its own rule descriptions before that was true.
func checkFile(f File) ([]Finding, Reading) {
	var findings []Finding
	var read Reading

	lines := strings.Split(strings.TrimSuffix(string(f.Bytes), "\n"), "\n")
	read.Lines = len(lines)

	var (
		openMarker string // the fence marker in force, empty outside a fence
		openLine   int    // where that fence was opened
		prevLevel  int    // the level of the last heading read
		prevProse  string // the last line read outside a block
	)

	finding := func(line int, rule, detail string) {
		findings = append(findings, Finding{File: f.Name, Line: line, Rule: rule, Detail: detail})
	}

	for i, raw := range lines {
		line := i + 1
		trimmed := strings.TrimLeft(raw, " ")

		if marker := fenceMarker(trimmed); marker != "" {
			if marker == "~~~" {
				finding(line, RuleFenceMarker, "a fenced block is opened or closed with tildes, and this repository writes fences with backticks")
			}
			if openMarker == "" {
				openMarker, openLine = marker, line
			} else if strings.HasPrefix(trimmed, openMarker) {
				openMarker, openLine = "", 0
			}
			prevProse = raw
			continue
		}
		if openMarker != "" {
			continue
		}
		if indented(raw) {
			prevProse = raw
			continue
		}

		read.Prose++

		if level, text, ok := heading(trimmed); ok {
			read.Headings++
			if closedHeading(text) {
				finding(line, RuleClosedHeading, "a heading carries a trailing hash run, which renders as nothing and is a second spelling of the same heading")
			}
			if prevLevel != 0 && level > prevLevel+1 {
				finding(line, RuleHeadingLevelSkip, fmt.Sprintf("a level %d heading follows a level %d one, so it is a subsection of a section nothing opened", level, prevLevel))
			}
			if i > 0 && strings.TrimSpace(prevProse) != "" {
				finding(line, RuleHeadingBlankLine, "a heading with no blank line before it is a paragraph beginning with hashes in a strict reader")
			}
			if i+1 < len(lines) && strings.TrimSpace(lines[i+1]) != "" {
				finding(line, RuleHeadingBlankLine, "a heading with no blank line after it runs into the text below it")
			}
			prevLevel = level
			prevProse = raw
			continue
		}

		if isUnderline(trimmed) && strings.TrimSpace(prevProse) != "" && !isListItem(strings.TrimLeft(prevProse, " ")) {
			finding(line, RuleSetextHeading, "a heading written as an underline, where this repository writes every heading with hashes")
		}

		if marker, ok := looseListMarker(raw); ok {
			finding(line, RuleListMarker, fmt.Sprintf("an unordered list item is written with %q, and this repository writes them with a hyphen", marker))
		}

		prevProse = raw
	}

	if openMarker != "" {
		finding(openLine, RuleUnclosedFence, "a fenced block is opened here and never closed, so every line after it stops being read as prose")
	}

	return findings, read
}

// fenceMarker reports the marker a line opens or closes a fenced block with,
// and the empty string where the line is not a fence line. The reading matches
// internal/doclint's deliberately: two readers of the same documents
// disagreeing about where a block starts is the failure this package's first
// three rules are about, and it would be a poor place to introduce one.
func fenceMarker(trimmed string) string {
	for _, marker := range []string{"```", "~~~"} {
		if strings.HasPrefix(trimmed, marker) {
			return marker
		}
	}
	return ""
}

// indented reports a line held inside an indented block. Four spaces is what
// markdown reads as one and it is how this repository writes a command.
//
// The bound is worth naming: a list item nested four spaces deep is indented
// by this reading, so the list rule does not reach it. That is the same bound
// internal/doclint carries, and widening it would mean deciding whether a line
// is a continuation or a block, which needs the surrounding list rather than
// the line.
func indented(raw string) bool {
	return strings.HasPrefix(raw, "    ") || strings.HasPrefix(raw, "\t")
}

// heading reports the level and the text of an ATX heading.
//
// A hash run has to be followed by a space to be a heading, which is not
// pedantry here: this repository's documents write an issue reference at the
// start of a sentence, so "#52 has to propagate a revocation" opens seven
// lines that are prose and are read as prose by every renderer. A rule that
// took a leading hash for a heading would refuse each of them.
func heading(trimmed string) (level int, text string, ok bool) {
	n := 0
	for n < len(trimmed) && trimmed[n] == '#' {
		n++
	}
	if n == 0 || n > 6 {
		return 0, "", false
	}
	rest := trimmed[n:]
	if rest != "" && !strings.HasPrefix(rest, " ") {
		return 0, "", false
	}
	return n, strings.TrimRight(strings.TrimSpace(rest), " "), true
}

// closedHeading reports the trailing hash run of "## Heading ##".
//
// The run has to be preceded by a space to be one, which is what separates the
// closed form from a heading whose last word ends in a hash. A language named
// with one is the case: "## Writing C#" is a heading with the text somebody
// wrote, and refusing it would make the rule an argument rather than a repair.
func closedHeading(text string) bool {
	trimmed := strings.TrimRight(text, "#")
	if trimmed == text || trimmed == "" {
		return false
	}
	return strings.HasSuffix(trimmed, " ")
}

// isUnderline reports a line that is nothing but equals signs or nothing but
// hyphens, which is the second half of a setext heading when a paragraph sits
// above it.
func isUnderline(trimmed string) bool {
	body := strings.TrimRight(trimmed, " ")
	if body == "" {
		return false
	}
	return strings.Trim(body, "=") == "" || strings.Trim(body, "-") == ""
}

// isListItem reports a line that opens a list item, in any of the three
// unordered markers. It is what keeps a hyphen run under a list from being
// read as a setext underline.
func isListItem(trimmed string) bool {
	for _, marker := range []string{"- ", "* ", "+ "} {
		if strings.HasPrefix(trimmed, marker) {
			return true
		}
	}
	return false
}

// looseListMarker reports an unordered list item written with a marker other
// than a hyphen. Up to three spaces of indent still opens a list item; four
// makes an indented block, which the caller has already passed over.
func looseListMarker(raw string) (string, bool) {
	trimmed := strings.TrimLeft(raw, " ")
	if len(raw)-len(trimmed) > 3 {
		return "", false
	}
	for _, marker := range []string{"*", "+"} {
		if strings.HasPrefix(trimmed, marker+" ") {
			return marker, true
		}
	}
	return "", false
}
