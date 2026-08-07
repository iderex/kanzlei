package treefmt

import (
	"os"
	"strings"
	"testing"
)

func mustParse(t *testing.T, src string) *Set {
	t.Helper()
	set, err := Parse("fixture.editorconfig", []byte(src))
	if err != nil {
		t.Fatalf("the fixture did not parse: %v", err)
	}
	return set
}

func TestALaterSectionLaysOverAnEarlierOne(t *testing.T) {
	set := mustParse(t, `root = true

[*]
indent_style = space
indent_size = 4

[*.yml]
indent_size = 2
`)
	if !set.Root {
		t.Error("root = true was not read")
	}
	if got := set.RulesFor("a/b/c.yml"); got.IndentStyle != "space" || got.IndentSize != 2 {
		t.Errorf("rules for a .yml are %+v", got)
	}
	if got := set.RulesFor("a/b/c.txt"); got.IndentSize != 4 {
		t.Errorf("rules for a .txt are %+v", got)
	}
}

func TestUnsetTurnsAnInheritedRuleOffRatherThanSettingItToSomething(t *testing.T) {
	set := mustParse(t, `root = true

[*]
charset = utf-8
trim_trailing_whitespace = true
indent_style = space
indent_size = 4

[testdata/fixtures/**]
charset = unset
trim_trailing_whitespace = false
indent_style = unset
indent_size = unset
`)
	got := set.RulesFor("testdata/fixtures/odd.bin")
	if got != (Rules{}) {
		t.Fatalf("a path under the unset section still carries %+v", got)
	}
}

func TestAPatternWithNoSeparatorMatchesTheNameAtAnyDepth(t *testing.T) {
	set := mustParse(t, "root = true\n\n[*.go]\nindent_style = tab\n")
	for _, path := range []string{"main.go", "cmd/kanzlei/main.go", "a/b/c/d.go"} {
		if got := set.RulesFor(path); got.IndentStyle != "tab" {
			t.Errorf("%s got %+v", path, got)
		}
	}
	if got := set.RulesFor("main.gox"); got.IndentStyle != "" {
		t.Errorf("main.gox matched: %+v", got)
	}
}

func TestAPatternWithASeparatorIsAnchoredAtTheRoot(t *testing.T) {
	set := mustParse(t, "root = true\n\n[testdata/fixtures/**]\ninsert_final_newline = false\nindent_size = unset\n")
	if !set.sections[0].matches("testdata/fixtures/a/b.bin") {
		t.Error("a path under the directory did not match")
	}
	// The near miss: the same directory name one level down. A pattern that
	// silently matched it would turn a rule about one directory into a rule
	// about every directory with that name.
	if set.sections[0].matches("vendor/testdata/fixtures/a.bin") {
		t.Error("a nested directory of the same name matched")
	}
}

func TestABraceListStandsForEachOfItsAlternatives(t *testing.T) {
	set := mustParse(t, "root = true\n\n[*.{yml,yaml}]\nindent_size = 2\n")
	for _, path := range []string{"a.yml", ".github/workflows/tests.yaml"} {
		if got := set.RulesFor(path); got.IndentSize != 2 {
			t.Errorf("%s got %+v", path, got)
		}
	}
	// The near miss: the literal text of the group. A pattern treated as
	// literal characters would match this and nothing else.
	if got := set.RulesFor("a.{yml,yaml}"); got.IndentSize != 0 {
		t.Errorf("the literal brace text matched: %+v", got)
	}
}

func TestAStarStopsAtASeparatorAndAStarStarDoesNot(t *testing.T) {
	if globMatch("*.go", "a/b.go") {
		t.Error("* crossed a separator")
	}
	if !globMatch("**/b.go", "a/c/b.go") {
		t.Error("** did not cross a separator")
	}
	if !globMatch("**/b.go", "b.go") {
		t.Error("**/ did not stand for no directory at all")
	}
	if !globMatch("a?c.md", "abc.md") {
		t.Error("? did not match one character")
	}
	if globMatch("a?c.md", "a/c.md") {
		t.Error("? matched a separator")
	}
}

func TestAPropertyThisCheckerDoesNotImplementIsRefusedWithItsLine(t *testing.T) {
	// A rule somebody wrote down and nothing applies is worse than no rule,
	// because the tree then looks governed and is not.
	_, err := Parse("f", []byte("root = true\n\n[*]\nindent_style = space\nmax_line_length = 80\n"))
	if err == nil {
		t.Fatal("an unimplemented property parsed")
	}
	if !strings.Contains(err.Error(), "f:5") || !strings.Contains(err.Error(), "max_line_length") {
		t.Fatalf("the error does not name the line and the property: %v", err)
	}
}

func TestAValueThisCheckerCannotCarryOutIsRefused(t *testing.T) {
	_, err := Parse("f", []byte("root = true\n\n[*]\nend_of_line = crlf\n"))
	if err == nil {
		t.Fatal("end_of_line = crlf parsed")
	}
	if !strings.Contains(err.Error(), "f:4") {
		t.Fatalf("the error does not name the line: %v", err)
	}
}

func TestAPropertyAboveTheFirstSectionIsRefused(t *testing.T) {
	// It applies to no path, so a contributor who wrote it there has a rule
	// they believe is in force and is not.
	_, err := Parse("f", []byte("root = true\nindent_style = space\n\n[*]\ncharset = utf-8\n"))
	if err == nil {
		t.Fatal("a property in the preamble parsed")
	}
	if !strings.Contains(err.Error(), "f:2") {
		t.Fatalf("the error does not name the line: %v", err)
	}
}

func TestPatternSyntaxThisCheckerDoesNotImplementIsRefused(t *testing.T) {
	cases := map[string]string{
		"a character class": "[*.[ch]]",
		"a numeric range":   "[file{1..9}.txt]",
		"an unclosed brace": "[*.{yml,yaml]",
	}
	for what, header := range cases {
		if _, err := Parse("f", []byte("root = true\n\n"+header+"\ncharset = utf-8\n")); err == nil {
			t.Errorf("%s parsed", what)
		}
	}
}

func TestALineThatIsNeitherASectionNorAPropertyIsRefused(t *testing.T) {
	for _, src := range []string{
		"root = true\n\n[*]\ncharset\n",
		"root = true\n\n[*\ncharset = utf-8\n",
		"root = yes\n",
		"root = true\n\n[*]\nindent_size = 0\n",
		"root = true\n\n[*]\nindent_style = tabs\n",
		"root = true\n\n[*]\ninsert_final_newline = 1\n",
		"root = true\n\n[*]\ncharset = latin-1\n",
	} {
		if _, err := Parse("f", []byte(src)); err == nil {
			t.Errorf("this parsed and should not have: %q", src)
		}
	}
}

func TestCommentsAndBlankLinesAreSkippedAndKeysAreCaseInsensitive(t *testing.T) {
	set := mustParse(t, "# a comment\n; another\nroot = TRUE\n\n[*]\nIndent_Style = SPACE\nindent_size = 4\n")
	if !set.Root {
		t.Error("root was not read")
	}
	if got := set.RulesFor("a.txt"); got.IndentStyle != "space" {
		t.Errorf("rules are %+v", got)
	}
}

func TestAPathNoSectionMatchesIsLeftAlone(t *testing.T) {
	set := mustParse(t, "root = true\n\n[*.go]\nindent_style = tab\n")
	if got := set.RulesFor("README"); got != (Rules{}) {
		t.Fatalf("a path nothing matches carries %+v", got)
	}
}

func TestThisRepositorysOwnRuleSetParsesAndDeclaresRoot(t *testing.T) {
	// This one is about the tree rather than about the guard: it says the file
	// at the root today is one this package can read, so a rule written in it
	// is a rule that is in force. The cases above are what prove the guard.
	src, err := os.ReadFile("../../.editorconfig")
	if err != nil {
		t.Fatalf("read the rule set: %v", err)
	}
	set, err := Parse(".editorconfig", src)
	if err != nil {
		t.Fatalf("the repository's own rule set does not parse: %v", err)
	}
	if !set.Root {
		t.Error(".editorconfig does not declare root = true")
	}
	if got := set.RulesFor("main.go"); got.IndentStyle != "tab" {
		t.Errorf("a .go file is judged with %+v", got)
	}
	if got := set.RulesFor("testdata/fixtures/a.bin"); got != (Rules{}) {
		t.Errorf("a fixture is judged with %+v rather than left alone", got)
	}
}
