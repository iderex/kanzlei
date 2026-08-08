// Command scanfloor refuses a code scanning run that found something the tree
// may not carry.
//
// It is not part of the service. It is here because the threshold is a decision
// about whether the tree is acceptable, and docs/decisions/0001-means.md
// refuses that kind of decision in a shell block inside a workflow: a rule with
// no fixtures in front of it is a rule nobody can prove bites, and this rule
// has a direction that is easy to write backwards.
// internal/scanfloor holds the logic and its fixtures; this file is the
// argument parsing and the exit status, and it takes no decision of its own.
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/iderex/kanzlei/internal/scanfloor"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fmt.Fprintln(os.Stderr, "scanfloor:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("scanfloor", flag.ContinueOnError)
	fs.SetOutput(stderr)

	sarifPath := fs.String("sarif", "", "the sarif file the analysis wrote")
	floorPath := fs.String("floor", "scan-floor.txt", "the file holding the severity at or above which a finding refuses the run")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}
	if *sarifPath == "" {
		return errors.New("no -sarif given, and this program will not judge a file it was not pointed at")
	}

	document, err := os.ReadFile(*sarifPath)
	if err != nil {
		// The failure this program most has to get right. An analysis that
		// crashed before writing anything leaves no file, and reporting that as
		// a tree with no findings would let a scan that never happened clear
		// the gate.
		return fmt.Errorf("read the sarif: %w", err)
	}
	findings, err := scanfloor.Read(bytes.NewReader(document))
	if err != nil {
		return fmt.Errorf("%s: %w", *sarifPath, err)
	}

	floorFile, err := os.ReadFile(*floorPath)
	if err != nil {
		return fmt.Errorf("read the floor: %w", err)
	}
	floor, err := scanfloor.ReadFloor(bytes.NewReader(floorFile))
	if err != nil {
		return fmt.Errorf("%s: %w", *floorPath, err)
	}

	blocking := scanfloor.Blocking(findings, floor)
	verdict := scanfloor.Verdict(findings, blocking, floor)
	if len(blocking) > 0 {
		for _, f := range blocking {
			if _, err := fmt.Fprintln(stderr, f); err != nil {
				return err
			}
		}
		return errors.New(verdict)
	}
	if _, err := fmt.Fprintln(stdout, verdict); err != nil {
		return err
	}
	return nil
}
