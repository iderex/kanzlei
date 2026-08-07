package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestMain removes the directory the built binary went into. The build is shared
// by every case in this file, so it cannot hang off any one case's t.TempDir.
func TestMain(m *testing.M) {
	code := m.Run()
	if buildOnce.path != "" {
		_ = os.RemoveAll(filepath.Dir(buildOnce.path)) // a temporary directory that could not be removed is not worth failing a green run over
	}
	os.Exit(code)
}

// The values the binary under test is stamped with. They are what makes this a
// release build rather than a plain one: a plain `go build` stamps no version,
// and the assertion below is that the route which does stamp one produces a
// binary that reports it.
const (
	e2eVersion = "v0.0.0-e2e"
	e2eCommit  = "0000000000000000000000000000000000000000"
)

// TestTheProcessStartsAnswersAndExitsCleanly is the end-to-end case: the binary
// is built the way the release route builds it, started as its own process,
// asked whether it is alive, told to stop, and required to leave with a zero
// status.
//
// It needs no display, no administrative rights, no accelerator and no outbound
// network. The only socket is a loopback one this test's own child process
// bound a moment earlier, and the build is run with the module proxy switched
// off, so a machine with no route out runs this the same way.
func TestTheProcessStartsAnswersAndExitsCleanly(t *testing.T) {
	bin := buildBinary(t)

	cmd := exec.Command(bin, "-addr", "127.0.0.1:0", "-stop-on-stdin-close")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		t.Fatalf("start %s: %v", bin, err)
	}
	// Whatever else happens below, this process does not outlive the test.
	t.Cleanup(func() {
		if cmd.ProcessState == nil {
			_ = cmd.Process.Kill() // this runs only where the case already failed, and a process that is already gone reports an error
			_ = cmd.Wait()         // reaping the killed process; its status is the kill, not the case
		}
	})

	addr := readListeningAddress(t, stdout)

	resp, err := http.Get("http://" + addr + "/livez") // loopback: the address is the one this case's own child process was told to bind and printed back
	if err != nil {
		t.Fatalf("get liveness at %s: %v (stderr %q)", addr, err, stderr.String())
	}
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close() // the body is fully read above, so a close error says nothing about it
	if err != nil {
		t.Fatalf("read liveness body: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("liveness status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if got, want := string(body), "ok\n"; got != want {
		t.Errorf("liveness body = %q, want %q", got, want)
	}

	// Closing the pipe is the stop request. The process leaves through the same
	// graceful path a signal would take it through, which is what makes a zero
	// status below mean something.
	if err := stdin.Close(); err != nil {
		t.Fatalf("close stdin: %v", err)
	}

	waited := make(chan error, 1)
	go func() { waited <- cmd.Wait() }()
	select {
	case err := <-waited:
		if err != nil {
			t.Fatalf("the process did not exit cleanly: %v (stderr %q)", err, stderr.String())
		}
	case <-time.After(30 * time.Second):
		t.Fatalf("the process was still running thirty seconds after being told to stop (stderr %q)", stderr.String())
	}
}

// TestAReleaseBuildReportsItsVersionAndCommit is the assertion that the
// stamping route works. An operator who cannot say which build they are running
// is reporting a defect against a tree nobody can identify.
func TestAReleaseBuildReportsItsVersionAndCommit(t *testing.T) {
	bin := buildBinary(t)

	out, err := exec.Command(bin, "-version").Output()
	if err != nil {
		t.Fatalf("run -version: %v", err)
	}

	lines := strings.Split(strings.TrimSuffix(string(out), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("-version printed %d lines, want 3: %q", len(lines), out)
	}
	for i, line := range lines {
		_, value, found := strings.Cut(line, " ")
		if !found || strings.TrimSpace(value) == "" {
			t.Errorf("-version line %d = %q, has no value", i+1, line)
		}
	}
	if got, want := lines[0], "kanzlei "+e2eVersion; got != want {
		t.Errorf("version line = %q, want %q", got, want)
	}
	if got, want := lines[1], "commit "+e2eCommit; got != want {
		t.Errorf("commit line = %q, want %q", got, want)
	}
}

// buildOnce holds the one build shared by every case in this file. Building per
// case costs a full compile per case for no extra assurance: both cases are
// about the binary the release route produces, and that is one binary.
var buildOnce struct {
	sync.Once
	path string
	err  error
	log  string
}

// buildBinary returns the path to the binary built the way the release route
// builds it, building it on first use.
func buildBinary(t *testing.T) string {
	t.Helper()

	buildOnce.Do(func() { buildOnce.path, buildOnce.log, buildOnce.err = buildRelease() })
	if buildOnce.err != nil {
		t.Fatalf("go build: %v\n%s", buildOnce.err, buildOnce.log)
	}
	return buildOnce.path
}

func buildRelease() (path, log string, err error) {
	dir, err := os.MkdirTemp("", "kanzlei-e2e")
	if err != nil {
		return "", "", err
	}
	out := filepath.Join(dir, "kanzlei")
	if runtime.GOOS == "windows" {
		out += ".exe"
	}

	root, err := moduleRoot()
	if err != nil {
		return "", "", err
	}
	tool, err := goTool()
	if err != nil {
		return "", "", err
	}

	const pkg = "github.com/iderex/kanzlei/internal/build"
	// Stamped the way the release route stamps, and not otherwise identical to
	// it: -trimpath is left off deliberately, because it changes the build
	// cache key and makes every run of this suite recompile the standard
	// library for no assurance this case is about. What is under test is that a
	// stamped binary reports what it was stamped with. #118 owns the release
	// route and the rest of its flags.
	cmd := exec.Command(tool,
		"build",
		"-ldflags", "-X "+pkg+".version="+e2eVersion+" -X "+pkg+".commit="+e2eCommit,
		"-o", out,
		"./cmd/kanzlei",
	)
	cmd.Dir = root
	// The build is required to succeed with the module proxy switched off and
	// the requirements taken as written. If a dependency is ever added without
	// its lock, this is where that shows up rather than on somebody's machine
	// with no route out.
	cmd.Env = append(os.Environ(), "GOPROXY=off", "GOFLAGS=-mod=readonly", "GOWORK=off")

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", string(output), err
	}
	return out, string(output), nil
}

// goTool prefers the toolchain that is running this test over whatever the
// search path happens to hold, so the binary under test is built by the version
// go.mod pinned rather than by a second one that happens to be installed.
func goTool() (string, error) {
	candidate := filepath.Join(runtime.GOROOT(), "bin", "go")
	if runtime.GOOS == "windows" {
		candidate += ".exe"
	}
	if _, err := os.Stat(candidate); err == nil {
		return candidate, nil
	}
	path, err := exec.LookPath("go")
	if err != nil {
		return "", fmt.Errorf("no go toolchain: not at %s and not on the search path: %w", candidate, err)
	}
	return path, nil
}

// moduleRoot walks up from the test's directory to the directory holding
// go.mod, so the build does not depend on where the test was started from.
func moduleRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("no go.mod above the test's working directory")
		}
		dir = parent
	}
}

// readListeningAddress reads the one line the process prints on startup and
// returns the address it bound. Reading the address rather than guessing a port
// is what lets this test run beside anything else on the same machine.
func readListeningAddress(t *testing.T, stdout io.Reader) string {
	t.Helper()

	type result struct {
		line string
		err  error
	}
	lines := make(chan result, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		if scanner.Scan() {
			lines <- result{line: scanner.Text()}
			return
		}
		err := scanner.Err()
		if err == nil {
			err = errors.New("the process printed nothing before its output ended")
		}
		lines <- result{err: err}
	}()

	select {
	case got := <-lines:
		if got.err != nil {
			t.Fatalf("read the startup line: %v", got.err)
		}
		// kanzlei <version> listening on <host:port>
		fields := strings.Fields(got.line)
		if len(fields) != 5 || fields[0] != "kanzlei" || fields[2] != "listening" || fields[3] != "on" {
			t.Fatalf("startup line = %q, want `kanzlei <version> listening on <address>`", got.line)
		}
		if fields[1] != e2eVersion {
			t.Errorf("startup line reports version %q, want %q", fields[1], e2eVersion)
		}
		return fields[4]
	case <-time.After(30 * time.Second):
		t.Fatal("the process printed no startup line within thirty seconds")
		return ""
	}
}
