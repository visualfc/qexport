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
	typ  *types.Func
	recv *types.Named
}

func (p *GoFunc) Name() string {
	if p.recv == nil {
		return p.id.Name
	} else {
		return p.recv.Obj().Name() + "." + p.id.Name
	}
}

func (p *GoFunc) qRegName() string {
	if p.recv == nil {
		return p.id.Name
	} else {
		return "(*" + p.recv.Obj().Name() + ")." + p.id.Name
	}
}

func (p *GoFunc) CallName() string {
	if p.recv == nil {
		return p.obj.Pkg().Name() + "." + p.id.Name
	} else {
		return "(*" + p.obj.Pkg().Name() + "." + p.recv.Obj().Name() + ")." + p.id.Name
	}
}

func (p *GoFunc) Variadic() bool {
	return p.Signature().Variadic()
}

// func execName/execStructMethod
func (p *GoFunc) qExecName() string {
	if p.recv == nil {
		return "exec" + p.id.Name
	} else {
		return "exec" + p.recv.Obj().Name() + p.id.Name
	}
}

func (p *GoFunc) Signature() *types.Signature {
	return p.typ.Type().Underlying().(*types.Signature)
}

func (v *GoFunc) ExportRegister() string {
	if v.Variadic() {
		return fmt.Sprintf("I.Funcv(%q, %v, %v)", v.qRegName(), v.CallName(), v.qExecName())
	}
	return fmt.Sprintf("I.Func(%q, %v, %v)", v.qRegName(), v.CallName(), v.qExecName())
}

/*
func execReplacerReplace(zero int, p *qlang.Context) {
	args := p.GetArgs(2)
	ret := args[0].(*strings.Replacer).Replace(args[1].(string))
	p.Ret(2, ret)
}
func execNewReplacer(arity int, p *qlang.Context) {
	args := p.GetArgs(arity)
	repl := strings.NewReplacer(qlang.ToStrings(args)...)
	p.Ret(arity, repl)
}
func QexecPrintf(arity int, p *qlang.Context) {
	args := p.GetArgs(arity)
	n, err := fmt.Printf(args[0].(string), args[1:]...)
	p.Ret(arity, n, err)
}

// ToStrings converts []interface{} into []string.
func ToStrings(args []interface{}) []string {
	ret := make([]string, len(args))
	for i, arg := range args {
		ret[i] = arg.(string)
	}
	return ret
}
*/

func (v *GoFunc) exportDeclV() string {
	var decl string
	argLen := v.Signature().Params().Len()
	retLen := v.Signature().Results().Len()
	var paramList []string
	var retList []string
	var convfn string = `	conv := func(args []interface{}) []T {
		ret := make([]T, len(args))
		for i, arg := range args {
			ret[i] = arg.(T)
		}
		return ret
	}
`

	var argBase int
	if v.recv != nil {
		argLen++ // arg[0] is recv
		argBase = 1
	}

	decl += fmt.Sprintf("// %v\n", simpleObjInfo(v.obj))
	decl += fmt.Sprintf("func %v(arity int, p *%v.Context) {\n", v.qExecName(), qlang)
	decl += fmt.Sprint("\targs := p.GetArgs(arity)\n")
	if retLen >= 1 {
		retList = append(retList, "ret")
		for i := 1; i < v.Signature().Results().Len(); i++ {
			retList = append(retList, fmt.Sprintf("ret%v", i))
		}
	}
	paramLen := v.Signature().Params().Len()
	for i := 0; i < paramLen; i++ {
		iv := v.Signature().Params().At(i)
		it := simpleType(iv.Type().String())
		if i == paramLen-1 {
			vt := iv.Type().(*types.Slice).Elem()
			// switch vt.Underlying().(type) {
			// case *types.Interface:
			// 	paramList = append(paramList, fmt.Sprintf("args[%v:]...", argBase+i))
			// 	convfn = ""
			// default:
			// 	convfn = strings.ReplaceAll(convfn, "T", simpleType(vt.String()))
			// 	paramList = append(paramList, fmt.Sprintf("conv(args[%v:])...", argBase+i))
			// }
			ct := simpleType(vt.String())
			if ct == "interface{}" {
				convfn = ""
				paramList = append(paramList, fmt.Sprintf("args[%v:]...", argBase+i))
			} else {
				convfn = strings.ReplaceAll(convfn, "T", simpleType(vt.String()))
				paramList = append(paramList, fmt.Sprintf("conv(args[%v:])...", argBase+i))
			}
		} else {
			var basic string
			switch vt := iv.Type().Underlying().(type) {
			case *types.Basic:
				basic = vt.String()
			}
			if basic != "" && basic != it {
				paramList = append(paramList, fmt.Sprintf("%v(args[%v].(%v))", it, argBase+i, basic))
			} else {
				paramList = append(paramList, fmt.Sprintf("args[%v].(%v)", argBase+i, it))
			}
		}
	}
	// add conv func
	decl += convfn
	// add call func
	decl += "\t"
	if retLen > 0 {
		decl += strings.Join(retList, ",") + " := "
	}
	if v.recv != nil {
		decl += "args[0]."
	}
	decl += v.CallName() + "(" + strings.Join(paramList, ", ") + ")\n"
	if retLen > 0 {
		decl += fmt.Sprintf("\tp.Ret(arity, %v)\n", strings.Join(retList, ","))
	}
	decl += "}"
	return decl
}

func (v *GoFunc) ExportDecl() string {
	if v.Variadic() {
		return v.exportDeclV()
	}
	var decl string
	argLen := v.Signature().Params().Len()
	retLen := v.Signature().Results().Len()
	var paramList []string
	var retList []string

	var argBase int
	if v.recv != nil {
		argLen++ // arg[0] is recv
		argBase = 1
	}

	decl += fmt.Sprintf("// %v\n", simpleObjInfo(v.obj))
	decl += fmt.Sprintf("func %v(zero int, p *%v.Context) {\n", v.qExecName(), qlang)
	if argLen != 0 {
		decl += fmt.Sprintf("\targs := p.GetArgs(%v)\n", argLen)
	}
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
			paramList = append(paramList, fmt.Sprintf("%v(args[%v].(%v))", it, argBase+i, basic))
		} else {
			paramList = append(paramList, fmt.Sprintf("args[%v].(%v)", argBase+i, it))
		}
	}
	decl += "\t"
	if retLen > 0 {
		decl += strings.Join(retList, ",") + " := "
	}
	if v.recv != nil {
		decl += "args[0]."
	}
	decl += v.CallName() + "(" + strings.Join(paramList, ", ") + ")\n"
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

func (v *GoType) ExportRegister() (string, error) {
	kind, err := v.toQlangKind()
	if err != nil {
		return "", err
	}
	var item string
	if strings.HasPrefix(kind, qexec+".") {
		item = fmt.Sprintf("I.Type(%q, %v)", v.id.Name, kind)
	} else {
		item = fmt.Sprintf("I.Rtype(%v)", kind)
	}
	return item, nil
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

func funcRecvType(ident *ast.Ident, typ types.Type) *types.Named {
	switch t := typ.(type) {
	case *types.Pointer:
		return funcRecvType(ident, t.Elem())
	case *types.Named:
		return t
	case *types.Interface:
		return nil
	default:
		log.Fatalf("uncheck funcRecvType %v %v\n", ident, t)
	}
	return nil
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
			if obj.Parent() == p.Pkg.Types.Scope() {
				p.Consts = append(p.Consts, &GoConst{GoObject{ident, obj}, t})
			}
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
					p.Funcs = append(p.Funcs, &GoFunc{GoObject{ident, obj}, t, nil})
				} else {
					named := funcRecvType(ident, sig.Recv().Type())
					if named != nil && named.Obj().Exported() {
						switch nt := named.Underlying().(type) {
						case *types.Struct:
							p.Funcs = append(p.Funcs, &GoFunc{GoObject{ident, obj}, t, named})
						case *types.Basic, *types.Slice, *types.Map, *types.Signature:
							p.Funcs = append(p.Funcs, &GoFunc{GoObject{ident, obj}, t, named})
						case *types.Interface:
							// TODO skip interface
						default:
							log.Fatalf("uncheck types.Signature recv %v %v %T\n", p.Pkg.Fset.Position(ident.Pos()), t, nt)
						}
					}
				}
			default:
				log.Printf("warring, unexport types.Func %v %T\n", ident, t.Type().Underlying())
			}
		case *types.TypeName:
			//p.checkTypeName(ident, obj, t.Type().Underlying())
			if obj.Parent() == p.Pkg.Types.Scope() {
				p.Types = append(p.Types, &GoType{GoObject{ident, obj}, t})
			}
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
	return fmt.Sprintf("I.Const(%q, %v, %v)", v.id.Name, kind, v.FullName())
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
		// TODO
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
