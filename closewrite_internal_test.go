package closewrite

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"math"
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

	body := &ast.BlockStmt{}
	collectOpened(info, body, nil, nil, opened)
	collectOpened(info, body,
		[]ast.Expr{ast.NewIdent("f")},
		[]ast.Expr{ast.NewIdent("x")},
		opened)

	assert.Empty(t, opened, "neither an empty binding nor a non-call right-hand side opens a file")
}

// TestFlagValueYieldsZeroWithoutAnExactIntConstant pins the two refusals that
// keep the mask honest: a name the package does not declare as a constant
// contributes nothing, and an integer too large for int64 contributes nothing
// rather than a truncated bit pattern.
func TestFlagValueYieldsZeroWithoutAnExactIntConstant(t *testing.T) {
	t.Parallel()

	pkg := types.NewPackage("os", "os")
	assert.Zero(t, flagValue(pkg, "O_NOT_DECLARED"), "an absent constant contributes nothing")

	huge := types.NewConst(0, pkg, "O_HUGE", types.Typ[types.UntypedInt], constant.MakeUint64(math.MaxUint64))
	pkg.Scope().Insert(huge)
	assert.Zero(t, flagValue(pkg, "O_HUGE"), "an inexact value contributes nothing rather than truncating")

	exact := types.NewConst(0, pkg, "O_REAL", types.Typ[types.UntypedInt], constant.MakeInt64(4))
	pkg.Scope().Insert(exact)
	assert.Equal(t, flagMask(4), flagValue(pkg, "O_REAL"))
}

// TestResultsIncludeErrorJudgesEveryResultShape pins the settlement guard: a
// call with no recorded type proves nothing, a tuple settles only when one of
// its members is the error type, and a single result settles only when it IS
// the error type.
func TestResultsIncludeErrorJudgesEveryResultShape(t *testing.T) {
	t.Parallel()

	errType := types.Universe.Lookup("error").Type()
	intType := types.Typ[types.Int]
	pair := func(at types.Type) *ast.CallExpr {
		call := &ast.CallExpr{Fun: ast.NewIdent("f")}
		info := &types.Info{Types: map[ast.Expr]types.TypeAndValue{call: {Type: at}}}
		if !resultsIncludeError(info, call) {
			return call
		}
		return nil
	}

	assert.NotNil(t, pair(nil), "no recorded type proves nothing")
	assert.NotNil(t, pair(intType), "a lone non-error result proves nothing")
	assert.Nil(t, pair(errType), "a lone error result settles")
	assert.NotNil(t, pair(types.NewTuple(
		types.NewVar(0, nil, "", intType), types.NewVar(0, nil, "", intType))),
		"a tuple without an error proves nothing")
	assert.Nil(t, pair(types.NewTuple(
		types.NewVar(0, nil, "", intType), types.NewVar(0, nil, "", errType))),
		"a tuple carrying an error settles")
}

// TestAssignedFlagWritesRefusesANilObject pins the chase's guard: an ident
// the checker never resolved chases nothing.
func TestAssignedFlagWritesRefusesANilObject(t *testing.T) {
	t.Parallel()

	info := &types.Info{}
	assert.False(t, assignedFlagWrites(info, &ast.BlockStmt{}, flagMask(1), nil, token.NoPos))
}
