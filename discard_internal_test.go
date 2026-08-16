package closewrite

import (
	"go/ast"
	"go/token"
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

// TestOnSuccessPathDropsWhatAFailingBranchAlreadyHandles names the invariant
// onSuccessPath documents: a discard is dropped exactly when it sits in a
// region reached with a failure already in hand, and where that region is makes
// no difference. Inside a deferred closure the guarded body is the rollback
// idiom — the closure runs on every path, its `if err != nil` body does not —
// and treating the closure as having no failing branch reported that idiom as a
// defect against the standard library.
func TestOnSuccessPathDropsWhatAFailingBranchAlreadyHandles(t *testing.T) {
	t.Parallel()

	info, body := typedBody(t, `package probe

func droppedInline() error   { return nil }
func droppedInDefer() error  { return nil }
func keptInline() error      { return nil }
func keptInDefer() error     { return nil }

func probe() (err error) {
	defer func() {
		if err != nil {
			droppedInDefer()
		}
		keptInDefer()
	}()
	if err != nil {
		droppedInline()
	}
	keptInline()
	return nil
}
`)
	called := map[string]bool{}
	for _, call := range onSuccessPath(failingSpans(info, body), callsOf(body)) {
		if ident, ok := call.Fun.(*ast.Ident); ok {
			called[ident.Name] = true
		}
	}

	assert.False(
		t,
		called["droppedInline"],
		"a discard inside `if err != nil` is cleanup the caller is already being told about",
	)
	assert.False(t, called["droppedInDefer"], "the same check inside a deferred closure guards the same cleanup")
	assert.True(t, called["keptInline"], "a discard on the success path is the last word on the file")
	assert.True(t, called["keptInDefer"], "and so is one in a deferred closure that runs unguarded")
}
