// Package invariants reads a declaration file and refuses the shapes in this
// repository's own Go source that it forbids.
//
// The invariants worth holding here are the ones where a single wrong line
// silently removes a control: a route registered somewhere the routing table
// does not show, a credential written into the source, a suite behind a build
// constraint that never reaches the gate stating what it needs, a right granted
// by an environment variable that no group mapping shows. None of them is
// visible to a type checker, all of them are visible in the syntax, and each
// arrives as one line somebody wrote in a hurry.
//
// The rules are in invariants.txt rather than in this file. That is the same
// arrangement import-rules.txt and .editorconfig already have here, and it is
// what makes adding an invariant a declaration rather than a change to a
// checker. It holds for an invariant whose shape one of the operators below
// already expresses; a shape none of them expresses needs an operator first,
// and Parse refuses a record naming a shape that does not exist rather than
// passing over it.
//
// This package is not part of the binary. It reads syntax and never types, so
// what it says about a call is that a call with that name is written there,
// which is what a grep would have said with none of the fence-and-comment
// exceptions a grep gets wrong.
package invariants

import (
	"fmt"
	"go/ast"
	"go/build/constraint"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// The shapes an invariant may declare. A record naming anything else is
// refused when the file is read, before any tree is looked at, so a rule
// nothing applies cannot sit in the file looking like one that does.
const (
	ShapeCallOnlyIn         = "call-only-in"
	ShapeNamedStringLiteral = "named-string-literal"
	ShapeConstrainedTest    = "constrained-test-calls"
	ShapeLiteralArgument    = "literal-argument"
)

// An Invariant is one record from the declaration file.
type Invariant struct {
	ID         string
	Shape      string
	Reason     string
	Names      []string
	Paths      []string
	Words      []string
	Constraint string
	Line       int // where the record starts, for a refusal that names it
}

// A Violation is one place in the tree where an invariant does not hold.
type Violation struct {
	Invariant string
	Reason    string
	File      string
	Line      int
	Detail    string
}

func (v Violation) String() string {
	return fmt.Sprintf("%s:%d: %s: %s", v.File, v.Line, v.Invariant, v.Detail)
}

// Parse reads the declaration file.
//
// filename is used for the reported position and for nothing else, so a caller
// with bytes in hand does not have to write them to disk first. That is what
// lets this package's own fixtures stay in source.
func Parse(filename string, src []byte) ([]Invariant, error) {
	var (
		out     []Invariant
		current *Invariant
		seen    = map[string]int{}
	)

	finish := func() error {
		if current == nil {
			return nil
		}
		if err := current.validate(filename); err != nil {
			return err
		}
		if at, dup := seen[current.ID]; dup {
			return fmt.Errorf("%s:%d: invariant %s is already declared at line %d", filename, current.Line, current.ID, at)
		}
		seen[current.ID] = current.Line
		out = append(out, *current)
		current = nil
		return nil
	}

	for i, raw := range strings.Split(string(src), "\n") {
		line := i + 1
		text := strings.TrimRight(raw, "\r")
		if strings.HasPrefix(strings.TrimSpace(text), "#") {
			continue
		}
		if strings.TrimSpace(text) == "" {
			if err := finish(); err != nil {
				return nil, err
			}
			continue
		}

		key, value, found := strings.Cut(text, ":")
		if !found {
			return nil, fmt.Errorf("%s:%d: %q is not a key and a value separated by a colon", filename, line, text)
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)

		if key == "invariant" {
			if err := finish(); err != nil {
				return nil, err
			}
			if value == "" {
				return nil, fmt.Errorf("%s:%d: an invariant with no name", filename, line)
			}
			current = &Invariant{ID: value, Line: line}
			continue
		}
		if current == nil {
			return nil, fmt.Errorf("%s:%d: %q before any invariant is declared", filename, line, key)
		}

		switch key {
		case "shape":
			current.Shape = value
		case "reason":
			current.Reason = value
		case "names":
			current.Names = strings.Fields(value)
		case "paths":
			current.Paths = strings.Fields(value)
		case "words":
			current.Words = strings.Fields(value)
		case "constraint":
			current.Constraint = value
		default:
			// An unknown key is refused rather than ignored, for the reason
			// the shape list gives: a line that looks like a rule and is read
			// by nothing is worse than an absent one.
			return nil, fmt.Errorf("%s:%d: invariant %s declares %q, which nothing reads", filename, line, current.ID, key)
		}
	}
	if err := finish(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%s: no invariant is declared", filename)
	}
	return out, nil
}

func (inv *Invariant) validate(filename string) error {
	where := fmt.Sprintf("%s:%d: invariant %s", filename, inv.Line, inv.ID)
	if inv.Reason == "" {
		return fmt.Errorf("%s states no reason", where)
	}
	switch inv.Shape {
	case ShapeCallOnlyIn:
		if len(inv.Names) == 0 {
			return fmt.Errorf("%s names no call", where)
		}
		if len(inv.Paths) == 0 {
			return fmt.Errorf("%s names no path the call is allowed in", where)
		}
	case ShapeNamedStringLiteral:
		if len(inv.Names) == 0 {
			return fmt.Errorf("%s names no identifier", where)
		}
	case ShapeConstrainedTest:
		if inv.Constraint == "" {
			return fmt.Errorf("%s names no build constraint", where)
		}
		if len(inv.Names) == 0 {
			return fmt.Errorf("%s names no call a case has to reach", where)
		}
	case ShapeLiteralArgument:
		if len(inv.Names) == 0 {
			return fmt.Errorf("%s names no call", where)
		}
		if len(inv.Words) == 0 {
			return fmt.Errorf("%s names no word the argument may not carry", where)
		}
	case "":
		return fmt.Errorf("%s declares no shape", where)
	default:
		return fmt.Errorf("%s declares the shape %q, which no operator implements", where, inv.Shape)
	}
	return nil
}

// Check reports every violation of inv in one Go file.
//
// filename is the path from the repository root, in slash form, because two of
// the three shapes decide by where a file sits.
func Check(inv []Invariant, filename string, src []byte) ([]Violation, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, src, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", filename, err)
	}

	var found []Violation
	for _, rule := range inv {
		switch rule.Shape {
		case ShapeCallOnlyIn:
			found = append(found, callOnlyIn(rule, fset, file, filename)...)
		case ShapeNamedStringLiteral:
			found = append(found, namedStringLiteral(rule, fset, file, filename)...)
		case ShapeConstrainedTest:
			found = append(found, constrainedTestCalls(rule, fset, file, filename)...)
		case ShapeLiteralArgument:
			found = append(found, literalArgument(rule, fset, file, filename)...)
		}
	}
	sort.SliceStable(found, func(i, j int) bool { return found[i].Line < found[j].Line })
	return found, nil
}

// callOnlyIn refuses a call to one of the named functions from a file outside
// the directories the rule allows.
//
// The name is matched on its own, so both a plain call and one through a
// receiver or a package are read. That is deliberate: what the rule is about
// is that a route is registered, and which value it was registered on is the
// detail a refactor changes.
func callOnlyIn(rule Invariant, fset *token.FileSet, file *ast.File, filename string) []Violation {
	if within(path.Dir(filename), rule.Paths) {
		return nil
	}
	var found []Violation
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		name := calledName(call.Fun)
		if name == "" || !contains(rule.Names, name) {
			return true
		}
		found = append(found, Violation{
			Invariant: rule.ID,
			Reason:    rule.Reason,
			File:      filename,
			Line:      fset.Position(call.Pos()).Line,
			Detail:    fmt.Sprintf("%s is called here, and only %s may call it", name, strings.Join(rule.Paths, " ")),
		})
		return true
	})
	return found
}

// namedStringLiteral refuses a non-empty string literal bound to an identifier
// whose name carries one of the declared words.
//
// An empty literal is left alone. A name declared with no value in it is a
// placeholder rather than a credential, and refusing it would push the field
// out of the source and teach nothing.
func namedStringLiteral(rule Invariant, fset *token.FileSet, file *ast.File, filename string) []Violation {
	var found []Violation
	report := func(name string, pos token.Pos) {
		found = append(found, Violation{
			Invariant: rule.ID,
			Reason:    rule.Reason,
			File:      filename,
			Line:      fset.Position(pos).Line,
			Detail:    fmt.Sprintf("%s holds a string literal", name),
		})
	}

	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.ValueSpec:
			for i, name := range node.Names {
				if i < len(node.Values) && carries(rule.Names, name.Name) && isNonEmptyString(node.Values[i]) {
					report(name.Name, name.Pos())
				}
			}
		case *ast.AssignStmt:
			for i, lhs := range node.Lhs {
				if i >= len(node.Rhs) {
					break
				}
				name := boundName(lhs)
				if name != "" && carries(rule.Names, name) && isNonEmptyString(node.Rhs[i]) {
					report(name, lhs.Pos())
				}
			}
		case *ast.KeyValueExpr:
			key, ok := node.Key.(*ast.Ident)
			if ok && carries(rule.Names, key.Name) && isNonEmptyString(node.Value) {
				report(key.Name, key.Pos())
			}
		}
		return true
	})
	return found
}

// literalArgument refuses a call to one of the named functions whose string
// literal argument carries one of the declared words.
//
// The rule above reads the name a value is bound to inside a file. This one
// reads a name the process is handed at startup, which no file binds and no
// analyser sees, and the two together are what one done-when line in #34 asks
// for.
//
// Every argument is read rather than only the first. Which position holds the
// name differs between the two spellings a lookup is written in, and a rule
// that counted positions would admit the second spelling of the first mistake.
func literalArgument(rule Invariant, fset *token.FileSet, file *ast.File, filename string) []Violation {
	var found []Violation
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || !contains(rule.Names, calledName(call.Fun)) {
			return true
		}
		for _, arg := range call.Args {
			text, ok := literalText(arg)
			if !ok || !carries(rule.Words, text) {
				continue
			}
			found = append(found, Violation{
				Invariant: rule.ID,
				Reason:    rule.Reason,
				File:      filename,
				Line:      fset.Position(arg.Pos()).Line,
				Detail:    fmt.Sprintf("%s is read here, through %s", text, calledName(call.Fun)),
			})
		}
		return true
	})
	return found
}

// constrainedTestCalls refuses a test case in a constrained file that reaches
// none of the declared calls.
//
// The whole file is skipped when the constraint is not the one the rule names,
// which is what keeps this from reading the default suite.
func constrainedTestCalls(rule Invariant, fset *token.FileSet, file *ast.File, filename string) []Violation {
	if !carriesConstraint(file, rule.Constraint) {
		return nil
	}
	var found []Violation
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv != nil || fn.Body == nil || !isCase(fn) {
			continue
		}
		reached := false
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if contains(rule.Names, calledName(call.Fun)) {
				reached = true
			}
			return !reached
		})
		if reached {
			continue
		}
		found = append(found, Violation{
			Invariant: rule.ID,
			Reason:    rule.Reason,
			File:      filename,
			Line:      fset.Position(fn.Pos()).Line,
			Detail:    fmt.Sprintf("%s reaches none of %s", fn.Name.Name, strings.Join(rule.Names, " ")),
		})
	}
	return found
}

// carriesConstraint reports whether the file is in the build the tag selects
// and out of the build without it.
//
// The expression is evaluated rather than matched as text, and both directions
// are asked. Matching the text would read a file constrained out of that build
// with a negation as though it were in it, which is the opposite of what the
// rule is about: those cases run in the default suite, where the gate this rule
// requires does not exist. Asking only the first direction would read every
// file whose constraint happens to be true with nothing set, which is most of
// the ones that carry a platform.
func carriesConstraint(file *ast.File, tag string) bool {
	set := func(t string) bool { return t == tag }
	unset := func(string) bool { return false }
	for _, group := range file.Comments {
		for _, comment := range group.List {
			expr, err := constraint.Parse(comment.Text)
			if err != nil {
				continue
			}
			if expr.Eval(set) && !expr.Eval(unset) {
				return true
			}
		}
	}
	return false
}

// calledName is the name a call is written under: the identifier for a plain
// call, and the final segment for a call through a selector.
func calledName(fun ast.Expr) string {
	switch f := fun.(type) {
	case *ast.Ident:
		return f.Name
	case *ast.SelectorExpr:
		return f.Sel.Name
	}
	return ""
}

// boundName is the name an assignment writes to, for the two shapes a setting
// is written in: a plain identifier and a field on something.
func boundName(lhs ast.Expr) string {
	switch l := lhs.(type) {
	case *ast.Ident:
		return l.Name
	case *ast.SelectorExpr:
		return l.Sel.Name
	}
	return ""
}

// isNonEmptyString reports whether the expression is a string literal with
// something written between its quotes.
func isNonEmptyString(expr ast.Expr) bool {
	text, ok := literalText(expr)
	return ok && strings.TrimSpace(text) != ""
}

// literalText is what stands between the quotes of a string literal, as it is
// written.
//
// It judges the literal as written rather than as it decodes. Decoding it would
// make an escape sequence disappear into the character it stands for, and its
// two callers ask only whether anything is there and whether a declared word is
// in it, so the two answers differ nowhere that matters and the second needs an
// error case for bytes the parser has already accepted.
func literalText(expr ast.Expr) (string, bool) {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	return strings.Trim(lit.Value, "`\""), true
}

// isCase reports whether a function is a case the test binary would run, which
// is the name the toolchain reads plus the one parameter it passes.
//
// The parameter is what the rune after the prefix cannot do on its own. TestMain
// carries the name and takes an *testing.M, and it is the file's entry point
// rather than a case: it is where the roster is printed, so requiring it to
// reach a suite gate would refuse the one function that reports the suites that
// never did.
func isCase(fn *ast.FuncDecl) bool {
	rest, ok := strings.CutPrefix(fn.Name.Name, "Test")
	if !ok || rest == "" {
		return false
	}
	if first := rest[0]; first >= 'a' && first <= 'z' {
		return false
	}
	if fn.Type.Params == nil || len(fn.Type.Params.List) != 1 {
		return false
	}
	star, ok := fn.Type.Params.List[0].Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	sel, ok := star.X.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == "T"
}

func contains(list []string, name string) bool {
	for _, item := range list {
		if item == name {
			return true
		}
	}
	return false
}

// carries matches a declared word against an identifier case-insensitively, so
// one word covers the several spellings the same setting is written in.
func carries(words []string, name string) bool {
	lower := strings.ToLower(name)
	for _, word := range words {
		if strings.Contains(lower, strings.ToLower(word)) {
			return true
		}
	}
	return false
}

func within(dir string, paths []string) bool {
	for _, p := range paths {
		if dir == p || strings.HasPrefix(dir, p+"/") {
			return true
		}
	}
	return false
}

// CheckTree reports every violation of inv in the .go files under root.
//
// Directories a Go build never looks at are skipped: anything starting with a
// dot or an underscore, and testdata. A violation inside a fixture is part of
// the fixture.
func CheckTree(inv []Invariant, root string) ([]Violation, int, error) {
	var (
		found []Violation
		read  int
	)
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() {
			if p != root && (strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") || name == "testdata") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(name, ".go") {
			return nil
		}
		src, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		read++
		violations, err := Check(inv, filepath.ToSlash(rel), src)
		if err != nil {
			return err
		}
		found = append(found, violations...)
		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	return found, read, nil
}
