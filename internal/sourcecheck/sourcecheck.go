// Package sourcecheck reads this repository's own Go source and refuses things
// no analyser can see.
//
// It holds one rule today: a comment that switches an analyser off has to say
// why, on the line it switches it off. An analyser suppression is the one edit
// that makes a gate green without making the code right, and it is invisible in
// a summary that reports the gate passed. A reason next to it is what turns a
// suppression into a decision somebody can disagree with later.
//
// This package is not part of the binary. It is here because the check is
// logic, and logic that decides whether the tree is acceptable belongs
// somewhere a fixture can be put in front of it rather than in a shell block
// nobody can run a case against.
package sourcecheck

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// directives are the comment prefixes that turn an analyser off. Each is
// matched at the start of a comment's text, after the slashes.
//
// The list is what has to be kept current, and nothing derives it: an analyser
// added later with a suppression syntax of its own is invisible here until its
// prefix is added. That is the known limit of this check and it is written into
// its test.
var directives = []string{
	"nolint",             // golangci-lint
	"lint:ignore",        // staticcheck
	"lint:file-ignore",   // staticcheck, whole file
	"errcheck:ignore",    // errcheck
	"staticcheck:ignore", // written by people who expect it to work
	"noinspection",       // the editors that write this one
}

// A Suppression is one analyser directive found in source.
type Suppression struct {
	File      string
	Line      int
	Directive string // the directive as written, without the slashes
	Reason    string // what followed it on the same line
}

func (s Suppression) String() string {
	return fmt.Sprintf("%s:%d: %s", s.File, s.Line, s.Directive)
}

// minimumReasonWords is what separates a reason from a gesture. One word is a
// label, and a label is what somebody writes when the reason is "to make it
// pass".
const minimumReasonWords = 3

// Bare reports every suppression in src that carries no reason.
//
// filename is used for the reported position and for nothing else, so a caller
// with bytes in hand does not have to write them to disk first. That is what
// lets the fixtures below stay in source.
func Bare(filename string, src []byte) ([]Suppression, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, src, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", filename, err)
	}

	// Line comments by the line they are on, which is how a reason written
	// after a discard is found.
	byLine := map[int]string{}
	var bare []Suppression
	for _, group := range file.Comments {
		for _, comment := range group.List {
			text := strings.TrimPrefix(comment.Text, "//")
			if text == comment.Text {
				// A /* */ comment. An analyser directive is never written as
				// one, because none of the tools reads it there.
				continue
			}
			line := fset.Position(comment.Slash).Line
			if _, seen := byLine[line]; !seen {
				byLine[line] = text
			}

			_, reason, found := split(text)
			if !found {
				continue
			}
			if words(reason) >= minimumReasonWords {
				continue
			}
			bare = append(bare, Suppression{
				File:      filename,
				Line:      line,
				Directive: strings.TrimSpace(text),
				Reason:    strings.TrimSpace(reason),
			})
		}
	}

	// The second shape of suppression, and the one the analysers cannot see at
	// all: a call whose result is assigned to the blank identifier. errcheck
	// finds it only with -blank, and -blank cannot be told the difference
	// between a discard somebody thought about and one somebody typed to make a
	// compiler complaint go away. The reason on the line is that difference.
	ast.Inspect(file, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok || !discardsACallResult(assign) {
			return true
		}
		line := fset.Position(assign.Pos()).Line
		if words(byLine[line]) >= minimumReasonWords {
			return true
		}
		bare = append(bare, Suppression{
			File:      filename,
			Line:      line,
			Directive: "a discarded call result",
			Reason:    strings.TrimSpace(byLine[line]),
		})
		return true
	})

	sort.Slice(bare, func(i, j int) bool { return bare[i].Line < bare[j].Line })
	return bare, nil
}

// discardsACallResult reports whether an assignment throws away the last thing
// a call returned.
//
// It reads the syntax and not the types, so it cannot say whether the thing
// thrown away was an error. What it uses instead is the convention every Go
// program follows: the error comes last. So the position is the test, and a
// blank anywhere else is left alone, which is what keeps an ordinary
// destructuring of three return values out of this.
//
// The bound is that convention. A function returning its error first, or
// returning two, is read by this as a discard of something else or not read at
// all. Nothing here consults the types, and #111 is where a check that does
// would belong.
func discardsACallResult(assign *ast.AssignStmt) bool {
	if len(assign.Lhs) == 0 {
		return false
	}
	last, ok := assign.Lhs[len(assign.Lhs)-1].(*ast.Ident)
	if !ok || last.Name != "_" {
		return false
	}
	for _, rhs := range assign.Rhs {
		if _, ok := rhs.(*ast.CallExpr); ok {
			return true
		}
	}
	return false
}

// split reports whether text starts with a directive, and returns the directive
// and the reason written after it.
//
// What is stepped over before the reason begins is the part the tool reads: the
// analyser list after a colon, the check identifier a staticcheck directive
// takes, and the second pair of slashes people write to separate the two. None
// of that is a reason, and counting it as one would let `//nolint:errcheck`
// pass for holding three words.
func split(text string) (directive, reason string, found bool) {
	trimmed := strings.TrimLeft(text, " 	")
	for _, d := range directives {
		if !strings.HasPrefix(trimmed, d) {
			continue
		}
		after := trimmed[len(d):]
		switch {
		case strings.HasPrefix(after, ":"):
			// A colon and then the analyser list, with no space between them.
			// The example is not written out here: this check reads its own
			// source, and a comment holding the literal shape would be refused
			// by the rule it documents.
			after = firstTokenRemoved(after[1:])
		case d == "lint:ignore" || d == "lint:file-ignore":
			// A space and then the check identifier, then the reason.
			after = firstTokenRemoved(strings.TrimLeft(after, " 	"))
		}
		after = strings.TrimLeft(after, " 	")
		after = strings.TrimPrefix(after, "//")
		return d, after, true
	}
	return "", "", false
}

// firstTokenRemoved drops everything up to the first space.
func firstTokenRemoved(s string) string {
	if i := strings.IndexAny(s, " 	"); i >= 0 {
		return s[i:]
	}
	return ""
}

func words(s string) int { return len(strings.Fields(s)) }

// BareInTree reports every bare suppression in a .go file under root.
//
// Directories a Go build never looks at are skipped: anything starting with a
// dot or an underscore, and testdata. A suppression in a fixture is part of the
// fixture.
func BareInTree(root string) ([]Suppression, error) {
	var bare []Suppression
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() {
			if path != root && (strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") || name == "testdata") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(name, ".go") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		found, err := Bare(filepath.ToSlash(path), src)
		if err != nil {
			return err
		}
		bare = append(bare, found...)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return bare, nil
}
