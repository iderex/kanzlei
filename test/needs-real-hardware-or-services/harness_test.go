//go:build needsreal

package needsreal

import (
	"errors"
	"strings"
	"testing"
)

func TestAMissingRequirementIsNamed(t *testing.T) {
	harness.Start(t)

	suite := &Suite{
		Name: "example",
		Needs: []Requirement{
			{Name: "always there", Met: func() error { return nil }},
			{Name: "the accelerator", Met: func() error { return errors.New("no accelerator: this machine has no supported device") }},
			{Name: "never reached", Met: func() error { return errors.New("this requirement is after the missing one") }},
		},
	}

	missing, err := suite.Missing()
	if missing == nil {
		t.Fatal("Missing() found nothing missing")
	}
	if missing.Name != "the accelerator" {
		t.Errorf("Missing() named %q, want the first unmet requirement", missing.Name)
	}
	// The refusal repeats what the requirement said rather than a generic
	// sentence. Somebody who meets this message has to be able to act on it
	// without opening the source.
	if got := suite.Refusal(err); !strings.Contains(got, "no accelerator") || !strings.Contains(got, "example") {
		t.Errorf("Refusal() = %q, want it to name the suite and what is missing", got)
	}
}

func TestEveryRequirementMetIsNotARefusal(t *testing.T) {
	harness.Start(t)

	suite := &Suite{Name: "example", Needs: []Requirement{{Name: "always there", Met: func() error { return nil }}}}

	missing, err := suite.Missing()
	if missing != nil || err != nil {
		t.Fatalf("Missing() = %v, %v, want nothing missing", missing, err)
	}
}

func TestASuiteNobodyAskedForSaysSo(t *testing.T) {
	harness.Start(t)

	// A roster of this test's own, so what is asserted here is the reporting
	// and not the state of the run printing it.
	var roster Roster
	asked := &Suite{Name: "asked"}
	unasked := &Suite{Name: "unasked"}
	refused := &Suite{Name: "refused", Needs: []Requirement{{Name: "a service", Met: func() error { return errors.New("nothing is listening") }}}}
	roster.Register(asked)
	roster.Register(unasked)
	roster.Register(refused)

	asked.record(Ran, "")
	missing, err := refused.Missing()
	if missing == nil {
		t.Fatal("the refused suite reported nothing missing")
	}
	refused.record(Refused, refused.Refusal(err))

	var report strings.Builder
	if err := roster.Report(&report); err != nil {
		t.Fatalf("Report: %v", err)
	}
	got := report.String()

	for _, want := range []string{
		"3 suite(s)",
		"asked: ran",
		"unasked: not asked for",
		"refused: refused",
		"nothing is listening",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the report does not contain %q:\n%s", want, got)
		}
	}
}

func TestTheEnvironmentRequirementSaysWhatToSet(t *testing.T) {
	harness.Start(t)

	const how = "the address of a model runtime, as host:port"
	req := EnvSet("KANZLEI_TEST_NOTHING_SETS_THIS", how)

	err := req.Met()
	if err == nil {
		t.Fatal("an unset variable was reported as met")
	}
	if !strings.Contains(err.Error(), "KANZLEI_TEST_NOTHING_SETS_THIS") || !strings.Contains(err.Error(), how) {
		t.Errorf("Met() = %v, want it to name the variable and what to set it to", err)
	}

	t.Setenv("KANZLEI_TEST_NOTHING_SETS_THIS", "127.0.0.1:11434")
	if err := req.Met(); err != nil {
		t.Errorf("Met() with the variable set = %v, want nil", err)
	}

	// Whitespace is not a value. A variable set to a space in a compose file is
	// the shape that produces a suite running against an address of nothing.
	t.Setenv("KANZLEI_TEST_NOTHING_SETS_THIS", "   ")
	if err := req.Met(); err == nil {
		t.Error("a variable holding only whitespace was reported as met")
	}
}

func TestStartRefusesRatherThanFailing(t *testing.T) {
	harness.Start(t)

	suite := &Suite{Name: "example", Needs: []Requirement{{Name: "a service", Met: func() error { return errors.New("nothing is listening") }}}}

	// Start refuses by skipping, and a skip is only observable from inside the
	// case it skipped, so the case is run as a subtest and its result is read
	// afterwards.
	var reached bool
	ok := t.Run("refused", func(t *testing.T) {
		suite.Start(t)
		reached = true
	})

	if !ok {
		t.Error("a refused case failed rather than being turned away")
	}
	if reached {
		t.Error("the case body ran after its requirement was found missing")
	}
	if outcome, refusal := suite.Outcome(); outcome != Refused || !strings.Contains(refusal, "nothing is listening") {
		t.Errorf("Outcome() = %v, %q, want Refused naming what is missing", outcome, refusal)
	}
}

func TestTheHarnessSuiteReportsThatItRan(t *testing.T) {
	harness.Start(t)

	if outcome, _ := harness.Outcome(); outcome != Ran { // the refusal is empty for a suite that ran, so only the outcome is read
		t.Errorf("the harness suite reports %v after starting a case, want ran", outcome)
	}
}
