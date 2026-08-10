package closewrite

import "go/ast"

// deferredness says whether a visited node lives inside a deferred closure's
// body, where a `return` reaches no caller.
type deferredness bool

// The two flow contexts a node can be visited in.
const (
	// ownFlow is the function's ordinary body.
	ownFlow deferredness = false
	// deferredFlow is the body of a `defer func() { … }()` closure.
	deferredFlow deferredness = true
)

// walkOwn visits the nodes that belong to ONE function's flow: everything in
// the body except the insides of nested function literals, which are their own
// functions and get their own visit. The single exception is a DEFERRED
// closure — `defer func() { … }()` runs as part of this function's exit, so
// its body is walked as this function's own statements, and the visitor is
// told it is inside one: a `return` there reaches no caller, so a binding
// judgment reads it as no binding at all. Without the cut, an open-and-defer
// written inside a closure was reported twice: once by the enclosing
// function's walk and once by the literal's own.
func walkOwn(node ast.Node, visit func(node ast.Node, isDeferred deferredness)) {
	walkFlow(node, ownFlow, visit)
}

// walkFlow is walkOwn carrying the flow context through recursion.
func walkFlow(node ast.Node, isDeferred deferredness, visit func(node ast.Node, isDeferred deferredness)) {
	ast.Inspect(node, func(n ast.Node) bool {
		switch at := n.(type) {
		case nil:
			return true
		case *ast.DeferStmt:
			walkDeferred(at, isDeferred, visit)
			return false
		case *ast.FuncLit:
			return false
		}
		visit(n, isDeferred)
		return true
	})
}

// walkDeferred visits a defer statement and, when it defers a closure, walks
// the closure's body as deferred flow.
func walkDeferred(
	deferred *ast.DeferStmt,
	isDeferred deferredness,
	visit func(node ast.Node, isDeferred deferredness),
) {
	visit(deferred, isDeferred)
	if lit, ok := deferred.Call.Fun.(*ast.FuncLit); ok {
		walkFlow(lit.Body, deferredFlow, visit)
	}
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
