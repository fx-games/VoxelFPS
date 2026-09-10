package quads

import "testing"

// The window is what makes the title stable: one expensive frame for every
// nine cheap ones must not move the median, and must still show up in the p95.
func TestWorkTimePercentilesHoldTheMedianAndShowTheTail(t *testing.T) {
	var app GlApplication
	for i := 0; i < workTimeWindow; i++ {
		worked := 0.001
		if i%10 == 0 {
			worked = 0.004
		}
		app.recordWorkTime(worked)
	}

	median, p95 := app.WorkTimePercentiles()
	if median != 0.001 {
		t.Errorf("median work time = %v, want the 1 ms the frame usually costs", median)
	}
	if p95 != 0.004 {
		t.Errorf("p95 work time = %v, want the 4 ms hitch", p95)
	}
}

// The window wraps, so a scene that got cheaper must not keep reporting what the
// previous one cost.
func TestWorkTimePercentilesForgetOlderThanTheWindow(t *testing.T) {
	var app GlApplication
	for i := 0; i < workTimeWindow; i++ {
		app.recordWorkTime(0.010)
	}
	for i := 0; i < workTimeWindow; i++ {
		app.recordWorkTime(0.002)
	}

	if median, _ := app.WorkTimePercentiles(); median != 0.002 {
		t.Errorf("median work time = %v, want 0.002 once the slow frames rolled out", median)
	}
}

// Before the first frame there is nothing measured, and the title still has to
// render.
func TestWorkTimePercentilesBeforeAnyFrame(t *testing.T) {
	var app GlApplication
	median, p95 := app.WorkTimePercentiles()
	if median != 0 || p95 != 0 {
		t.Errorf("percentiles before any frame = %v, %v, want 0, 0", median, p95)
	}
}

// The count is the question the percentiles cannot answer: a window whose median
// and p95 both sit well under the budget can still hold frames that blew it.
func TestWorkFramesOverBudgetCountsWhatThePercentilesMiss(t *testing.T) {
	var app GlApplication
	const budget = 1.0 / 60.0
	for i := 0; i < workTimeWindow; i++ {
		worked := 0.001
		if i%40 == 0 {
			worked = 0.020
		}
		app.recordWorkTime(worked)
	}

	if _, p95 := app.WorkTimePercentiles(); p95 > budget {
		t.Fatalf("p95 = %v, this test is pointless unless it stays under the budget", p95)
	}
	over, of := app.WorkFramesOverBudget(budget)
	if over != 3 || of != workTimeWindow {
		t.Errorf("over budget = %d of %d, want 3 of %d", over, of, workTimeWindow)
	}
}

// A frame exactly on the budget made its refresh, so it is not over.
func TestWorkFramesOverBudgetExcludesTheExactBudget(t *testing.T) {
	var app GlApplication
	const budget = 1.0 / 60.0
	app.recordWorkTime(budget)

	if over, of := app.WorkFramesOverBudget(budget); over != 0 || of != 1 {
		t.Errorf("over budget = %d of %d, want 0 of 1", over, of)
	}
}

// With no monitor to ask there is no budget, and nothing can be over it.
func TestWorkFramesOverBudgetWithoutABudget(t *testing.T) {
	var app GlApplication
	app.recordWorkTime(0.5)

	if over, of := app.WorkFramesOverBudget(0); over != 0 || of != 1 {
		t.Errorf("over budget = %d of %d, want 0 of 1", over, of)
	}
}
