package treefmt

import (
	"strings"
	"testing"
)

// The fixtures below are written with Go escape sequences rather than as raw
// bytes. A carriage return, a byte order mark or a trailing space written
// literally in this file would be a byte .gitattributes converts on the way
// into git, which deletes the thing the case exists to prove. An escape is
// ASCII on disk and the exact byte in memory, so every tool leaves the source
// alone and the case still tests what it says.

// everything is the rule set the paths in this repository are judged under,
// written out here so a case names the rules it depends on rather than
// depending on whatever the root file happens to say today.
var everything = Rules{
	Charset:            "utf-8",
	EndOfLine:          "lf",
	InsertFinalNewline: true,
	TrimTrailing:       true,
	IndentStyle:        "space",
	IndentSize:         4,
}

func TestABitOfWhitespaceAtTheEndOfALineIsRemovedAndNamed(t *testing.T) {
	src := []byte("one   \ntwo\nthree\t\t\n")
	out, found := Format("doc.md", src, everything)

	if string(out) != "one\ntwo\nthree\n" {
		t.Fatalf("formatted bytes are %q", out)
	}
	if len(found) != 2 {
		t.Fatalf("want 2 findings, got %d: %v", len(found), found)
	}
	if found[0].Line != 1 || found[0].Rule != "trim_trailing_whitespace" {
		t.Errorf("first finding is %v", found[0])
	}
	if !strings.Contains(found[0].Detail, "3 character(s)") {
		t.Errorf("the finding does not say how much was at the end: %v", found[0])
	}
	if found[1].Line != 3 {
		t.Errorf("second finding is on line %d, want 3", found[1].Line)
	}
}

func TestACarriageReturnIsRemovedAndTheLineIsNamed(t *testing.T) {
	src := []byte("one\r\ntwo\n")
	out, found := Format("doc.md", src, everything)

	if string(out) != "one\ntwo\n" {
		t.Fatalf("formatted bytes are %q", out)
	}
	if len(found) != 1 || found[0].Line != 1 || found[0].Rule != "end_of_line" {
		t.Fatalf("findings are %v", found)
	}
}

func TestALoneCarriageReturnBecomesALineRatherThanDisappearing(t *testing.T) {
	// The old Mac ending. Dropping it would join two lines into one and change
	// what the file says, which is a formatter editing meaning.
	out, found := Format("doc.md", []byte("one\rtwo\n"), everything)
	if string(out) != "one\ntwo\n" {
		t.Fatalf("formatted bytes are %q", out)
	}
	if len(found) != 1 {
		t.Fatalf("findings are %v", found)
	}
}

func TestAByteOrderMarkIsRemoved(t *testing.T) {
	out, found := Format("doc.md", []byte("\xEF\xBB\xBFtext\n"), everything)
	if string(out) != "text\n" {
		t.Fatalf("formatted bytes are %q", out)
	}
	if len(found) != 1 || found[0].Rule != "charset" || found[0].Line != 1 {
		t.Fatalf("findings are %v", found)
	}
}

func TestBytesThatDoNotDecodeAreReportedAndNotRewritten(t *testing.T) {
	// A repair here would be a guess at an encoding nobody has decided, and
	// -write would put the guess in the tree.
	src := []byte("fine\n\xff\xfe not utf-8   \n")
	out, found := Format("doc.md", src, everything)

	if string(out) != string(src) {
		t.Fatalf("the bytes were rewritten: %q", out)
	}
	if len(found) != 1 || found[0].Rule != "charset" || found[0].Line != 2 {
		t.Fatalf("findings are %v", found)
	}
	if strings.Contains(found[0].Detail, "byte order mark") {
		t.Errorf("the finding blames the wrong thing: %v", found[0])
	}
}

func TestAFileThatDoesNotEndWithANewlineGetsOne(t *testing.T) {
	out, found := Format("doc.md", []byte("one\ntwo"), everything)
	if string(out) != "one\ntwo\n" {
		t.Fatalf("formatted bytes are %q", out)
	}
	if len(found) != 1 || found[0].Rule != "insert_final_newline" || found[0].Line != 2 {
		t.Fatalf("findings are %v", found)
	}
}

func TestAnEmptyFileIsLeftEmpty(t *testing.T) {
	// A newline added to nothing produces a file holding one blank line, which
	// is a byte this rule was not asked to add.
	out, found := Format("doc.md", nil, everything)
	if len(out) != 0 {
		t.Fatalf("formatted bytes are %q", out)
	}
	if len(found) != 0 {
		t.Fatalf("findings are %v", found)
	}
}

func TestATabIndentBecomesSpacesWhereThePathIsIndentedWithSpaces(t *testing.T) {
	r := everything
	r.IndentSize = 2
	out, found := Format("workflow.yml", []byte("a:\n\tb: 1\n"), r)

	if string(out) != "a:\n  b: 1\n" {
		t.Fatalf("formatted bytes are %q", out)
	}
	if len(found) != 1 || found[0].Rule != "indent_style" || found[0].Line != 2 {
		t.Fatalf("findings are %v", found)
	}
}

func TestATabInsideALineIsLeftWhereItIs(t *testing.T) {
	// Only the leading run is indentation. A tab further along may be data, and
	// a formatter that rewrote it would edit the file's content.
	out, found := Format("table.txt", []byte("name\tvalue\n"), everything)
	if string(out) != "name\tvalue\n" {
		t.Fatalf("formatted bytes are %q", out)
	}
	if len(found) != 0 {
		t.Fatalf("findings are %v", found)
	}
}

func TestASpaceIndentIsReportedAndNotRepairedWhereThePathIsIndentedWithTabs(t *testing.T) {
	// How many spaces stand for one tab is not a thing this package gets to
	// guess, so it says what is wrong and stops.
	r := everything
	r.IndentStyle = "tab"
	src := []byte("head\n    body\n")
	out, found := Format("notes.txt", src, r)

	if string(out) != string(src) {
		t.Fatalf("the bytes were rewritten: %q", out)
	}
	if len(found) != 1 || found[0].Rule != "indent_style" || found[0].Line != 2 {
		t.Fatalf("findings are %v", found)
	}
}

func TestABlankLineOfSpacesIsNotReportedAsAnIndent(t *testing.T) {
	// Trimming has already emptied it in the same pass, and reporting it twice
	// under two rules sends a reader looking for an indent that is not there.
	r := everything
	r.IndentStyle = "tab"
	out, found := Format("notes.txt", []byte("head\n   \n"), r)

	if string(out) != "head\n\n" {
		t.Fatalf("formatted bytes are %q", out)
	}
	if len(found) != 1 || found[0].Rule != "trim_trailing_whitespace" {
		t.Fatalf("findings are %v", found)
	}
}

func TestAGoFileIsFormattedByTheToolchainAndByNothingElseHere(t *testing.T) {
	src := []byte("package p\n\nfunc  f( ) int {\nreturn 1\n}\n")
	out, found := Format("p.go", src, Rules{Charset: "utf-8", EndOfLine: "lf", IndentStyle: "tab"})

	if !strings.Contains(string(out), "func f() int {") {
		t.Fatalf("formatted bytes are %q", out)
	}
	if len(found) != 1 || found[0].Rule != "gofmt" {
		t.Fatalf("findings are %v", found)
	}
	if found[0].Line != 3 {
		t.Errorf("the finding points at line %d, want the first line that differs, 3", found[0].Line)
	}
}

func TestARawStringLiteralInAGoFileKeepsItsTrailingSpace(t *testing.T) {
	// This is the whole reason a .go file is left to go/format. A line-based
	// trim cannot see the inside of a raw literal, so it would silently edit a
	// program's data.
	src := "package p\n\nconst s = `keep me   \nand me\t\n`\n"
	out, found := Format("p.go", []byte(src), everything)

	if string(out) != src {
		t.Fatalf("the literal was edited: %q", out)
	}
	if len(found) != 0 {
		t.Fatalf("findings are %v", found)
	}
}

func TestAGoFileThatDoesNotParseIsReportedRatherThanReportedAsFormatted(t *testing.T) {
	src := []byte("package p\n\nfunc f( {\n")
	out, found := Format("p.go", src, everything)

	if string(out) != string(src) {
		t.Fatalf("the bytes were rewritten: %q", out)
	}
	if len(found) != 1 || found[0].Rule != "gofmt" {
		t.Fatalf("findings are %v", found)
	}
	if !strings.Contains(found[0].Detail, "does not parse") {
		t.Errorf("the finding does not say why: %v", found[0])
	}
	if found[0].Line != 3 {
		t.Errorf("the finding points at line %d, want 3", found[0].Line)
	}
}

func TestAGoFileWithACarriageReturnIsConvertedBeforeItIsParsed(t *testing.T) {
	out, found := Format("p.go", []byte("package p\r\n"), everything)
	if string(out) != "package p\n" {
		t.Fatalf("formatted bytes are %q", out)
	}
	if len(found) != 1 || found[0].Rule != "end_of_line" {
		t.Fatalf("findings are %v", found)
	}
}

func TestEveryRuleUnsetLeavesAFileThatViolatesAllOfThemAlone(t *testing.T) {
	// This is the shape testdata/fixtures/ is given: the bytes are the thing
	// under test, and a formatter that normalised them would delete the case
	// while leaving the test that reads it green.
	src := []byte("\xEF\xBB\xBFone   \r\n\ttwo")
	out, found := Format("testdata/fixtures/odd.bin", src, Rules{})

	if string(out) != string(src) {
		t.Fatalf("the bytes were rewritten: %q", out)
	}
	if len(found) != 0 {
		t.Fatalf("findings are %v", found)
	}
}

func TestAFindingReadsAsAPositionAndAReason(t *testing.T) {
	f := Finding{File: "docs/layout.md", Line: 12, Rule: "end_of_line", Detail: "holds a carriage return"}
	if got := f.String(); got != "docs/layout.md:12: end_of_line: holds a carriage return" {
		t.Fatalf("a finding prints as %q", got)
	}
}

func TestNothingReportedMeansNothingRewritten(t *testing.T) {
	// The checking mode reads the findings and the writing mode reads the
	// bytes. A file with no findings whose bytes changed would make -write
	// produce a diff the check had said was not there.
	//
	// The other direction does not hold and is not asserted: bytes that do not
	// decode, a .go file that does not parse and a space indent where the rule
	// set says tabs are all reported and deliberately not repaired.
	cases := []struct {
		name string
		src  string
	}{
		{"clean.md", "one\ntwo\n"},
		{"dirty.md", "one   \ntwo"},
		{"clean.go", "package p\n"},
		{"broken.go", "package p\n\nfunc f( {\n"},
		{"empty.md", ""},
		{"bad-utf8.md", "\xff\n"},
	}
	for _, c := range cases {
		out, found := Format(c.name, []byte(c.src), everything)
		if len(found) == 0 && string(out) != c.src {
			t.Errorf("%s: nothing was reported and the bytes became %q", c.name, out)
		}
	}
}

func TestFormattingIsSettledInOnePass(t *testing.T) {
	// Running the tool twice has to be running it once. A rule that moved a
	// byte another rule then moved back would make the check red forever and
	// -write never finish it.
	src := []byte("\xEF\xBB\xBFone   \r\n\ttwo\t\r\n")
	once, first := Format("doc.md", src, everything)
	twice, second := Format("doc.md", once, everything)

	if len(first) == 0 {
		t.Fatal("the fixture was already formatted, so this case proves nothing")
	}
	if string(twice) != string(once) {
		t.Fatalf("a second pass changed the bytes again: %q then %q", once, twice)
	}
	if len(second) != 0 {
		t.Fatalf("a second pass still reports %v", second)
	}
}
