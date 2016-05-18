// qexport project main.go
package main

import (
	"bufio"
	"bytes"
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

var (
	flagExportPath string
)

func init() {
	flag.StringVar(&flagExportPath, "outpath", "qlang", "output path")
}

func main() {
	flag.Parse()

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

	args := flag.Args()
	var pkgs []string
	if len(args) > 0 {
		if args[0] == "std" {
			out, err := exec.Command("go", "list", "-e", args[0]).Output()
			if err != nil {
				log.Fatal(err)
			}
			pkgs = strings.Fields(string(out))
		} else {
			pkgs = args
		}
	}

	for _, pkg := range pkgs {
		err := export(pkg, outpath, true)
		if err != nil {
			log.Printf("export pkg %q error, %s.\n", pkg, err)
		} else {
			log.Printf("export pkg %q success.\n", pkg)
		}
	}
}

var sym = regexp.MustCompile(`^pkg (\S+)\s?(.*)?, (?:(var|func|type|const)) ([A-Z]\w*)`)

func export(pkg string, outpath string, skip_osarch bool) error {
	out, err := exec.Command("go", "tool", "api", pkg).Output()
	if err != nil {
		return err
	}
	sc := bufio.NewScanner(bytes.NewBuffer(out))
	fullImport := map[string]string{} // "zip.NewReader" => "archive/zip"
	ambiguous := map[string]bool{}
	var keys []string
	var funcs []string
	var cons []string
	var vars []string
	var structs []string
	for sc.Scan() {
		l := sc.Text()
		has := func(v string) bool { return strings.Contains(l, v) }
		if has("interface, ") || has(", method (") {
			continue
		}
		if m := sym.FindStringSubmatch(l); m != nil {
			// 1 pkgname
			// 2 os-arch-cgo
			// 3 var|func|type|const
			// 4 name
			if skip_osarch && m[2] != "" {
				//	log.Println("skip", m[2], m[4])
				continue
			}
			full := m[1]
			key := path.Base(full) + "." + m[4]
			if exist, ok := fullImport[key]; ok {
				if exist != full {
					ambiguous[key] = true
				}
			} else {
				fullImport[key] = full
				keys = append(keys, key)
				if m[3] == "func" {
					funcs = append(funcs, m[4])
				} else if m[3] == "const" {
					cons = append(cons, m[4])
				} else if m[3] == "var" {
					vars = append(vars, m[4])
				} else if m[3] == "type" && strings.HasSuffix(l, m[4]+" struct") {
					structs = append(structs, m[4])
				}
			}
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	sort.Strings(keys)

	if len(cons) == 0 && len(funcs) == 0 {
		return errors.New("empty funcs and const")
	}

	root := filepath.Join(outpath, pkg)
	err = os.MkdirAll(root, 0777)
	if err != nil {
		return err
	}

	pkgName := path.Base(pkg)

	var buf bytes.Buffer
	outf := func(format string, a ...interface{}) (err error) {
		_, err = buf.WriteString(fmt.Sprintf(format, a...))
		return
	}

	//write package
	outf("package %s\n", pkgName)

	//write imports
	outf("import (\n")
	outf("\t%q\n", pkg)
	outf(")\n\n")

	//check new func map
	nmap := make(map[string][]string)
	skip := make(map[string]bool)
	for _, s := range structs {
		fnNew := "New" + s
		find := false
		for _, v := range funcs {
			if strings.HasPrefix(v, fnNew) {
				nmap[s] = append(nmap[s], v)
				skip[v] = true
				find = true
			}
		}
		if !find {
			outf("func new%s() *%s{\n", s, pkgName+"."+s)
			outf("return new(%s)\n", pkgName+"."+s)
			outf("}\n\n")
			nmap[s] = append(nmap[s], "new"+s)
		}
	}
	//write exports
	outf("var Exports = map[string]interface{}{\n")
	//write new func
	for s, fns := range nmap {
		if len(fns) == 1 {
			qname := toQlangName(s)
			fnNew := fns[0]
			if ast.IsExported(fnNew) {
				fnNew = pkgName + "." + fnNew
			}
			outf("\t%q:\t%s,\n", qname, fnNew)
		} else if len(fns) > 1 {
			for _, fn := range fns {
				qname := toQlangName(fn)
				fnNew := fn
				if ast.IsExported(fnNew) {
					fnNew = pkgName + "." + fnNew
				}
				outf("\t%q:\t%s,\n", qname, fnNew)
			}
		}
	}
	if len(nmap) != 0 {
		outf("\n")
	}

	//var
	for _, v := range vars {
		name := toQlangName(v)
		fn := pkgName + "." + v
		outf("\t%q:\t%s,\n", name, fn)
	}
	if len(vars) != 0 {
		outf("\n")
	}

	//const
	for _, v := range cons {
		name := toQlangName(v)
		fn := pkgName + "." + v
		outf("\t%q:\t%s,\n", name, fn)
	}

	if len(cons) != 0 {
		outf("\n")
	}

	//funcs
	for _, v := range funcs {
		name := toQlangName(v)
		fn := pkgName + "." + v
		if skip[v] {
			continue
		}
		outf("\t%q:\t%s,\n", name, fn)
	}

	outf("}")

	data, err := format.Source(buf.Bytes())
	if err != nil {
		return err
	}

	file, err := os.Create(filepath.Join(root, pkgName+".go"))
	if err != nil {
		return err
	}
	defer file.Close()
	file.Write(data)

	return nil
}

func toQlangName(s string) string {
	if len(s) <= 1 {
		return s
	}

	if unicode.IsLower(rune(s[1])) {
		return strings.ToLower(s[0:1]) + s[1:]
	}
	return s
}
