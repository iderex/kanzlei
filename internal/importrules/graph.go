package importrules

import (
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// A Graph is which package in this module imports which, read from source.
//
// Shipped and Test are separate because the rule about fakes needs them to be:
// a package that may only be reached from a test file is a package whose
// presence in a shipped file is the defect, and a graph that merged the two
// could not tell the two cases apart.
//
// Every package found has an entry in Shipped, empty where it imports nothing
// from this module, so a caller can tell a package with no imports from a
// package that is not there.
type Graph struct {
	Packages []string // every package found, sorted, as a path relative to the module root
	Shipped  map[string][]string
	Test     map[string][]string
}

// Load reads the graph out of the tree at root.
//
// It reads imports out of source rather than asking the build tool for them,
// for two reasons. A build constraint hides a file from the toolchain as
// effectively as deleting it, and the file under a constraint is exactly where
// a forbidden import would survive; parsing every file regardless of its tags
// is what closes that. And a function over bytes is a function fixtures can be
// put in front of, which the toolchain's own answer about this module is not.
//
// What it costs is that an import list is a syntactic fact. A package reached
// through a blank import, a linker flag or reflection is invisible here, and so
// is a file this walk skips.
//
// Directories a Go build never looks at are skipped: anything starting with a
// dot or an underscore, and testdata. A package named in a fixture is part of
// the fixture.
func Load(root, modulePath string) (*Graph, error) {
	if modulePath == "" {
		return nil, fmt.Errorf("the module path is empty; without it no import can be told from a dependency")
	}
	g := &Graph{Shipped: map[string][]string{}, Test: map[string][]string{}}
	prefix := modulePath + "/"

	shipped := map[string]map[string]bool{}
	test := map[string]map[string]bool{}

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
		dir, err := filepath.Rel(root, filepath.Dir(path))
		if err != nil {
			return err
		}
		pkg := filepath.ToSlash(dir)
		if pkg == "." {
			pkg = ""
		}
		if _, seen := shipped[pkg]; !seen {
			shipped[pkg] = map[string]bool{}
			test[pkg] = map[string]bool{}
		}

		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, src, parser.ImportsOnly|parser.SkipObjectResolution)
		if err != nil {
			return fmt.Errorf("parse %s: %w", filepath.ToSlash(path), err)
		}
		into := shipped[pkg]
		if strings.HasSuffix(name, "_test.go") {
			into = test[pkg]
		}
		for _, spec := range file.Imports {
			imported, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				return fmt.Errorf("%s: import path %s is not a string", filepath.ToSlash(path), spec.Path.Value)
			}
			if !strings.HasPrefix(imported, prefix) {
				continue
			}
			into[strings.TrimPrefix(imported, prefix)] = true
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	for pkg := range shipped {
		g.Packages = append(g.Packages, pkg)
		g.Shipped[pkg] = sorted(shipped[pkg])
		g.Test[pkg] = sorted(test[pkg])
	}
	sort.Strings(g.Packages)
	return g, nil
}

func sorted(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// chainTo reports how a binary reaches pkg, as the shortest chain of shipped
// imports from a command, or nil where no command reaches it.
//
// The chain is what turns a refusal into something a reader can act on. A
// handler that imports the datastore is not usually a line somebody typed on
// purpose; it arrives through a package that looked like a helper, and the
// question the reader has is which one.
func (g *Graph) chainTo(pkg string) []string {
	var roots []string
	for _, p := range g.Packages {
		if strings.HasPrefix(p, "cmd/") {
			roots = append(roots, p)
		}
	}
	sort.Strings(roots)

	from := map[string]string{}
	seen := map[string]bool{}
	var queue []string
	for _, root := range roots {
		if !seen[root] {
			seen[root] = true
			queue = append(queue, root)
		}
	}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if current == pkg {
			var chain []string
			for at := pkg; ; {
				chain = append([]string{at}, chain...)
				previous, more := from[at]
				if !more {
					break
				}
				at = previous
			}
			return chain
		}
		for _, next := range g.Shipped[current] {
			if seen[next] {
				continue
			}
			seen[next] = true
			from[next] = current
			queue = append(queue, next)
		}
	}
	return nil
}

// ModulePath reads the module path out of a go.mod file.
//
// The module declares its own path, so nothing here has to be told what this
// module is called, and a rename cannot leave a hardcoded prefix behind that
// silently stops matching every import in the tree.
func ModulePath(src []byte) (string, error) {
	for _, line := range strings.Split(string(src), "\n") {
		trimmed := strings.TrimSpace(line)
		rest, ok := strings.CutPrefix(trimmed, "module ")
		if !ok {
			continue
		}
		path := strings.TrimSpace(rest)
		if path == "" {
			return "", fmt.Errorf("the module line names no path")
		}
		return path, nil
	}
	return "", fmt.Errorf("no module line found")
}
