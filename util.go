package main

import (
	"fmt"
	"go/ast"
	"go/types"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
)

func isSkipPkg(pkg string) bool {
	for _, path := range strings.Split(pkg, "/") {
		if path == "internal" {
			return true
		} else if path == "vendor" {
			return true
		}
	}
	return false
}

func checkConstType(value string) KeyType {
	_, err := strconv.ParseInt(value, 10, 32)
	if err != nil {
		if value[0] == '-' {
			return ConstInt64
		} else {
			return ConstUnit64
		}
	}
	return Normal
}

func checkStructHasUnexportField(decl *ast.GenDecl) bool {
	if len(decl.Specs) > 0 {
		if ts, ok := decl.Specs[0].(*ast.TypeSpec); ok {
			if st, ok := ts.Type.(*ast.StructType); ok {
				if st.Fields != nil {
					for _, f := range st.Fields.List {
						for _, n := range f.Names {
							if !ast.IsExported(n.Name) {
								return true
							}
						}
					}
				}
			}
		}
	}
	return false
}

func simpleObjInfo(obj types.Object) string {
	s := obj.String()
	pkg := obj.Pkg()
	if pkg != nil {
		s = strings.Replace(s, pkg.Path(), pkg.Name(), -1)
		s = simpleType(s)
		if pkg.Name() == "main" {
			s = strings.Replace(s, "main.", "", -1)
		}
	}
	return s
}

func simpleType(src string) string {
	re, _ := regexp.Compile("[\\w\\./]+")
	return re.ReplaceAllStringFunc(src, func(s string) string {
		r := s
		if i := strings.LastIndex(s, "/"); i != -1 {
			r = s[i+1:]
		}
		if strings.Count(r, ".") > 1 {
			r = r[strings.Index(r, ".")+1:]
		}
		return r
	})
}

func CopyFile(source string, dest string) (err error) {
	sourcefile, err := os.Open(source)
	if err != nil {
		return err
	}
	defer sourcefile.Close()
	destfile, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer destfile.Close()
	_, err = io.Copy(destfile, sourcefile)
	if err == nil {
		sourceinfo, err := os.Stat(source)
		if err != nil {
			err = os.Chmod(dest, sourceinfo.Mode())
		}
	}
	return
}

func CopyDir(source string, dest string, subDir bool) (err error) {
	// get properties of source dir
	sourceinfo, err := os.Stat(source)
	if err != nil {
		return err
	}
	// create dest dir
	err = os.MkdirAll(dest, sourceinfo.Mode())
	if err != nil {
		return err
	}
	directory, _ := os.Open(source)
	objects, err := directory.Readdir(-1)
	for _, obj := range objects {
		sourcefilepointer := source + "/" + obj.Name()
		destinationfilepointer := dest + "/" + obj.Name()
		if obj.IsDir() {
			if subDir {
				// create sub-directories - recursively
				err = CopyDir(sourcefilepointer, destinationfilepointer, subDir)
				if err != nil {
					fmt.Println(err)
				}
			}
		} else {
			// perform copy
			err = CopyFile(sourcefilepointer, destinationfilepointer)
			if err != nil {
				fmt.Println(err)
			}
		}
	}
	return
}
