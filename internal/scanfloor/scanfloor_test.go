package scanfloor

import (
	"errors"
	"strings"
	"testing"
)

// A minimal SARIF holding one rule and one result under it. The severity and
// the rule the result names are parameters so a case can take them apart, which
// is what the near-miss below does.
func sarifDoc(ruleID, severity, resultRule string) string {
	return `{"runs":[{"tool":{"driver":{"name":"CodeQL"},"extensions":[{"rules":[
	  {"id":"` + ruleID + `","properties":{"security-severity":"` + severity + `"}}]}]},
	  "results":[{"ruleId":"` + resultRule + `","message":{"text":"a message"},
	  "locations":[{"physicalLocation":{"artifactLocation":{"uri":"internal/x/y.go"},
	  "region":{"startLine":12}}}]}]}]}`
}

func TestReadResolvesSeverityFromTheExtensionRules(t *testing.T) {
	findings, err := Read(strings.NewReader(sarifDoc("go/path-injection", "7.5", "go/path-injection")))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("got %d findings, want 1", len(findings))
	}
	got := findings[0]
	if got.RuleID != "go/path-injection" || !got.SeverityKnown || got.Severity != 7.5 {
		t.Errorf("got %+v, want go/path-injection at a known 7.5", got)
	}
	if got.URI != "internal/x/y.go" || got.Line != 12 {
		t.Errorf("got location %s:%d, want internal/x/y.go:12", got.URI, got.Line)
	}
	if !strings.Contains(got.String(), "internal/x/y.go:12") || !strings.Contains(got.String(), "severity 7.5") {
		t.Errorf("String() = %q, want it to name the location and the severity", got.String())
	}
}

func TestReadResolvesSeverityFromTheDriverRules(t *testing.T) {
	doc := `{"runs":[{"tool":{"driver":{"name":"CodeQL","rules":[
	  {"id":"go/sql-injection","properties":{"security-severity":"9.8"}}]}},
	  "results":[{"ruleId":"go/sql-injection","message":{"text":"m"}}]}]}`
	findings, err := Read(strings.NewReader(doc))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(findings) != 1 || !findings[0].SeverityKnown || findings[0].Severity != 9.8 {
		t.Fatalf("got %+v, want one finding at a known 9.8", findings)
	}
	// A finding with no location still has to print something a reader can act
	// on, because the rule id is the part that names what was found.
	if !strings.Contains(findings[0].String(), "an unnamed location") {
		t.Errorf("String() = %q, want it to say the location is unnamed", findings[0].String())
	}
}

func TestReadTakesTheRuleIdFromTheRuleObjectWhenRuleIdIsAbsent(t *testing.T) {
	doc := `{"runs":[{"tool":{"driver":{"name":"CodeQL"}},
	  "results":[{"rule":{"id":"go/unsafe-quoting"},"message":{"text":"m"}}]}]}`
	findings, err := Read(strings.NewReader(doc))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(findings) != 1 || findings[0].RuleID != "go/unsafe-quoting" {
		t.Fatalf("got %+v, want the id read from the rule object", findings)
	}
}

func TestReadNamesAResultThatNamesNoRule(t *testing.T) {
	doc := `{"runs":[{"tool":{"driver":{"name":"CodeQL"}},"results":[{"message":{"text":"m"}}]}]}`
	findings, err := Read(strings.NewReader(doc))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(findings) != 1 || findings[0].SeverityKnown {
		t.Fatalf("got %+v, want one finding whose severity is not known", findings)
	}
	if !strings.Contains(findings[0].RuleID, "no rule") {
		t.Errorf("RuleID = %q, want it to say the result named no rule", findings[0].RuleID)
	}
}

func TestReadRefusesADocumentWithNoRuns(t *testing.T) {
	// The case this exists for: an analysis that did not run writes a document
	// with no runs, and reading that as a clean tree is the failure the whole
	// gate is against.
	if _, err := Read(strings.NewReader(`{"runs":[]}`)); err == nil {
		t.Fatal("Read accepted a document with no runs, and a scan that did not happen must not read as a scan that found nothing")
	}
}

func TestReadRefusesADocumentThatIsNotJson(t *testing.T) {
	if _, err := Read(strings.NewReader("not json at all")); err == nil {
		t.Fatal("Read accepted a document that is not JSON")
	}
}

func TestReadAcceptsARunThatFoundNothing(t *testing.T) {
	findings, err := Read(strings.NewReader(`{"runs":[{"tool":{"driver":{"name":"CodeQL"}},"results":[]}]}`))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("got %d findings, want none", len(findings))
	}
}

func TestSeverityIsUnknownWhenTheRuleIsMissingOrUnreadable(t *testing.T) {
	cases := []struct {
		name string
		doc  string
	}{
		{
			// The result names a rule that is in no component of the document.
			name: "the rule is not in the document",
			doc:  sarifDoc("go/path-injection", "7.5", "go/some-other-query"),
		},
		{
			name: "the rule carries no severity",
			doc:  `{"runs":[{"tool":{"driver":{"name":"CodeQL","rules":[{"id":"go/x"}]}},"results":[{"ruleId":"go/x","message":{"text":"m"}}]}]}`,
		},
		{
			name: "the severity is not a number",
			doc:  sarifDoc("go/path-injection", "high", "go/path-injection"),
		},
		{
			name: "the rule carries a severity under no id",
			doc:  `{"runs":[{"tool":{"driver":{"name":"CodeQL","rules":[{"properties":{"security-severity":"9.1"}}]}},"results":[{"ruleId":"go/x","message":{"text":"m"}}]}]}`,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			findings, err := Read(strings.NewReader(c.doc))
			if err != nil {
				t.Fatalf("Read: %v", err)
			}
			if len(findings) != 1 {
				t.Fatalf("got %d findings, want 1", len(findings))
			}
			if findings[0].SeverityKnown {
				t.Fatalf("severity read as known %v, want unknown", findings[0].Severity)
			}
			if !strings.Contains(findings[0].String(), "no severity declared") {
				t.Errorf("String() = %q, want it to say no severity was declared", findings[0].String())
			}
			// The direction that matters: unknown blocks, at any floor.
			if len(Blocking(findings, 9.9)) != 1 {
				t.Error("a finding whose severity is unknown did not block, and unknown must mean blocked")
			}
		})
	}
}

func TestBlockingComparesAgainstTheFloor(t *testing.T) {
	findings := []Finding{
		{RuleID: "low", Severity: 3.1, SeverityKnown: true},
		{RuleID: "exactly-the-floor", Severity: 7.0, SeverityKnown: true},
		{RuleID: "high", Severity: 9.8, SeverityKnown: true},
	}
	blocking := Blocking(findings, 7.0)
	if len(blocking) != 2 {
		t.Fatalf("got %d blocking, want 2", len(blocking))
	}
	// At or above, not above: a finding sitting exactly on the floor blocks,
	// which is the off-by-one somebody will write the other way round.
	if blocking[0].RuleID != "high" || blocking[1].RuleID != "exactly-the-floor" {
		t.Fatalf("got %v, want the worst first and the floor itself included", blocking)
	}
}

func TestBlockingSortsTheUnknownSeverityFirst(t *testing.T) {
	findings := []Finding{
		{RuleID: "critical", Severity: 9.8, SeverityKnown: true},
		{RuleID: "unknown"},
		{RuleID: "also-unknown"},
	}
	blocking := Blocking(findings, 7.0)
	if len(blocking) != 3 {
		t.Fatalf("got %d blocking, want 3", len(blocking))
	}
	if blocking[0].SeverityKnown || blocking[1].SeverityKnown {
		t.Fatalf("got %v, want both unknowns ahead of the known severity", blocking)
	}
	if blocking[0].RuleID != "also-unknown" {
		t.Errorf("ties are not ordered by rule id: got %v", blocking)
	}
}

func TestBlockingIsEmptyWhenNothingReachesTheFloor(t *testing.T) {
	findings := []Finding{{RuleID: "low", Severity: 3.1, SeverityKnown: true}}
	if got := Blocking(findings, 7.0); len(got) != 0 {
		t.Fatalf("got %v, want nothing blocking", got)
	}
}

func TestAFloorOfZeroBlocksEverySecurityFinding(t *testing.T) {
	findings := []Finding{
		{RuleID: "lowest", Severity: 0.0, SeverityKnown: true},
		{RuleID: "low", Severity: 2.5, SeverityKnown: true},
	}
	if got := Blocking(findings, 0.0); len(got) != 2 {
		t.Fatalf("got %d blocking at a floor of zero, want 2", len(got))
	}
}

func TestReadFloor(t *testing.T) {
	cases := []struct {
		name string
		body string
		want float64
		bad  bool
	}{
		{name: "a bare number", body: "7.0\n", want: 7.0},
		{name: "comments and blank lines are skipped", body: "# why\n\n  0.0  \n", want: 0.0},
		{name: "the first number wins", body: "4.0\n9.0\n", want: 4.0},
		{name: "an empty file is refused", body: "", bad: true},
		{name: "a file of only comments is refused", body: "# nothing but this\n", bad: true},
		{name: "a word is refused", body: "high\n", bad: true},
		{name: "a negative floor is refused", body: "-1\n", bad: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ReadFloor(strings.NewReader(c.body))
			if c.bad {
				if err == nil {
					t.Fatalf("ReadFloor(%q) = %v, want an error", c.body, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ReadFloor(%q): %v", c.body, err)
			}
			if got != c.want {
				t.Fatalf("ReadFloor(%q) = %v, want %v", c.body, got, c.want)
			}
		})
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("the disk said no") }

func TestReadFloorReportsAReadThatFailed(t *testing.T) {
	if _, err := ReadFloor(failingReader{}); err == nil {
		t.Fatal("ReadFloor accepted a reader that failed")
	}
}

func TestVerdictSaysWhatWasJudged(t *testing.T) {
	none := Verdict(nil, nil, 7.0)
	if !strings.Contains(none, "no findings") || !strings.Contains(none, "7.0") {
		t.Errorf("Verdict for a clean run = %q, want it to name the floor it judged against", none)
	}

	found := []Finding{{RuleID: "low", Severity: 1, SeverityKnown: true}}
	under := Verdict(found, nil, 7.0)
	if !strings.Contains(under, "none at or above") {
		t.Errorf("Verdict for findings under the floor = %q", under)
	}

	over := Verdict(found, found, 0.0)
	if !strings.Contains(over, "1 of 1") {
		t.Errorf("Verdict for a refusal = %q, want it to count both", over)
	}
}
