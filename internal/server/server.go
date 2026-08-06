// Package server holds the HTTP surface. Today that is one endpoint that says
// the process is alive, and nothing else. The shape exists so that later work
// has somewhere to attach; it deliberately carries no configuration, no
// identity and no datastore, because a skeleton that already holds those is a
// skeleton nobody can change.
package server

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"time"
)

// LivenessPath answers whether this process is running and able to serve. It
// says nothing about whether the process is ready to do useful work: that is a
// different question with a different answer, and collapsing the two produces a
// deployment that takes traffic before it can serve it.
const LivenessPath = "/livez"

// Server owns a listener and the HTTP server over it.
type Server struct {
	http *http.Server
	ln   net.Listener
}

// New binds addr and returns a Server that has not started serving.
//
// Binding here rather than inside Serve is what makes the address knowable
// before any request is made: a caller may pass a port of 0, and Addr then
// reports what the operating system chose. A test that has to guess a free port
// is a test that fails on a machine where something else took it.
func New(addr string) (*Server, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	return &Server{
		ln: ln,
		http: &http.Server{
			Handler: Handler(),
			// A client that opens a connection and then says nothing holds a
			// goroutine and a file descriptor for as long as it likes without
			// these. The numbers are conservative starting points, not tuning.
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout:       30 * time.Second,
			WriteTimeout:      30 * time.Second,
			IdleTimeout:       60 * time.Second,
		},
	}, nil
}

// Addr reports the address actually bound, including the chosen port.
func (s *Server) Addr() string { return s.ln.Addr().String() }

// Serve serves until Shutdown or Close is called, and reports nil in that case
// rather than the sentinel the standard library returns. A caller that has
// asked for a shutdown and gets an error back has to know which errors mean
// "as you asked", and that is the check being made once here instead of at
// every call site.
func (s *Server) Serve() error {
	err := s.http.Serve(s.ln)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// Shutdown stops accepting connections and waits for the ones in flight, up to
// the deadline on ctx.
func (s *Server) Shutdown(ctx context.Context) error { return s.http.Shutdown(ctx) }

// Handler is the routing table. It is a function rather than a package variable
// so that a test gets its own, and so that nothing can be added to a shared one
// from an init somewhere else in the tree.
func Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+LivenessPath, live)
	return mux
}

func live(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	// Nothing about the host, the build or the state of any dependency. A
	// liveness endpoint is reachable by anything that can reach the port, which
	// at this stage of the project is everything, so it answers the one bit it
	// was asked for and discloses nothing else.
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, "ok\n")
}
