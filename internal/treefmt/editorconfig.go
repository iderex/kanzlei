package treefmt

import (
	"bufio"
	"bytes"
	"fmt"
	"strconv"
	"strings"
)

// A Set is a parsed rule set: the sections of an .editorconfig file in the
// order they were written, because a later section lays its properties over an
// earlier one and the order is the whole of that.
type Set struct {
	Root     bool
	sections []section
}

type section struct {
	pattern  string
	patterns []string // pattern with its brace alternatives expanded
	props    map[string]string
	line     int
}

// The properties this package implements. A property outside this set is
// refused when the file is parsed rather than passed over, so a rule somebody
// wrote down is either applied or reported, and never silently absent.
var implemented = map[string]bool{
	"charset":                  true,
	"end_of_line":              true,
	"insert_final_newline":     true,
	"trim_trailing_whitespace": true,
	"indent_style":             true,
	"indent_size":              true,
}

// Parse reads an .editorconfig file. name is used for the position in an error.
//
// Every error names the line, because the thing a reader wants first is which
// line to open. A file with several problems reports the first: the parse has
// already stopped being meaningful, and a list of consequences of one mistake
// reads as several mistakes.
func Parse(name string, src []byte) (*Set, error) {
	set := &Set{}
	sc := bufio.NewScanner(bytes.NewReader(src))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	lineNo := 0
	var current *section

	for sc.Scan() {
		lineNo++
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}

		if strings.HasPrefix(line, "[") {
			if !strings.HasSuffix(line, "]") {
				return nil, fmt.Errorf("%s:%d: section header does not end with ]", name, lineNo)
			}
			pattern := line[1 : len(line)-1]
			expanded, err := expandBraces(pattern)
			if err != nil {
				return nil, fmt.Errorf("%s:%d: %v", name, lineNo, err)
			}
			set.sections = append(set.sections, section{
				pattern:  pattern,
				patterns: expanded,
				props:    map[string]string{},
				line:     lineNo,
			})
			current = &set.sections[len(set.sections)-1]
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("%s:%d: not a section header and not a key = value line", name, lineNo)
		}
		// EditorConfig lowercases keys and, for everything this package reads,
		// values too. Doing it here means the rest of the package compares
		// against one spelling.
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.ToLower(strings.TrimSpace(value))

		if current == nil {
			// The preamble. root is the only thing that belongs there, and a
			// property written above the first section applies to nothing,
			// which is the mistake worth naming rather than ignoring.
			if key != "root" {
				return nil, fmt.Errorf("%s:%d: %q sits above the first section, where it applies to no path", name, lineNo, key)
			}
			b, err := parseBool(value)
			if err != nil {
				return nil, fmt.Errorf("%s:%d: root: %v", name, lineNo, err)
			}
			set.Root = b
			continue
		}

		if !implemented[key] {
			return nil, fmt.Errorf("%s:%d: %q is not a property this repository's checker implements, so a file matching %q would be judged without it", name, lineNo, key, current.pattern)
		}
		if err := checkValue(key, value); err != nil {
			return nil, fmt.Errorf("%s:%d: %s: %v", name, lineNo, key, err)
		}
		current.props[key] = value
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("%s: %v", name, err)
	}
	return set, nil
}

// checkValue refuses a value the transformations cannot carry out. crlf is the
// case worth naming: this repository stores LF, and a rule set that could ask
// for a carriage return would be asking for the byte a published check already
// refuses in the index.
func checkValue(key, value string) error {
	if value == "unset" {
		return nil
	}
	switch key {
	case "charset":
		if value != "utf-8" {
			return fmt.Errorf("%q is not implemented; this repository stores utf-8", value)
		}
	case "end_of_line":
		if value != "lf" {
			return fmt.Errorf("%q is not implemented; this repository stores lf, and .gitattributes is what holds that in the index", value)
		}
	case "insert_final_newline", "trim_trailing_whitespace":
		if _, err := parseBool(value); err != nil {
			return err
		}
	case "indent_style":
		if value != "tab" && value != "space" {
			return fmt.Errorf("%q is neither tab nor space", value)
		}
	case "indent_size":
		n, err := strconv.Atoi(value)
		if err != nil || n <= 0 {
			return fmt.Errorf("%q is not a positive number of spaces", value)
		}
	}
	return nil
}

func parseBool(value string) (bool, error) {
	switch value {
	case "true":
		return true, nil
	case "false":
		return false, nil
	}
	return false, fmt.Errorf("%q is neither true nor false", value)
}

// RulesFor lays every matching section over the ones before it and returns what
// is left. path is slash separated and relative to the directory holding the
// rule set.
//
// The zero value of Rules is every rule unset, so a path no section matches is
// left alone rather than given a default chosen here.
func (s *Set) RulesFor(path string) Rules {
	var r Rules
	for _, sec := range s.sections {
		if !sec.matches(path) {
			continue
		}
		for key, value := range sec.props {
			apply(&r, key, value)
		}
	}
	return r
}

func apply(r *Rules, key, value string) {
	unset := value == "unset"
	switch key {
	case "charset":
		r.Charset = ""
		if !unset {
			r.Charset = value
		}
	case "end_of_line":
		r.EndOfLine = ""
		if !unset {
			r.EndOfLine = value
		}
	case "insert_final_newline":
		r.InsertFinalNewline = value == "true"
	case "trim_trailing_whitespace":
		r.TrimTrailing = value == "true"
	case "indent_style":
		r.IndentStyle = ""
		if !unset {
			r.IndentStyle = value
		}
	case "indent_size":
		r.IndentSize = 0
		if !unset {
			// Refused at parse time unless it converts, so the error here
			// cannot be reached from a parsed set.
			n, err := strconv.Atoi(value)
			if err == nil {
				r.IndentSize = n
			}
		}
	}
}

func (s section) matches(path string) bool {
	for _, pat := range s.patterns {
		// A pattern with no separator in it is about the name of a file
		// wherever it sits. One with a separator is anchored at the directory
		// holding the rule set, which here is the repository root.
		if !strings.Contains(pat, "/") {
			pat = "**/" + pat
		}
		if globMatch(pat, path) {
			return true
		}
	}
	return false
}

// expandBraces turns one pattern into the list of patterns its alternatives
// stand for. A range like {1..9} and a character class like [ch] are refused
// rather than treated as literal text: a pattern that matched nothing while
// looking like it matched something is the failure this whole file is arranged
// against.
func expandBraces(pattern string) ([]string, error) {
	if strings.ContainsAny(pattern, "[]") {
		return nil, fmt.Errorf("pattern %q uses a character class, which this checker does not implement", pattern)
	}
	open := strings.IndexByte(pattern, '{')
	if open < 0 {
		if strings.IndexByte(pattern, '}') >= 0 {
			return nil, fmt.Errorf("pattern %q closes a brace it never opened", pattern)
		}
		return []string{pattern}, nil
	}
	depth := 0
	closeAt := -1
	for i := open; i < len(pattern); i++ {
		switch pattern[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				closeAt = i
			}
		}
		if closeAt >= 0 {
			break
		}
	}
	if closeAt < 0 {
		return nil, fmt.Errorf("pattern %q opens a brace it never closes", pattern)
	}
	inner := pattern[open+1 : closeAt]
	if strings.Contains(inner, "..") {
		return nil, fmt.Errorf("pattern %q uses a numeric range, which this checker does not implement", pattern)
	}
	var out []string
	for _, alt := range splitTop(inner) {
		rest, err := expandBraces(pattern[:open] + alt + pattern[closeAt+1:])
		if err != nil {
			return nil, err
		}
		out = append(out, rest...)
	}
	return out, nil
}

// splitTop splits on the commas that are not inside a nested brace group.
func splitTop(s string) []string {
	var out []string
	depth, start := 0, 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
		case ',':
			if depth == 0 {
				out = append(out, s[start:i])
				start = i + 1
			}
		}
	}
	return append(out, s[start:])
}

// globMatch is the EditorConfig subset this package implements: * for a run
// inside one path element, ** for a run across them, ? for one character that
// is not a separator, and literal text. Everything else was refused when the
// pattern was expanded.
func globMatch(pat, name string) bool {
	switch {
	case pat == "":
		return name == ""
	case strings.HasPrefix(pat, "**"):
		rest := pat[2:]
		// **/ stands for zero or more directories, so the case where it stands
		// for none has to be tried without the separator it is written with.
		if strings.HasPrefix(rest, "/") && globMatch(rest[1:], name) {
			return true
		}
		for i := 0; i <= len(name); i++ {
			if globMatch(rest, name[i:]) {
				return true
			}
		}
		return false
	case strings.HasPrefix(pat, "*"):
		rest := pat[1:]
		for i := 0; i <= len(name); i++ {
			if i > 0 && name[i-1] == '/' {
				break
			}
			if globMatch(rest, name[i:]) {
				return true
			}
		}
		return false
	case strings.HasPrefix(pat, "?"):
		if name == "" || name[0] == '/' {
			return false
		}
		return globMatch(pat[1:], name[1:])
	default:
		if name == "" || pat[0] != name[0] {
			return false
		}
		return globMatch(pat[1:], name[1:])
	}
}
