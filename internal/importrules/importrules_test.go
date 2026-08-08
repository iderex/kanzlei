package importrules_test

import (
	"strings"
	"testing"

	"github.com/iderex/kanzlei/internal/importrules"
)

// parse is the fixture form: a declaration written in source, read with the
// name a failure will quote back.
func parse(t *testing.T, text string) *importrules.Rules {
	t.Helper()
	rules, err := importrules.Parse("fixture-rules.txt", []byte(text))
	if err != nil {
		t.Fatalf("the fixture declaration was refused: %v", err)
	}
	return rules
}

func refused(t *testing.T, text, want string) {
	t.Helper()
	_, err := importrules.Parse("fixture-rules.txt", []byte(text))
	if err == nil {
		t.Fatalf("the declaration was read without complaint and should not have been")
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("the refusal says %q, which does not mention %q", err, want)
	}
}

// graph builds the second half of a fixture without touching a disk.
func graph(shipped, test map[string][]string) *importrules.Graph {
	g := &importrules.Graph{Shipped: map[string][]string{}, Test: map[string][]string{}}
	for pkg, imports := range shipped {
		g.Packages = append(g.Packages, pkg)
		g.Shipped[pkg] = imports
		g.Test[pkg] = test[pkg]
	}
	return g
}

func TestAPackageMayImportWhatItsLineNames(t *testing.T) {
	rules := parse(t, `
cmd/kanzlei: internal/server
internal/server:
`)
	findings := importrules.Check(rules, graph(map[string][]string{
		"cmd/kanzlei":     {"internal/server"},
		"internal/server": nil,
	}, nil))
	if len(findings) != 0 {
		t.Fatalf("a permitted import was refused: %v", findings)
	}
}

// The first of the four rules the layout note carries: the index is reached
// through the retrieval package and through nothing else.
func TestAHandlerThatImportsTheStoreIsRefusedWithTheChain(t *testing.T) {
	rules := parse(t, `
cmd/kanzlei: internal/server
internal/server: internal/retrieval
internal/retrieval: internal/store
internal/store:
`)
	findings := importrules.Check(rules, graph(map[string][]string{
		"cmd/kanzlei":        {"internal/server"},
		"internal/server":    {"internal/retrieval", "internal/store"},
		"internal/retrieval": {"internal/store"},
		"internal/store":     nil,
	}, nil))
	if len(findings) != 1 {
		t.Fatalf("want exactly the one forbidden edge, got %v", findings)
	}
	got := findings[0].String()
	for _, want := range []string{
		"internal/server -> internal/store",
		"is not permitted to import it",
		"reached by cmd/kanzlei -> internal/server",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the finding reads %q, which does not mention %q", got, want)
		}
	}
}

// The near miss for that rule, and it is the reason the permission list is a
// list of what is allowed. A handler reaching the store through a package that
// looked like a helper is the shape nobody types on purpose, and the chain is
// what tells the reader which package let it in.
func TestTheStoreReachedThroughAHelperIsStillRefused(t *testing.T) {
	rules := parse(t, `
cmd/kanzlei: internal/server
internal/server: internal/helper
internal/helper:
internal/store:
`)
	findings := importrules.Check(rules, graph(map[string][]string{
		"cmd/kanzlei":     {"internal/server"},
		"internal/server": {"internal/helper"},
		"internal/helper": {"internal/store"},
		"internal/store":  nil,
	}, nil))
	if len(findings) != 1 {
		t.Fatalf("want exactly the one forbidden edge, got %v", findings)
	}
	if want := "reached by cmd/kanzlei -> internal/server -> internal/helper"; !strings.Contains(findings[0].String(), want) {
		t.Fatalf("the finding reads %q, which does not name the chain %q", findings[0], want)
	}
}

// The third rule: the model runtime is not reachable from the store package.
func TestTheStoreMayNotImportTheRuntime(t *testing.T) {
	rules := parse(t, `
internal/store:
internal/runtime:
`)
	findings := importrules.Check(rules, graph(map[string][]string{
		"internal/store":   {"internal/runtime"},
		"internal/runtime": nil,
	}, nil))
	if len(findings) != 1 || findings[0].Import != "internal/runtime" {
		t.Fatalf("want the store's import of the runtime refused, got %v", findings)
	}
}

// The fourth rule, in both directions: a fake is reachable from a test file and
// from nothing that ships.
func TestAFakeIsRefusedInAShippedFileAndAllowedInATestFile(t *testing.T) {
	declaration := `
test-only internal/sources/fake #48
internal/sources/fake:
internal/retrieval:
internal/retrieval +test: internal/sources/fake
`
	rules := parse(t, declaration)

	fromTest := importrules.Check(rules, graph(
		map[string][]string{"internal/retrieval": nil, "internal/sources/fake": nil},
		map[string][]string{"internal/retrieval": {"internal/sources/fake"}},
	))
	if len(fromTest) != 0 {
		t.Fatalf("a fake imported from a test file was refused: %v", fromTest)
	}

	fromShipped := importrules.Check(rules, graph(
		map[string][]string{"internal/retrieval": {"internal/sources/fake"}, "internal/sources/fake": nil},
		nil,
	))
	if len(fromShipped) != 1 {
		t.Fatalf("want the shipped import of the fake refused, got %v", fromShipped)
	}
	if want := "is test-only (#48) and this is a file that ships"; !strings.Contains(fromShipped[0].String(), want) {
		t.Fatalf("the finding reads %q, which does not say why the import is refused", fromShipped[0])
	}
}

// The declaration cannot permit what it declares test-only, so the repair for
// the case above is not one line in this file.
func TestAShippedLineMayNotNameATestOnlyPackage(t *testing.T) {
	refused(t, `
test-only internal/sources/fake #48
internal/sources/fake:
internal/retrieval: internal/sources/fake
`, "which is test-only")
}

func TestAPackageInTheTreeWithNoLineIsRefused(t *testing.T) {
	rules := parse(t, "internal/server:\n")
	findings := importrules.Check(rules, graph(map[string][]string{
		"internal/server": nil,
		"internal/audit":  nil,
	}, nil))
	if len(findings) != 1 || findings[0].Kind != "undeclared package" {
		t.Fatalf("want the undeclared package reported, got %v", findings)
	}
	if want := "internal/audit"; !strings.Contains(findings[0].String(), want) {
		t.Fatalf("the finding reads %q and does not name %q", findings[0], want)
	}
}

func TestALineForAPackageThatIsNotThereIsRefused(t *testing.T) {
	rules := parse(t, "internal/server:\ninternal/audit:\n")
	findings := importrules.Check(rules, graph(map[string][]string{"internal/server": nil}, nil))
	if len(findings) != 1 || findings[0].Package != "internal/audit" {
		t.Fatalf("want the line for the absent package reported, got %v", findings)
	}
	if want := "mark it planned or remove the line"; !strings.Contains(findings[0].String(), want) {
		t.Fatalf("the finding reads %q and does not say what to do about it", findings[0])
	}
}

func TestAPlannedPackageIsNotReportedAsAbsentAndIsReportedOnceItArrives(t *testing.T) {
	rules := parse(t, "planned internal/store #9\ninternal/store:\n")

	absent := importrules.Check(rules, graph(map[string][]string{}, nil))
	if len(absent) != 0 {
		t.Fatalf("a planned package was reported as missing: %v", absent)
	}

	arrived := importrules.Check(rules, graph(map[string][]string{"internal/store": nil}, nil))
	if len(arrived) != 1 || arrived[0].Kind != "stale planned marker" {
		t.Fatalf("want the stale marker reported, got %v", arrived)
	}
	if want := "still marked planned (#9)"; !strings.Contains(arrived[0].String(), want) {
		t.Fatalf("the finding reads %q and does not carry the reference", arrived[0])
	}
}

// The external test package: a test file importing the package it tests. It
// needs no line, and every package in this tree with a test does it.
func TestATestFileMayImportThePackageItTests(t *testing.T) {
	rules := parse(t, "internal/server:\n")
	findings := importrules.Check(rules, graph(
		map[string][]string{"internal/server": nil},
		map[string][]string{"internal/server": {"internal/server"}},
	))
	if len(findings) != 0 {
		t.Fatalf("an external test package was refused: %v", findings)
	}
}

func TestATestFileImportIsRefusedSeparatelyFromAShippedOne(t *testing.T) {
	rules := parse(t, "internal/server:\ninternal/store:\n")
	findings := importrules.Check(rules, graph(
		map[string][]string{"internal/server": nil, "internal/store": nil},
		map[string][]string{"internal/server": {"internal/store"}},
	))
	if len(findings) != 1 || findings[0].Kind != "forbidden import in a test file" {
		t.Fatalf("want the test import refused as its own kind, got %v", findings)
	}
}

func TestATestLineAddsToTheShippedLineRatherThanReplacingIt(t *testing.T) {
	rules := parse(t, "internal/server: internal/build\ninternal/server +test: internal/store\ninternal/build:\ninternal/store:\n")
	findings := importrules.Check(rules, graph(
		map[string][]string{"internal/server": {"internal/build"}, "internal/build": nil, "internal/store": nil},
		map[string][]string{"internal/server": {"internal/build", "internal/store"}},
	))
	if len(findings) != 0 {
		t.Fatalf("a test file importing what the shipped line permits was refused: %v", findings)
	}
}

func TestEveryDisagreementIsReportedRatherThanTheFirst(t *testing.T) {
	rules := parse(t, "internal/server:\ninternal/store:\n")
	findings := importrules.Check(rules, graph(map[string][]string{
		"internal/server": {"internal/store"},
		"internal/store":  {"internal/server"},
		"internal/audit":  nil,
	}, nil))
	if len(findings) != 3 {
		t.Fatalf("want all three disagreements, got %v", findings)
	}
	if findings[0].Package != "internal/audit" || findings[1].Package != "internal/server" || findings[2].Package != "internal/store" {
		t.Fatalf("the findings are not ordered by package: %v", findings)
	}
}

func TestAnImportOfAPackageNobodyDeclaredSaysThatRatherThanNotPermitted(t *testing.T) {
	rules := parse(t, "internal/server:\n")
	findings := importrules.Check(rules, graph(map[string][]string{"internal/server": {"internal/vendorish"}}, nil))
	if len(findings) != 1 {
		t.Fatalf("want the one finding, got %v", findings)
	}
	if want := "has no line in the rule file at all"; !strings.Contains(findings[0].String(), want) {
		t.Fatalf("the finding reads %q, which sends the reader at the wrong repair", findings[0])
	}
}

func TestAFindingWithNoChainDoesNotInventOne(t *testing.T) {
	rules := parse(t, "internal/server:\ninternal/store:\n")
	findings := importrules.Check(rules, graph(map[string][]string{
		"internal/server": {"internal/store"},
		"internal/store":  nil,
	}, nil))
	if len(findings) != 1 {
		t.Fatalf("want the one finding, got %v", findings)
	}
	if strings.Contains(findings[0].String(), "reached by") {
		t.Fatalf("no command reaches this package and the finding claims a chain: %q", findings[0])
	}
}

// The near misses in the declaration itself. Each is a mistake somebody makes
// while editing the file rather than a violation somebody could not have
// written.
func TestTheDeclarationRefusesTheMistakesSomebodyActuallyMakes(t *testing.T) {
	t.Run("a target with no declaration of its own", func(t *testing.T) {
		// internal/serve rather than internal/server: one character, and
		// without this refusal it would permit an import of a package that
		// does not exist while forbidding the one that does.
		refused(t, "cmd/kanzlei: internal/serve\ninternal/server:\n", "which has no declaration of its own")
	})
	t.Run("a package declared twice", func(t *testing.T) {
		refused(t, "internal/server:\ninternal/server: internal/build\ninternal/build:\n", "is declared twice")
	})
	t.Run("two test lines", func(t *testing.T) {
		refused(t, "internal/server:\ninternal/server +test:\ninternal/server +test:\n", "has two test lines")
	})
	t.Run("a package permitted to import itself", func(t *testing.T) {
		refused(t, "internal/server: internal/server\n", "may not import itself")
	})
	t.Run("a test line naming its own package", func(t *testing.T) {
		refused(t, "internal/server:\ninternal/server +test: internal/server\n", "without saying so")
	})
	t.Run("a duplicated target", func(t *testing.T) {
		refused(t, "internal/build:\ncmd/kanzlei: internal/build internal/build\n", "is listed twice")
	})
	t.Run("the plural of +test", func(t *testing.T) {
		refused(t, "internal/server:\ninternal/server +tests:\n", "the only thing that may follow a package is +test")
	})
	t.Run("a line with no colon", func(t *testing.T) {
		refused(t, "internal/server\n", "a package line carries a colon")
	})
	t.Run("a marker with no reference", func(t *testing.T) {
		refused(t, "planned internal/store\ninternal/store:\n", "names no reference")
	})
	t.Run("a marker for a package with no line", func(t *testing.T) {
		refused(t, "planned internal/store #9\ninternal/server:\n", "has no package line")
	})
	t.Run("a test line for a package with no line", func(t *testing.T) {
		refused(t, "internal/server:\ninternal/store +test: internal/server\n", "has a test line and no package line")
	})
	t.Run("the same marker twice", func(t *testing.T) {
		refused(t, "planned internal/store #9\nplanned internal/store #17\ninternal/store:\n", "is declared twice")
	})
	t.Run("a file holding only its own explanation", func(t *testing.T) {
		refused(t, "# every line here is a comment\n\n", "an empty rule set permits everything")
	})
}

func TestACommentIsAWholeLineSoAReferenceSurvives(t *testing.T) {
	rules := parse(t, "# the store, planned\nplanned internal/store #9\ninternal/store:\n")
	if declared := rules.Declared(); len(declared) != 1 || declared[0] != "internal/store" {
		t.Fatalf("want the one declared package, got %v", declared)
	}
}

func TestDeclaredReportsTheFileOrderAndACopy(t *testing.T) {
	rules := parse(t, "internal/build:\ncmd/kanzlei: internal/build\n")
	declared := rules.Declared()
	if len(declared) != 2 || declared[0] != "internal/build" || declared[1] != "cmd/kanzlei" {
		t.Fatalf("want the file's own order, got %v", declared)
	}
	declared[0] = "edited"
	if again := rules.Declared(); again[0] != "internal/build" {
		t.Fatalf("the caller edited the rules through the list it was handed: %v", again)
	}
}

func TestPlannedReportsOnlyTheMarkedPackages(t *testing.T) {
	rules := parse(t, "internal/build:\nplanned internal/store #9\ninternal/store:\nplanned internal/runtime #71\ninternal/runtime:\n")

	planned := rules.Planned()
	if len(planned) != 2 || planned[0] != "internal/store" || planned[1] != "internal/runtime" {
		t.Fatalf("want the two marked packages in the file's own order, got %v", planned)
	}

	planned[0] = "edited"
	if again := rules.Planned(); again[0] != "internal/store" {
		t.Fatalf("the caller edited the rules through the list it was handed: %v", again)
	}
}

func TestPlannedIsEmptyWhereNothingIsMarked(t *testing.T) {
	// internal/doclint reads this to decide which paths a document may name
	// before they exist. A register with no marker has to answer none rather
	// than answer everything, because the second reading would let a document
	// name anything under a directory the tree already has.
	if planned := parse(t, "internal/build:\n").Planned(); len(planned) != 0 {
		t.Fatalf("want no planned package, got %v", planned)
	}
}
