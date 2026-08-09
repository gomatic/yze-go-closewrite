package closewrite

import (
	"go/ast"
	"go/token"
	"go/types"
	"testing"

	"github.com/stretchr/testify/assert"
)

// The helpers below carry guards the go/analysis driver cannot reach: it only
// ever hands the analyzer well-formed nodes of the kinds the Preorder filter
// names. The guards still have to hold, because they are what keeps a
// malformed or unexpected shape from becoming a panic inside a linter that the
// whole fleet's gate depends on — a crash there fails every repository at
// once. Driving them directly is the design being testable, not a workaround.

// TestWriteOpenedCollectsOnlyFilesTheCallerMeansToWrite names writeOpened's
// claim: the open itself is the evidence of write intent, so a reader's file
// never enters the set and its Close is never reported.
func TestWriteOpenedCollectsOnlyFilesTheCallerMeansToWrite(t *testing.T) {
	t.Parallel()

	assert.True(t, writeOpeners["Create"], "os.Create means to write")
	assert.True(t, writeOpeners["CreateTemp"], "so does os.CreateTemp")
	assert.False(t, writeOpeners["Open"], "os.Open is read-only and must never enter the set")
	assert.False(t, writeOpeners["OpenFile"], "os.OpenFile qualifies only with a write flag")

	assert.True(t, writeFlags["O_WRONLY"], "a write flag establishes intent")
	assert.False(t, writeFlags["O_RDONLY"], "a read-only flag does not")

	assert.Empty(t, writeOpened(&types.Info{}, &ast.BlockStmt{}), "an empty body opens nothing")
}

// TestBodyOfYieldsNilForAnythingButAFunction pins the type switch's default.
func TestBodyOfYieldsNilForAnythingButAFunction(t *testing.T) {
	t.Parallel()

	body := &ast.BlockStmt{}

	assert.Same(t, body, bodyOf(&ast.FuncDecl{Body: body}))
	assert.Same(t, body, bodyOf(&ast.FuncLit{Body: body}))
	assert.Nil(t, bodyOf(&ast.ReturnStmt{}), "a non-function node has no body")
}

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

// TestIsOSPackageMustNotMatchALocalVariableNamedOS pins the invariant
// isOSPackage documents: resolution goes through the type info, so an aliased
// import is still recognized and a local variable spelled os is not.
func TestIsOSPackageMustNotMatchALocalVariableNamedOS(t *testing.T) {
	t.Parallel()

	info := &types.Info{Uses: map[*ast.Ident]types.Object{}, Defs: map[*ast.Ident]types.Object{}}

	assert.False(t, isOSPackage(info, &ast.SelectorExpr{}), "a non-identifier names no package")

	local := ast.NewIdent("os")
	info.Uses[local] = types.NewVar(token.NoPos, nil, "os", types.Typ[types.String])
	assert.False(t, isOSPackage(info, local), "a variable named os is not package os")

	unresolved := ast.NewIdent("os")
	assert.False(t, isOSPackage(info, unresolved), "an unresolved identifier names no package")
}

// TestClosedObjectRejectsAnythingButANamedReceiversClose pins the receiver
// resolution.
func TestClosedObjectRejectsAnythingButANamedReceiversClose(t *testing.T) {
	t.Parallel()

	info := &types.Info{Uses: map[*ast.Ident]types.Object{}, Defs: map[*ast.Ident]types.Object{}}

	_, ok := closedObject(info, &ast.CallExpr{Fun: ast.NewIdent("Close")})
	assert.False(t, ok, "a bare call has no receiver")

	sync := &ast.SelectorExpr{X: ast.NewIdent("f"), Sel: ast.NewIdent("Sync")}
	_, ok = closedObject(info, &ast.CallExpr{Fun: sync})
	assert.False(t, ok, "only Close counts")

	field := &ast.SelectorExpr{
		X:   &ast.SelectorExpr{X: ast.NewIdent("h"), Sel: ast.NewIdent("f")},
		Sel: ast.NewIdent("Close"),
	}
	_, ok = closedObject(info, &ast.CallExpr{Fun: field})
	assert.False(t, ok, "a field receiver is not a local file this pass owns")

	unresolved := &ast.SelectorExpr{X: ast.NewIdent("f"), Sel: ast.NewIdent("Close")}
	_, ok = closedObject(info, &ast.CallExpr{Fun: unresolved})
	assert.False(t, ok, "an unresolved receiver names no object")
}

// TestCollectOpenedIgnoresAssignmentsThatCannotBeAFileOpen pins the guards on
// the assignment shape.
func TestCollectOpenedIgnoresAssignmentsThatCannotBeAFileOpen(t *testing.T) {
	t.Parallel()

	info := &types.Info{Uses: map[*ast.Ident]types.Object{}, Defs: map[*ast.Ident]types.Object{}}
	opened := map[types.Object]bool{}

	collectOpened(info, &ast.AssignStmt{}, opened)
	collectOpened(info, &ast.AssignStmt{
		Lhs: []ast.Expr{ast.NewIdent("f")},
		Rhs: []ast.Expr{ast.NewIdent("x")},
	}, opened)

	assert.Empty(t, opened, "neither an empty assignment nor a non-call right-hand side opens a file")
}
