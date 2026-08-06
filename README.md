# kanzlei

A self-hosted assistant stack with single sign-on, audit logging, document-accurate access control and permission-aware retrieval over organisational sources. The sensitive data never leaves the operator's infrastructure.

Planning happens on the issue tracker first. Every decision that shapes
the architecture is written down there with its reasons before the code
that depends on it exists.

See [NOTICE.md](NOTICE.md) for the intended-use notice.

## Build

One command, from a clean clone:

    go build ./cmd/kanzlei

The toolchain version is pinned in `go.mod` and nothing else in the tree names
one. A clone with an older Go installed is told so by the toolchain instead of
producing a build that differs from this one for a reason nobody can see. There
are no dependencies yet, so the build needs no network and there is no lock file
beside `go.mod` for the toolchain to write.

## Run

    ./kanzlei

The process serves one endpoint, `/livez`, which answers `ok` while it is
running. It says nothing about whether the process is ready to do useful work;
that is a different question and it does not have an answer yet.

`-addr` chooses the address, `127.0.0.1:8080` by default. A port of `0` asks the
operating system to choose one, and the address actually bound is printed on
startup. `-version` prints the version, the commit the binary was built from and
the toolchain, then exits. An interrupt or a termination signal stops the
process, and requests already in flight are given ten seconds to finish.

Nothing here reads configuration, holds state or authenticates anybody. That is
the whole of it today, and every later piece of this project attaches to it.

## Test

    go test ./...
