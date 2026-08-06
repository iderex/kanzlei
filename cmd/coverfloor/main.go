// Command coverfloor refuses a run whose coverage fell below the floor the
// tree holds.
//
// It is not part of the service. It is here because the floor is a decision
// about whether the tree is acceptable, and docs/decisions/0001-means.md
// refuses that kind of decision in a shell block inside a workflow: a rule with
// no fixtures in front of it is a rule nobody can prove bites.
// internal/coverfloor holds the logic and its fixtures; this file is the
// argument parsing and the exit status, and it takes no decision of its own.
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/iderex/kanzlei/internal/coverfloor"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fmt.Fprintln(os.Stderr, "coverfloor:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("coverfloor", flag.ContinueOnError)
	fs.SetOutput(stderr)

	profilePath := fs.String("profile", "coverage.out", "the coverage profile written by the test run")
	floorPath := fs.String("floor", "coverage-floor.txt", "the file holding the number the run has to reach")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}

	// Both files are read whole rather than streamed. A coverage profile for a
	// tree this size is measured in kilobytes, and reading it in one call
	// removes the close whose error would otherwise have to be argued about in
	// a program that is about to exit anyway.
	profile, err := os.ReadFile(*profilePath)
	if err != nil {
		// A missing profile is the failure this program most has to get right.
		// Reporting it as a tree with nothing covered would let a test run that
		// never wrote one be read as a measurement, and reporting it as a pass
		// would let a run that never happened clear the floor.
		return fmt.Errorf("read the profile: %w", err)
	}
	total, err := coverfloor.Measure(bytes.NewReader(profile))
	if err != nil {
		return fmt.Errorf("%s: %w", *profilePath, err)
	}

	floorFile, err := os.ReadFile(*floorPath)
	if err != nil {
		return fmt.Errorf("read the floor: %w", err)
	}
	floor, err := coverfloor.ReadFloor(bytes.NewReader(floorFile))
	if err != nil {
		return fmt.Errorf("%s: %w", *floorPath, err)
	}

	clears, said := coverfloor.Clears(floor, total)
	if !clears {
		return errors.New(said)
	}
	if _, err := fmt.Fprintln(stdout, said); err != nil {
		return err
	}
	return nil
}
