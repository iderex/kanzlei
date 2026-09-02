// Package testreach reads this repository's own test files and refuses a test
// in the default run that reaches outside the process for something the default
// run is not allowed to have.
//
// The condition is that every test in the default run works with no display
// server, no administrative rights, no accelerator and no outbound network. An
// intention like that decays on the first test that quietly needs one of them,
// and the decay is invisible until somebody without that thing tries to
// contribute. So it is checked rather than intended.
//
// # Why this is a check over source and not a trap at runtime
//
// Go has no supported way for a test package to refuse an outbound connection
// made by code that constructs its own dialler. There is no hook to install.
// Refusing it from outside the process means a network namespace, a firewall
// rule or a runner with no route, and the first two need privileges that the
// same condition forbids the default run from having. So the isolation is the
// runner's job and this is the half that can be done in the tree: the
// constructs are read out of the source before anything runs, and a test that
// carries one is named with its file and its line rather than failing later on
// a connection error somebody will read as a defect in the code under test.
//
// # What it cannot see
//
// It reads direct calls through a package selector. A test that calls a helper
// which dials is invisible to it, and so is a dial through an interface value
// or a variable holding a function. The reason comment that excuses an address
// this check cannot read is a sentence a person writes and nothing verifies. It
// is a floor under the condition, not a proof of it. What re-proves it on
// evidence from a run rather than from source landed under #114 and is the step
// in .github/workflows/tests.yml that removes the route out and then proves it
// gone. #184 is what neither reaches: a test needing a display or elevated
// rights is refused by nothing here and by no fixture there.
package testreach

import (
	"fmt"
	"go/ast"
	"go/build/constraint"
	"go/parser"
	"go/token"
	"io/fs"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// MarkTag is the build constraint that marks a test as needing something the
// default run does not have. It is the mechanism from #8 rather than a second
// one invented here: a constraint is configuration the toolchain reads, so the
// marked set is excluded by construction and no contributor has to remember a
// flag.
const MarkTag = "needsreal"

// MarkedDir is where a marked file belongs. Holding the marked set in one place
// is what makes the listing command in the contributor guide complete: a marked
// file anywhere else would be excluded from the default run and absent from the
// list of what was excluded, which is the one state worse than either.
const MarkedDir = "test/"

// A Finding is one place a test reaches for something the default run may not
// have.
type Finding struct {
	File   string
	Line   int
	What   string // the construct as written, or the marking that is in the wrong place
	Detail string // what it reaches for, in the words the guide uses
}

func (f Finding) String() string {
	return fmt.Sprintf("%s:%d: %s: %s", f.File, f.Line, f.What, f.Detail)
}

// minimumReasonWords is what separates a reason from a gesture, and it is the
// same number internal/sourcecheck uses for the same judgement. One word is a
// label, and a label is what somebody writes when the reason is "to make it
// pass".
const minimumReasonWords = 3

// reachers are the calls that leave the process, by import path and function
// name, with the position of the argument naming where they go.
//
// The list is what has to be kept current and nothing derives it. A function
// added to the standard library, or a package this project starts using, is
// invisible here until it is added. That is the known limit of this check and
// it is in its test.
var reachers = map[string]map[string]reacher{
	"net": {
		"Dial":           {addr: 1, kind: addrHostPort},
		"DialTimeout":    {addr: 1, kind: addrHostPort},
		"Listen":         {addr: 1, kind: addrHostPort},
		"ListenPacket":   {addr: 1, kind: addrHostPort},
		"ResolveTCPAddr": {addr: 1, kind: addrHostPort},
		"ResolveUDPAddr": {addr: 1, kind: addrHostPort},
		// A name lookup is an outbound request whatever it resolves to, so
		// there is no address to read and no loopback form of it.
		"LookupAddr":  {kind: addrAlways},
		"LookupCNAME": {kind: addrAlways},
		"LookupHost":  {kind: addrAlways},
		"LookupIP":    {kind: addrAlways},
		"LookupMX":    {kind: addrAlways},
		"LookupNS":    {kind: addrAlways},
		"LookupPort":  {kind: addrAlways},
		"LookupSRV":   {kind: addrAlways},
		"LookupTXT":   {kind: addrAlways},
	},
	"net/http": {
		"Get":      {addr: 0, kind: addrURL},
		"Head":     {addr: 0, kind: addrURL},
		"Post":     {addr: 0, kind: addrURL},
		"PostForm": {addr: 0, kind: addrURL},
	},
}

type addrKind int

const (
	addrHostPort addrKind = iota // a "host:port" string
	addrURL                      // an absolute URL
	addrAlways                   // no address to read: the call is outbound whatever it is given
)

type reacher struct {
	addr int
	kind addrKind
}

// devicesThatAreNotDevices are the paths under /dev that carry no hardware
// behind them. Every one of them is present on any machine that can run the
// suite at all, so none of them is a thing the condition is about.
var devicesThatAreNotDevices = map[string]bool{
	"/dev/null":    true,
	"/dev/zero":    true,
	"/dev/full":    true,
	"/dev/random":  true,
	"/dev/urandom": true,
	"/dev/stdin":   true,
	"/dev/stdout":  true,
	"/dev/stderr":  true,
}

// Marked reports whether src is excluded from the default run.
//
// The question asked is the one that matters rather than whether the tag
// appears: with the mark off and every other build tag on, is this file built.
// A file that is not is the marked set, and one that is is the default run
// whatever its constraint says.
func Marked(src []byte) bool {
	for _, line := range strings.Split(string(src), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "package ") {
			return false
		}
		if !constraint.IsGoBuild(line) {
			continue
		}
		expr, err := constraint.Parse(line)
		if err != nil {
			continue
		}
		if !expr.Eval(func(tag string) bool { return tag != MarkTag }) {
			return true
		}
	}
	return false
}

// InFile reports every place the test file src reaches outside the process.
//
// filename is used for the reported position and for nothing else, so the
// fixtures in this package's test stay in source rather than being written to
// disk first.
func InFile(filename string, src []byte) ([]Finding, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, src, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", filename, err)
	}

	// The reason written after a call, by the line it is on. It excuses an
	// address this check cannot read and it never excuses one it can read as
	// outbound.
	reasons := map[int]string{}
	for _, group := range file.Comments {
		for _, comment := range group.List {
			text, ok := strings.CutPrefix(comment.Text, "//")
			if !ok {
				continue
			}
			line := fset.Position(comment.Slash).Line
			if _, seen := reasons[line]; !seen {
				reasons[line] = text
			}
		}
	}

	// Which local name stands for which import path in this file. Reading the
	// selector without this would judge a call on a local variable called
	// "net" as a call into the standard library.
	imported := map[string]string{}
	for _, spec := range file.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			continue
		}
		name := path[strings.LastIndex(path, "/")+1:]
		if spec.Name != nil {
			name = spec.Name.Name
		}
		imported[name] = path
	}

	var found []Finding
	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.CallExpr:
			if f, ok := reachCall(fset, filename, imported, reasons, node); ok {
				found = append(found, f)
			}
		case *ast.BinaryExpr:
			// A path built by joining literals is the same path. Folding it
			// here and not descending is what stops the check being walked
			// past by splitting the name in two.
			if s, ok := literalString(node); ok {
				if f, found2 := devicePath(fset, filename, node.Pos(), s); found2 {
					found = append(found, f)
				}
				return false
			}
		case *ast.BasicLit:
			if node.Kind != token.STRING {
				return true
			}
			s, err := strconv.Unquote(node.Value)
			if err != nil {
				return true
			}
			if f, ok := devicePath(fset, filename, node.Pos(), s); ok {
				found = append(found, f)
			}
		}
		return true
	})

	sort.Slice(found, func(i, j int) bool { return found[i].Line < found[j].Line })
	return found, nil
}

func reachCall(fset *token.FileSet, filename string, imported map[string]string, reasons map[int]string, call *ast.CallExpr) (Finding, bool) {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return Finding{}, false
	}
	pkg, ok := selector.X.(*ast.Ident)
	if !ok {
		return Finding{}, false
	}
	path, ok := imported[pkg.Name]
	if !ok {
		return Finding{}, false
	}
	r, ok := reachers[path][selector.Sel.Name]
	if !ok {
		return Finding{}, false
	}

	line := fset.Position(call.Pos()).Line
	what := pkg.Name + "." + selector.Sel.Name

	if r.kind == addrAlways {
		return Finding{
			File:   filename,
			Line:   line,
			What:   what,
			Detail: "a name lookup leaves this machine whatever it is asked to resolve; mark the test with the " + MarkTag + " constraint or do not resolve a name",
		}, true
	}

	if r.addr >= len(call.Args) {
		return Finding{File: filename, Line: line, What: what, Detail: "called with no address to read"}, true
	}
	address, readable := literalString(call.Args[r.addr])
	if !readable {
		if words(reasons[line]) >= minimumReasonWords {
			// The one way past this check, and it is only past the half that
			// could not read the address. A reason is a sentence somebody
			// wrote and nothing here verifies it.
			return Finding{}, false
		}
		return Finding{
			File:   filename,
			Line:   line,
			What:   what,
			Detail: "the address is not written here, so this check cannot say where it goes; write a loopback address, or say on this line why the address is a loopback one",
		}, true
	}

	host, err := hostOf(address, r.kind)
	if err != nil {
		return Finding{File: filename, Line: line, What: what, Detail: err.Error()}, true
	}
	if isLoopback(host) {
		return Finding{}, false
	}
	return Finding{
		File:   filename,
		Line:   line,
		What:   what,
		Detail: fmt.Sprintf("reaches %q, which is not a loopback address; the default run has no route out", host),
	}, true
}

// literalString reads a string that is written out in the source. A
// concatenation of literals is read as one; a concatenation holding anything
// else is not readable, because the part this check cannot see is exactly the
// part that would carry the address.
func literalString(expr ast.Expr) (string, bool) {
	switch e := expr.(type) {
	case *ast.BasicLit:
		if e.Kind != token.STRING {
			return "", false
		}
		s, err := strconv.Unquote(e.Value)
		if err != nil {
			return "", false
		}
		return s, true
	case *ast.BinaryExpr:
		if e.Op != token.ADD {
			return "", false
		}
		left, ok := literalString(e.X)
		if !ok {
			return "", false
		}
		right, ok := literalString(e.Y)
		if !ok {
			return "", false
		}
		return left + right, true
	}
	return "", false
}

func hostOf(address string, kind addrKind) (string, error) {
	if kind == addrURL {
		u, err := url.Parse(address)
		if err != nil {
			return "", fmt.Errorf("the address %q is not a URL this check can read: %v", address, err)
		}
		return u.Hostname(), nil
	}
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		// An address with no port is still an address, and a listener may be
		// given an empty one.
		return address, nil
	}
	return host, nil
}

// isLoopback reports whether a host names this machine and nothing else.
//
// An empty host is the shape a listener takes when it is given ":0", which
// binds every interface. That is not loopback and it is not treated as one:
// binding every interface on a machine with no route out works, and the same
// test on a machine with one is reachable from outside it.
func isLoopback(host string) bool {
	switch host {
	case "localhost":
		return true
	case "":
		return false
	}
	if ip, err := parseIP(host); err == nil {
		return ip.IsLoopback()
	}
	return false
}

func parseIP(host string) (net.IP, error) {
	ip := net.ParseIP(strings.Trim(host, "[]"))
	if ip == nil {
		return nil, fmt.Errorf("%q is not an IP address", host)
	}
	return ip, nil
}

// devicePath reports whether s names something under /dev that is not on every
// machine. The prefix on its own names nothing and is left alone; what is
// refused is a name under it that is not in the short list of paths with no
// hardware behind them.
func devicePath(fset *token.FileSet, filename string, pos token.Pos, s string) (Finding, bool) {
	const dev = "/dev/"
	if !strings.HasPrefix(s, dev) || len(s) == len(dev) {
		return Finding{}, false
	}
	if devicesThatAreNotDevices[s] {
		return Finding{}, false
	}
	return Finding{
		File:   filename,
		Line:   fset.Position(pos).Line,
		What:   s,
		Detail: "a device this machine may not have; mark the test with the " + MarkTag + " constraint",
	}, true
}

func words(s string) int { return len(strings.Fields(s)) }

// InTree reports every finding in the test files under root that are part of
// the default run, and every marked file that is in the wrong place.
//
// A marked file outside MarkedDir is refused rather than skipped. It would be
// excluded from the default run and absent from the command the contributor
// guide names for listing what was excluded, and a test nobody runs and nobody
// can list is worse than either on its own.
func InTree(root string) ([]Finding, error) {
	var found []Finding
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
		slash := filepath.ToSlash(path)
		if Marked(src) {
			if !strings.Contains(slash, "/"+MarkedDir) && !strings.HasPrefix(slash, MarkedDir) {
				found = append(found, Finding{
					File:   slash,
					Line:   1,
					What:   "the " + MarkTag + " constraint",
					Detail: "a marked file outside " + MarkedDir + " is excluded from the default run and absent from the list of what was excluded",
				})
			}
			return nil
		}
		// Only test files. What the service itself does with a socket is the
		// service's business, and internal/server binds one on purpose.
		if !strings.HasSuffix(name, "_test.go") {
			return nil
		}
		inFile, err := InFile(slash, src)
		if err != nil {
			return err
		}
		found = append(found, inFile...)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return found, nil
}
