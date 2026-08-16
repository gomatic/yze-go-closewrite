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

// TestCollectOpenedIgnoresAssignmentsThatCannotBeAFileOpen pins the guards on
// the assignment shape, and drives the empty-left-hand-side one with a
// right-hand side that DOES open a file — the only input that reaches it. An
// earlier version passed no right-hand side at all, so the walk returned at the
// arity check and the guard was never evaluated: deleting it left the suite
// green.
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

	create := osCreateCall(info)
	collectOpened(info, body, nil, []ast.Expr{create}, opened)
	assert.Empty(
		t,
		opened,
		"a binding with no names on the left opens nothing, and must not index a left-hand side that is not there",
	)

	name := ast.NewIdent("f")
	file := types.NewVar(token.NoPos, nil, "f", types.Typ[types.Int])
	info.Defs[name] = file
	collectOpened(info, body, []ast.Expr{name}, []ast.Expr{create}, opened)
	assert.True(
		t,
		opened[file],
		"the SAME call bound to a name is an open, so the case above is silent because of the guard and not because the call failed to qualify",
	)
}

// osCreateCall builds an os.Create call the analyzer resolves through info,
// so a guard reached only by a qualifying open can be driven directly.
func osCreateCall(info *types.Info) *ast.CallExpr {
	qualifier := ast.NewIdent("os")
	info.Uses[qualifier] = types.NewPkgName(token.NoPos, nil, "os", types.NewPackage("os", "os"))
	return &ast.CallExpr{Fun: &ast.SelectorExpr{X: qualifier, Sel: ast.NewIdent("Create")}}
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

// TestAssignedFlagWritesRefusesANilObject pins the chase's guard: an ident the
// checker never resolved chases nothing, and in particular does not match the
// equally unresolved target of an assignment that DOES carry a write bit.
//
// The body below is what makes that an assertion rather than a decoration. An
// earlier version passed an empty block, whose End() precedes the position
// being chased to, so the walk returned at the first node and the object was
// never compared — the assertion held for any implementation, and deleting the
// guard left the suite green at 100.0% coverage.
func TestAssignedFlagWritesRefusesANilObject(t *testing.T) {
	t.Parallel()

	target := &ast.Ident{NamePos: 1, Name: "flags"}
	value := &ast.BasicLit{ValuePos: 10, Kind: token.INT, Value: "1"}
	body := &ast.BlockStmt{List: []ast.Stmt{&ast.AssignStmt{
		Lhs: []ast.Expr{target}, TokPos: 8, Tok: token.ASSIGN, Rhs: []ast.Expr{value},
	}}}
	info := &types.Info{
		Types: map[ast.Expr]types.TypeAndValue{value: {Value: constant.MakeInt64(1)}},
		Uses:  map[*ast.Ident]types.Object{},
	}
	after := token.Pos(100)

	assert.False(t, assignedFlagWrites(info, body, flagMask(1), nil, after),
		"an unresolved ident chases nothing, whatever the assignment's target resolves to")

	resolved := types.NewVar(token.NoPos, nil, "flags", types.Typ[types.Int])
	info.Uses[target] = resolved
	assert.True(
		t,
		assignedFlagWrites(info, body, flagMask(1), resolved, after),
		"the SAME body reached with a resolved object does carry the write bit, so the refusal above is the guard and not the walk falling short",
	)
}
