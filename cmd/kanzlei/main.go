// Command kanzlei is the service binary.
//
// It starts, reads no configuration it does not have, serves one endpoint that
// says it is alive, and exits cleanly. That is the whole of it today. Every
// later piece of this project attaches here, and a skeleton that already
// guessed at configuration, identity or storage would be a skeleton that has to
// be argued with before it can be extended.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/iderex/kanzlei/internal/build"
	"github.com/iderex/kanzlei/internal/server"
)

// shutdownGrace bounds how long a request already in flight may hold the
// process open once a stop has been asked for. Without a bound, one slow
// request turns a stop into a hang, and whatever is supervising the process
// eventually kills it, which is the ungraceful exit the grace period exists to
// avoid.
const shutdownGrace = 10 * time.Second

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fmt.Fprintln(os.Stderr, "kanzlei:", err)
		os.Exit(1)
	}
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("kanzlei", flag.ContinueOnError)
	fs.SetOutput(stderr)

	addr := fs.String("addr", "127.0.0.1:8080", "address to serve on, as host:port; a port of 0 asks the operating system to choose one")
	showVersion := fs.Bool("version", false, "print the version, the commit it was built from and the toolchain, then exit")
	stopOnStdinClose := fs.Bool("stop-on-stdin-close", false, "also stop when standard input reaches end of file, for a supervisor that has no signal to send")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}
	if *showVersion {
		return build.WriteTo(stdout)
	}

	srv, err := server.New(*addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", *addr, err)
	}

	// The bound address is printed rather than the requested one, because a
	// requested port of 0 is not an address anything can connect to. Anything
	// supervising this process reads this line to learn where it went.
	if _, err := fmt.Fprintf(stdout, "kanzlei %s listening on %s\n", build.Version(), srv.Addr()); err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if *stopOnStdinClose {
		ctx = stopWhenClosed(ctx, stdin)
	}

	served := make(chan error, 1)
	go func() { served <- srv.Serve() }()

	select {
	case err := <-served:
		// Serving stopped without anybody asking, so the error is the reason
		// and it is not a clean exit.
		if err == nil {
			return errors.New("stopped serving without being asked to")
		}
		return err
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	return <-served
}

// stopWhenClosed returns a context that is also cancelled when r reaches end of
// file.
//
// A container sends SIGTERM and this is not needed. It is here for the
// supervisor, and for the test, that starts this process with a pipe and has no
// portable way to signal it: closing the pipe is then the stop request, and the
// process still leaves through the same graceful path rather than being killed.
// It is off unless asked for, so a process whose standard input is a closed
// file descriptor does not exit the moment it starts.
func stopWhenClosed(parent context.Context, r io.Reader) context.Context {
	ctx, cancel := context.WithCancel(parent)
	go func() {
		defer cancel()
		// Nothing is read from standard input, so everything read is discarded.
		// What is being waited for is the end of it.
		_, _ = io.Copy(io.Discard, r) // a read error ends the wait exactly as end of file does, and both mean stop
	}()
	return ctx
}
