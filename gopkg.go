package main

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/types"
	"log"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
)

//qexec "github.com/qiniu/qlang/v6/exec"
//qlang "github.com/qiniu/qlang/v6/spec"
var (
	qexec = "qexec" // "exec"
	qlang = "qlang" // "spec"
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

type GoObject struct {
	id  *ast.Ident
	obj types.Object
}

func (v *GoObject) Name() string {
	return v.id.Name
}

func (v *GoObject) FullName() string {
	return v.obj.Pkg().Name() + "." + v.id.Name
}

type GoConst struct {
	GoObject
	typ *types.Const
}

type GoVar struct {
	GoObject
	typ *types.Var
}

func (v *GoVar) ExportRegister() string {
	return fmt.Sprintf("I.Var(%q, &%v)", v.Name(), v.FullName())
}

type GoFunc struct {
	GoObject
	typ *types.Func
}

func (p *GoFunc) Variadic() bool {
	return p.Signature().Variadic()
}

// func execName/execStructMethod
func (p *GoFunc) qExecName() string {
	return "exec" + p.id.Name
}

func (p *GoFunc) Signature() *types.Signature {
	return p.typ.Type().Underlying().(*types.Signature)
}

func (v *GoFunc) ExportRegister() string {
	return fmt.Sprintf("I.Func(%q, %v, %v)", v.Name(), v.FullName(), v.qExecName())
}

func (v *GoFunc) ExportDecl() string {
	var decl string
	argLen := v.Signature().Params().Len()
	retLen := v.Signature().Results().Len()
	var paramList []string
	var retList []string

	decl += fmt.Sprintf("// %v\n", simpleObjInfo(v.obj))
	decl += fmt.Sprintf("func %v(zero int, p *%v.Context) {\n", v.qExecName(), qlang)
	decl += fmt.Sprintf("\targs := p.GetArgs(%v)\n", argLen)
	if retLen >= 1 {
		retList = append(retList, "ret")
		for i := 1; i < v.Signature().Results().Len(); i++ {
			retList = append(retList, fmt.Sprintf("ret%v", i))
		}
	}
	for i := 0; i < v.Signature().Params().Len(); i++ {
		iv := v.Signature().Params().At(i)
		it := simpleType(iv.Type().String())
		var basic string
		switch vt := iv.Type().Underlying().(type) {
		case *types.Basic:
			basic = vt.String()
		}
		if basic != "" && basic != it {
			paramList = append(paramList, fmt.Sprintf("%v(args[%v].(%v))", it, i, basic))
		} else {
			paramList = append(paramList, fmt.Sprintf("args[%v].(%v)", i, it))
		}
	}
	decl += "\t"
	if retLen > 0 {
		decl += strings.Join(retList, ",") + " := "
	}
	decl += v.FullName() + "(" + strings.Join(paramList, ", ") + ")\n"
	if retLen > 0 {
		decl += fmt.Sprintf("\tp.Ret(%v, %v)\n", argLen, strings.Join(retList, ","))
	}
	decl += "}"
	return decl
}

type GoType struct {
	GoObject
	typ *types.TypeName
}

func (v *GoType) ExportRegister() string {
	kind, err := v.toQlangKind()
	if err != nil {
		log.Fatalln(err)
	}
	var item string
	if strings.HasPrefix(kind, qexec+".") {
		item = fmt.Sprintf("I.Type(%q, %v)", v.id.Name, kind)
	} else {
		item = fmt.Sprintf("I.Rtype(%v)", kind)
	}
	return item
}

type GoPkg struct {
	Pkg    *packages.Package
	Consts []*GoConst
	Vars   []*GoVar
	Funcs  []*GoFunc
	Types  []*GoType
}

func LoadGoPkg(pkg string) (*GoPkg, error) {
	cfg := &packages.Config{Mode: packages.NeedFiles |
		packages.NeedSyntax |
		packages.NeedTypesInfo |
		packages.NeedTypes}
	pkgs, err := packages.Load(cfg, pkg)
	if err != nil {
		return nil, err
	}
	if len(pkgs) < 1 {
		return nil, fmt.Errorf("error load pkg %v", pkg)
	}
	return &GoPkg{Pkg: pkgs[0]}, nil
}

func (p *GoPkg) checkTypeName(ident *ast.Ident, obj types.Object, underlying types.Type) {
	switch typ := underlying.(type) {
	case *types.Struct:
	case *types.Interface:
	case *types.Basic:
	case *types.Signature:
	case *types.Slice:
	case *types.Array:
	case *types.Map:
	case *types.Chan:
	case *types.Pointer:
		p.checkTypeName(ident, obj, typ.Elem())
	default:
		log.Printf("warring, unexport types.TypeName %v %T\n", ident, typ)
	}
}

func (p *GoPkg) checkSignature(ident *ast.Ident, v *types.Var, typ types.Type) {
	switch typ := typ.(type) {
	case *types.Basic:
	case *types.Named:
	case *types.Interface:
	case *types.Pointer:
		p.checkSignature(ident, v, typ.Elem())
	default:
		log.Printf("warring, checkSignature %v %T\n", ident, typ)
	}
}

func (p *GoPkg) Sort() {
	sort.Slice(p.Consts, func(i, j int) bool {
		return p.Consts[i].Name() < p.Consts[j].Name()
	})
	sort.Slice(p.Vars, func(i, j int) bool {
		return p.Vars[i].Name() < p.Vars[j].Name()
	})
	sort.Slice(p.Types, func(i, j int) bool {
		return p.Types[i].Name() < p.Types[j].Name()
	})
	sort.Slice(p.Funcs, func(i, j int) bool {
		return p.Funcs[i].Name() < p.Funcs[j].Name()
	})
}

func (p *GoPkg) LoadAll(exported bool) error {
	for ident, obj := range p.Pkg.TypesInfo.Defs {
		if exported && !ident.IsExported() {
			continue
		}
		if obj == nil {
			continue
		}
		switch t := obj.(type) {
		case *types.Const:
			p.Consts = append(p.Consts, &GoConst{GoObject{ident, obj}, t})
		case *types.Var:
			// t.IsField has bug: go/build -> Sfiles
			// if t.IsField() {
			// 	continue
			// }
			if obj.Parent() == p.Pkg.Types.Scope() {
				p.Vars = append(p.Vars, &GoVar{GoObject{ident, obj}, t})
			}
		case *types.Func:
			switch sig := t.Type().Underlying().(type) {
			case *types.Signature:
				if sig.Recv() == nil {
					p.Funcs = append(p.Funcs, &GoFunc{GoObject{ident, obj}, t})
				}
			default:
				log.Printf("warring, unexport types.Func %v %T\n", ident, t.Type().Underlying())
			}
		case *types.TypeName:
			//p.checkTypeName(ident, obj, t.Type().Underlying())
			p.Types = append(p.Types, &GoType{GoObject{ident, obj}, t})
		case *types.Label:
			// skip
		case *types.PkgName:
			// skip
			// import mypkg "pkg"
		default:
			log.Printf("warring, unexport %v %T\n", ident, t)
		}
	}
	return nil
}

/*
	ConstBoundRune = spec.ConstBoundRune
	// ConstBoundString - bound type: string
	ConstBoundString = spec.ConstBoundString
	// ConstUnboundInt - unbound int type
	ConstUnboundInt = spec.ConstUnboundInt
	// ConstUnboundFloat - unbound float type
	ConstUnboundFloat = spec.ConstUnboundFloat
	// ConstUnboundComplex - unbound complex type
	ConstUnboundComplex = spec.ConstUnboundComplex
*/
func (p *GoConst) toQlangKind() (string, error) {
	switch p.typ.Val().Kind() {
	case constant.Bool:
		return "reflect.Bool", nil
	case constant.String:
		return qexec + ".ConstBoundString", nil
	case constant.Int:
		return qexec + ".ConstUnboundInt", nil
	case constant.Float:
		return qexec + ".ConstUnboundFloat", nil
	case constant.Complex:
		return qexec + ".ConstUnboundComplex", nil
	default:
		return "", fmt.Errorf("unknow kind of const %v %v", p.id, p.typ)
	}
}

func (v *GoConst) Name() string {
	return v.id.Name
}

func (v *GoConst) ExportRegister() string {
	kind, err := v.toQlangKind()
	if err != nil {
		log.Fatalln(err)
	}
	return fmt.Sprintf("I.Const(%q, %v, %v)", v.id.Name, kind, v.typ.Val())
}

func typesBasicToQlang(typ *types.Basic) string {
	switch typ.Kind() {
	case types.Bool:
		return qexec + ".TyBool"
	case types.Int:
		return qexec + ".TyInt"
	case types.Int8:
		return qexec + ".TyInt8"
	case types.Int16:
		return qexec + ".TyInt16"
	case types.Int32:
		return qexec + ".TyInt32"
	case types.Int64:
		return qexec + ".TyInt64"
	case types.Uint:
		return qexec + ".TyUint"
	case types.Uint8:
		return qexec + ".TyUint8"
	case types.Uint16:
		return qexec + ".TyUint16"
	case types.Uint32:
		return qexec + ".TyUint32"
	case types.Uint64:
		return qexec + ".TyUint64"
	case types.Uintptr:
		return qexec + ".TyUintptr"
	case types.Float32:
		return qexec + ".TyFloat32"
	case types.Float64:
		return qexec + ".TyFloat64"
	case types.Complex64:
		return qexec + ".TyComplex64"
	case types.Complex128:
		return qexec + ".TyComplex128"
	case types.String:
		return qexec + ".TyString"
	case types.UnsafePointer:
		return qexec + ".TyUnsafePointer"
	default:
		log.Printf("uncheck types.Basic kind %v\n", typ)
	}
	return ""
}

func (p *GoType) typeNameToQlangKind() string {
	switch typ := p.typ.Type().Underlying().(type) {
	case *types.Struct:
		return fmt.Sprintf("reflect.TypeOf((*%v)(nil))", p.FullName())
	case *types.Interface:
	case *types.Basic:
		return typesBasicToQlang(typ)
	case *types.Signature:
	case *types.Slice:
	case *types.Array:
	case *types.Map:
	case *types.Chan:
	case *types.Pointer:
	default:
		log.Printf("unparse GoTypes typ %v %T\n", typ, typ)
	}
	return ""
}

func (p *GoType) toQlangKind() (string, error) {
	kind := p.typeNameToQlangKind()
	if kind != "" {
		return kind, nil
	}
	return "", fmt.Errorf("unparser GoTypes %v %v", p.id, p.typ)
}
