// Package coverfloor reads a coverage profile and decides whether the run that
// produced it reached the floor the tree holds.
//
// The floor is a weak instrument and it is here anyway, for one reason: it
// makes a package that arrived with no tests visible on the day it lands rather
// than a year later. It measures reach and not quality, and nothing in this
// package pretends otherwise. #109 is where the measurement gets teeth.
//
// The profile is parsed here rather than read out of `go tool cover -func`,
// because the decision this package takes is whether the tree is acceptable,
// and that decision belongs somewhere a fixture can be put in front of it.
// docs/decisions/0001-means.md is where that rule is argued. The format is the
// toolchain's own and is stable: a mode line, then one line per block.
package coverfloor

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// A Total is what a profile says about a run: how many statements it holds and
// how many of them were executed at least once.
//
// Both numbers are kept rather than only the percentage, because a percentage
// on its own cannot be added to another one and cannot say how large the thing
// measured was.
type Total struct {
	Statements int
	Covered    int
}

// Percent is the share of statements executed at least once, as a number
// between 0 and 100.
//
// A profile holding no statements reports 0 rather than dividing by zero. That
// is the honest answer: a run that measured nothing did not reach anything, and
// reporting 100 for it would let an empty measurement clear every floor.
func (t Total) Percent() float64 {
	if t.Statements == 0 {
		return 0
	}
	return 100 * float64(t.Covered) / float64(t.Statements)
}

// String is the one-line form the gate prints.
func (t Total) String() string {
	return fmt.Sprintf("%.1f%% of %d statements", t.Percent(), t.Statements)
}

// block identifies one counted region of one file. It is the key the counts are
// accumulated under, because the same block appears once per test binary that
// loaded the package, and a block executed by one binary and not by another is
// covered.
type block struct {
	file  string
	where string
}

// Measure reads a coverage profile and reports what it covers.
//
// Counts for the same block are summed before anything is decided, which is
// what `go tool cover -func` does and what makes the number here comparable to
// the one that command prints. Summing rather than taking the last occurrence
// matters in exactly the case a floor is for: a package exercised from two test
// binaries would otherwise report the reach of whichever binary the toolchain
// happened to write last.
func Measure(r io.Reader) (Total, error) {
	counts := map[block]int{}
	statements := map[block]int{}
	order := 0

	scanner := bufio.NewScanner(r)
	// A profile line is short, but a generated file can carry a long path, and
	// the default 64KiB limit failing mid-file would report a smaller tree as a
	// complete measurement.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "mode:") {
			continue
		}
		b, stmts, count, err := parseLine(line)
		if err != nil {
			return Total{}, err
		}
		if _, seen := statements[b]; !seen {
			order++
		}
		statements[b] = stmts
		counts[b] += count
	}
	if err := scanner.Err(); err != nil {
		return Total{}, fmt.Errorf("read the profile: %w", err)
	}
	if order == 0 {
		return Total{}, fmt.Errorf("the profile holds no counted blocks; a run that measured nothing is not a measurement")
	}

	var total Total
	for b, stmts := range statements {
		total.Statements += stmts
		if counts[b] > 0 {
			total.Covered += stmts
		}
	}
	return total, nil
}

// parseLine reads one profile line: a file name and a block position, then the
// number of statements in the block and the number of times it was entered.
//
//	github.com/x/y/z.go:12.34,15.2 3 1
//
// The file name may itself hold a colon on a system that allows one, so the
// position is cut from the right rather than the left.
func parseLine(line string) (b block, statements, count int, err error) {
	fields := strings.Fields(line)
	if len(fields) != 3 {
		return b, 0, 0, fmt.Errorf("profile line %q has %d fields, want 3", line, len(fields))
	}
	cut := strings.LastIndex(fields[0], ":")
	if cut < 0 {
		return b, 0, 0, fmt.Errorf("profile line %q names no block position", line)
	}
	b = block{file: fields[0][:cut], where: fields[0][cut+1:]}
	if b.file == "" || b.where == "" {
		return b, 0, 0, fmt.Errorf("profile line %q names no file or no block position", line)
	}
	if statements, err = strconv.Atoi(fields[1]); err != nil {
		return b, 0, 0, fmt.Errorf("profile line %q: statement count: %w", line, err)
	}
	if count, err = strconv.Atoi(fields[2]); err != nil {
		return b, 0, 0, fmt.Errorf("profile line %q: execution count: %w", line, err)
	}
	if statements < 0 || count < 0 {
		return b, 0, 0, fmt.Errorf("profile line %q holds a negative count", line)
	}
	return b, statements, count, nil
}

// ReadFloor reads the number the tree holds.
//
// The file is a number and the lines that explain it. Blank lines and lines
// starting with a hash are the explanation; exactly one other line is expected,
// and two would leave a reader unable to say which one the gate used.
func ReadFloor(r io.Reader) (float64, error) {
	var found []string
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		found = append(found, line)
	}
	if err := scanner.Err(); err != nil {
		return 0, fmt.Errorf("read the floor: %w", err)
	}
	if len(found) != 1 {
		return 0, fmt.Errorf("the floor file holds %d numbers, want exactly 1", len(found))
	}
	floor, err := strconv.ParseFloat(strings.TrimSuffix(found[0], "%"), 64)
	if err != nil {
		return 0, fmt.Errorf("the floor %q is not a number: %w", found[0], err)
	}
	if floor < 0 || floor > 100 {
		return 0, fmt.Errorf("the floor %v is not a percentage between 0 and 100", floor)
	}
	return floor, nil
}

// Clears reports whether the measured total reaches the floor, and the sentence
// to print either way.
//
// The comparison is made on the rounded number rather than on the exact one,
// because the rounded number is what every report of this measurement shows. A
// floor set from a printed 69.9 and refused by an exact 69.8999 would be a gate
// failing against a number nobody can see.
func Clears(floor float64, total Total) (bool, string) {
	measured := round1(total.Percent())
	if measured < round1(floor) {
		return false, fmt.Sprintf("coverage %.1f%% is below the floor of %.1f%% (%d of %d statements)", measured, floor, total.Covered, total.Statements)
	}
	return true, fmt.Sprintf("coverage %.1f%% clears the floor of %.1f%% (%d of %d statements)", measured, floor, total.Covered, total.Statements)
}

func round1(v float64) float64 {
	// strconv rather than arithmetic, so that the number compared is exactly
	// the number formatted by every other line in this package.
	rounded, err := strconv.ParseFloat(strconv.FormatFloat(v, 'f', 1, 64), 64)
	if err != nil {
		return v
	}
	return rounded
}
