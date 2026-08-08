package doclint

import (
	"strings"
	"testing"
)

// tree is the smallest tree that still exercises a file, a directory, a
// planned package and a root this repository does not have. It is written out
// here rather than read from the repository, because a case that judges
// against the real tree proves the state of the tree on the day it ran and not
// the rule.
func tree() *Tree {
	return NewTree(
		[]string{
			"README.md",
			"docs/layout.md",
			"docs/decisions/0003-permission-model.md",
			"internal/authz/authz.go",
		},
		[]string{"internal/store"},
	)
}

func check(t *testing.T, body string) []Finding {
	t.Helper()
	return Check([]File{{Name: "docs/layout.md", Bytes: []byte(body)}}, tree())
}

func TestAPathTheTreeHoldsResolves(t *testing.T) {
	for _, body := range []string{
		"The evaluator is `internal/authz/authz.go` and nothing else.\n",
		"Records live under `docs/decisions/`.\n",
		"Written as `./internal/authz` it is the same package.\n",
		"A trailing slash changes nothing: `internal/authz/`.\n",
	} {
		if findings := check(t, body); len(findings) != 0 {
			t.Fatalf("%q was refused: %v", body, findings)
		}
	}
}

// The bite, shown by running rather than asserted. One character is the
// mistake somebody actually makes: a record renamed in the tree and not in the
// sentence that points at it.
func TestAPathOneCharacterOffIsRefusedWithItsLine(t *testing.T) {
	body := "fine\nThe model is in `docs/decisions/0003-permission-models.md`.\nfine\n"

	findings := check(t, body)
	if len(findings) != 1 {
		t.Fatalf("want one finding, got %d: %v", len(findings), findings)
	}
	got := findings[0].String()
	for _, want := range []string{"docs/layout.md:2", "code span", "0003-permission-models.md"} {
		if !strings.Contains(got, want) {
			t.Fatalf("the finding does not carry %q: %s", want, got)
		}
	}

	// The same sentence with the character put back is green, which is what
	// says the refusal was about the path and not about the sentence.
	fixed := strings.Replace(body, "permission-models", "permission-model", 1)
	if findings := check(t, fixed); len(findings) != 0 {
		t.Fatalf("the corrected sentence was refused: %v", findings)
	}
}

func TestAPlannedPackageIsAPathADocumentMayName(t *testing.T) {
	if findings := check(t, "The datastore is `internal/store/`.\n"); len(findings) != 0 {
		t.Fatalf("a planned package was refused: %v", findings)
	}
	// A neighbour of it is not planned and is not in the tree, so the register
	// admits the one line it holds rather than the directory around it.
	if findings := check(t, "The datastore is `internal/storage/`.\n"); len(findings) != 1 {
		t.Fatalf("want one finding for an unplanned neighbour, got %d", len(findings))
	}
}

// What the rule deliberately does not read. Each of these would be a finding
// if the guess were wider, and each is a sentence this repository's own
// documents contain.
func TestWhatIsNotReadAsAPath(t *testing.T) {
	cases := map[string]string{
		"a standard library package":  "A `.go` file is formatted by `go/format` and by nothing else.\n",
		"a media type":                "The route answers `application/json`.\n",
		"a command with an argument":  "Run `git ls-files -z` first.\n",
		"an address":                  "See `https://example.invalid/docs/missing.md`.\n",
		"a word with no separator":    "The package is `authz`.\n",
		"a heading link":              "See [the decision](#the-decision).\n",
		"an address in a link":        "See [upstream](https://example.invalid/missing.md).\n",
		"a fenced block":              "```\nsee `docs/decisions/0099-nothing.md`\n```\n",
		"an indented block":           "text\n\n    see `docs/decisions/0099-nothing.md`\n\ntext\n",
		"a fenced block with a label": "```sh\nsee `internal/nothing/at/all.go`\n```\n",
	}
	for name, body := range cases {
		if findings := check(t, body); len(findings) != 0 {
			t.Fatalf("%s was read as a path: %v", name, findings)
		}
	}
}

func TestALinkResolvesAgainstTheDocumentItSitsIn(t *testing.T) {
	sibling := File{
		Name:  "docs/decisions/0006-connectors.md",
		Bytes: []byte("The model is [0003](0003-permission-model.md).\n"),
	}
	if findings := Check([]File{sibling}, tree()); len(findings) != 0 {
		t.Fatalf("a sibling link was refused: %v", findings)
	}

	// The same target written in a document one directory up points at nothing,
	// which is the whole reason a link is resolved against its own directory
	// rather than against the root.
	elsewhere := File{
		Name:  "docs/layout.md",
		Bytes: []byte("The model is [0003](0003-permission-model.md).\n"),
	}
	findings := Check([]File{elsewhere}, tree())
	if len(findings) != 1 {
		t.Fatalf("want one finding, got %d: %v", len(findings), findings)
	}
	if findings[0].Kind != "link" {
		t.Fatalf("want a link finding, got %q", findings[0].Kind)
	}
}

func TestALinkCarryingAFragmentOrATitleIsStillAPath(t *testing.T) {
	for _, body := range []string{
		"See [the record](decisions/0003-permission-model.md#the-decision).\n",
		"See [the record](decisions/0003-permission-model.md \"the record\").\n",
	} {
		if findings := check(t, body); len(findings) != 0 {
			t.Fatalf("%q was refused: %v", body, findings)
		}
	}
	// And the same two forms against a path that is not there are refused, so
	// the stripping is not a route past the rule.
	for _, body := range []string{
		"See [the record](decisions/0099-nothing.md#the-decision).\n",
		"See [the record](decisions/0099-nothing.md \"the record\").\n",
	} {
		if findings := check(t, body); len(findings) != 1 {
			t.Fatalf("%q was not refused: %v", body, findings)
		}
	}
}

// The near miss that was found by running this rule against the tree it
// governs: the section documenting the link syntax writes the syntax inside a
// code span, and reading the span as prose refused the document for being
// correct.
func TestALinkInsideACodeSpanIsShownRatherThanMade(t *testing.T) {
	body := "A link target, `[text](target)`, resolved against the directory.\n"
	if findings := check(t, body); len(findings) != 0 {
		t.Fatalf("a link written inside a span was read as a link: %v", findings)
	}

	// The same target outside a span is a link and is refused, so the split is
	// about where the syntax sits and not about the word in it.
	if findings := check(t, "A [text](docs/nothing.md) here.\n"); len(findings) != 1 {
		t.Fatalf("want one finding for a real link, got %d: %v", len(findings), findings)
	}
}

func TestASpanHoldingABacktickIsReadWhole(t *testing.T) {
	// A doubled fence is how a span carries a backtick. Reading it as two spans
	// would cut the path in half and refuse a document that is correct.
	body := "The file is ``docs/decisions/0003-permission-model.md`` today.\n"
	if findings := check(t, body); len(findings) != 0 {
		t.Fatalf("a doubled-fence span was refused: %v", findings)
	}
}

func TestFindingsAreOrderedByFileThenLine(t *testing.T) {
	files := []File{
		{Name: "docs/layout.md", Bytes: []byte("`docs/b.md`\n`docs/a.md`\n")},
		{Name: "README.md", Bytes: []byte("`docs/c.md`\n")},
	}
	findings := Check(files, tree())
	if len(findings) != 3 {
		t.Fatalf("want three findings, got %d: %v", len(findings), findings)
	}
	want := []string{"README.md", "docs/layout.md", "docs/layout.md"}
	for i, f := range findings {
		if f.File != want[i] {
			t.Fatalf("finding %d is in %s, want %s", i, f.File, want[i])
		}
	}
	if findings[1].Line != 1 || findings[2].Line != 2 {
		t.Fatalf("lines out of order: %d then %d", findings[1].Line, findings[2].Line)
	}
}

func TestAnUnclosedSpanOrLinkEndsTheLineRatherThanTheRead(t *testing.T) {
	// A stray backtick and a half-written link are typing rather than defects,
	// and neither may take the rest of the document out of the check.
	body := "a stray ` backtick and a half [link](\nthen `docs/decisions/0099-nothing.md`\n"
	findings := check(t, body)
	if len(findings) != 1 {
		t.Fatalf("want one finding on the second line, got %d: %v", len(findings), findings)
	}
	if findings[0].Line != 2 {
		t.Fatalf("want the finding on line 2, got %d", findings[0].Line)
	}
}

func TestReferencesReportsWhatWasRead(t *testing.T) {
	// The count printed by the command comes from here, so a change that
	// quietly stopped reading one of the two positions would be invisible
	// without this case.
	f := File{
		Name:  "docs/layout.md",
		Bytes: []byte("`internal/authz/authz.go` and [the model](decisions/0003-permission-model.md).\n"),
	}
	refs := References(f, tree())
	if len(refs) != 2 {
		t.Fatalf("want two references, got %d: %v", len(refs), refs)
	}
	kinds := map[string]bool{refs[0].Kind: true, refs[1].Kind: true}
	if !kinds["code span"] || !kinds["link"] {
		t.Fatalf("both positions were not read: %v", refs)
	}
}
