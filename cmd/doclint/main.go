// Command doclint reads the documents this repository tracks and refuses a
// reference that does not resolve.
//
// It prints every unresolved reference as a path, a line and what was named,
// and exits non-zero. There is no writing mode: where a document names a path
// that is not there, either the document is wrong or the path is missing, and
// a tool cannot tell which.
//
// It is not part of the service. internal/doclint holds the rule and its
// fixtures; this file is the argument parsing, the file listing and the exit
// status.
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

	"github.com/iderex/kanzlei/internal/doclint"
	"github.com/iderex/kanzlei/internal/importrules"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fmt.Fprintln(os.Stderr, "doclint:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("doclint", flag.ContinueOnError)
	fs.SetOutput(stderr)

	root := fs.String("root", ".", "the repository root, which is the directory holding import-rules.txt")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}

	tracked, err := trackedPaths(*root)
	if err != nil {
		return err
	}

	rulesPath := filepath.Join(*root, "import-rules.txt")
	src, err := os.ReadFile(rulesPath)
	if err != nil {
		// A missing register is a refusal rather than an empty one. Reading it
		// as no planned packages would turn every document naming one into a
		// finding, which is a gate that teaches people to ignore it.
		return fmt.Errorf("read the planned register: %w", err)
	}
	rules, err := importrules.Parse("import-rules.txt", src)
	if err != nil {
		return err
	}

	tree := doclint.NewTree(tracked, rules.Planned())

	var files []doclint.File
	for _, p := range tracked {
		if !strings.HasSuffix(p, ".md") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(*root, filepath.FromSlash(p)))
		if err != nil {
			return fmt.Errorf("read %s: %w", p, err)
		}
		files = append(files, doclint.File{Name: p, Bytes: b})
	}
	if len(files) == 0 {
		// No document is not a green run. It is this program pointed somewhere
		// that holds none, and reporting every rule satisfied would be a lie
		// about a tree it never read.
		return fmt.Errorf("git tracks no document under %s", *root)
	}

	findings := doclint.Check(files, tree)
	for _, f := range findings {
		if _, err := fmt.Fprintln(stdout, f); err != nil {
			return err
		}
	}
	// The count of what was read is printed on both routes. A gate that says
	// only that it found nothing cannot be told apart from one that read
	// nothing, and this rule reads two positions out of a document rather than
	// every word, so the number is what makes its reach legible.
	read := 0
	for _, f := range files {
		read += len(doclint.References(f, tree))
	}
	if len(findings) > 0 {
		return fmt.Errorf("%d unresolved reference(s) out of %d read across %d tracked document(s)", len(findings), read, len(files))
	}
	_, err = fmt.Fprintf(stdout, "doclint: %d path reference(s) across %d tracked document(s), all resolved\n", read, len(files))
	return err
}

// trackedPaths lists the paths git holds, which is both the set of documents
// this rule reads and the set a reference resolves against. A walk of the
// directory would resolve a reference against a build output or an untracked
// draft, so a document naming a file nobody else has would pass here and fail
// for every reader.
func trackedPaths(root string) ([]string, error) {
	cmd := exec.Command("git", "-C", root, "ls-files", "-z")
	out, err := cmd.Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return nil, fmt.Errorf("git ls-files: %s", strings.TrimSpace(string(exit.Stderr)))
		}
		return nil, fmt.Errorf("git ls-files: %w", err)
	}
	var paths []string
	for _, p := range strings.Split(string(out), "\x00") {
		if p != "" {
			paths = append(paths, p)
		}
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("git tracks no file under %s", root)
	}
	return paths, nil
}
