package closewrite_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/analysis/analysistest"

	closewrite "github.com/gomatic/yze-go-closewrite"
)

// TestAnalyzerReportsDiscardedClosesOnWrittenFilesOnly drives the analyzer over
// the corpus in testdata, which carries both directions: the three shapes that
// discard a Close error on a write-opened file, and the four that must stay
// silent.
//
// The silent half is the load-bearing half. This rule replaced a proposal to
// turn on errcheck's check-blank, which measurement showed would add 1032
// findings across 97 repositories and catch ZERO of the Close defects it was
// meant to find — golangci's EXC0001 suppresses .*Close unconditionally. A
// replacement that merely fires more often would be the same mistake wearing a
// narrower name, so the reader's Close, the read-only OpenFile, the checked
// Close, and the named-return closure all have to stay quiet.
func TestAnalyzerReportsDiscardedClosesOnWrittenFilesOnly(t *testing.T) {
	t.Parallel()

	analysistest.Run(t, analysistest.TestData(), closewrite.Analyzer, "a")
}

// TestRegistrationIsWellFormed pins the analyzer's declaration to the suite:
// the framework validates the shape, and a malformed registration would be
// discovered only when the aggregate refused to build.
func TestRegistrationIsWellFormed(t *testing.T) {
	t.Parallel()

	require.NoError(t, closewrite.Registration.Validate())
	assert.Equal(t, "yze/closewrite", closewrite.Registration.RuleID())
	assert.Same(t, closewrite.Analyzer, closewrite.Registration.Analyzer)
}

// TestOnlyASeamSettlesAFile pins the settlement rule in both directions. A file
// is settled only by a bound Close or by a call with the shape of one — a
// single argument, the file, and a single error result — so the safety-net
// pair, the return-bound close, the errors.Join'd close and the two seams stay
// silent, while everything wider still reports: an unbound write through fmt, a
// BOUND write through fmt, io.Copy into the file, a call with a second result,
// a one-argument call handed a literal instead of the file, a bufio wrapper, a
// result whose POINTER is the error, a result-less call, and the two discards.
func TestOnlyASeamSettlesAFile(t *testing.T) {
	t.Parallel()

	analysistest.Run(t, analysistest.TestData(), closewrite.Analyzer, "bound")
}

// TestTheFailingPathIsWhatExemptsADiscard pins the predicate the rule turns on.
// Every spelling of a discard on the success path reports — bare call, blank
// assignment, blank tuple, defer — and only a discard reached with a failure
// already in hand is silent, whether that is the body of `if err != nil` or the
// else of `if err == nil`. The near-misses each deviate in one place: a
// condition comparing a non-error to nil, an error to a sentinel, a count, a
// bare boolean, and a nil check with no else at all.
func TestTheFailingPathIsWhatExemptsADiscard(t *testing.T) {
	t.Parallel()

	analysistest.Run(t, analysistest.TestData(), closewrite.Analyzer, "paths")
}

// TestEveryWriteFlagSpellingIsRecognised pins flag resolution beyond the
// literal selector: a folded named constant, a local flag variable, a
// parenthesised one, os.CreateTemp, and each write flag alone — O_WRONLY,
// O_RDWR, O_APPEND, O_TRUNC, and the documented O_CREATE decision. Silent
// beside them: the read-only open, the read-only open carrying a permission
// that collides with the write mask on either platform, a flag arriving as a
// parameter, and a flag bound at index 1 of a tuple — the last two being the
// documented limits of the chase, and the position it must not index past.
func TestEveryWriteFlagSpellingIsRecognised(t *testing.T) {
	t.Parallel()

	analysistest.Run(t, analysistest.TestData(), closewrite.Analyzer, "flags")
}

// TestAClosureIsItsOwnFunction pins the one-visit-per-function property: an
// open-and-discard inside a literal is the literal's finding exactly once —
// analysistest fails on an extra diagnostic, so a double report cannot pass.
func TestAClosureIsItsOwnFunction(t *testing.T) {
	t.Parallel()

	analysistest.Run(t, analysistest.TestData(), closewrite.Analyzer, "closures")
}
