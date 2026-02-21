package taskboard

import (
	"testing"
)

func TestTaskStatusTransitions(t *testing.T) {
	tests := []struct {
		from  TaskStatus
		to    TaskStatus
		valid bool
	}{
		{StatusBacklog, StatusReady, true},
		{StatusBacklog, StatusCancelled, true},
		{StatusBacklog, StatusRunning, false},
		{StatusReady, StatusRunning, true},
		{StatusReady, StatusBacklog, true},
		{StatusReady, StatusCancelled, true},
		{StatusRunning, StatusDone, true},
		{StatusRunning, StatusFailed, true},
		{StatusRunning, StatusCancelled, true},
		{StatusRunning, StatusReady, false},
		{StatusFailed, StatusReady, true},
		{StatusFailed, StatusCancelled, true},
		{StatusDone, StatusReady, false},
		{StatusDone, StatusCancelled, false},
		{StatusCancelled, StatusReady, false},
	}

	for _, tt := range tests {
		result := tt.from.ValidTransition(tt.to)
		if result != tt.valid {
			t.Errorf("%s -> %s: got %v, want %v", tt.from, tt.to, result, tt.valid)
		}
	}
}

func TestTaskStatusIsTerminal(t *testing.T) {
	if !StatusDone.IsTerminal() {
		t.Error("Done should be terminal")
	}
	if !StatusCancelled.IsTerminal() {
		t.Error("Cancelled should be terminal")
	}
	if StatusRunning.IsTerminal() {
		t.Error("Running should not be terminal")
	}
	if StatusReady.IsTerminal() {
		t.Error("Ready should not be terminal")
	}
}

func TestStatusCounts(t *testing.T) {
	c := StatusCounts{Backlog: 1, Ready: 2, Running: 1, Done: 3, Failed: 0, Cancelled: 1}
	if c.Total() != 8 {
		t.Errorf("Total = %d, want 8", c.Total())
	}
	if c.AllDone() {
		t.Error("AllDone should be false when there are non-done tasks")
	}

	c2 := StatusCounts{Done: 5}
	if !c2.AllDone() {
		t.Error("AllDone should be true when only done tasks exist")
	}

	c3 := StatusCounts{} // zero counts
	if c3.AllDone() {
		t.Error("AllDone should be false when no tasks exist")
	}
}

func TestPlanResultValidate(t *testing.T) {
	// Valid plan
	plan := &PlanResult{
		WorkspaceName: "test",
		Tasks: []PlannedTask{
			{Title: "task1", Prompt: "do task1"},
			{Title: "task2", Prompt: "do task2", DependsOn: []int{0}},
		},
	}
	if err := plan.Validate(); err != nil {
		t.Errorf("valid plan returned error: %v", err)
	}

	// Missing workspace name
	bad1 := &PlanResult{Tasks: []PlannedTask{{Title: "t", Prompt: "p"}}}
	if err := bad1.Validate(); err == nil {
		t.Error("missing workspace_name should fail")
	}

	// No tasks
	bad2 := &PlanResult{WorkspaceName: "test"}
	if err := bad2.Validate(); err == nil {
		t.Error("no tasks should fail")
	}

	// Missing title
	bad3 := &PlanResult{WorkspaceName: "test", Tasks: []PlannedTask{{Prompt: "p"}}}
	if err := bad3.Validate(); err == nil {
		t.Error("missing title should fail")
	}

	// Self-dependency
	bad4 := &PlanResult{WorkspaceName: "test", Tasks: []PlannedTask{
		{Title: "t", Prompt: "p", DependsOn: []int{0}},
	}}
	if err := bad4.Validate(); err == nil {
		t.Error("self-dependency should fail")
	}

	// Out of range dependency
	bad5 := &PlanResult{WorkspaceName: "test", Tasks: []PlannedTask{
		{Title: "t", Prompt: "p", DependsOn: []int{5}},
	}}
	if err := bad5.Validate(); err == nil {
		t.Error("out of range dependency should fail")
	}
}
