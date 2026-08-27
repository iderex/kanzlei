// The register of adapters, and the reading of the tree that keeps it honest.
//
// The fifth done-condition of #73 asks that an adapter cannot be added without
// being covered. A list alone cannot carry that: a list somebody forgets to
// add to is exactly the adapter nobody proved. So the list is compared against
// the tree in both directions, which is the shape internal/importrules already
// uses for a package with no rule and a rule with no package.
//
// Three findings, and each is a different lie. An adapter in the tree that is
// not registered is one nothing proved. A registration naming a package that
// is not there is a register that reads as coverage and is not. And a
// registered adapter whose own tests never hand a subject to Run is the same
// absence written one step further in.

package contract

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

// Adapters is every package in this module that implements runtime.Runtime and
// is therefore owed a run of this suite, written as its path from the module
// root.
//
// It is a permission list rather than a list of exceptions, for the reason
// import-rules.txt gives about the edge nobody thought of: a package added to
// the tree and to nothing else is refused, and the refusal names it.
var Adapters = []string{
	"internal/runtime/fake",
}

// operations is what a type has to have before it is an implementation of the
// contract. It is derived from the interface at run time by the case beside
// this file, so a fourth operation added to internal/runtime cannot leave this
// list behind.
var operations = []string{"Capabilities", "Embed", "Generate"}

// Check reads a tree and reports every way the register and the tree disagree.
//
// An empty result is the two agreeing. The order is stable so a run that
// reports several is readable.
func Check(root string, registered []string) ([]string, error) {
	implementing, err := Implementations(root)
	if err != nil {
		return nil, err
	}
	covered, err := Covered(root)
	if err != nil {
		return nil, err
	}

	var findings []string
	for _, pkg := range implementing {
		if !slices.Contains(registered, pkg) {
			findings = append(findings, fmt.Sprintf("%s implements the runtime contract and is not registered, so nothing hands it to the suite", pkg))
		}
	}
	for _, pkg := range registered {
		if !slices.Contains(implementing, pkg) {
			findings = append(findings, fmt.Sprintf("%s is registered as an adapter and implements nothing in the tree, so the register reads as coverage that is not there", pkg))
			continue
		}
		if !slices.Contains(covered, pkg) {
			findings = append(findings, fmt.Sprintf("%s is registered as an adapter and no test in it hands a subject to the contract suite", pkg))
		}
	}
	return findings, nil
}

// Implementations reports every package under root holding a type with all
// three operations of the contract on it.
//
// It reads the syntax and never the types, which is the same bound
// internal/invariants states for itself: it says a method with that name is
// declared on that type, and a type that satisfies the interface by embedding
// another one is invisible to it. That direction is the safe one to be wrong
// in here, because an adapter written to be handed to this suite is written
// with the three methods on it.
//
// Test files are not read. An adapter declared inside a test file is a stub a
// case built for itself, and the register is about what ships.
func Implementations(root string) ([]string, error) {
	return scan(root, func(file *ast.File) bool {
		methods := map[string]map[string]bool{}
		for _, declaration := range file.Decls {
			function, isFunction := declaration.(*ast.FuncDecl)
			if !isFunction || function.Recv == nil || len(function.Recv.List) == 0 {
				continue
			}
			receiver := receiverName(function.Recv.List[0].Type)
			if receiver == "" {
				continue
			}
			if methods[receiver] == nil {
				methods[receiver] = map[string]bool{}
			}
			methods[receiver][function.Name.Name] = true
		}
		for _, declared := range methods {
			complete := true
			for _, operation := range operations {
				if !declared[operation] {
					complete = false
					break
				}
			}
			if complete {
				return true
			}
		}
		return false
	}, false)
}

// Covered reports every package under root whose test files hand a subject to
// Run.
//
// It reads the call rather than the import, because an import is satisfied by
// a package that merely mentions the suite.
func Covered(root string) ([]string, error) {
	return scan(root, func(file *ast.File) bool {
		found := false
		ast.Inspect(file, func(node ast.Node) bool {
			call, isCall := node.(*ast.CallExpr)
			if !isCall {
				return true
			}
			selector, isSelector := call.Fun.(*ast.SelectorExpr)
			if !isSelector || selector.Sel.Name != "Run" {
				return true
			}
			pkg, isIdentifier := selector.X.(*ast.Ident)
			if isIdentifier && pkg.Name == "contract" {
				found = true
			}
			return true
		})
		return found
	}, true)
}

// scan walks a tree and reports the packages holding a file the predicate
// accepts, as paths from root with forward slashes.
func scan(root string, accept func(*ast.File) bool, tests bool) ([]string, error) {
	var found []string
	walk := func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if entry.Name() == ".git" || entry.Name() == "testdata" {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(name, ".go") {
			return nil
		}
		if strings.HasSuffix(name, "_test.go") != tests {
			return nil
		}

		source, err := os.ReadFile(name)
		if err != nil {
			return err
		}
		// A file that does not parse is not this check's finding: the
		// toolchain refuses it first and with a better message.
		file, err := parser.ParseFile(token.NewFileSet(), name, source, parser.SkipObjectResolution)
		if err != nil {
			return nil
		}
		if !accept(file) {
			return nil
		}

		relative, err := filepath.Rel(root, filepath.Dir(name))
		if err != nil {
			return err
		}
		pkg := path.Clean(filepath.ToSlash(relative))
		if !slices.Contains(found, pkg) {
			found = append(found, pkg)
		}
		return nil
	}
	if err := filepath.WalkDir(root, walk); err != nil {
		return nil, err
	}
	slices.Sort(found)
	return found, nil
}

// receiverName is the type a method is declared on, with the pointer taken
// off.
func receiverName(expression ast.Expr) string {
	if star, isPointer := expression.(*ast.StarExpr); isPointer {
		expression = star.X
	}
	if identifier, isIdentifier := expression.(*ast.Ident); isIdentifier {
		return identifier.Name
	}
	return ""
}
