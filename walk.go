package closewrite

import "go/ast"

// walkOwn visits the nodes that belong to ONE function's flow: everything in
// the body except the insides of nested function literals, which are their own
// functions and get their own visit. The single exception is a DEFERRED
// closure — `defer func() { … }()` runs as part of this function's exit, so
// its body is walked as this function's own statements. Without the cut, an
// open-and-defer written inside a closure was reported twice: once by the
// enclosing function's walk and once by the literal's own.
func walkOwn(node ast.Node, visit func(ast.Node)) {
	ast.Inspect(node, func(n ast.Node) bool {
		switch at := n.(type) {
		case *ast.DeferStmt:
			visit(at)
			walkDeferred(at, visit)
			return false
		case *ast.FuncLit:
			return false
		}
		visit(n)
		return true
	})
}

// walkDeferred walks a deferred statement's own flow: the deferred closure's
// body when the call is a literal, and the call expression itself otherwise.
func walkDeferred(deferred *ast.DeferStmt, visit func(ast.Node)) {
	if lit, ok := deferred.Call.Fun.(*ast.FuncLit); ok {
		walkOwn(lit.Body, visit)
		return
	}
	walkOwn(deferred.Call, visit)
}

// callsUnder collects every call expression beneath the given expressions,
// nested ones included — `errors.Join(err, f.Close())` carries the Close.
func callsUnder(exprs ...ast.Expr) []*ast.CallExpr {
	var calls []*ast.CallExpr
	for _, expr := range exprs {
		ast.Inspect(expr, func(n ast.Node) bool {
			if _, ok := n.(*ast.FuncLit); ok {
				return false
			}
			if call, ok := n.(*ast.CallExpr); ok {
				calls = append(calls, call)
			}
			return true
		})
	}
	return calls
}
