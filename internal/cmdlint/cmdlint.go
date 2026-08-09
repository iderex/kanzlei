// Package cmdlint reads the commands this repository's documents show and
// refuses one whose shell syntax does not close.
//
// It is the fourth rule under the documentation gate. internal/doclint asks
// whether a sentence points at something that is there, internal/mdlint asks
// whether the document around that sentence is written in the shape every
// reader of it assumes, and internal/linkcheck asks the other end about an
// address. All three read prose and deliberately pass over the blocks that
// hold commands. This one reads inside those blocks, so the two halves of a
// document are each read by something.
//
// What it refuses is a command a reader cannot run: a quote that never closes,
// a substitution that never closes, a pipeline with nothing after its
// operator, and a command left hanging on a continuation the document never
// finished. Each of those is what a command acquires when it is edited rather
// than when it is written.
//
// # What it reads, and why the boundary is where it is
//
// Two shapes, and nothing else.
//
// A block that declares a shell, which is the block that claims to be a
// command rather than being read as one by convention. Every line in one is a
// command. No document in this repository writes one today.
//
// A continued command anywhere else: a run of lines where every line but the
// last ends in an unescaped backslash, so the run is one logical command
// however many lines it occupies.
//
// The reason for the second boundary is a fact about this repository's
// documents rather than a limitation of the parser. A command block here is
// indented rather than fenced, and it holds the command and the output the
// command produced, written identically, with no prompt, marker or blank line
// between them:
//
//	go run ./cmd/linkcheck
//	linkcheck: 0 external link(s) across 22 tracked document(s)
//
// Nothing in those two lines says which is which. A rule reading every line as
// a command would judge output as shell, and output is prose: it carries
// apostrophes, unmatched brackets and anything else the program printed. A
// line ending in an unescaped backslash is the one thing in such a block that
// no output produces, so a continued command is identifiable without a guess
// and a single-line one is not. Reading the second would mean refusing a
// document for a sentence a program printed, and a gate that is wrong about
// prose is one people learn to ignore.
//
// A line an undeclared block holds that is not part of a continued command is
// therefore not read, and Reading counts it so a run says how much it passed
// over rather than leaving the reach to be assumed.
//
// What is not read at all, and is not read rather than being read badly: a
// heredoc body, a substitution written in backticks, and a block in a document
// this repository does not track. It is a floor under the commands these
// documents show rather than a proof about them, and #113 is where the rest of
// the documentation gate is argued.
package cmdlint

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

// The rules, named as they are printed. A finding names its rule so a reader
// can find the paragraph that says what the rule is for, rather than having to
// infer the rule from the one line that broke it.
const (
	// RuleUnclosedContinuation refuses a command that is still continued when
	// its block ends. The last line ends in a backslash, so the command the
	// document shows is cut off and a reader who copies it gets a shell
	// waiting for the rest.
	RuleUnclosedContinuation = "unclosed-continuation"

	// RuleUnbalancedQuote refuses a command whose single or double quote never
	// closes. The shell would read the rest of the command, and everything
	// after it, as one string, so the command runs and does something other
	// than what the document says it does.
	RuleUnbalancedQuote = "unbalanced-quote"

	// RuleUnclosedSubstitution refuses a command substitution opened with "$("
	// and never closed. Only the unclosed direction is refused: a closing
	// parenthesis with no opener is ordinary shell in a case arm and in a
	// subshell written another way, and refusing it would be an argument
	// rather than a repair.
	RuleUnclosedSubstitution = "unclosed-substitution"

	// RuleTruncatedPipeline refuses a command ending in a pipe or a logical
	// operator with nothing after it. That is the same truncation as an
	// unclosed continuation seen from one character further on, and it is
	// worth its own name because the repair is different: one needs the rest
	// of the command and the other needs the operator removed.
	RuleTruncatedPipeline = "truncated-pipeline"
)

// A Finding is one refusal, at the line the command it is about begins on.
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
// as the red one: this rule reads a narrow part of a document, so a run that
// found nothing and a run that read nothing print the same line otherwise.
// Passed is the negative half of that and is printed with the rest, because a
// count of what was read says nothing about what was stepped over.
type Reading struct {
	Documents int
	Blocks    int
	Commands  int
	Lines     int
	Passed    int
}

func (r Reading) String() string {
	return fmt.Sprintf("%d command(s) over %d line(s) in %d block(s) across %d tracked document(s), %d line(s) passed over",
		r.Commands, r.Lines, r.Blocks, r.Documents, r.Passed)
}

// Check reads every document and reports every refusal, ordered by file and
// then by line so a run's output is stable.
func Check(files []File) ([]Finding, Reading) {
	var findings []Finding
	var read Reading
	for _, f := range files {
		fs, r := checkFile(f)
		findings = append(findings, fs...)
		read.Documents++
		read.Blocks += r.Blocks
		read.Commands += r.Commands
		read.Lines += r.Lines
		read.Passed += r.Passed
	}
	sort.SliceStable(findings, func(i, j int) bool {
		if findings[i].File != findings[j].File {
			return findings[i].File < findings[j].File
		}
		return findings[i].Line < findings[j].Line
	})
	return findings, read
}

// A command is one logical command a block shows, with the line it starts on
// so a finding points at the first line a reader would copy rather than at the
// line the parser stopped on.
type command struct {
	line int
	text string
}

// checkFile reads one document.
//
// The fence boundary is internal/doclint's and internal/mdlint's, and it is
// deliberately the same one: several readers of these documents disagreeing
// about where a block starts is the failure mdlint's first three rules exist
// to refuse, and this would be a poor place to introduce another reading.
func checkFile(f File) ([]Finding, Reading) {
	var findings []Finding
	var read Reading

	lines := strings.Split(strings.TrimSuffix(string(f.Bytes), "\n"), "\n")

	var (
		fenceMarker string // the fence marker in force, empty outside a fence
		fenceShell  bool   // whether that fence declared a shell
		inIndented  bool   // whether an indented block is open
		open        []string
		openLine    int
	)

	// closeBlock ends whatever block was open. A continuation still open at
	// that point is the block ending mid-command, which is a refusal rather
	// than something to carry into the next block.
	closeBlock := func() {
		if len(open) > 0 {
			findings = append(findings, Finding{
				File: f.Name, Line: openLine, Rule: RuleUnclosedContinuation,
				Detail: "the block ends while this command is still continued, so what a reader would copy is cut off",
			})
			open = nil
		}
	}

	// take offers one line of a block to the reader. declared says the block
	// said what it holds, so a single line in it is a command; where it did
	// not, only a continued run is one.
	take := func(line int, raw string, declared bool) {
		body := strings.TrimLeft(raw, " \t")
		if len(open) == 0 {
			if !declared && !endsWithContinuation(body) {
				read.Passed++
				return
			}
			openLine = line
		}
		read.Lines++
		open = append(open, body)
		if endsWithContinuation(body) {
			return
		}
		text := joinContinued(open)
		open = nil
		read.Commands++
		findings = append(findings, judge(f.Name, command{line: openLine, text: text})...)
	}

	for i, raw := range lines {
		line := i + 1
		trimmed := strings.TrimLeft(raw, " ")

		if marker, info, ok := fence(trimmed); ok {
			if fenceMarker == "" {
				closeBlock()
				inIndented = false
				fenceMarker, fenceShell = marker, declaresShell(info)
				if fenceShell {
					read.Blocks++
				}
			} else if strings.HasPrefix(trimmed, fenceMarker) {
				closeBlock()
				fenceMarker, fenceShell = "", false
			}
			continue
		}
		if fenceMarker != "" {
			if fenceShell && strings.TrimSpace(raw) != "" {
				take(line, raw, true)
			}
			continue
		}
		if indented(raw) {
			if strings.TrimSpace(raw) == "" {
				continue
			}
			if !inIndented {
				inIndented = true
				read.Blocks++
			}
			take(line, raw, false)
			continue
		}
		if strings.TrimSpace(raw) == "" {
			// A blank line alone does not end an indented block; the next
			// unindented line does. Ending it here would split one block in
			// two and would report a continuation as unclosed across a gap
			// that a markdown reader does not see.
			continue
		}
		closeBlock()
		inIndented = false
	}
	closeBlock()

	return findings, read
}

// judge reads one logical command and reports what did not close.
func judge(name string, c command) []Finding {
	var findings []Finding
	add := func(rule, detail string) {
		findings = append(findings, Finding{File: name, Line: c.line, Rule: rule, Detail: detail})
	}

	s := scan(c.text)
	switch s.quote {
	case '\'':
		add(RuleUnbalancedQuote, "a single quote is opened and never closed, so the shell reads the rest of the command as one string")
	case '"':
		add(RuleUnbalancedQuote, "a double quote is opened and never closed, so the shell reads the rest of the command as one string")
	}
	if s.substitutions > 0 {
		add(RuleUnclosedSubstitution, fmt.Sprintf("%d command substitution(s) opened with \"$(\" and never closed", s.substitutions))
	}
	if s.quote == 0 && s.substitutions == 0 {
		// A command whose quoting did not close is already refused above, and
		// what its last characters are says nothing further about it: the
		// operator a reader sees at the end may be inside the string that
		// never closed.
		if op := trailingOperator(s.code); op != "" {
			add(RuleTruncatedPipeline, fmt.Sprintf("the command ends in %q with nothing after it", op))
		}
	}
	return findings
}

// state is what a walk of one command's characters ended in.
type state struct {
	quote         byte   // the quote still open, or zero
	substitutions int    // command substitutions still open
	code          string // the command with its comment removed
}

// scan walks a command in the shell's own quoting states.
//
// Three things it has to get right, each of which is a false refusal if it
// does not. Inside single quotes nothing is special, so an apostrophe in a
// double-quoted string and a backslash in a single-quoted one are literal
// text. A hash begins a comment only at the start of a word, so a fragment
// like "issue#113" is not one. And a substitution is counted only where the
// shell would expand it, which is anywhere outside single quotes.
func scan(text string) state {
	var s state
	var code []byte
	depth := 0
	for i := 0; i < len(text); i++ {
		ch := text[i]
		switch s.quote {
		case '\'':
			if ch == '\'' {
				s.quote = 0
			}
			code = append(code, ch)
			continue
		case '"':
			if ch == '\\' && i+1 < len(text) {
				code = append(code, ch)
				i++
				code = append(code, text[i])
				continue
			}
			if ch == '"' {
				s.quote = 0
			}
			if ch == '$' && i+1 < len(text) && text[i+1] == '(' {
				depth++
				code = append(code, "$("...)
				i++
				continue
			}
			if ch == ')' && depth > 0 {
				depth--
			}
			code = append(code, ch)
			continue
		}

		switch {
		case ch == '\\' && i+1 < len(text):
			code = append(code, ch)
			i++
			code = append(code, text[i])
		case ch == '\'' || ch == '"':
			s.quote = ch
			code = append(code, ch)
		case ch == '#' && startsWord(text, i):
			// A comment runs to the end of the command. Everything after it is
			// prose the shell never reads, so nothing in it is judged.
			s.substitutions = depth
			s.code = strings.TrimRight(string(code), " \t")
			return s
		case ch == '$' && i+1 < len(text) && text[i+1] == '(':
			depth++
			code = append(code, "$("...)
			i++
		case ch == ')' && depth > 0:
			depth--
			code = append(code, ch)
		default:
			code = append(code, ch)
		}
	}
	s.substitutions = depth
	s.code = strings.TrimRight(string(code), " \t")
	return s
}

// startsWord reports a hash that opens a comment rather than one inside a
// word. This repository's documents are full of the second kind, because an
// issue reference is written with one.
func startsWord(text string, i int) bool {
	if i == 0 {
		return true
	}
	switch text[i-1] {
	case ' ', '\t', ';', '|', '&', '(':
		return true
	}
	return false
}

// trailingOperator reports the pipe or logical operator a command ends on,
// with nothing after it. A single ampersand is left alone: it backgrounds the
// command and is a complete line.
func trailingOperator(code string) string {
	code = strings.TrimRight(code, " \t")
	switch {
	case strings.HasSuffix(code, "&&"):
		return "&&"
	case strings.HasSuffix(code, "||"):
		return "||"
	case strings.HasSuffix(code, "|") && !strings.HasSuffix(code, ">|"):
		return "|"
	}
	return ""
}

// endsWithContinuation reports a line the shell would join to the next one.
// The count matters: a line ending in two backslashes ends in an escaped
// backslash and continues nothing.
func endsWithContinuation(line string) bool {
	line = strings.TrimRight(line, " \t")
	n := 0
	for i := len(line) - 1; i >= 0 && line[i] == '\\'; i-- {
		n++
	}
	return n%2 == 1
}

// joinContinued turns the lines of a continued command into the one line the
// shell sees, which is what the rules are then applied to.
func joinContinued(lines []string) string {
	parts := make([]string, 0, len(lines))
	for i, l := range lines {
		l = strings.TrimRight(l, " \t")
		if i < len(lines)-1 {
			l = strings.TrimSuffix(l, "\\")
		}
		parts = append(parts, strings.TrimSpace(l))
	}
	return strings.Join(parts, " ")
}

// fence reports the marker and the info string of a line that opens or closes
// a fenced block. The marker reading is internal/mdlint's, which treats a
// backtick run and a tilde run alike; mdlint is what refuses a document that
// mixes them, so this reader does not have to.
func fence(trimmed string) (marker, info string, ok bool) {
	for _, m := range []string{"```", "~~~"} {
		if strings.HasPrefix(trimmed, m) {
			return m, strings.TrimSpace(strings.TrimLeft(trimmed[len(m):], "`~")), true
		}
	}
	return "", "", false
}

// declaresShell reports an info string naming a shell. It is the only way a
// block in a markdown document says what it holds, and this repository writes
// none today: every command here is an indented block, which is read by
// convention rather than by declaration.
func declaresShell(info string) bool {
	name := info
	if i := strings.IndexAny(name, " \t"); i >= 0 {
		name = name[:i]
	}
	switch strings.ToLower(name) {
	case "sh", "bash", "shell", "zsh", "console", "shell-session":
		return true
	}
	return false
}

// indented reports a line held inside an indented block. Four spaces is what
// markdown reads as one and it is how this repository writes a command, which
// is the same reading internal/doclint and internal/mdlint take.
func indented(raw string) bool {
	return strings.HasPrefix(raw, "    ") || strings.HasPrefix(raw, "\t")
}
