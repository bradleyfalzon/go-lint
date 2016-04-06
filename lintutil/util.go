// Copyright (c) 2013 The Go Authors. All rights reserved.
//
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file or at
// https://developers.google.com/open-source/licenses/bsd.

// Package lintutil provides helpers for writing linter command lines.
package lintutil // import "honnef.co/go/lint/lintutil"

import (
	"flag"
	"fmt"
	"go/build"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"honnef.co/go/lint"

	"github.com/kisielk/gotool"
)

func usage(name string) func() {
	return func() {
		fmt.Fprintf(os.Stderr, "Usage of %s:\n", name)
		fmt.Fprintf(os.Stderr, "\t%s [flags] # runs on package in current directory\n", name)
		fmt.Fprintf(os.Stderr, "\t%s [flags] packages\n", name)
		fmt.Fprintf(os.Stderr, "\t%s [flags] directory\n", name)
		fmt.Fprintf(os.Stderr, "\t%s [flags] files... # must be a single package\n", name)
		fmt.Fprintf(os.Stderr, "Flags:\n")
		flag.PrintDefaults()
	}
}

type runner struct {
	funcs         []lint.Func
	minConfidence float64
}

func resolveRelative(importPaths []string) (goFiles bool, err error) {
	if len(importPaths) == 0 {
		return false, nil
	}
	if strings.HasSuffix(importPaths[0], ".go") {
		// User is specifying a package in terms of .go files, don't resolve
		return true, nil
	}
	wd, err := os.Getwd()
	if err != nil {
		return false, err
	}
	ctx := build.Default
	// ctx.BuildTags = c.Tags
	for i, path := range importPaths {
		bpkg, err := ctx.Import(path, wd, build.FindOnly)
		if err != nil {
			return false, fmt.Errorf("can't load package %q: %v", path, err)
		}
		importPaths[i] = bpkg.ImportPath
	}
	return false, nil
}

func ProcessArgs(name string, funcs []lint.Func, args []string) {
	flags := flag.FlagSet{
		Usage: usage(name),
	}
	var minConfidence = flags.Float64("min_confidence", 0.8, "minimum confidence of a problem to print it")
	flags.Parse(args)

	runner := runner{funcs, *minConfidence}
	paths := gotool.ImportPaths(flags.Args())
	goFiles, err := resolveRelative(paths)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
	if goFiles {
		runner.lintFiles(paths...)
	} else {
		for _, path := range paths {
			runner.lintPackage(path)
		}
	}
}

func isDir(filename string) bool {
	fi, err := os.Stat(filename)
	return err == nil && fi.IsDir()
}

func exists(filename string) bool {
	_, err := os.Stat(filename)
	return err == nil
}

func (runner runner) lintFiles(filenames ...string) {
	files := make(map[string][]byte)
	for _, filename := range filenames {
		src, err := ioutil.ReadFile(filename)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			continue
		}
		files[filename] = src
	}

	l := &lint.Linter{
		Funcs: runner.funcs,
	}
	ps, err := l.LintFiles(files)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return
	}
	for _, p := range ps {
		if p.Confidence >= runner.minConfidence {
			fmt.Printf("%v: %s\n", p.Position, p.Text)
		}
	}
}

func (runner runner) lintPackage(pkgname string) {
	pkg, err := build.Import(pkgname, ".", 0)
	runner.lintImportedPackage(pkg, err)
}

func (runner runner) lintImportedPackage(pkg *build.Package, err error) {
	if err != nil {
		if _, nogo := err.(*build.NoGoError); nogo {
			// Don't complain if the failure is due to no Go source files.
			return
		}
		fmt.Fprintln(os.Stderr, err)
		return
	}

	var files []string
	xtest := pkg.XTestGoFiles
	files = append(files, pkg.GoFiles...)
	files = append(files, pkg.CgoFiles...)
	files = append(files, pkg.TestGoFiles...)
	if pkg.Dir != "." {
		for i, f := range files {
			files[i] = filepath.Join(pkg.Dir, f)
		}
		for i, f := range xtest {
			xtest[i] = filepath.Join(pkg.Dir, f)
		}
	}
	runner.lintFiles(xtest...)
	runner.lintFiles(files...)
}
