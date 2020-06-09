package main

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/types"
	"log"
	"os"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
)

// "github.com/qiniu/goplus/gop"
// "github.com/qiniu/goplus/exec.spec"
// "github.com/qiniu/goplus/exec/bytecode"

const (
	qlang_lib = "github.com/qiniu/goplus/gop"
	qspec_lib = "github.com/qiniu/goplus/exec.spec"
	qexec_lib = "github.com/qiniu/goplus/exec/bytecode"
)

const (
	qspec_def = "spec"     // "github.com/qiniu/goplus/exec.spec"
	qexec_def = "bytecode" // "github.com/qiniu/goplus/exec/bytecode"
	qlang_def = "gop"      // "github.com/qiniu/goplus/gop"
)

var (
	qspec = "qspec"
	qexec = "qexec"
	qlang = "gop"
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

func (v *GoVar) ExportRegister() (string, error) {
	return fmt.Sprintf("I.Var(%q, &%v)", v.Name(), v.FullName()), nil
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
		info := "("
		if p.RecvIsPointer() {
			info += "*"
		}
		return info + p.recv.Obj().Name() + ")." + p.id.Name
	}
}

func (p *GoFunc) CallName() string {
	if p.recv == nil {
		return p.obj.Pkg().Name() + "." + p.id.Name
	} else {
		info := "("
		if p.RecvIsPointer() {
			info += "*"
		}
		return info + p.obj.Pkg().Name() + "." + p.recv.Obj().Name() + ")." + p.id.Name
	}
}

func (p *GoFunc) RecvIsPointer() bool {
	if p.recv == nil {
		return false
	}
	_, ok := p.Signature().Recv().Type().(*types.Pointer)
	return ok
}

func (p *GoFunc) Variadic() bool {
	return p.Signature().Variadic()
}

// func execName/execStructMethod
func (p *GoFunc) qExecName() string {
	if p.recv == nil {
		return "exec" + p.id.Name
	} else {
		return "execm" + p.recv.Obj().Name() + p.id.Name
	}
}

func (p *GoFunc) Signature() *types.Signature {
	return p.typ.Type().(*types.Signature)
}

func (v *GoFunc) ExportRegister() (string, error) {
	if v.Variadic() {
		return fmt.Sprintf("I.Funcv(%q, %v, %v)", v.qRegName(), v.CallName(), v.qExecName()), nil
	}
	return fmt.Sprintf("I.Func(%q, %v, %v)", v.qRegName(), v.CallName(), v.qExecName()), nil
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

func (v *GoFunc) exportDeclV() (string, error) {
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
	return decl, nil
}

func (v *GoFunc) ExportDecl() (string, error) {
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
		switch vt := iv.Type().(type) {
		case *types.Basic:
			basic = vt.String()
		case *types.Named:
			if !vt.Obj().Exported() {
				return "", fmt.Errorf("param type is internal %v", vt)
			}
		default:
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
	return decl, nil
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
	if strings.HasPrefix(kind, qspec+".") {
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
	cfg.Env = append(os.Environ(), "GOOS=js", "GOARCH=wasm")
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
		if obj == nil || !ident.IsExported() {
			continue
		}
		if obj.Parent() == p.Pkg.Types.Scope() {
			switch typ := obj.(type) {
			case *types.Const:
				p.Consts = append(p.Consts, &GoConst{GoObject{ident, obj}, typ})
			case *types.Var:
				p.Vars = append(p.Vars, &GoVar{GoObject{ident, obj}, typ})
			case *types.Func:
				p.Funcs = append(p.Funcs, &GoFunc{GoObject{ident, obj}, typ, nil})
			case *types.TypeName:
				p.Types = append(p.Types, &GoType{GoObject{ident, obj}, typ})
			case *types.Label:
				// skip
			case *types.PkgName:
			// skip
			default:
				log.Printf("warring, uncheck %v %T, %v \n", ident, typ, p.Pkg.Fset.Position(ident.Pos()))
			}
		} else {
			if typ, ok := obj.(*types.Func); ok {
				sig, ok := typ.Type().Underlying().(*types.Signature)
				if ok && sig.Recv() != nil {
					named := funcRecvType(ident, sig.Recv().Type())
					if named != nil && named.Obj().Exported() && named.Obj().Parent() == p.Pkg.Types.Scope() {
						switch nt := named.Underlying().(type) {
						case *types.Struct:
							p.Funcs = append(p.Funcs, &GoFunc{GoObject{ident, obj}, typ, named})
						case *types.Basic, *types.Slice, *types.Map, *types.Signature:
							p.Funcs = append(p.Funcs, &GoFunc{GoObject{ident, obj}, typ, named})
						case *types.Interface:
							// TODO skip interface
						default:
							log.Fatalf("uncheck types.Func %v %v %T\n", ident, obj, nt)
						}
					}
				}
			}
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
func (p *GoConst) toQlangKind(pkg string) (string, error) {
	baisc, ok := p.typ.Type().Underlying().(*types.Basic)
	if !ok {
		return "", fmt.Errorf("un basic of const %v %v", p.id, p.typ)
	}
	switch baisc.Kind() {
	case types.UntypedBool:
		return "reflect.Bool", nil
	case types.UntypedInt:
		return pkg + ".ConstUnboundInt", nil
	case types.UntypedRune:
		return pkg + ".ConstUnboundRune", nil
	case types.UntypedFloat:
		return pkg + ".ConstUnboundFloat", nil
	case types.UntypedComplex:
		return pkg + ".ConstUnboundComplex", nil
	case types.UntypedString:
		return pkg + ".ConstUnboundString", nil
	case types.UntypedNil:
		return pkg + ".ConstUnboundPtr", nil
	}
	// TODO
	return "reflect." + strings.Title(baisc.Name()), nil

	// switch p.typ.Val().Kind() {
	// case constant.Bool:
	// 	return "reflect.Bool", nil
	// case constant.String:
	// 	return pkg + ".ConstBoundString", nil
	// case constant.Int:
	// 	return pkg + ".ConstUnboundInt", nil
	// case constant.Float:
	// 	return pkg + ".ConstUnboundFloat", nil
	// case constant.Complex:
	// 	return pkg + ".ConstUnboundComplex", nil
	// default:
	// 	return "", fmt.Errorf("unknow kind of const %v %v", p.id, p.typ)
	// }
}

func (v *GoConst) Name() string {
	return v.id.Name
}
func (v *GoConst) ExportRegister() (string, error) {
	kind, err := v.toQlangKind(qspec)
	if err != nil {
		return "", err
	}
	if v.typ.Val().Kind() == constant.Int {
		ck := checkConstType(v.typ.Val().String())
		if ck == ConstInt64 {
			return fmt.Sprintf("I.Const(%q, %v, int64(%v))", v.id.Name, kind, v.FullName()), nil
		} else if ck == ConstUnit64 {
			return fmt.Sprintf("I.Const(%q, %v, uint64(%v))", v.id.Name, kind, v.FullName()), nil
		}
	}
	return fmt.Sprintf("I.Const(%q, %v, %v)", v.id.Name, kind, v.FullName()), nil
}

func typesBasicToQlang(pkg string, typ *types.Basic) string {
	switch typ.Kind() {
	case types.Bool:
		return pkg + ".TyBool"
	case types.Int:
		return pkg + ".TyInt"
	case types.Int8:
		return pkg + ".TyInt8"
	case types.Int16:
		return pkg + ".TyInt16"
	case types.Int32:
		return pkg + ".TyInt32"
	case types.Int64:
		return pkg + ".TyInt64"
	case types.Uint:
		return pkg + ".TyUint"
	case types.Uint8:
		return pkg + ".TyUint8"
	case types.Uint16:
		return pkg + ".TyUint16"
	case types.Uint32:
		return pkg + ".TyUint32"
	case types.Uint64:
		return pkg + ".TyUint64"
	case types.Uintptr:
		return pkg + ".TyUintptr"
	case types.Float32:
		return pkg + ".TyFloat32"
	case types.Float64:
		return pkg + ".TyFloat64"
	case types.Complex64:
		return pkg + ".TyComplex64"
	case types.Complex128:
		return pkg + ".TyComplex128"
	case types.String:
		return pkg + ".TyString"
	case types.UnsafePointer:
		return pkg + ".TyUnsafePointer"
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
		return typesBasicToQlang(qspec, typ)
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
	return "", fmt.Errorf("unparser type %v %T", p.id, p.typ.Type().Underlying())
}
