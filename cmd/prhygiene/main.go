// Command prhygiene refuses a pull request that does not name the work it
// belongs to.
//
// It is not part of the service. internal/prhygiene holds the decision and its
// fixtures; this file is the input gathering and the exit status, and it takes
// no decision of its own, for the reason docs/decisions/0001-means.md gives.
//
// The body arrives in an environment variable and never as an argument. A pull
// request body is written by whoever opened it, and text somebody else wrote
// has no business being expanded into a command line.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/iderex/kanzlei/internal/prhygiene"
)

// bodyVariable is where the body is read from. The name is in one place so the
// workflow and the failure message cannot disagree about it.
const bodyVariable = "PR_BODY"

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr, os.Getenv); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fmt.Fprintln(os.Stderr, "prhygiene:", err)
		os.Exit(1)
	}
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer, getenv func(string) string) error {
	fs := flag.NewFlagSet("prhygiene", flag.ContinueOnError)
	fs.SetOutput(stderr)

	changed := fs.Int("changed", -1, "lines added plus removed, or a negative number where the size was not measured")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}

	// The commit list arrives on standard input, one line per commit, as
	// `git log --format=%p%x09%s` writes it. The command does not run git
	// itself: the workflow already has the checkout, and a program that shells
	// out to git is a program whose cases need a repository to exist.
	log, err := io.ReadAll(stdin)
	if err != nil {
		return fmt.Errorf("read the commit list: %w", err)
	}
	commits, err := prhygiene.ParseLog(string(log))
	if err != nil {
		return err
	}

	body := getenv(bodyVariable)
	report := prhygiene.Judge(prhygiene.Request{Body: body, Commits: commits, Changed: *changed})

	for _, line := range report.Lines() {
		if _, err := fmt.Fprintln(stdout, line); err != nil {
			return err
		}
	}
	if report.Refused() {
		return fmt.Errorf("%d refusal(s); every change here starts as an issue and says which one", len(report.Refusals))
	}
	return nil
}
