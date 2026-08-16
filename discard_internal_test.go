package closewrite

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDiscardedInIgnoresStatementsThatBindNothingAway pins that only the two
// discarding shapes are recognized.
func TestDiscardedInIgnoresStatementsThatBindNothingAway(t *testing.T) {
	t.Parallel()

	call := &ast.CallExpr{Fun: ast.NewIdent("f")}

	got, ok := discardedIn(&ast.ExprStmt{X: call})
	assert.True(t, ok)
	assert.Same(t, call, got)

	_, ok = discardedIn(&ast.ExprStmt{X: ast.NewIdent("x")})
	assert.False(t, ok, "an expression statement that is not a call discards nothing")

	_, ok = discardedIn(&ast.ReturnStmt{})
	assert.False(t, ok, "a return binds its value")
}

// TestBlankAssignedRequiresOneCallAndAllBlankTargets pins the assignment form.
func TestBlankAssignedRequiresOneCallAndAllBlankTargets(t *testing.T) {
	t.Parallel()

	call := &ast.CallExpr{Fun: ast.NewIdent("f")}
	blank := []ast.Expr{ast.NewIdent("_")}

	got, ok := blankAssigned(&ast.AssignStmt{Lhs: blank, Rhs: []ast.Expr{call}})
	assert.True(t, ok)
	assert.Same(t, call, got)

	_, ok = blankAssigned(&ast.AssignStmt{Lhs: blank, Rhs: []ast.Expr{call, call}})
	assert.False(t, ok, "two right-hand expressions are not a single discarded call")

	_, ok = blankAssigned(&ast.AssignStmt{Lhs: blank, Rhs: []ast.Expr{ast.NewIdent("x")}})
	assert.False(t, ok, "a non-call right-hand side is not a discarded call")

	_, ok = blankAssigned(&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent("err")}, Rhs: []ast.Expr{call}})
	assert.False(t, ok, "a named target binds the error")

	assert.False(t, allBlank(nil), "an empty target list discards nothing")
}

// TestBoundCallsCollectsOnlyResultsThatAreBound names the invariant boundCalls
// documents: a call proves nothing unless its result is bound, so a bare call
// statement and a wholly blank assignment are never collected — and neither is
// a return inside a deferred closure, whose value the defer runtime throws away
// before any caller sees it.
func TestBoundCallsCollectsOnlyResultsThatAreBound(t *testing.T) {
	t.Parallel()

	_, body := typedBody(t, `package probe

func made() error { return nil }

func probe() error {
	made()
	_ = made()
	kept := made()
	defer func() error { return made() }()
	_ = kept
	return made()
}
`)
	bound := boundCalls(body)
	all := callsOf(body)
	require.Len(t, all, 6, "five calls to made plus the deferred literal's own call")

	positions := map[token.Pos]bool{}
	for _, call := range bound {
		positions[call.Pos()] = true
	}

	assert.Len(t, bound, 2, "only the assigned call and the returned one bind anything")
	assert.False(t, positions[all[0].Pos()], "a bare call statement binds nothing")
	assert.False(t, positions[all[1].Pos()], "a wholly blank assignment binds nothing")
	assert.True(t, positions[all[2].Pos()], "a call assigned to a name binds its result")
	assert.False(t, positions[all[4].Pos()], "a return inside a deferred closure reaches no caller")
	assert.True(t, positions[all[5].Pos()], "a return in the function's own flow binds its result")
}

// TestDiscardedClosesJudgesOnlyAnUnconditionalDiscard names the invariant
// discardedCloses documents: a discard reached only through a branch, a loop or
// a label is not judged at all, wherever that branch is. Deciding which branch
// is the already-failing one cannot be done from the syntax — an earlier
// revision tried, reported the same cleanup written five other ways, and made
// the exemption forgeable with `if <some non-nil error> != nil { … }`.
//
// A `defer` statement is the exception, and deliberately: deferring IS the
// unconditional act, so `if x { defer f.Close() }` stays judged and cannot
// become a two-line silence.
func TestDiscardedClosesJudgesOnlyAnUnconditionalDiscard(t *testing.T) {
	t.Parallel()

	_, body := typedBody(t, `package probe

func plain() error       { return nil }
func inIf() error        { return nil }
func inLoop() error      { return nil }
func inSwitch() error    { return nil }
func deferredInIf() error { return nil }
func inClosure() error   { return nil }
func guardedInClosure() error { return nil }
func blanked() error     { return nil }

func probe(err error, ready bool) error {
	plain()
	if ready {
		inIf()
	}
	for range 2 {
		inLoop()
	}
	switch {
	case err != nil:
		inSwitch()
	}
	if ready {
		defer deferredInIf()
	}
	defer func() {
		inClosure()
		if err != nil {
			guardedInClosure()
		}
	}()
	_ = blanked()
	return nil
}
`)
	judged := map[string]bool{}
	for _, call := range discardedCloses(body) {
		if ident, ok := call.Fun.(*ast.Ident); ok {
			judged[ident.Name] = true
		}
	}

	assert.True(t, judged["plain"], "a bare call at the top of the body is unconditional")
	assert.True(t, judged["blanked"], "so is a wholly blank assignment there")
	assert.False(t, judged["inIf"], "a discard inside an if is not judged")
	assert.False(t, judged["inLoop"], "nor one inside a loop")
	assert.False(t, judged["inSwitch"], "nor one inside a switch case")
	assert.True(t, judged["deferredInIf"], "a DEFER is judged wherever it sits, so a branch cannot silence it")
	assert.True(t, judged["inClosure"], "a deferred closure's own top-level statement is unconditional")
	assert.False(t, judged["guardedInClosure"], "one guarded inside that closure is the rollback idiom")
}

// typedBody parses one snippet, type-checks it, and yields the checker's info
// with the body of its `probe` function — so a helper can be driven on nodes
// the checker actually produced rather than on a hand-built approximation of
// them, which is how an earlier guard test came to be satisfied by the walk
// falling short instead of by the guard.
func typedBody(t *testing.T, source probeSource) (*types.Info, *ast.BlockStmt) {
	t.Helper()

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "probe.go", string(source), 0)
	require.NoError(t, err)

	info := &types.Info{
		Types: map[ast.Expr]types.TypeAndValue{},
		Defs:  map[*ast.Ident]types.Object{},
		Uses:  map[*ast.Ident]types.Object{},
	}
	_, err = (&types.Config{}).Check("probe", fset, []*ast.File{file}, info)
	require.NoError(t, err)

	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if ok && fn.Name.Name == "probe" {
			return info, fn.Body
		}
	}
	require.FailNow(t, "the snippet declares no probe function")
	return nil, nil
}

// probeSource is the Go source of one type-checkable snippet.
type probeSource string

// callsOf yields the call expressions of a body in source order.
func callsOf(body *ast.BlockStmt) []*ast.CallExpr {
	var found []*ast.CallExpr
	ast.Inspect(body, func(node ast.Node) bool {
		if call, ok := node.(*ast.CallExpr); ok {
			found = append(found, call)
		}
		return true
	})
	return found
}

// TestUnconditionalDiscardsStopsAtOneBlock names the invariant
// unconditionalDiscards documents: it applies its collector to the statements
// of ONE block and never descends, which is what keeps a discard reached
// through a branch, a loop or a label out of the judged set without the
// analyzer having to decide anything about that branch.
func TestUnconditionalDiscardsStopsAtOneBlock(t *testing.T) {
	t.Parallel()

	top := &ast.ExprStmt{X: &ast.CallExpr{Fun: ast.NewIdent("top")}}
	nested := &ast.ExprStmt{X: &ast.CallExpr{Fun: ast.NewIdent("nested")}}
	stmts := []ast.Stmt{top, &ast.IfStmt{Body: &ast.BlockStmt{List: []ast.Stmt{nested}}}}

	collected := unconditionalDiscards(stmts, inlineDiscards)

	require.Len(t, collected, 1, "the nested statement belongs to the branch, not to this block")
	assert.Same(t, top.X, collected[0])
	assert.Empty(t, unconditionalDiscards(nil, inlineDiscards), "an empty block discards nothing")
}
