package testreach_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iderex/kanzlei/internal/testreach"
)

// The fixtures are Go source held in raw strings rather than files in
// testdata/. They are read by the parser and never by a build, so a file on
// disk would add a directory and buy nothing. They are also whole files rather
// than fragments, because a constraint line and an import block are part of
// what is being read.
//
// Each one is deliberately close to the mistake somebody will actually make: a
// test that dials the thing it is testing, with the address a line away from
// being one this check could read.

// source builds a fixture with the given body inside a test function.
func source(imports, body string) []byte {
	return []byte("package p\n\nimport (\n" + imports + ")\n\nfunc TestSomething(t *testing.T) {\n" + body + "\n}\n")
}

func findings(t *testing.T, src []byte) []testreach.Finding {
	t.Helper()
	found, err := testreach.InFile("p_test.go", src)
	if err != nil {
		t.Fatalf("read the fixture: %v", err)
	}
	return found
}

func TestALoopbackAddressIsNotAReach(t *testing.T) {
	for name, body := range map[string]string{
		"listen on a chosen port": `	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	_ = ln`,
		"dial the six form": `	c, _ := net.Dial("tcp", "[::1]:8080")
	_ = c`,
		"dial by name": `	c, _ := net.Dial("tcp", "localhost:8080")
	_ = c`,
		"a URL built out of literals": `	r, _ := http.Get("http://" + "127.0.0.1:8080" + "/livez")
	_ = r`,
	} {
		t.Run(name, func(t *testing.T) {
			if found := findings(t, source("\t\"net\"\n\t\"net/http\"\n\t\"testing\"\n", body)); len(found) != 0 {
				t.Fatalf("a loopback address was refused: %v", found)
			}
		})
	}
}

// The bite. Each of these is a test that leaves the machine, and each has to be
// named with what it reaches rather than left to fail on a connection error.
func TestAnAddressThatLeavesTheMachineIsRefused(t *testing.T) {
	for name, c := range map[string]struct{ body, names string }{
		"dial a host on the internet": {`	conn, _ := net.Dial("tcp", "example.com:443")
	_ = conn`, "example.com"},
		"fetch a URL": {`	r, _ := http.Get("https://example.com/x")
	_ = r`, "example.com"},
		"listen on every interface": {`	ln, _ := net.Listen("tcp", ":0")
	_ = ln`, "net.Listen"},
		"resolve a name": {`	addrs, _ := net.LookupHost("example.com")
	_ = addrs`, "net.LookupHost"},
	} {
		t.Run(name, func(t *testing.T) {
			found := findings(t, source("\t\"net\"\n\t\"net/http\"\n\t\"testing\"\n", c.body))
			if len(found) != 1 {
				t.Fatalf("got %d findings, want 1: %v", len(found), found)
			}
			if !strings.Contains(found[0].String(), c.names) {
				t.Fatalf("the refusal %q does not name %q", found[0], c.names)
			}
		})
	}
}

// A reason on the same line never excuses an address this check can read as
// outbound. If it did, the escape hatch for the unreadable case would be the
// escape hatch for everything.
func TestAReasonDoesNotExcuseAnAddressThatIsWrittenOut(t *testing.T) {
	body := `	r, _ := http.Get("https://example.com/x") // this is a loopback address and the reason is long enough
	_ = r`
	if found := findings(t, source("\t\"net/http\"\n\t\"testing\"\n", body)); len(found) != 1 {
		t.Fatalf("a reason excused an address the check could read: %v", found)
	}
}

// An address the check cannot read is refused, because the part it cannot see
// is the part that carries the address. Unknown means refused here for the same
// reason it means deny everywhere else in this project.
func TestAnAddressThisCheckCannotReadIsRefused(t *testing.T) {
	body := `	r, _ := http.Get(where)
	_ = r`
	found := findings(t, source("\t\"net/http\"\n\t\"testing\"\n", body))
	if len(found) != 1 {
		t.Fatalf("got %d findings, want 1: %v", len(found), found)
	}
	if !strings.Contains(found[0].Detail, "cannot say where it goes") {
		t.Fatalf("the refusal %q does not say the address could not be read", found[0])
	}
}

func TestAReasonExcusesAnAddressThisCheckCannotRead(t *testing.T) {
	body := `	r, _ := http.Get(where) // loopback: the address this test's own child process printed a moment ago
	_ = r`
	if found := findings(t, source("\t\"net/http\"\n\t\"testing\"\n", body)); len(found) != 0 {
		t.Fatalf("a reason did not excuse an unreadable address: %v", found)
	}
}

// One word is a label rather than a reason, and it is what somebody writes to
// make a check go green. The number is the one internal/sourcecheck uses.
func TestAOneWordReasonExcusesNothing(t *testing.T) {
	body := `	r, _ := http.Get(where) // loopback
	_ = r`
	if found := findings(t, source("\t\"net/http\"\n\t\"testing\"\n", body)); len(found) != 1 {
		t.Fatalf("a one word reason was accepted: %v", found)
	}
}

// A local variable named for a package is not that package. Reading the
// selector without the import block would refuse this.
func TestACallOnSomethingElseCalledNetIsNotARefusal(t *testing.T) {
	src := []byte(`package p

import "testing"

type fake struct{}

func (fake) Dial(network, address string) (any, error) { return nil, nil }

func TestSomething(t *testing.T) {
	net := fake{}
	c, _ := net.Dial("tcp", "example.com:443")
	_ = c
}
`)
	if found := findings(t, src); len(found) != 0 {
		t.Fatalf("a call on a local value was refused: %v", found)
	}
}

// An import under another name is the same call. The check reads the import
// block rather than assuming the package name.
func TestARenamedImportIsStillTheSamePackage(t *testing.T) {
	src := []byte(`package p

import (
	stdnet "net"
	"testing"
)

func TestSomething(t *testing.T) {
	c, _ := stdnet.Dial("tcp", "example.com:443")
	_ = c
}
`)
	if found := findings(t, src); len(found) != 1 {
		t.Fatalf("got %v, want one refusal", found)
	}
}

func TestADeviceIsRefusedAndTheOnesThatAreNotDevicesAreNot(t *testing.T) {
	// The one-character mistake worth spending the fixture on is not the plain
	// path. It is the path split in two, which is what somebody writes when a
	// device name is built from a prefix and a number, and it is the shape a
	// check reading one literal at a time walks straight past.
	split := []byte(`package p

import (
	"os"
	"testing"
)

func TestSomething(t *testing.T) {
	f, _ := os.Open("/dev/" + "nvidia0")
	_ = f
}
`)
	if found := findings(t, split); len(found) != 1 {
		t.Fatalf("a device path split across two literals was not refused: %v", found)
	}

	whole := []byte(`package p

import (
	"os"
	"testing"
)

func TestSomething(t *testing.T) {
	f, _ := os.Open("/dev/nvidia0")
	_ = f
}
`)
	found := findings(t, whole)
	if len(found) != 1 {
		t.Fatalf("got %d findings, want 1: %v", len(found), found)
	}
	if !strings.Contains(found[0].Detail, "device this machine may not have") {
		t.Fatalf("the refusal %q does not say what it is about", found[0])
	}

	fine := []byte(`package p

import (
	"os"
	"testing"
)

func TestSomething(t *testing.T) {
	f, _ := os.Open("/dev/null")
	_ = f
}
`)
	if found := findings(t, fine); len(found) != 0 {
		t.Fatalf("a path with no hardware behind it was refused: %v", found)
	}
}

func TestMarkedReadsWhetherTheFileIsInTheDefaultRun(t *testing.T) {
	for name, c := range map[string]struct {
		src    string
		marked bool
	}{
		"marked":            {"//go:build " + testreach.MarkTag + "\n\npackage p\n", true},
		"marked and joined": {"//go:build " + testreach.MarkTag + " && linux\n\npackage p\n", true},
		"unconstrained":     {"package p\n", false},
		"the other side":    {"//go:build !" + testreach.MarkTag + "\n\npackage p\n", false},
		"another tag":       {"//go:build linux\n\npackage p\n", false},
	} {
		t.Run(name, func(t *testing.T) {
			if got := testreach.Marked([]byte(c.src)); got != c.marked {
				t.Fatalf("Marked = %v, want %v", got, c.marked)
			}
		})
	}
}

// A marked file in the wrong place is excluded from the default run and absent
// from the command the guide names for listing what was excluded. That is the
// one state worse than either on its own, so it is refused where it is found
// rather than where somebody eventually looks.
func TestAMarkedFileOutsideTheMarkedDirectoryIsRefused(t *testing.T) {
	root := t.TempDir()
	marked := "//go:build " + testreach.MarkTag + "\n\npackage p\n\nimport \"testing\"\n\nfunc TestSomething(t *testing.T) {}\n"

	stray := filepath.Join(root, "internal", "thing")
	if err := os.MkdirAll(stray, 0o755); err != nil {
		t.Fatalf("make the fixture tree: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stray, "thing_test.go"), []byte(marked), 0o600); err != nil {
		t.Fatalf("write the stray file: %v", err)
	}

	found, err := testreach.InTree(root)
	if err != nil {
		t.Fatalf("read the tree: %v", err)
	}
	if len(found) != 1 {
		t.Fatalf("got %d findings, want 1: %v", len(found), found)
	}
	if !strings.Contains(found[0].Detail, "absent from the list of what was excluded") {
		t.Fatalf("the refusal %q does not say why the placement matters", found[0])
	}

	// The same file under the marked directory is the marked set doing its job.
	home := filepath.Join(root, strings.TrimSuffix(testreach.MarkedDir, "/"), "needs-something")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatalf("make the marked directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, "thing_test.go"), []byte(marked), 0o600); err != nil {
		t.Fatalf("write the marked file: %v", err)
	}
	if err := os.Remove(filepath.Join(stray, "thing_test.go")); err != nil {
		t.Fatalf("remove the stray file: %v", err)
	}
	found, err = testreach.InTree(root)
	if err != nil {
		t.Fatalf("read the tree: %v", err)
	}
	if len(found) != 0 {
		t.Fatalf("a marked file in the right place was refused: %v", found)
	}
}

// The check over this repository. It is the case that makes an unmarked test
// reaching outside the process turn the default run red, rather than a check
// somebody has to remember to run.
func TestNoTestInTheDefaultRunReachesOutsideThisProcess(t *testing.T) {
	root := filepath.Join("..", "..")
	found, err := testreach.InTree(root)
	if err != nil {
		t.Fatalf("read the tree: %v", err)
	}
	if len(found) == 0 {
		return
	}
	for _, f := range found {
		t.Errorf("%s", f)
	}
	t.Fatalf("%d test(s) in the default run reach outside this process; mark them with the %s constraint under %s, or reach a loopback address written out in the source", len(found), testreach.MarkTag, testreach.MarkedDir)
}
