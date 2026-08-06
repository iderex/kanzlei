//go:build needsreal

package needsreal

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"
)

// A Requirement is one thing a suite cannot run without: a service that has to
// be reachable, an accelerator that has to be present, an address that has to
// be configured. Met reports nil when the requirement is there, and otherwise
// an error naming what is missing in the words the readme uses.
type Requirement struct {
	Name string
	Met  func() error
}

// EnvSet is the common case: a suite that needs to be told where something is.
// The how string is repeated back to whoever runs into the refusal, so it says
// what to set the variable to rather than only that it is unset.
func EnvSet(name, how string) Requirement {
	return Requirement{
		Name: name,
		Met: func() error {
			if strings.TrimSpace(os.Getenv(name)) == "" {
				return fmt.Errorf("%s is not set: %s", name, how)
			}
			return nil
		},
	}
}

// Outcome is what happened to a suite in one run.
type Outcome int

const (
	// NotAsked is the starting state and the one a suite keeps when the run
	// selected no case from it. It is the state this harness exists to make
	// visible: a suite nobody asked for produces no failure, and without a
	// roster it produces no trace either.
	NotAsked Outcome = iota
	// Refused means a case in the suite was selected and the suite turned it
	// away because a requirement was missing.
	Refused
	// Ran means a case in the suite was selected and every requirement was met.
	Ran
)

func (o Outcome) String() string {
	switch o {
	case Ran:
		return "ran"
	case Refused:
		return "refused"
	default:
		return "not asked for"
	}
}

// A Suite is a named group of cases and the requirements they share.
//
// It is a value a suite declares once, at package level, and passes to Start at
// the top of each of its cases. The declaration is what the roster reads, so a
// suite that is never selected still appears in the report.
type Suite struct {
	Name    string
	Needs   []Requirement
	Why     string // one line saying what the suite proves that the default run cannot
	mu      sync.Mutex
	outcome Outcome
	refusal string
}

// Missing reports the first requirement that is not met, and the error naming
// it. It reports nil, nil when every requirement is there.
//
// The check is a plain function rather than something buried in Start so that
// the decision is testable without a test binary that has already been run.
func (s *Suite) Missing() (*Requirement, error) {
	for i := range s.Needs {
		if err := s.Needs[i].Met(); err != nil {
			return &s.Needs[i], err
		}
	}
	return nil, nil
}

// Refusal is the message a refused suite prints.
func (s *Suite) Refusal(err error) string {
	return fmt.Sprintf("%s needs something this machine does not have: %v", s.Name, err)
}

// Start refuses the calling case when a requirement is missing, naming the
// missing one, and otherwise lets it run.
//
// Refusing here rather than letting the case proceed is the whole point. A case
// that runs without its service fails somewhere inside an assertion, on a
// connection error or a nil value, and the person reading that failure starts
// by suspecting the code under test.
func (s *Suite) Start(t *testing.T) {
	t.Helper()

	missing, err := s.Missing()
	if missing != nil {
		s.record(Refused, s.Refusal(err))
		t.Skip(s.refused())
		return
	}
	s.record(Ran, "")
}

func (s *Suite) record(o Outcome, refusal string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Ran wins over Refused, and both win over NotAsked, so a suite whose
	// requirements changed mid-run cannot report less than what happened.
	if o > s.outcome {
		s.outcome = o
	}
	if refusal != "" && s.refusal == "" {
		s.refusal = refusal
	}
}

// Outcome reports what happened to this suite so far.
func (s *Suite) Outcome() (Outcome, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.outcome, s.refusal
}

func (s *Suite) refused() string {
	_, refusal := s.Outcome()
	return refusal
}

// A Roster is the set of suites a test binary knows about and what happened to
// each. Printing it is what stops a partial run from reading like a full one.
type Roster struct {
	mu     sync.Mutex
	suites []*Suite
}

// Register adds a suite to the roster. A suite that is registered and never
// started is reported as not asked for, which is the case a run that selected a
// subset produces.
func (r *Roster) Register(s *Suite) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.suites = append(r.suites, s)
}

// Report writes the roster: every registered suite, what happened to it, and
// for a refused one the requirement that was missing.
//
// It is not called WriteTo, because that name belongs to io.WriterTo and this
// is not one.
func (r *Roster) Report(w io.Writer) error {
	r.mu.Lock()
	registered := make([]*Suite, len(r.suites))
	copy(registered, r.suites)
	r.mu.Unlock()

	sort.Slice(registered, func(i, j int) bool { return registered[i].Name < registered[j].Name })

	var b strings.Builder
	fmt.Fprintf(&b, "needs-real-hardware-or-services: %d suite(s)\n", len(registered))
	for _, s := range registered {
		outcome, refusal := s.Outcome()
		fmt.Fprintf(&b, "  %s: %s\n", s.Name, outcome)
		if refusal != "" {
			fmt.Fprintf(&b, "      %s\n", refusal)
		}
	}
	fmt.Fprint(&b, "A suite reported as not asked for proved nothing in this run.\n")

	_, err := io.WriteString(w, b.String())
	return err
}

// suites is the roster of this test binary. Every suite in this directory
// registers into it, and Run prints it.
var suites = &Roster{}

// Register adds a suite to this binary's roster.
func Register(s *Suite) { suites.Register(s) }

// Run is the TestMain body for this directory. It runs the cases and then
// prints the roster, so the last thing on the terminal says which suites proved
// something and which did not.
func Run(m *testing.M) {
	code := m.Run()
	if err := suites.Report(os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "needs-real-hardware-or-services: could not write the roster:", err)
		if code == 0 {
			code = 1
		}
	}
	os.Exit(code)
}
