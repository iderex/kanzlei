// Package scanfloor reads the SARIF a code scanning run produced and decides
// whether the tree it describes is acceptable.
//
// The decision is here rather than in a shell block inside the workflow for the
// reason docs/decisions/0001-means.md gives: a rule that decides whether the
// tree is acceptable belongs where a fixture can be put in front of it. That
// matters more here than it does for the coverage floor, because this rule has
// a conservative direction and a direction is the kind of thing that is easy to
// get backwards and impossible to notice afterwards.
//
// The conservative direction, stated once. A finding whose severity this
// package cannot determine blocks. It is not skipped, not treated as low and
// not reported as a zero. An analyser that emits a result under a rule this
// package cannot find, or under a rule carrying no severity at all, has told us
// something and we do not know how bad it is, and reading that as acceptable is
// how a gate stops gating. docs/decisions/0003-permission-model.md takes the
// same direction for permissions and #18 makes it refusable there; this is the
// same rule applied to the tree's own defects.
//
// SARIF is parsed here rather than read out of a query tool, for the same
// reason the coverage profile is. The parts read are the ones the format
// guarantees: runs, their results, and the rule metadata a result points at.
// CodeQL carries the severity of a query in a rule property rather than on the
// result, and it puts the rules in the tool extensions rather than on the
// driver, so both places are searched.
package scanfloor

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
)

// A Finding is one result from a scan, with the severity of the rule it was
// reported under already resolved.
//
// Severity is kept beside the flag saying whether it is known, rather than
// using a sentinel number, because every sentinel that could be chosen here is
// also a real severity somewhere. A missing severity is a different fact from a
// severity of zero and this type refuses to collapse them.
type Finding struct {
	RuleID        string
	URI           string
	Line          int
	Message       string
	Severity      float64
	SeverityKnown bool
}

// String is what the gate prints for a finding it is refusing. It names the
// rule, where the result is, and what the severity was, because a refusal a
// reader cannot act on is a refusal they will switch off.
func (f Finding) String() string {
	where := f.URI
	if where == "" {
		where = "an unnamed location"
	}
	if f.Line > 0 {
		where = fmt.Sprintf("%s:%d", where, f.Line)
	}
	severity := "no severity declared"
	if f.SeverityKnown {
		severity = fmt.Sprintf("severity %.1f", f.Severity)
	}
	line := fmt.Sprintf("%s: %s (%s)", where, f.RuleID, severity)
	if f.Message != "" {
		line += ": " + f.Message
	}
	return line
}

// The shape of the parts of SARIF this package reads. Everything the format
// allows and this package does not use is left out deliberately: a struct that
// mirrored the whole schema would suggest the rest was being checked.
type sarif struct {
	Runs []struct {
		Tool struct {
			Driver     component   `json:"driver"`
			Extensions []component `json:"extensions"`
		} `json:"tool"`
		Results []result `json:"results"`
	} `json:"runs"`
}

type component struct {
	Rules []rule `json:"rules"`
}

type rule struct {
	ID         string `json:"id"`
	Properties struct {
		// CodeQL writes this as a string holding a number, which is what the
		// hosting service reads to place an alert in a severity band. It is a
		// string in the format and it is parsed rather than assumed.
		SecuritySeverity string `json:"security-severity"`
	} `json:"properties"`
}

type result struct {
	RuleID string `json:"ruleId"`
	Rule   struct {
		ID string `json:"id"`
	} `json:"rule"`
	Message struct {
		Text string `json:"text"`
	} `json:"message"`
	Locations []struct {
		PhysicalLocation struct {
			ArtifactLocation struct {
				URI string `json:"uri"`
			} `json:"artifactLocation"`
			Region struct {
				StartLine int `json:"startLine"`
			} `json:"region"`
		} `json:"physicalLocation"`
	} `json:"locations"`
}

// Read parses a SARIF document and returns every result in it.
//
// A document with no runs is an error rather than an empty result set. A run
// that produced nothing writes one run holding an empty results array; zero
// runs means the analysis did not happen, and a scan that did not happen must
// not be readable as a scan that found nothing.
func Read(r io.Reader) ([]Finding, error) {
	var doc sarif
	if err := json.NewDecoder(r).Decode(&doc); err != nil {
		return nil, fmt.Errorf("parse the sarif: %w", err)
	}
	if len(doc.Runs) == 0 {
		return nil, fmt.Errorf("the sarif holds no runs, so no analysis is described by it")
	}

	var findings []Finding
	for _, run := range doc.Runs {
		severities := map[string]float64{}
		components := append([]component{run.Tool.Driver}, run.Tool.Extensions...)
		for _, c := range components {
			for _, rule := range c.Rules {
				raw := strings.TrimSpace(rule.Properties.SecuritySeverity)
				if rule.ID == "" || raw == "" {
					continue
				}
				value, err := strconv.ParseFloat(raw, 64)
				if err != nil {
					// Left out of the map rather than defaulted, so the result
					// reported under it is one whose severity is unknown, which
					// is the direction this package refuses in.
					continue
				}
				severities[rule.ID] = value
			}
		}

		for _, res := range run.Results {
			id := res.RuleID
			if id == "" {
				id = res.Rule.ID
			}
			if id == "" {
				id = "a result naming no rule"
			}
			finding := Finding{RuleID: id, Message: res.Message.Text}
			if len(res.Locations) > 0 {
				finding.URI = res.Locations[0].PhysicalLocation.ArtifactLocation.URI
				finding.Line = res.Locations[0].PhysicalLocation.Region.StartLine
			}
			if severity, ok := severities[id]; ok {
				finding.Severity, finding.SeverityKnown = severity, true
			}
			findings = append(findings, finding)
		}
	}
	return findings, nil
}

// Blocking returns the findings that refuse the run, worst first.
//
// A finding blocks when its severity is at or above the floor, and when its
// severity is not known at all. The second half is the whole point of the
// function and it is why this is not a filter written at the call site.
func Blocking(findings []Finding, floor float64) []Finding {
	var blocking []Finding
	for _, f := range findings {
		if !f.SeverityKnown || f.Severity >= floor {
			blocking = append(blocking, f)
		}
	}
	sort.SliceStable(blocking, func(i, j int) bool {
		a, b := blocking[i], blocking[j]
		// An unknown severity sorts first. It is the case a reader most needs
		// to see, because it is the one where the number in the floor file did
		// not decide anything.
		if a.SeverityKnown != b.SeverityKnown {
			return !a.SeverityKnown
		}
		if a.Severity != b.Severity {
			return a.Severity > b.Severity
		}
		return a.RuleID < b.RuleID
	})
	return blocking
}

// ReadFloor reads the number the tree holds. Comment lines beginning with a
// hash and blank lines are skipped, so the file can carry the argument for the
// number beside it.
//
// A file holding no number is an error rather than a floor of zero. A floor of
// zero is a real setting here, meaning every finding blocks, and it has to be
// written down deliberately rather than arrived at by an empty file.
func ReadFloor(r io.Reader) (float64, error) {
	body, err := io.ReadAll(r)
	if err != nil {
		return 0, fmt.Errorf("read the floor: %w", err)
	}
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		value, err := strconv.ParseFloat(line, 64)
		if err != nil {
			return 0, fmt.Errorf("the floor %q is not a number", line)
		}
		if value < 0 {
			return 0, fmt.Errorf("the floor %q is below zero, and a severity never is", line)
		}
		return value, nil
	}
	return 0, fmt.Errorf("the floor file holds no number")
}

// Verdict says in one line what was judged and what was decided, for the run
// that passes as well as for the run that does not. A gate that prints nothing
// when it is happy is a gate nobody can tell ran.
func Verdict(findings, blocking []Finding, floor float64) string {
	switch {
	case len(findings) == 0:
		return fmt.Sprintf("no findings, against a floor of %.1f", floor)
	case len(blocking) == 0:
		return fmt.Sprintf("%d finding(s), none at or above the floor of %.1f", len(findings), floor)
	default:
		return fmt.Sprintf("%d of %d finding(s) at or above the floor of %.1f, or carrying no severity", len(blocking), len(findings), floor)
	}
}
