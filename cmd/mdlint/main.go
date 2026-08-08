// Command mdlint reads the documents this repository tracks and refuses one
// written outside the shape the rest of the tree assumes.
//
// It prints every departure as a path, a line, the rule and what was wrong,
// and exits non-zero. There is no writing mode. Two of the rules have more
// than one legal repair, a heading level skip among them, and a tool that
// picked one would be editing the structure of a document rather than its
// bytes.
//
// It is not part of the service. internal/mdlint holds the rules and the
// fixtures that prove each one bites; this file is the argument parsing, the
// file listing and the exit status.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/iderex/kanzlei/internal/mdlint"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fmt.Fprintln(os.Stderr, "mdlint:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("mdlint", flag.ContinueOnError)
	fs.SetOutput(stderr)

	root := fs.String("root", ".", "the repository root, whose tracked documents are read")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}

	files, err := documents(*root)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		// No document is not a green run. It is this program pointed
		// somewhere that holds none, and reporting every rule satisfied would
		// be a claim about a tree it never read.
		return fmt.Errorf("git tracks no document under %s", *root)
	}

	findings, read := mdlint.Check(files)
	for _, f := range findings {
		if _, err := fmt.Fprintln(stdout, f); err != nil {
			return err
		}
	}
	if len(findings) > 0 {
		return fmt.Errorf("%d departure(s) across %d tracked document(s)", len(findings), read.Documents)
	}
	// What was read is printed on the green route too, for the reason
	// internal/mdlint's Reading says: a run that found nothing and a run that
	// read nothing print the same thing otherwise.
	_, err = fmt.Fprintf(stdout, "mdlint: %d line(s) in %d tracked document(s), %d read as prose, %d heading(s), no departures\n",
		read.Lines, read.Documents, read.Prose, read.Headings)
	return err
}

// documents lists the tracked documents, which is what git holds rather than
// what a directory walk finds. A walk would read a build output or an
// untracked draft, so a departure in a file nobody else has would be reported
// here and nowhere a reviewer can see it.
func documents(root string) ([]mdlint.File, error) {
	cmd := exec.Command("git", "-C", root, "ls-files", "-z")
	out, err := cmd.Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return nil, fmt.Errorf("git ls-files: %s", strings.TrimSpace(string(exit.Stderr)))
		}
		return nil, fmt.Errorf("git ls-files: %w", err)
	}
	var files []mdlint.File
	for _, p := range strings.Split(string(out), "\x00") {
		if p == "" || !strings.HasSuffix(p, ".md") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(p)))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", p, err)
		}
		files = append(files, mdlint.File{Name: p, Bytes: b})
	}
	return files, nil
}
