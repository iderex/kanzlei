// Package importrules decides whether the import graph inside this module is
// the one the tree declares.
//
// docs/layout.md argues which package may import which and why. This package is
// what refuses a violation, and import-rules.txt is the data it reads. The note
// is the reason and the file is the rule, so the two cannot drift into
// disagreeing about what is permitted.
//
// The declaration is a permission list rather than a list of forbidden edges.
// An edge nobody thought about is refused rather than allowed, which is the
// direction that matters here: the boundary this project rests on is that the
// index is reached through the one package that applies the permission filter,
// and a rule that only refuses the shapes somebody imagined is a rule the next
// shape walks past.
//
// The list fails closed in both directions. A package in the tree with no line
// is refused, so the declaration cannot fall behind the tree. A line naming a
// package that is not in the tree is refused as a typo, unless it is marked
// planned, which is a debt carrying the issue that retires it rather than a
// place to hide one.
//
// This package is not part of the binary. It is here for the reason
// docs/decisions/0001-means.md gives: it decides whether the tree is
// acceptable, and that decision belongs somewhere fixtures can be put in front
// of it.
package importrules

import (
	"bufio"
	"bytes"
	"fmt"
	"sort"
	"strings"
)

// Rules is what import-rules.txt declares.
//
// A package is declared by exactly one shipped line, which is the complete set
// of packages inside this module its non-test files may import. Everything else
// hangs off that line: a test addition, a planned marker and a test-only marker
// all name a package that has one.
type Rules struct {
	order    []string            // declaration order, so a report reads like the file
	shipped  map[string][]string // package -> what its shipped files may import
	test     map[string][]string // package -> what its test files may import as well
	planned  map[string]string   // package -> the reference that creates it
	testOnly map[string]string   // package -> the reference that says why
}

// Parse reads a declaration.
//
// name is used for the reported position and for nothing else, so a caller with
// bytes in hand does not have to write them to disk first. That is what lets
// the fixtures below stay in source.
//
// Everything that can be decided from the file alone is decided here rather
// than at check time: a duplicate line, a target with no declaration of its
// own, a package permitted to import itself, and a shipped line naming a
// test-only package are all refused without reading a tree. A declaration that
// cannot be read is a worse failure than one that is violated, because the
// violation is at least reported.
func Parse(name string, src []byte) (*Rules, error) {
	r := &Rules{
		shipped:  map[string][]string{},
		test:     map[string][]string{},
		planned:  map[string]string{},
		testOnly: map[string]string{},
	}

	scanner := bufio.NewScanner(bytes.NewReader(src))
	line := 0
	for scanner.Scan() {
		line++
		text := strings.TrimSpace(scanner.Text())
		// A comment is a whole line. There is no comment that starts in the
		// middle of one, because a reference to an issue begins with the same
		// character and a rule file that ate half of its own lines would be
		// read by nobody twice.
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		if err := r.parseLine(name, line, text); err != nil {
			return nil, err
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read %s: %w", name, err)
	}
	if len(r.order) == 0 {
		return nil, fmt.Errorf("%s declares no package; an empty rule set permits everything", name)
	}
	if err := r.resolve(name); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *Rules) parseLine(name string, line int, text string) error {
	where := fmt.Sprintf("%s:%d", name, line)

	if marker, rest, ok := strings.Cut(text, " "); ok && (marker == "planned" || marker == "test-only") {
		fields := strings.Fields(rest)
		if len(fields) < 2 {
			return fmt.Errorf("%s: %s names no reference; a marker carries the issue that retires it", where, marker)
		}
		pkg, reference := fields[0], strings.Join(fields[1:], " ")
		target := r.planned
		if marker == "test-only" {
			target = r.testOnly
		}
		if _, seen := target[pkg]; seen {
			return fmt.Errorf("%s: %s %s is declared twice", where, marker, pkg)
		}
		target[pkg] = reference
		return nil
	}

	left, right, ok := strings.Cut(text, ":")
	if !ok {
		return fmt.Errorf("%s: %q is neither a marker nor a package line; a package line carries a colon", where, text)
	}
	head := strings.Fields(left)
	if len(head) == 0 {
		return fmt.Errorf("%s: the line names no package", where)
	}
	pkg := head[0]
	kind := "shipped"
	switch {
	case len(head) == 1:
	case len(head) == 2 && head[1] == "+test":
		kind = "test"
	default:
		return fmt.Errorf("%s: %q is not a package line; the only thing that may follow a package is +test", where, left)
	}

	allowed := strings.Fields(right)
	for i, target := range allowed {
		if target == pkg {
			if kind == "test" {
				return fmt.Errorf("%s: %s may not be listed on its own test line; a test file may import the package it tests without saying so", where, pkg)
			}
			return fmt.Errorf("%s: %s may not import itself", where, pkg)
		}
		for _, earlier := range allowed[:i] {
			if earlier == target {
				return fmt.Errorf("%s: %s is listed twice", where, target)
			}
		}
	}

	if kind == "test" {
		if _, seen := r.test[pkg]; seen {
			return fmt.Errorf("%s: %s has two test lines", where, pkg)
		}
		r.test[pkg] = allowed
		return nil
	}
	if _, seen := r.shipped[pkg]; seen {
		return fmt.Errorf("%s: %s is declared twice", where, pkg)
	}
	r.shipped[pkg] = allowed
	r.order = append(r.order, pkg)
	return nil
}

// resolve refuses what only the whole file can show: a line hanging off a
// package that has no declaration, and a permission naming one.
func (r *Rules) resolve(name string) error {
	for _, hangers := range []struct {
		what string
		set  map[string]string
	}{{"planned", r.planned}, {"test-only", r.testOnly}} {
		for pkg := range hangers.set {
			if _, declared := r.shipped[pkg]; !declared {
				return fmt.Errorf("%s: %s %s has no package line; a marker without one declares nothing", name, hangers.what, pkg)
			}
		}
	}
	for pkg := range r.test {
		if _, declared := r.shipped[pkg]; !declared {
			return fmt.Errorf("%s: %s has a test line and no package line", name, pkg)
		}
	}

	for _, pkg := range r.order {
		for _, target := range r.shipped[pkg] {
			if _, declared := r.shipped[target]; !declared {
				return fmt.Errorf("%s: %s is permitted to import %s, which has no declaration of its own", name, pkg, target)
			}
			if reference, only := r.testOnly[target]; only {
				return fmt.Errorf("%s: %s is permitted to import %s, which is test-only (%s)", name, pkg, target, reference)
			}
		}
		for _, target := range r.test[pkg] {
			if _, declared := r.shipped[target]; !declared {
				return fmt.Errorf("%s: the test files of %s are permitted to import %s, which has no declaration of its own", name, pkg, target)
			}
		}
	}
	return nil
}

// Declared reports the packages the file holds, in the order it holds them.
func (r *Rules) Declared() []string { return append([]string(nil), r.order...) }

// A Finding is one thing the tree and the declaration disagree about.
type Finding struct {
	Kind    string   // what shape of disagreement this is
	Package string   // the package the finding is about
	Import  string   // the import that was refused, where there is one
	Chain   []string // how a shipped build reaches Package, where anything does
	Detail  string
}

func (f Finding) String() string {
	s := f.Package
	if f.Import != "" {
		s += " -> " + f.Import
	}
	s += ": " + f.Detail
	if len(f.Chain) > 1 {
		s += "; reached by " + strings.Join(f.Chain, " -> ")
	}
	return s
}

// Check reports every disagreement between the declaration and the graph.
//
// It reports all of them rather than the first, because a reader repairing a
// declaration wants the list and not one line of it at a time.
//
// What it does not read is the reason an edge exists. An import that is
// permitted here may still be the wrong idea, and nothing in this package has
// an opinion about that.
func Check(rules *Rules, graph *Graph) []Finding {
	var findings []Finding

	for _, pkg := range graph.Packages {
		if _, declared := rules.shipped[pkg]; declared {
			continue
		}
		findings = append(findings, Finding{
			Kind:    "undeclared package",
			Package: pkg,
			Detail:  "is in the tree and has no line in the rule file",
		})
	}

	for _, pkg := range rules.order {
		_, inTree := graph.Shipped[pkg]
		reference, planned := rules.planned[pkg]
		switch {
		case inTree && planned:
			findings = append(findings, Finding{
				Kind:    "stale planned marker",
				Package: pkg,
				Detail:  fmt.Sprintf("is in the tree and is still marked planned (%s)", reference),
			})
		case !inTree && !planned:
			findings = append(findings, Finding{
				Kind:    "declaration for a package that is not there",
				Package: pkg,
				Detail:  "is declared and is not in the tree; mark it planned or remove the line",
			})
		}
	}

	for _, pkg := range graph.Packages {
		permitted := map[string]bool{}
		for _, target := range rules.shipped[pkg] {
			permitted[target] = true
		}
		for _, imported := range graph.Shipped[pkg] {
			if permitted[imported] {
				continue
			}
			findings = append(findings, Finding{
				Kind:    "forbidden import",
				Package: pkg,
				Import:  imported,
				Chain:   graph.chainTo(pkg),
				Detail:  detailFor(rules, pkg, imported, false),
			})
		}
		// A test file may import the package it tests, which is what an
		// external test package does, and it needs no line to say so.
		permitted[pkg] = true
		for _, target := range rules.test[pkg] {
			permitted[target] = true
		}
		for _, imported := range graph.Test[pkg] {
			if permitted[imported] {
				continue
			}
			findings = append(findings, Finding{
				Kind:    "forbidden import in a test file",
				Package: pkg,
				Import:  imported,
				Detail:  detailFor(rules, pkg, imported, true),
			})
		}
	}

	sort.Slice(findings, func(i, j int) bool {
		if findings[i].Package != findings[j].Package {
			return findings[i].Package < findings[j].Package
		}
		if findings[i].Import != findings[j].Import {
			return findings[i].Import < findings[j].Import
		}
		return findings[i].Kind < findings[j].Kind
	})
	return findings
}

// detailFor says why the import was refused in the terms the reader needs,
// which is not the same sentence every time. An import of a package that is
// declared test-only is a different mistake from an import nobody permitted,
// and a reader told only "not permitted" would go looking for the wrong repair.
func detailFor(rules *Rules, pkg, imported string, inTest bool) string {
	if reference, only := rules.testOnly[imported]; only && !inTest {
		return fmt.Sprintf("%s is test-only (%s) and this is a file that ships", imported, reference)
	}
	if _, declared := rules.shipped[imported]; !declared {
		return fmt.Sprintf("%s has no line in the rule file at all", imported)
	}
	if inTest {
		return fmt.Sprintf("the test files of %s are not permitted to import it", pkg)
	}
	return fmt.Sprintf("%s is not permitted to import it", pkg)
}
