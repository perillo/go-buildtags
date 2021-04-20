// Copyright 2021 Manlio Perillo. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The code for the parsename function has been adapted from the goodOSArchFile
// method from src/go/build/build.go in the Go source distribution.
// Copyright 2011 The Go Authors. All rights reserved.

package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"go/build/constraint"
	"go/parser"
	"go/token"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/perillo/go-buildtags/internal/invoke"
)

// gocmd is the go command to use.  It can be overridden using the GOCMD
// environment variable.
var gocmd = "go"

// List of past, present, and future known GOOS and GOARCH values.
// Taken from cmd/go/internal/imports/build.go in the Go distribution.
var (
	knownOS = map[string]bool{
		"aix":       true,
		"android":   true,
		"darwin":    true,
		"dragonfly": true,
		"freebsd":   true,
		"hurd":      true,
		"illumos":   true,
		"ios":       true,
		"js":        true,
		"linux":     true,
		"nacl":      true,
		"netbsd":    true,
		"openbsd":   true,
		"plan9":     true,
		"solaris":   true,
		"windows":   true,
		"zos":       true,
	}

	knownArch = map[string]bool{
		"386":         true,
		"amd64":       true,
		"amd64p32":    true,
		"arm":         true,
		"armbe":       true,
		"arm64":       true,
		"arm64be":     true,
		"mips":        true,
		"mipsle":      true,
		"mips64":      true,
		"mips64le":    true,
		"mips64p32":   true,
		"mips64p32le": true,
		"ppc":         true,
		"ppc64":       true,
		"ppc64le":     true,
		"riscv":       true,
		"riscv64":     true,
		"s390":        true,
		"s390x":       true,
		"sparc":       true,
		"sparc64":     true,
		"wasm":        true,
	}
)

// List of past, present and future known release tags.
var knownReleaseTag = map[string]bool{
	"go1": true,
}

// List of know special build tags.
var knownSpecialTag = map[string]bool{
	"cgo":   true,
	"gc":    true,
	"gccgo": true,

	// TODO(mperillo): Add msan and race to knownSpecialTag?
}

type tagset map[string]struct{}

func (set tagset) add(tag string) {
	set[tag] = struct{}{}
}

func (set tagset) sorted() []string {
	list := make([]string, 0, len(set))
	for tag := range set {
		list = append(list, tag)
	}
	sort.Strings(list)

	return list
}

func init() {
	if value, ok := os.LookupEnv("GOCMD"); ok {
		gocmd = value
	}

	// Add all possible release tags.
	for i := 1; i < 256; i++ {
		knownReleaseTag["go1."+strconv.Itoa(i)] = true
	}
}

func main() {
	// Setup log.
	log.SetFlags(0)

	// Parse command line.
	flag.Usage = func() {
		w := flag.CommandLine.Output()
		fmt.Fprintln(w, "Usage: go-buildtags [packages]")
		fmt.Fprintf(w, "Options:\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	args := flag.Args()

	directories, err := golist(args)
	if err != nil {
		log.Fatal(err)
	}

	if err := run(directories); err != nil {
		log.Fatal(err)
	}
}

// run categorizes and prints all the Go build tags in the specified package
// directories.
func run(directories []string) error {
	// Parse the tags.
	tags := make(tagset)
	for _, dir := range directories {
		gofiles, err := readdir(dir)
		if err != nil {
			return err
		}
		for _, name := range gofiles {
			if err := parse(tags, dir, name); err != nil {
				return err
			}
		}
	}

	// Categorize the tags.
	goos := make(tagset)
	goarch := make(tagset)
	release := make(tagset)
	special := make(tagset)
	build := make(tagset)
	for tag := range tags {
		switch {
		case knownOS[tag]:
			goos.add(tag)
		case knownArch[tag]:
			goarch.add(tag)
		case knownReleaseTag[tag]:
			release.add(tag)
		case knownSpecialTag[tag]:
			special.add(tag)
		default:
			build.add(tag)
		}
	}

	// Print the tags.
	fmt.Println("GOOS:", goos.sorted())
	fmt.Println("GOARCH:", goarch.sorted())
	fmt.Println("release-tag:", release.sorted())
	fmt.Println("special-tag:", special.sorted())
	fmt.Println("build-tag:", build.sorted())

	return nil
}

// readdir returns a list of all Go files in the specified package directory.
func readdir(dir string) ([]string, error) {
	list := make([]string, 0)
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, file := range files {
		name := file.Name()
		if file.Type() == 0 && filepath.Ext(name) == ".go" {
			list = append(list, name)
		}
	}

	return list, nil
}

// parsename returns the tags specified in the Go file name.
func parsename(name string) (tags [2]string) {
	// Strip the file extension.
	if dot := strings.Index(name, "."); dot != -1 {
		name = name[:dot]
	}

	// Skip normal files.
	i := strings.Index(name, "_")
	if i < 0 {
		return tags
	}

	l := strings.Split(name[i+1:], "_")
	if n := len(l); n > 0 && l[n-1] == "test" {
		l = l[:n-1]
	}
	n := len(l)

	if n >= 2 && knownOS[l[n-2]] && knownArch[l[n-1]] {
		return [2]string{l[n-1], l[n-2]}
	}
	if n >= 1 && (knownOS[l[n-1]] || knownArch[l[n-1]]) {
		return [2]string{l[n-1]}
	}

	return tags
}

// parseheader returns the named Go file header, from the start of the file
// until the start of the package statement.
func parseheader(path string) ([]byte, error) {
	// We use go/parser for convenience.
	const mode = parser.PackageClauseOnly | parser.ParseComments

	src, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("parseheader: %v", err)
	}

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, mode)
	if err != nil {
		return nil, fmt.Errorf("parseheader: %v", err)
	}

	return src[:f.Package-1], nil
}

// parse adds all the build tags in the named Go file to tags.
func parse(tags tagset, dir, name string) error {
	// Parse the build tags defined in the Go file name.
	autotags := parsename(name)
	if tag := autotags[0]; tag != "" {
		tags.add(tag)
	}
	if tag := autotags[1]; tag != "" {
		tags.add(tag)
	}

	// Parse the build tags in the Go file header.
	path := filepath.Join(dir, name)
	header, err := parseheader(path)
	if err != nil {
		return fmt.Errorf("parse %s: %v", path, err)
	}
	if err := parsetags(tags, header); err != nil {
		return fmt.Errorf("parse %s: %v", path, err)
	}

	return nil
}

// parsetags adds all the build tags in the Go file header to tags.
func parsetags(tags tagset, header []byte) error {
	// Try to parse each line of the file header.
	sc := bufio.NewScanner(bytes.NewReader(header))
	for sc.Scan() {
		line := sc.Text()
		if !isBuildLine(line) {
			continue
		}
		expr, err := constraint.Parse(line)
		if err != nil {
			return fmt.Errorf("parsetags: %v", err)
		}
		addtags(tags, expr)
	}
	if err := sc.Err(); err != nil {
		return fmt.Errorf("parsetags: internal error: %v", err)
	}

	return nil
}

// addtags adds all the build tags in expr to tags.
func addtags(tags tagset, expr constraint.Expr) {
	switch tag := expr.(type) {
	case *constraint.NotExpr:
		addtags(tags, tag.X)
	case *constraint.OrExpr:
		addtags(tags, tag.X)
		addtags(tags, tag.Y)
	case *constraint.TagExpr:
		tags.add(tag.Tag)
	}
}

func isBuildLine(line string) bool {
	if constraint.IsGoBuild(line) || constraint.IsPlusBuild(line) {
		return true
	}

	return false
}

// golist returns a list of directories containing package sources, for the
// packages named by the given patterns.
func golist(patterns []string) ([]string, error) {
	args := append([]string{"list", "-f", "{{.Dir}}"}, patterns...)
	cmd := exec.Command(gocmd, args...)
	stdout, err := invoke.Output(cmd)
	if err != nil {
		return nil, err
	}

	// Parse the list of package directories.
	list := make([]string, 0)
	sc := bufio.NewScanner(bytes.NewReader(stdout))
	for sc.Scan() {
		list = append(list, sc.Text())
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("golist: internal error: %v", err)
	}

	return list, nil
}
