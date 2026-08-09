// Command linkcheck reads the documents this repository tracks and refuses an
// external link the other end says is not there.
//
// It prints every link it could not judge as well as every one it refuses, and
// exits non-zero only for a refusal. That split is the rule rather than a
// convenience: a network is unreliable, and a gate that goes red because a host
// was slow is a gate people re-run until it is green.
//
// It is not part of the service. internal/linkcheck holds the rule and its
// fixtures; this file is the argument parsing, the file listing, the request
// and the exit status. The request is here because it is the one part no
// fixture can stand in for, and keeping it out of the rule is what lets every
// judgement this program makes be proved without a network.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/iderex/kanzlei/internal/linkcheck"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fmt.Fprintln(os.Stderr, "linkcheck:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("linkcheck", flag.ContinueOnError)
	fs.SetOutput(stderr)

	root := fs.String("root", ".", "the repository root to read documents from")
	attempts := fs.Int("attempts", 3, "how many times an address that answered nothing is asked again")
	pause := fs.Duration("pause", 2*time.Second, "how long to wait before asking again")
	timeout := fs.Duration("timeout", 15*time.Second, "how long one request may take")

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
		// No document is not a green run. It is this program pointed somewhere
		// that holds none, and reporting every link resolved would be a claim
		// about a tree it never read.
		return fmt.Errorf("git tracks no document under %s", *root)
	}

	checker := linkcheck.Checker{
		Probe:    prober(*timeout),
		Attempts: *attempts,
		Pause:    *pause,
	}
	findings, report := checker.Check(files)

	refused := 0
	for _, f := range findings {
		fmt.Fprintln(stdout, f)
		if f.Refused {
			refused++
		}
	}
	// The count of what was read is printed on both routes. This rule reads two
	// positions out of a document rather than every word, and it reaches
	// nothing at all in a tree whose documents carry no external link, so the
	// number is what makes its reach legible.
	fmt.Fprintln(stdout, "linkcheck:", report)
	if refused > 0 {
		return fmt.Errorf("%d link(s) the other end says are not there", refused)
	}
	return nil
}

// prober makes the one request this program cannot prove with a fixture.
//
// GET rather than HEAD, because a server that answers HEAD with a status it
// would not answer GET with is common enough that the cheaper request would
// produce findings about the method rather than about the address. The body is
// discarded without being read: nothing here judges what came back.
func prober(timeout time.Duration) func(string) linkcheck.Answer {
	client := &http.Client{Timeout: timeout}
	return func(url string) linkcheck.Answer {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return linkcheck.Answer{Err: err}
		}
		// A blank agent is refused outright by some hosts, which would arrive
		// here as a status about this program rather than about the address.
		req.Header.Set("User-Agent", "kanzlei-linkcheck")

		resp, err := client.Do(req)
		if err != nil {
			return linkcheck.Answer{Err: err}
		}
		_ = resp.Body.Close() // the status is the whole answer, and a close error changes no verdict
		return linkcheck.Answer{Status: resp.StatusCode}
	}
}

// documents lists the markdown git holds. A walk of the directory would read an
// untracked draft, so a link nobody else has would be judged here and be absent
// for every other reader.
func documents(root string) ([]linkcheck.File, error) {
	cmd := exec.Command("git", "-C", root, "ls-files", "-z")
	out, err := cmd.Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return nil, fmt.Errorf("git ls-files: %s", strings.TrimSpace(string(exit.Stderr)))
		}
		return nil, fmt.Errorf("git ls-files: %w", err)
	}
	var files []linkcheck.File
	for _, p := range strings.Split(string(out), "\x00") {
		if p == "" || !strings.HasSuffix(p, ".md") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(p)))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", p, err)
		}
		files = append(files, linkcheck.File{Name: p, Bytes: b})
	}
	return files, nil
}
