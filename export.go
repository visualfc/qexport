package main

import (
	"bytes"
	"fmt"
	"go/format"
	"log"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/tools/imports"
)

func formatCode(src []byte) ([]byte, error) {
	return format.Source(src)
}

func goimports(src []byte) ([]byte, error) {
	return imports.Process("", src, nil)
}

func export(pkg string, outpath string, skipOSArch bool) error {
	p, err := LoadGoPkg(pkg)
	if err != nil {
		return err
	}
	log.Println(p.Pkg.ID)
	p.LoadAll(true)
	p.Sort()

	// export const
	var consts []string
	consts = append(consts, "I.RegisterConsts(")
	for _, v := range p.Consts {
		if !filterSym(v.Name()) {
			continue
		}
		info, err := v.ExportRegister()
		if err != nil {
			log.Printf("warning, skip const %v, %v\n", v.id, err)
			continue
		}
		consts = append(consts, "\t"+info+",")
	}
	consts = append(consts, ")")

	// export var
	var vars []string
	vars = append(vars, "I.RegisterVars(")
	for _, v := range p.Vars {
		if !filterSym(v.Name()) {
			continue
		}
		info, err := v.ExportRegister()
		if err != nil {
			log.Printf("warning, skip var %v, %v\n", v.id, err)
			continue
		}
		vars = append(vars, "\t"+info+",")
	}
	vars = append(vars, ")")

	// export type
	var types []string
	types = append(types, "I.RegisterTypes(")
	for _, v := range p.Types {
		if !filterSym(v.Name()) {
			continue
		}
		info, err := v.ExportRegister()
		if err != nil {
			log.Printf("warning, skip type %v, %v\n", v.id, err)
			continue
		}
		types = append(types, "\t"+info+",")
	}
	types = append(types, ")")

	// export func
	var funcreg []string
	var funcvreg []string
	var funcdec []string
	funcreg = append(funcreg, "I.RegisterFuncs(")
	funcvreg = append(funcvreg, "I.RegisterFuncvs(")
	for _, v := range p.Funcs {
		if !filterSym(v.Name()) {
			continue
		}
		decl, err := v.ExportDecl()
		if err != nil {
			log.Printf("warning, skip func %v, %v\n", v.id, err)
			continue
		}
		funcdec = append(funcdec, decl)
		info, _ := v.ExportRegister()
		if v.Variadic() {
			funcvreg = append(funcvreg, "\t"+info+",")
		} else {
			funcreg = append(funcreg, "\t"+info+",")
		}
	}
	funcreg = append(funcreg, ")")
	funcvreg = append(funcvreg, ")")

	var heads []string

	heads = append(heads, fmt.Sprintf("package %v\n", p.Pkg.Types.Name()))
	heads = append(heads, "import (")
	heads = append(heads, fmt.Sprintf("\t%q", p.Pkg.Types.Path()))
	heads = append(heads, "\t\"reflect\"")
	if qexec == "exec" {
		heads = append(heads, "\t\"github.com/qiniu/qlang/v6/exec\"")
	} else {
		heads = append(heads, "\t"+qexec+"\"github.com/qiniu/qlang/v6/exec\"")
	}
	if qlang == "spec" {
		heads = append(heads, "\t\"github.com/qiniu/qlang/v6/spec\"")
	} else {
		heads = append(heads, "\t"+qlang+"\"github.com/qiniu/qlang/v6/spec\"")
	}
	heads = append(heads, ")")

	var buf bytes.Buffer
	buf.WriteString(strings.Join(heads, "\n"))
	buf.WriteString("\n\n")
	buf.WriteString(strings.Join(funcdec, "\n"))
	buf.WriteString("\n\n")
	buf.WriteString("// I is a Go package instance.\n")
	buf.WriteString(fmt.Sprintf("var I = %v.NewGoPackage(%q)", qlang, p.Pkg.Types.Path()))
	buf.WriteString("\n\n")
	buf.WriteString("func init(){\n")
	if len(consts) > 2 {
		buf.WriteString(strings.Join(consts, "\n"))
		buf.WriteString("\n")
	}
	if len(vars) > 2 {
		buf.WriteString(strings.Join(vars, "\n"))
		buf.WriteString("\n")
	}
	if len(types) > 2 {
		buf.WriteString(strings.Join(types, "\n"))
		buf.WriteString("\n")
	}
	if len(funcreg) > 2 {
		buf.WriteString(strings.Join(funcreg, "\n"))
		buf.WriteString("\n")
	}
	if len(funcvreg) > 2 {
		buf.WriteString(strings.Join(funcvreg, "\n"))
	}
	buf.WriteString("}")

	// format
	data, err := goimports(buf.Bytes())
	if err != nil {
		fmt.Println(buf.String())
		return err
	}
	//fmt.Println(string(data))

	// write root dir
	root := filepath.Join(outpath, pkg)
	os.MkdirAll(root, 0777)

	file, err := os.Create(filepath.Join(root, "exports.go"))
	if err != nil {
		return err
	}
	file.Write(data)
	file.Close()

	return nil
}
