# Go Build Tags Enumerator

[![Go Reference](https://pkg.go.dev/badge/github.com/perillo/go-buildtags.svg)](https://pkg.go.dev/github.com/perillo/go-buildtags)

## Installation

go-buildtags requires [Go 1.16](https://golang.org/doc/devel/release.html#go1.16).

    go install github.com/perillo/go-buildtags@latest

## Purpose

go-buildtags parses, categorizes and shows all build tags specified in a
package.

Build tags are categorizes as:
  - GOOS
  - GOARCH
  - release-tag
  - special-tag
  - build-tag

Note that a tag count represents how many times a tag has been specified in a
`+build` line, a `go:build` line or in a file name.

## Usage

    go-buildtags [packages]

Invoke `go-buildtags` with one or more import paths.  go-buildtags uses the
same [import path syntax](https://golang.org/cmd/go/#hdr-Import_path_syntax) as
the `go` command and therefore also supports relative import paths like
`./...`. Additionally the `...` wildcard can be used as suffix on relative and
absolute file paths to recurse into them.

By default, `go-buildtags` uses the `go` command installed on the system, but
it is possible to specify a different version using the `GOCMD` environment
variable.
