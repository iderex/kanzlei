// Command cmdlint reads the commands this repository's documents show and
// refuses one whose shell syntax does not close.
//
// It prints every refusal as a path, a line, the rule and what was wrong, and
// exits non-zero. There is no writing mode. A command that does not close is
// missing something the document never held, and a tool that guessed at it
// would be writing the command rather than checking it.
//
// It is not part of the service. internal/cmdlint holds the rule and the
// fixtures that prove each part of it bites; this file is the argument
// parsing, the file listing and the exit status.
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

	"github.com/iderex/kanzlei/internal/cmdlint"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fmt.Fprintln(os.Stderr, "cmdlint:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("cmdlint", flag.ContinueOnError)
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
		// somewhere that holds none, and reporting every command whole would
		// be a claim about a tree it never read.
		return fmt.Errorf("git tracks no document under %s", *root)
	}

	findings, read := cmdlint.Check(files)
	for _, f := range findings {
		if _, err := fmt.Fprintln(stdout, f); err != nil {
			return err
		}
	}
	if len(findings) > 0 {
		return fmt.Errorf("%d command(s) that do not close across %d tracked document(s)", len(findings), read.Documents)
	}
	// What was read is printed on the green route too, and so is what was
	// passed over. This rule reads a narrow part of a document, so a run that
	// found nothing and a run that read nothing print the same line otherwise.
	_, err = fmt.Fprintln(stdout, "cmdlint:", read)
	return err
}

// documents lists the tracked documents, which is what git holds rather than
// what a directory walk finds. A walk would read a build output or an
// untracked draft, so a command nobody else has would be judged here and be
// absent for every other reader.
func documents(root string) ([]cmdlint.File, error) {
	cmd := exec.Command("git", "-C", root, "ls-files", "-z")
	out, err := cmd.Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return nil, fmt.Errorf("git ls-files: %s", strings.TrimSpace(string(exit.Stderr)))
		}
		return nil, fmt.Errorf("git ls-files: %w", err)
	}
	var files []cmdlint.File
	for _, p := range strings.Split(string(out), "\x00") {
		if p == "" || !strings.HasSuffix(p, ".md") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(p)))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", p, err)
		}
		files = append(files, cmdlint.File{Name: p, Bytes: b})
	}
	return files, nil
}
