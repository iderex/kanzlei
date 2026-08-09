// Command invariants reads invariants.txt and refuses this repository's own Go
// source where one of the rules in it does not hold.
//
// It prints every violation as a path, a line, the invariant that was violated
// and what was found, followed by the reason that invariant states, and exits
// non-zero. There is no writing mode: every one of these has more than one
// legal repair, and the cheapest of them is usually the wrong one.
//
// It is not part of the service. internal/invariants holds the rules and the
// fixtures that prove each one bites; this file is the argument parsing, the
// file listing and the exit status.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/iderex/kanzlei/internal/invariants"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fmt.Fprintln(os.Stderr, "invariants:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("invariants", flag.ContinueOnError)
	fs.SetOutput(stderr)

	root := fs.String("root", ".", "the repository root, whose Go source is read")
	rules := fs.String("rules", "invariants.txt", "the declaration file, relative to the root")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}

	name := filepath.Join(*root, filepath.FromSlash(*rules))
	src, err := os.ReadFile(name)
	if err != nil {
		return err
	}
	declared, err := invariants.Parse(filepath.ToSlash(*rules), src)
	if err != nil {
		return err
	}

	found, read, err := invariants.CheckTree(declared, *root)
	if err != nil {
		return err
	}
	for _, v := range found {
		if _, err := fmt.Fprintf(stdout, "%s\n    %s\n", v, v.Reason); err != nil {
			return err
		}
	}
	if len(found) > 0 {
		return fmt.Errorf("%d violation(s) of %d invariant(s) across %d file(s)", len(found), len(declared), read)
	}
	// What was read is printed on the green route too. A run that found
	// nothing and a run that read nothing print the same line otherwise, and
	// the second is what a wrong root produces.
	_, err = fmt.Fprintf(stdout, "invariants: %d invariant(s) over %d Go file(s), no violations\n", len(declared), read)
	return err
}
