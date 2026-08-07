// Package treefmt decides whether a file in this repository is formatted the
// way this repository formats files, and produces the bytes it should hold.
//
// The rule set is `.editorconfig` at the root. It is read rather than restated
// here, so a rule changes in one place and the check changes with it. A
// property or a value this package does not implement is refused when the file
// is parsed, rather than passed over: a rule somebody wrote down and nothing
// applies is worse than no rule, because the tree then looks governed and is
// not.
//
// A .go file is formatted by go/format and by nothing else in here. That is not
// a shortcut. A line-based rule cannot see the inside of a raw string literal,
// so trimming trailing whitespace or converting a leading tab would silently
// edit a program's data. The toolchain's own formatter parses first and is the
// authority for the body of a .go file; the rules below are for the text that
// has no formatter of its own.
//
// This package is not part of the binary. It is here because deciding whether
// the tree is acceptable is a rule, and docs/decisions/0001-means.md keeps that
// kind of decision somewhere a fixture can be put in front of it rather than in
// a shell block nobody can run a case against.
package treefmt

import (
	"bytes"
	"fmt"
	"go/format"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// A Finding is one way a file departs from the rule set, at the place it
// departs.
type Finding struct {
	File   string
	Line   int
	Rule   string // the .editorconfig property, or "gofmt"
	Detail string
}

func (f Finding) String() string {
	return fmt.Sprintf("%s:%d: %s: %s", f.File, f.Line, f.Rule, f.Detail)
}

// Rules are the properties that apply to one path, after every matching
// section of the rule set has been laid over the ones before it.
//
// The zero value is every rule unset, which is what a path no section matches
// gets. Unset means this package leaves that aspect of the file alone; it never
// means a default chosen here.
type Rules struct {
	Charset            string // "utf-8", or "" where unset
	EndOfLine          string // "lf", or "" where unset
	InsertFinalNewline bool
	TrimTrailing       bool
	IndentStyle        string // "tab", "space", or "" where unset
	IndentSize         int    // 0 where unset
}

// Format returns the bytes name should hold and every finding describing how
// the bytes it does hold differ. Where the finding list is empty the bytes are
// byte-identical, and callers may rely on that direction: the findings are
// produced by the transformations rather than derived from a diff afterwards,
// which is what lets each one carry the line it happened on.
//
// The reverse does not hold. Bytes that do not decode, a .go file that does not
// parse and a space indent on a path indented with tabs are each reported and
// deliberately returned unchanged, because every repair available there is a
// guess and -write would put the guess in the tree.
//
// name is used for the reported position and to decide whether the file is Go.
// A caller holding bytes does not have to write them to disk first, which is
// what lets the fixtures stay in source.
func Format(name string, src []byte, r Rules) ([]byte, []Finding) {
	var found []Finding
	out := src

	if r.Charset == "utf-8" {
		var f []Finding
		out, f = applyCharset(name, out)
		found = append(found, f...)
		if len(f) > 0 && !utf8.Valid(out) {
			// Not repairable here. Returning the original bytes rather than a
			// half-converted file keeps -write from writing a guess over
			// something whose encoding nobody has decided yet.
			sort.SliceStable(found, func(i, j int) bool { return found[i].Line < found[j].Line })
			return src, found
		}
	}

	if r.EndOfLine == "lf" {
		var f []Finding
		out, f = applyLF(name, out)
		found = append(found, f...)
	}

	if strings.HasSuffix(name, ".go") {
		formatted, err := format.Source(out)
		if err != nil {
			// A .go file that does not parse is a build failure, and the build
			// is where it is reported. Saying so here rather than reporting the
			// file as formatted keeps a broken file from passing this check on
			// the way to failing a clearer one.
			found = append(found, Finding{
				File: name, Line: parseErrorLine(err), Rule: "gofmt",
				Detail: "does not parse, so it cannot be formatted: " + err.Error(),
			})
			sort.SliceStable(found, func(i, j int) bool { return found[i].Line < found[j].Line })
			return src, found
		}
		if !bytes.Equal(formatted, out) {
			found = append(found, Finding{
				File: name, Line: firstDifferingLine(out, formatted), Rule: "gofmt",
				Detail: "not formatted as go/format formats it",
			})
			out = formatted
		}
		// go/format decides the rest of a .go file. The line rules below are
		// not applied to it, and the package comment says why.
		sort.SliceStable(found, func(i, j int) bool { return found[i].Line < found[j].Line })
		return out, found
	}

	if r.TrimTrailing {
		var f []Finding
		out, f = applyTrim(name, out)
		found = append(found, f...)
	}

	if r.IndentStyle == "space" && r.IndentSize > 0 {
		var f []Finding
		out, f = applySpaceIndent(name, out, r.IndentSize)
		found = append(found, f...)
	}
	if r.IndentStyle == "tab" {
		found = append(found, tabIndentFindings(name, out)...)
	}

	if r.InsertFinalNewline {
		var f []Finding
		out, f = applyFinalNewline(name, out)
		found = append(found, f...)
	}

	sort.SliceStable(found, func(i, j int) bool { return found[i].Line < found[j].Line })
	return out, found
}

// applyCharset removes a byte order mark and reports a line that does not
// decode. The mark is removed rather than reported and kept: this repository
// stores UTF-8 without one, and a mark at the head of a file is invisible in
// every tool a reviewer would open it in.
func applyCharset(name string, src []byte) ([]byte, []Finding) {
	var found []Finding
	out := src
	if bom := []byte{0xEF, 0xBB, 0xBF}; bytes.HasPrefix(out, bom) {
		out = out[len(bom):]
		found = append(found, Finding{
			File: name, Line: 1, Rule: "charset",
			Detail: "starts with a UTF-8 byte order mark; this repository stores UTF-8 without one",
		})
	}
	if utf8.Valid(out) {
		return out, found
	}
	for i, line := range splitLines(out) {
		if !utf8.Valid(line) {
			found = append(found, Finding{
				File: name, Line: i + 1, Rule: "charset",
				Detail: "does not decode as UTF-8; convert the file, or declare the path binary if its bytes are the thing under test",
			})
		}
	}
	return out, found
}

// applyLF converts CRLF and a lone CR to LF. .gitattributes already fixes what
// is stored, and a published check already refuses a text blob stored with CR
// bytes. This one reads the working tree, which is the copy a contributor is
// standing in front of, and it is the copy where the byte can still be removed
// without a renormalise.
func applyLF(name string, src []byte) ([]byte, []Finding) {
	if !bytes.ContainsRune(src, '\r') {
		return src, nil
	}
	var found []Finding
	for i, line := range splitLines(src) {
		if bytes.ContainsRune(line, '\r') {
			found = append(found, Finding{
				File: name, Line: i + 1, Rule: "end_of_line",
				Detail: "holds a carriage return; this repository stores text with LF endings",
			})
		}
	}
	out := bytes.ReplaceAll(src, []byte("\r\n"), []byte("\n"))
	out = bytes.ReplaceAll(out, []byte("\r"), []byte("\n"))
	return out, found
}

// applyTrim removes spaces and tabs at the end of a line.
func applyTrim(name string, src []byte) ([]byte, []Finding) {
	lines := splitLines(src)
	var found []Finding
	changed := false
	for i, line := range lines {
		trimmed := bytes.TrimRight(line, " \t")
		if len(trimmed) != len(line) {
			found = append(found, Finding{
				File: name, Line: i + 1, Rule: "trim_trailing_whitespace",
				Detail: fmt.Sprintf("ends with %d character(s) of whitespace", len(line)-len(trimmed)),
			})
			lines[i] = trimmed
			changed = true
		}
	}
	if !changed {
		return src, nil
	}
	return joinLines(lines, endsWithNewline(src)), found
}

// applySpaceIndent turns a tab used for indentation into size spaces. Only the
// leading run of whitespace is touched, so a tab inside a line, where it may be
// data, is left where it is.
func applySpaceIndent(name string, src []byte, size int) ([]byte, []Finding) {
	lines := splitLines(src)
	var found []Finding
	changed := false
	for i, line := range lines {
		lead := leadingWhitespace(line)
		if !bytes.ContainsRune(lead, '\t') {
			continue
		}
		found = append(found, Finding{
			File: name, Line: i + 1, Rule: "indent_style",
			Detail: fmt.Sprintf("indented with a tab where this path is indented with %d spaces", size),
		})
		lines[i] = append(bytes.ReplaceAll(lead, []byte("\t"), bytes.Repeat([]byte(" "), size)), line[len(lead):]...)
		changed = true
	}
	if !changed {
		return src, nil
	}
	return joinLines(lines, endsWithNewline(src)), found
}

// tabIndentFindings reports a line indented with spaces where the rule set says
// tabs. It reports and does not repair: how many spaces stand for one tab is
// not a thing this package gets to guess, and the only paths this rule applies
// to are handled by go/format before it is reached.
func tabIndentFindings(name string, src []byte) []Finding {
	var found []Finding
	for i, line := range splitLines(src) {
		if bytes.HasPrefix(line, []byte(" ")) && len(bytes.TrimLeft(line, " \t")) > 0 {
			found = append(found, Finding{
				File: name, Line: i + 1, Rule: "indent_style",
				Detail: "indented with a space where this path is indented with tabs",
			})
		}
	}
	return found
}

// applyFinalNewline gives the file the newline it ends with. An empty file is
// left empty: a newline added to nothing produces a file holding one blank
// line, which is a byte this rule was not asked to add.
func applyFinalNewline(name string, src []byte) ([]byte, []Finding) {
	if len(src) == 0 || endsWithNewline(src) {
		return src, nil
	}
	return append(append([]byte{}, src...), '\n'), []Finding{{
		File: name, Line: len(splitLines(src)), Rule: "insert_final_newline",
		Detail: "does not end with a newline",
	}}
}

// splitLines returns the lines of src without their terminators. A trailing
// newline does not produce an empty final element, so the count is the number
// of lines a reader would see in an editor.
func splitLines(src []byte) [][]byte {
	if len(src) == 0 {
		return nil
	}
	trimmed := src
	if endsWithNewline(trimmed) {
		trimmed = trimmed[:len(trimmed)-1]
	}
	return bytes.Split(trimmed, []byte("\n"))
}

func joinLines(lines [][]byte, final bool) []byte {
	out := bytes.Join(lines, []byte("\n"))
	if final {
		out = append(out, '\n')
	}
	return out
}

func endsWithNewline(src []byte) bool {
	return len(src) > 0 && src[len(src)-1] == '\n'
}

func leadingWhitespace(line []byte) []byte {
	n := 0
	for n < len(line) && (line[n] == ' ' || line[n] == '\t') {
		n++
	}
	return line[:n]
}

// firstDifferingLine is the line a reader should look at first. Where a
// formatter moved a block, that is the top of the block rather than every line
// under it, which is the position gofmt itself would not give at all.
func firstDifferingLine(before, after []byte) int {
	b, a := splitLines(before), splitLines(after)
	for i := range b {
		if i >= len(a) || !bytes.Equal(b[i], a[i]) {
			return i + 1
		}
	}
	if len(a) > len(b) {
		return len(b) + 1
	}
	return 1
}

// parseErrorLine digs the line out of a scanner error. go/format parses with no
// filename, so the text begins "line:col: message"; a parser given a name emits
// "file:line:col: message" instead. Scanning from the first field reads both,
// because a filename is not a number and a line is.
//
// Where it cannot be read, the answer is the first line rather than a guess: a
// wrong line sends a reader to the wrong place with no sign that it did.
func parseErrorLine(err error) int {
	for _, p := range strings.Split(err.Error(), ":") {
		if n, convErr := strconv.Atoi(strings.TrimSpace(p)); convErr == nil && n > 0 {
			return n
		}
	}
	return 1
}
