package contract

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/iderex/kanzlei/internal/runtime"
)

// TestTheOperationsAreTheOnesTheContractDeclares keeps the list this file
// matches a type against derived from the interface rather than typed beside
// it. Without it a fourth operation added to internal/runtime would leave the
// register recognising adapters by three of four, and an adapter missing the
// new one would still be counted as one.
func TestTheOperationsAreTheOnesTheContractDeclares(t *testing.T) {
	contract := reflect.TypeOf((*runtime.Runtime)(nil)).Elem()

	declared := make([]string, 0, contract.NumMethod())
	for i := range contract.NumMethod() {
		declared = append(declared, contract.Method(i).Name)
	}
	slices.Sort(declared)

	got := slices.Clone(operations)
	slices.Sort(got)
	if !slices.Equal(got, declared) {
		t.Fatalf("this file recognises an adapter by %v and the contract declares %v", got, declared)
	}
}

// TestThisTreeMatchesTheRegister is the case that judges this repository
// rather than a fixture. It proves the state of the tree on the day it runs
// and it does not prove the guard; what proves the guard is the fixtures
// below, each of which puts one disagreement in front of the same function.
func TestThisTreeMatchesTheRegister(t *testing.T) {
	root := filepath.Join("..", "..", "..")

	findings, err := Check(root, Adapters)
	if err != nil {
		t.Fatalf("the tree could not be read: %v", err)
	}
	for _, finding := range findings {
		t.Errorf("the adapter register and the tree disagree: %s", finding)
	}
	t.Logf("%d adapter(s) registered, every one of them handed to this suite by a test in its own package", len(Adapters))
}

// tree writes a fixture repository and reports its root. Each file is a
// package directory against a file, so a case reads as the tree it is about.
func tree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, source := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("the fixture directory could not be made: %v", err)
		}
		if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
			t.Fatalf("the fixture file could not be written: %v", err)
		}
	}
	return root
}

// adapterSource is a package that implements the contract, written as an
// adapter is written: the three operations on one type.
const adapterSource = `package engine

type Adapter struct{}

func (Adapter) Capabilities() {}
func (Adapter) Generate()     {}
func (Adapter) Embed()        {}
`

// coverSource is the test file that hands an adapter to the suite.
const coverSource = `package engine_test

func Test(t *testing.T) { contract.Run(t, contract.Subject{}) }
`

// TestAnAdapterInTheTreeWithNoRegistrationIsRefused is the failure this
// register exists for: an adapter lands, nobody adds the line, and the suite
// that was meant to cover it never sees it.
func TestAnAdapterInTheTreeWithNoRegistrationIsRefused(t *testing.T) {
	root := tree(t, map[string]string{
		"internal/runtime/engine/engine.go":      adapterSource,
		"internal/runtime/engine/engine_test.go": coverSource,
	})

	findings, err := Check(root, nil)
	if err != nil {
		t.Fatalf("the fixture tree could not be read: %v", err)
	}
	if !says(findings, "is not registered") {
		t.Fatalf("an unregistered adapter was accepted: %v", findings)
	}

	findings, err = Check(root, []string{"internal/runtime/engine"})
	if err != nil {
		t.Fatalf("the fixture tree could not be read: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("the same tree with the registration added was refused: %v", findings)
	}
}

// TestARegistrationNamingNoAdapterIsRefused is the other direction, and it is
// the one that matters after a package is renamed or removed: a register that
// still names it reads as coverage and there is nothing behind it.
func TestARegistrationNamingNoAdapterIsRefused(t *testing.T) {
	root := tree(t, map[string]string{"internal/runtime/runtime.go": "package runtime\n"})

	findings, err := Check(root, []string{"internal/runtime/engine"})
	if err != nil {
		t.Fatalf("the fixture tree could not be read: %v", err)
	}
	if !says(findings, "implements nothing in the tree") {
		t.Fatalf("a registration naming no adapter was accepted: %v", findings)
	}
}

// TestARegisteredAdapterWithNoRunIsRefused is the registration that looks
// complete. The package is there, the line is there, and no test in it ever
// hands a subject to the suite.
func TestARegisteredAdapterWithNoRunIsRefused(t *testing.T) {
	root := tree(t, map[string]string{
		"internal/runtime/engine/engine.go":      adapterSource,
		"internal/runtime/engine/engine_test.go": "package engine_test\n\nfunc Test(t *testing.T) {}\n",
	})

	findings, err := Check(root, []string{"internal/runtime/engine"})
	if err != nil {
		t.Fatalf("the fixture tree could not be read: %v", err)
	}
	if !says(findings, "hands a subject to the contract suite") {
		t.Fatalf("an adapter no test hands to the suite was accepted: %v", findings)
	}
}

// TestATypeWithTwoOfTheThreeOperationsIsNotAnAdapter is the near miss on the
// other side. A register that counted a partial type would refuse a helper
// somebody wrote beside an adapter, and a check that refuses honest work is
// one people switch off.
func TestATypeWithTwoOfTheThreeOperationsIsNotAnAdapter(t *testing.T) {
	root := tree(t, map[string]string{"internal/runtime/engine/engine.go": `package engine

type Half struct{}

func (Half) Capabilities() {}
func (Half) Generate()     {}
`})

	implementing, err := Implementations(root)
	if err != nil {
		t.Fatalf("the fixture tree could not be read: %v", err)
	}
	if len(implementing) != 0 {
		t.Fatalf("a type with two of the three operations was read as an adapter: %v", implementing)
	}
}

// TestAnAdapterDeclaredInATestFileIsNotOne reads the boundary the register
// draws. A stub built by a case for itself is not a thing that ships, and
// counting one would put every proof in this package into the register.
func TestAnAdapterDeclaredInATestFileIsNotOne(t *testing.T) {
	root := tree(t, map[string]string{"internal/runtime/engine/engine_test.go": strings.Replace(adapterSource, "package engine", "package engine_test", 1)})

	implementing, err := Implementations(root)
	if err != nil {
		t.Fatalf("the fixture tree could not be read: %v", err)
	}
	if len(implementing) != 0 {
		t.Fatalf("a stub declared in a test file was read as an adapter: %v", implementing)
	}
}

// TestASourceFileThatDoesNotParseIsNotThisChecksFinding holds the boundary
// between this reading and the toolchain's. A file the compiler will refuse is
// passed over here, because a syntax error reported by a register of adapters
// is a worse message about the same defect and arrives beside the real one.
func TestASourceFileThatDoesNotParseIsNotThisChecksFinding(t *testing.T) {
	root := tree(t, map[string]string{"internal/runtime/engine/engine.go": "package engine\n\nfunc (Adapter) Capabilities( {}\n"})

	implementing, err := Implementations(root)
	if err != nil {
		t.Fatalf("a file that does not parse was reported as a failure to read the tree: %v", err)
	}
	if len(implementing) != 0 {
		t.Fatalf("a file that does not parse produced %v", implementing)
	}
}

// TestAMethodOnAReceiverThisReadingCannotNameIsPassedOver is the bound stated
// in Implementations, put in front of the reading rather than left in a
// sentence. A generic receiver is the shape that reaches it, and passing it
// over is the direction to be wrong in: it under-counts adapters, and an
// adapter that under-counts is caught by the register's other direction.
func TestAMethodOnAReceiverThisReadingCannotNameIsPassedOver(t *testing.T) {
	root := tree(t, map[string]string{"internal/runtime/engine/engine.go": `package engine

type Adapter[T any] struct{}

func (Adapter[T]) Capabilities() {}
func (Adapter[T]) Generate()     {}
func (Adapter[T]) Embed()        {}
`})

	implementing, err := Implementations(root)
	if err != nil {
		t.Fatalf("the fixture tree could not be read: %v", err)
	}
	if len(implementing) != 0 {
		t.Fatalf("a generic receiver was read as a name, and this reading takes the pointer off and expects an identifier: %v", implementing)
	}
}

// says reports whether any finding carries the phrase.
func says(findings []string, phrase string) bool {
	for _, finding := range findings {
		if strings.Contains(finding, phrase) {
			return true
		}
	}
	return false
}
