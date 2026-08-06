package coverfloor_test

import (
	"strings"
	"testing"

	"github.com/iderex/kanzlei/internal/coverfloor"
)

// The fixtures below are written as plain string literals rather than base64.
// The rule in CONTRIBUTING.md is for fixtures whose exact bytes are the thing
// under test; nothing here turns on a line ending or an encoding, and a profile
// a reader cannot read is a fixture nobody can check against the toolchain's
// own output.

// oneCovered is the shape the toolchain writes: a mode line, then one line per
// block, with the block position, the number of statements in it and the number
// of times it was entered.
const oneCovered = `mode: atomic
github.com/iderex/kanzlei/internal/a/a.go:10.20,12.3 2 1
github.com/iderex/kanzlei/internal/a/a.go:14.20,16.3 2 0
`

func TestMeasureCountsStatementsAndNotBlocks(t *testing.T) {
	total, err := coverfloor.Measure(strings.NewReader(oneCovered))
	if err != nil {
		t.Fatalf("measure: %v", err)
	}
	if total.Statements != 4 || total.Covered != 2 {
		t.Fatalf("got %d of %d statements covered, want 2 of 4", total.Covered, total.Statements)
	}
	if got := total.Percent(); got != 50 {
		t.Fatalf("got %v%%, want 50%%", got)
	}
}

// The case a floor is for. The same package loaded by two test binaries appears
// twice in one profile, and a block one binary entered is covered whatever the
// other one did. Taking the last occurrence instead of the sum would report the
// reach of whichever binary the toolchain wrote last.
func TestMeasureSumsRepeatedBlocksRatherThanTakingTheLast(t *testing.T) {
	profile := oneCovered + `github.com/iderex/kanzlei/internal/a/a.go:14.20,16.3 2 3
github.com/iderex/kanzlei/internal/a/a.go:10.20,12.3 2 0
`
	total, err := coverfloor.Measure(strings.NewReader(profile))
	if err != nil {
		t.Fatalf("measure: %v", err)
	}
	if total.Statements != 4 || total.Covered != 4 {
		t.Fatalf("got %d of %d statements covered, want 4 of 4", total.Covered, total.Statements)
	}
}

func TestMeasureRefusesAProfileThatMeasuredNothing(t *testing.T) {
	for name, profile := range map[string]string{
		"empty":          "",
		"mode line only": "mode: atomic\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := coverfloor.Measure(strings.NewReader(profile)); err == nil {
				t.Fatal("a profile holding no counted block was accepted; a run that measured nothing must not clear a floor")
			}
		})
	}
}

func TestMeasureRefusesAMalformedLine(t *testing.T) {
	for name, line := range map[string]string{
		"two fields":              "a.go:1.2,3.4 2\n",
		"no block position":       "a.go 2 1\n",
		"statements not a number": "a.go:1.2,3.4 x 1\n",
		"count not a number":      "a.go:1.2,3.4 2 x\n",
		"negative count":          "a.go:1.2,3.4 2 -1\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := coverfloor.Measure(strings.NewReader("mode: atomic\n" + line)); err == nil {
				t.Fatalf("a profile line %q was accepted", line)
			}
		})
	}
}

// A file name holding a colon is why the position is cut from the right. It is
// not a shape this repository produces, and it is the shape that would silently
// mis-key a block if the cut were made from the left.
func TestMeasureCutsTheBlockPositionFromTheRight(t *testing.T) {
	profile := "mode: atomic\nexample.com/x:y/a.go:10.20,12.3 3 1\n"
	total, err := coverfloor.Measure(strings.NewReader(profile))
	if err != nil {
		t.Fatalf("measure: %v", err)
	}
	if total.Covered != 3 {
		t.Fatalf("got %d covered, want 3", total.Covered)
	}
}

func TestReadFloorTakesTheNumberAndIgnoresTheExplanation(t *testing.T) {
	floor, err := coverfloor.ReadFloor(strings.NewReader("# why this number\n\n69.9\n"))
	if err != nil {
		t.Fatalf("read floor: %v", err)
	}
	if floor != 69.9 {
		t.Fatalf("got %v, want 69.9", floor)
	}
}

func TestReadFloorRefusesWhatCannotBeOneNumber(t *testing.T) {
	for name, content := range map[string]string{
		"no number":     "# only an explanation\n",
		"two numbers":   "50\n60\n",
		"not a number":  "most of it\n",
		"above hundred": "101\n",
		"negative":      "-1\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := coverfloor.ReadFloor(strings.NewReader(content)); err == nil {
				t.Fatalf("floor file %q was accepted", content)
			}
		})
	}
}

// The bite. A measurement below the floor is refused, and the refusal names
// both numbers, because a gate that says only "too low" sends a contributor to
// find out what the floor was.
func TestClearsRefusesAMeasurementBelowTheFloor(t *testing.T) {
	ok, said := coverfloor.Clears(69.9, coverfloor.Total{Statements: 1000, Covered: 698})
	if ok {
		t.Fatal("69.8% cleared a floor of 69.9%")
	}
	for _, want := range []string{"69.8", "69.9", "698", "1000"} {
		if !strings.Contains(said, want) {
			t.Fatalf("the refusal %q does not name %q", said, want)
		}
	}
}

func TestClearsAcceptsTheFloorItself(t *testing.T) {
	if ok, said := coverfloor.Clears(69.9, coverfloor.Total{Statements: 1000, Covered: 699}); !ok {
		t.Fatalf("69.9%% did not clear a floor of 69.9%%: %s", said)
	}
}

// The comparison is made on the number a reader can see. An exact percentage a
// hair under the floor prints as the floor, and a gate refusing a run whose
// every report says it passed is a gate nobody can act on.
func TestClearsComparesTheNumberThatIsPrinted(t *testing.T) {
	total := coverfloor.Total{Statements: 100000, Covered: 69899}
	if got := total.Percent(); got >= 69.9 {
		t.Fatalf("the fixture is not below the floor exactly: %v", got)
	}
	if ok, said := coverfloor.Clears(69.9, total); !ok {
		t.Fatalf("a total printing as 69.9%% was refused against a floor of 69.9%%: %s", said)
	}
}

func TestPercentOfNothingIsNotEverything(t *testing.T) {
	if got := (coverfloor.Total{}).Percent(); got != 0 {
		t.Fatalf("got %v%%, want 0%%", got)
	}
}
