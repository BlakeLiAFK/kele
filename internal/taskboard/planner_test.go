package taskboard

import (
	"testing"
)

func TestExtractJSON(t *testing.T) {
	tests := []struct {
		name  string
		input string
		valid bool
	}{
		{
			name:  "plain JSON",
			input: `{"workspace_name": "test", "tasks": []}`,
			valid: true,
		},
		{
			name: "JSON in markdown code block",
			input: "Here is the plan:\n```json\n{\"workspace_name\": \"test\", \"tasks\": []}\n```\nDone!",
			valid: true,
		},
		{
			name: "JSON in plain code block",
			input: "```\n{\"workspace_name\": \"test\", \"tasks\": []}\n```",
			valid: true,
		},
		{
			name:  "JSON embedded in text",
			input: `Some text before {"workspace_name": "test", "tasks": []} and after`,
			valid: true,
		},
		{
			name:  "no JSON",
			input: "just plain text with no json",
			valid: false,
		},
		{
			name:  "invalid JSON",
			input: `{"broken: json`,
			valid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractJSON(tt.input)
			if tt.valid && result == "" {
				t.Error("expected valid JSON to be extracted, got empty")
			}
			if !tt.valid && result != "" {
				t.Errorf("expected no JSON, got: %s", result)
			}
		})
	}
}

func TestParsePlan(t *testing.T) {
	validJSON := `{
		"workspace_name": "test-project",
		"workspace_context": "Go project",
		"max_concurrent": 2,
		"tasks": [
			{
				"title": "Define types",
				"description": "Define data types",
				"prompt": "Create types.go with struct definitions",
				"priority": 0,
				"depends_on": [],
				"tags": ["backend"]
			},
			{
				"title": "Implement store",
				"description": "SQLite storage",
				"prompt": "Create store.go with CRUD operations",
				"priority": 1,
				"depends_on": [0],
				"tags": ["backend"]
			}
		]
	}`

	plan, err := ParsePlan(validJSON)
	if err != nil {
		t.Fatalf("ParsePlan: %v", err)
	}
	if plan.WorkspaceName != "test-project" {
		t.Errorf("WorkspaceName = %s, want test-project", plan.WorkspaceName)
	}
	if len(plan.Tasks) != 2 {
		t.Errorf("Tasks len = %d, want 2", len(plan.Tasks))
	}
	if plan.Tasks[1].DependsOn[0] != 0 {
		t.Errorf("Tasks[1].DependsOn = %v, want [0]", plan.Tasks[1].DependsOn)
	}

	// Invalid JSON
	_, err = ParsePlan("not json")
	if err == nil {
		t.Error("invalid JSON should fail")
	}

	// Valid JSON but invalid plan
	_, err = ParsePlan(`{"tasks": []}`)
	if err == nil {
		t.Error("empty workspace name should fail")
	}
}
