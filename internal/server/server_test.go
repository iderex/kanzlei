package server_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/iderex/kanzlei/internal/server"
)

func TestLivenessAnswersOK(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, server.LivenessPath, nil))

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got, want := rec.Body.String(), "ok\n"; got != want {
		t.Errorf("body = %q, want %q", got, want)
	}
	if got, want := rec.Header().Get("Content-Type"), "text/plain; charset=utf-8"; got != want {
		t.Errorf("Content-Type = %q, want %q", got, want)
	}
}

func TestLivenessRefusesOtherMethods(t *testing.T) {
	t.Parallel()

	// A liveness probe is a read. A route that answers a write the same way is
	// a route somebody will later hang a side effect on.
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		rec := httptest.NewRecorder()
		server.Handler().ServeHTTP(rec, httptest.NewRequest(method, server.LivenessPath, nil))
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s %s: status = %d, want %d", method, server.LivenessPath, rec.Code, http.StatusMethodNotAllowed)
		}
	}
}

func TestNothingElseIsServed(t *testing.T) {
	t.Parallel()

	// The surface is one endpoint. This test is what makes a second one a
	// deliberate change rather than something that appeared.
	for _, path := range []string{"/", "/healthz", "/readyz", "/metrics", "/debug/pprof/", "/livez/"} {
		rec := httptest.NewRecorder()
		server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusNotFound {
			t.Errorf("GET %s: status = %d, want %d", path, rec.Code, http.StatusNotFound)
		}
	}
}

func TestPortZeroReportsTheChosenPort(t *testing.T) {
	t.Parallel()

	srv, err := server.New("127.0.0.1:0")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx) // cleanup after the case has already reported, where a shutdown error changes no verdict
	})

	if addr := srv.Addr(); addr == "" || addr == "127.0.0.1:0" {
		t.Fatalf("Addr() = %q, want the address the operating system chose", addr)
	}
}

func TestServeThenShutdownIsNotAnError(t *testing.T) {
	t.Parallel()

	srv, err := server.New("127.0.0.1:0")
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	served := make(chan error, 1)
	go func() { served <- srv.Serve() }()

	resp, err := http.Get("http://" + srv.Addr() + server.LivenessPath) // loopback: the address is the one this case bound a line above, on 127.0.0.1
	if err != nil {
		t.Fatalf("get liveness: %v", err)
	}
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close() // the body is fully read above, so a close error says nothing about it
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if resp.StatusCode != http.StatusOK || string(body) != "ok\n" {
		t.Fatalf("liveness over a real listener: status %d body %q", resp.StatusCode, body)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	// The point of the wrapper: an asked-for stop reports nil, so a caller does
	// not have to know which sentinel means "as you asked".
	select {
	case err := <-served:
		if err != nil {
			t.Fatalf("Serve after Shutdown = %v, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not return within five seconds of Shutdown")
	}
}

func TestNewReportsABindThatFailed(t *testing.T) {
	t.Parallel()

	if _, err := server.New("127.0.0.1:not-a-port"); err == nil {
		t.Fatal("New with an unusable address returned no error")
	}
}
