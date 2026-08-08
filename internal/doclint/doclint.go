// Package doclint reads this repository's own documents and refuses a
// reference that does not resolve.
//
// Several issues in this plan have a sentence in a document as their
// done-condition, which makes a document load bearing. A load-bearing document
// that names a path decays the same way an unlinted codebase does, and it
// decays invisibly: the file is renamed, the sentence still reads correctly,
// and the next person follows it to a path that is not there.
//
// It holds one rule today. Every path a document names has to be a path this
// repository has, or one it has declared it is going to have. What counts as
// naming a path is deliberately narrow and is written out in References below,
// because a checker that guesses is a checker whose findings get argued with
// rather than fixed.
//
// This package is not part of the binary. It is here rather than in a shell
// block inside a workflow for the reason docs/decisions/0001-means.md gives: a
// rule with no fixtures in front of it is a rule nobody can prove bites.
package doclint

import (
	"fmt"
	"path"
	"sort"
	"strings"
)

// A File is one document to read, named by its path from the repository root
// so that a relative reference inside it can be resolved against the directory
// it sits in.
type File struct {
	Name  string
	Bytes []byte
}

// A Reference is one place a document names a path.
type Reference struct {
	File string
	Line int
	Kind string // "code span" or "link"
	Text string // the path as the document wrote it
	Path string // the same path, resolved from the repository root
}

// A Finding is a reference that resolves to nothing.
type Finding struct {
	Reference
}

func (f Finding) String() string {
	return fmt.Sprintf("%s:%d: %s names %s, which is not in the tree and is not declared planned", f.File, f.Line, f.Kind, f.Text)
}

// A Tree is what a reference is resolved against.
//
// Planned is why this is a struct rather than a list of tracked paths. A
// document is allowed to name a package that has not been written yet, and
// this repository already has one register saying which those are, with the
// issue that retires each one. Reading that register here means a planned path
// is legible in one place instead of two, and the day the package lands the
// marker goes stale in the register rather than silently here.
type Tree struct {
	Tracked []string
	Planned []string

	files map[string]bool
	dirs  map[string]bool
	plan  map[string]bool
	roots map[string]bool
}

// NewTree indexes the paths a reference may resolve to. Every directory
// implied by a tracked path is one of them, because a document naming
// `docs/decisions/` is naming something the tree has even though git tracks no
// such entry.
func NewTree(tracked, planned []string) *Tree {
	t := &Tree{
		Tracked: append([]string(nil), tracked...),
		Planned: append([]string(nil), planned...),
		files:   map[string]bool{},
		dirs:    map[string]bool{},
		plan:    map[string]bool{},
		roots:   map[string]bool{},
	}
	for _, p := range tracked {
		p = path.Clean(p)
		t.files[p] = true
		for dir := path.Dir(p); dir != "." && dir != "/"; dir = path.Dir(dir) {
			t.dirs[dir] = true
		}
		t.roots[firstSegment(p)] = true
	}
	for _, p := range planned {
		p = path.Clean(p)
		t.plan[p] = true
		t.roots[firstSegment(p)] = true
	}
	return t
}

// Has reports whether a resolved path is something this repository holds or
// has declared it will hold.
func (t *Tree) Has(p string) bool {
	p = path.Clean(strings.TrimSuffix(p, "/"))
	return t.files[p] || t.dirs[p] || t.plan[p]
}

// names reports whether a string is written as a path into this repository at
// all. It is the whole of the guess this check makes, and it is made as small
// as it can be: the first segment has to be a directory this repository's own
// tree or its planned register already names.
//
// That is what keeps `go/format`, `application/json` and every other
// slash-carrying phrase out of the finding list. It also fixes what this check
// cannot see: a path whose first segment is not one of those, `go.mod` and
// `DCO` among them. This is a floor under the documents rather than a proof
// about them.
func (t *Tree) names(s string) bool {
	if s == "" || strings.ContainsAny(s, " \t") {
		return false
	}
	if strings.Contains(s, "://") || strings.HasPrefix(s, "mailto:") {
		return false
	}
	s = strings.TrimPrefix(s, "./")
	if !strings.Contains(s, "/") {
		return false
	}
	return t.roots[firstSegment(s)]
}

func firstSegment(p string) string {
	if i := strings.Index(p, "/"); i >= 0 {
		return p[:i]
	}
	return p
}

// References reads one document and reports every path it names.
//
// Two positions are read and no others.
//
// An inline code span, which is where this repository writes a path in prose.
// It is resolved from the repository root, because that is how the surrounding
// sentences read: a span saying `internal/authz` means the package of that
// name and not a sibling of the document.
//
// A link target, `[text](target)`, resolved against the directory the document
// sits in, because that is what a markdown reader does with it.
//
// What is not read is as important. A fenced block and an indented block are
// both passed over: they hold commands, and a command line carries file names
// that are outputs, arguments and revisions rather than paths in this tree.
// `coverage.out` is written by a command in this repository's own contributor
// guide and is deliberately not tracked, so a check that read command blocks
// would refuse the guide for being correct.
func References(f File, t *Tree) []Reference {
	var refs []Reference
	fenced := false
	for i, raw := range strings.Split(string(f.Bytes), "\n") {
		line := i + 1
		if fence := fenceDelimiter(raw); fence != "" {
			fenced = !fenced
			continue
		}
		if fenced || indented(raw) {
			continue
		}
		spans, prose := splitSpans(raw)
		for _, span := range spans {
			if !t.names(span) {
				continue
			}
			refs = append(refs, Reference{
				File: f.Name,
				Line: line,
				Kind: "code span",
				Text: span,
				Path: path.Clean(strings.TrimPrefix(span, "./")),
			})
		}
		for _, target := range linkTargets(prose) {
			local, ok := localTarget(target)
			if !ok {
				continue
			}
			refs = append(refs, Reference{
				File: f.Name,
				Line: line,
				Kind: "link",
				Text: target,
				Path: path.Clean(path.Join(path.Dir(f.Name), local)),
			})
		}
	}
	return refs
}

// Check reads every document and reports the references that resolve to
// nothing.
func Check(files []File, t *Tree) []Finding {
	var findings []Finding
	for _, f := range files {
		for _, r := range References(f, t) {
			if t.Has(r.Path) {
				continue
			}
			findings = append(findings, Finding{Reference: r})
		}
	}
	sort.SliceStable(findings, func(i, j int) bool {
		if findings[i].File != findings[j].File {
			return findings[i].File < findings[j].File
		}
		return findings[i].Line < findings[j].Line
	})
	return findings
}

// fenceDelimiter reports the fence a line opens or closes, and the empty
// string where it is not a fence line at all.
func fenceDelimiter(line string) string {
	trimmed := strings.TrimLeft(line, " ")
	for _, marker := range []string{"```", "~~~"} {
		if strings.HasPrefix(trimmed, marker) {
			return marker
		}
	}
	return ""
}

// indented reports a line held inside an indented code block. Four spaces is
// what markdown reads as one, and it is how this repository's own documents
// write a command.
func indented(line string) bool {
	return strings.HasPrefix(line, "    ") || strings.HasPrefix(line, "\t")
}

// splitSpans separates a line into the contents of its inline code spans and
// the prose around them, in the order they appear. Backtick runs delimit a
// span and the run lengths have to match, which is what lets a span hold a
// backtick of its own.
//
// The prose half is what the link rule reads, and the separation is the whole
// reason for it: a document explaining the link syntax writes `[text](target)`
// inside a span, and markdown shows that rather than linking it. This check
// refused its own documentation for that sentence before the two were split,
// which is the near miss the case for it is built from.
func splitSpans(line string) (spans []string, prose string) {
	out := make([]byte, 0, len(line))
	rest := line
	for {
		open := strings.IndexByte(rest, '`')
		if open < 0 {
			return spans, string(append(out, rest...))
		}
		out = append(out, rest[:open]...)
		rest = rest[open:]
		run := 0
		for run < len(rest) && rest[run] == '`' {
			run++
		}
		marker := rest[:run]
		body := rest[run:]
		end := strings.Index(body, marker)
		if end < 0 {
			// An unclosed run is a stray backtick rather than a span, so the
			// rest of the line stays prose and nothing is swallowed.
			return spans, string(append(out, rest...))
		}
		spans = append(spans, strings.TrimSpace(body[:end]))
		rest = body[end+run:]
	}
}

// linkTargets returns the target of every inline link on a line. A reference
// definition and an image share the shape and are read the same way, because a
// path that has moved is wrong in all three.
func linkTargets(line string) []string {
	var targets []string
	rest := line
	for {
		open := strings.Index(rest, "](")
		if open < 0 {
			return targets
		}
		body := rest[open+2:]
		end := strings.IndexByte(body, ')')
		if end < 0 {
			return targets
		}
		target := strings.TrimSpace(body[:end])
		// A title after the target is part of the link and not part of the
		// path: [text](path "title").
		if i := strings.IndexAny(target, " \t"); i >= 0 {
			target = target[:i]
		}
		targets = append(targets, target)
		rest = body[end+1:]
	}
}

// localTarget returns the path part of a link target that points inside this
// repository, and reports whether it is one. A fragment on its own points at a
// heading in the same document, which this rule does not read, and an absolute
// address points somewhere no reading of the tree can settle.
func localTarget(target string) (string, bool) {
	if target == "" || strings.HasPrefix(target, "#") {
		return "", false
	}
	if strings.Contains(target, "://") || strings.HasPrefix(target, "mailto:") {
		return "", false
	}
	if i := strings.IndexByte(target, '#'); i >= 0 {
		target = target[:i]
	}
	if target == "" {
		return "", false
	}
	return target, true
}
