package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	flagDefaultContext           bool
	flagRenameNewTypeFunc        bool
	flagSkipErrorImplementStruct bool
	flagQlangLowerCaseStyle      bool
	flagCustomContext            string
	flagExportPath               string
	flagUpdatePath               string
	flagFilterList               string
)

const help = `Export go packages to qlang modules.

Usage:
  qexport [option] packages

The packages for go package list or std for golang all standard packages.
`

func usage() {
	fmt.Fprintln(os.Stderr, help)
	flag.PrintDefaults()
}

func init() {
	// flag.StringVar(&flagCustomContext, "contexts", "", "optional comma-separated list of <goos>-<goarch>[-cgo] to override default contexts.")
	// flag.BoolVar(&flagDefaultContext, "defctx", false, "optional use default context for build, default use all contexts.")
	//flag.BoolVar(&flagSkipErrorImplementStruct, "skiperrimpl", true, "optional skip error interface implement struct.")
	flag.StringVar(&flagExportPath, "outdir", "./qlang", "optional set export output root path")
	flag.StringVar(&flagFilterList, "filter", "", "optional set export filter regular expression list, separated by spaces.")
}

var (
	ac     *ApiCheck
	reList []*regexp.Regexp
)

func main() {
	flag.Parse()
	args := flag.Args()

	if len(args) == 0 {
		usage()
		return
	}

	if flagCustomContext != "" {
		flagDefaultContext = false
		setCustomContexts(flagCustomContext)
	}
	if flagFilterList != "" {
		for _, expr := range strings.Split(flagFilterList, " ") {
			re, err := regexp.Compile(expr)
			if err != nil {
				log.Fatalln("regexp error", err)
			}
			reList = append(reList, re)
		}
	}

	//load ApiCheck
	ac = NewApiCheck()
	err := ac.LoadBase("go1", "go1.1", "go1.2", "go1.3", "go1.4", "go1.5", "go1.6", "go1.7", "go1.8", "go1.9", "go1.10", "go1.12", "go1.13")
	if err != nil {
		log.Println(err)
	}
	err = ac.LoadApi("go1.14")
	if err != nil {
		log.Println(err)
	}

	var outpath string
	if filepath.IsAbs(flagExportPath) {
		outpath = flagExportPath
	} else {
		dir, err := os.Getwd()
		if err != nil {
			log.Fatalln(err)
		}
		outpath = filepath.Join(dir, flagExportPath)
	}

	var pkgs []string
	if args[0] == "std" {
		out, err := exec.Command("go", "list", "-e", args[0]).Output()
		if err != nil {
			log.Fatal(err)
		}
		pkgs = strings.Fields(string(out))
	} else {
		pkgs = args
	}
	var exportd []string
	for _, pkg := range pkgs {
		if isSkipPkg(pkg) {
			continue
		}
		err := export(pkg, outpath, true)
		if err != nil {
			log.Printf("warning skip pkg %q, error %v.\n", pkg, err)
		} else {
			exportd = append(exportd, pkg)
		}
	}
	for _, pkg := range exportd {
		log.Printf("export pkg %q success.\n", pkg)
	}
}

func filterSym(sym string) bool {
	if len(reList) == 0 {
		return true
	}
	for _, re := range reList {
		if re.MatchString(sym) {
			return true
		}
	}
	return false
}
