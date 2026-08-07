// Command treefmt formats every tracked file the way .editorconfig says it is
// formatted, or reports what departs from it.
//
// Two modes and one decision behind both. Without -write it prints every
// departure with the file, the line and the rule, and exits non-zero.  With
// -write it puts the bytes the rule set asks for into the files.  The bytes and
// the findings come out of the same call, so the mode that checks and the mode
// that writes cannot disagree about what formatted means.
//
// It is not part of the service. It is here because whether the tree is
// formatted is a decision about whether the tree is acceptable, and
// docs/decisions/0001-means.md refuses that kind of decision in a shell block
// inside a workflow: a rule with no fixtures in front of it is a rule nobody
// can prove bites. internal/treefmt holds the logic and its fixtures; this file
// is the argument parsing, the file listing and the exit status.
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

	"github.com/iderex/kanzlei/internal/treefmt"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fmt.Fprintln(os.Stderr, "treefmt:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("treefmt", flag.ContinueOnError)
	fs.SetOutput(stderr)

	root := fs.String("root", ".", "the repository root, which is the directory holding .editorconfig")
	write := fs.Bool("write", false, "write the formatted bytes back instead of reporting what departs")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}

	rulesPath := filepath.Join(*root, ".editorconfig")
	src, err := os.ReadFile(rulesPath)
	if err != nil {
		// A missing rule set is a refusal rather than an empty one. Treating it
		// as no rules would let a deleted .editorconfig pass this gate green
		// while removing everything the gate enforces.
		return fmt.Errorf("read the rule set: %w", err)
	}
	set, err := treefmt.Parse(".editorconfig", src)
	if err != nil {
		return err
	}
	if !set.Root {
		return fmt.Errorf("%s does not declare root = true, so an .editorconfig outside this repository would be laid under it", rulesPath)
	}

	paths, err := tracked(*root)
	if err != nil {
		return err
	}

	files := make([]treefmt.File, 0, len(paths))
	for _, p := range paths {
		b, err := os.ReadFile(filepath.Join(*root, filepath.FromSlash(p)))
		if err != nil {
			// A tracked path that is not in the working tree is a checkout the
			// caller should know about rather than a file to pass over.
			return fmt.Errorf("read %s: %w", p, err)
		}
		files = append(files, treefmt.File{Name: p, Bytes: b})
	}

	results := treefmt.Check(set, files)

	if *write {
		written := 0
		for _, r := range results {
			if len(r.Findings) == 0 {
				continue
			}
			target := filepath.Join(*root, filepath.FromSlash(r.Name))
			info, err := os.Stat(target)
			if err != nil {
				return fmt.Errorf("stat %s: %w", r.Name, err)
			}
			if err := os.WriteFile(target, r.Formatted, info.Mode().Perm()); err != nil {
				return fmt.Errorf("write %s: %w", r.Name, err)
			}
			written++
		}
		_, err := fmt.Fprintf(stdout, "treefmt: %d file(s) rewritten out of %d tracked\n", written, len(files))
		return err
	}

	findings := treefmt.Findings(results)
	for _, f := range findings {
		if _, err := fmt.Fprintln(stdout, f); err != nil {
			return err
		}
	}
	if len(findings) > 0 {
		return fmt.Errorf("%d departure(s) from .editorconfig across %d tracked file(s); `go run ./cmd/treefmt -write` fixes what is fixable", len(findings), len(files))
	}
	_, err = fmt.Fprintf(stdout, "treefmt: %d tracked file(s) match .editorconfig\n", len(files))
	return err
}

// tracked lists the paths git holds, which is the set this rule is about. A
// walk of the directory would judge whatever a contributor happened to leave
// lying about, and would report a build output as a defect in the tree.
func tracked(root string) ([]string, error) {
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
		// Nothing tracked means the listing failed to mean what it looks like:
		// this program run against a directory that is not a checkout would
		// otherwise report every rule satisfied.
		return nil, fmt.Errorf("git tracks no file under %s", root)
	}
	return paths, nil
}
