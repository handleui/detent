package workflow

import (
	"strings"
	"testing"
)

func TestValidateWorkflow_RunsOn(t *testing.T) {
	tests := []struct {
		name       string
		workflow   *Workflow
		wantErr    bool
		wantErrMsg string
	}{
		{
			name: "supported ubuntu-latest",
			workflow: &Workflow{
				Jobs: map[string]*Job{
					"test": {RunsOn: "ubuntu-latest"},
				},
			},
			wantErr: false,
		},
		{
			name: "supported ubuntu-22.04",
			workflow: &Workflow{
				Jobs: map[string]*Job{
					"test": {RunsOn: "ubuntu-22.04"},
				},
			},
			wantErr: false,
		},
		{
			name: "supported ubuntu-20.04",
			workflow: &Workflow{
				Jobs: map[string]*Job{
					"test": {RunsOn: "ubuntu-20.04"},
				},
			},
			wantErr: false,
		},
		{
			name: "supported ubuntu-24.04",
			workflow: &Workflow{
				Jobs: map[string]*Job{
					"test": {RunsOn: "ubuntu-24.04"},
				},
			},
			wantErr: false,
		},
		{
			name: "unsupported macos-latest",
			workflow: &Workflow{
				Jobs: map[string]*Job{
					"test": {RunsOn: "macos-latest"},
				},
			},
			wantErr:    true,
			wantErrMsg: "macos-latest",
		},
		{
			name: "unsupported windows-latest",
			workflow: &Workflow{
				Jobs: map[string]*Job{
					"test": {RunsOn: "windows-latest"},
				},
			},
			wantErr:    true,
			wantErrMsg: "windows-latest",
		},
		{
			name: "unsupported self-hosted",
			workflow: &Workflow{
				Jobs: map[string]*Job{
					"test": {RunsOn: "self-hosted"},
				},
			},
			wantErr:    true,
			wantErrMsg: "self-hosted",
		},
		{
			name: "matrix expression allowed",
			workflow: &Workflow{
				Jobs: map[string]*Job{
					"test": {RunsOn: "${{ matrix.os }}"},
				},
			},
			wantErr: false,
		},
		{
			name: "array with self-hosted label",
			workflow: &Workflow{
				Jobs: map[string]*Job{
					"test": {RunsOn: []any{"self-hosted", "linux"}},
				},
			},
			wantErr:    true,
			wantErrMsg: "self-hosted",
		},
		{
			name: "runner group not supported",
			workflow: &Workflow{
				Jobs: map[string]*Job{
					"test": {
						RunsOn: map[string]any{
							"group":  "my-runner-group",
							"labels": []any{"linux"},
						},
					},
				},
			},
			wantErr:    true,
			wantErrMsg: "runner groups",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateWorkflow(tt.workflow)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateWorkflow() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.wantErrMsg != "" {
				if !strings.Contains(err.Error(), tt.wantErrMsg) {
					t.Errorf("error message should contain %q, got: %v", tt.wantErrMsg, err)
				}
			}
		})
	}
}

func TestValidateWorkflow_Services(t *testing.T) {
	workflow := &Workflow{
		Jobs: map[string]*Job{
			"test": {
				RunsOn: "ubuntu-latest",
				Services: map[string]any{
					"postgres": map[string]any{
						"image": "postgres:15",
					},
				},
			},
		},
	}

	err := ValidateWorkflow(workflow)
	if err == nil {
		t.Error("expected error for services, got nil")
		return
	}

	if !strings.Contains(err.Error(), "services") {
		t.Errorf("error should mention services: %v", err)
	}
}

func TestValidateWorkflow_Container(t *testing.T) {
	tests := []struct {
		name      string
		container any
		wantErr   bool
	}{
		{
			name:      "simple string container",
			container: "node:18",
			wantErr:   false,
		},
		{
			name: "container with image only",
			container: map[string]any{
				"image": "node:18",
			},
			wantErr: false,
		},
		{
			name: "container with credentials",
			container: map[string]any{
				"image": "private-registry/image",
				"credentials": map[string]any{
					"username": "${{ secrets.DOCKER_USER }}",
					"password": "${{ secrets.DOCKER_PASS }}",
				},
			},
			wantErr: true,
		},
		{
			name: "container with volumes",
			container: map[string]any{
				"image":   "node:18",
				"volumes": []any{"/tmp:/tmp"},
			},
			wantErr: true,
		},
		{
			name: "container with privileged option",
			container: map[string]any{
				"image":   "node:18",
				"options": "--privileged",
			},
			wantErr: true,
		},
		{
			name: "container with network host option",
			container: map[string]any{
				"image":   "node:18",
				"options": "--network=host",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workflow := &Workflow{
				Jobs: map[string]*Job{
					"test": {
						RunsOn:    "ubuntu-latest",
						Container: tt.container,
					},
				},
			}

			err := ValidateWorkflow(workflow)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateWorkflow() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateWorkflow_ReusableWorkflows(t *testing.T) {
	tests := []struct {
		name    string
		uses    string
		wantErr bool
	}{
		{
			name:    "local reusable workflow",
			uses:    "./.github/workflows/reusable.yml",
			wantErr: true,
		},
		{
			name:    "external reusable workflow",
			uses:    "org/repo/.github/workflows/reusable.yml@main",
			wantErr: true,
		},
		{
			name:    "regular action",
			uses:    "actions/checkout@v4",
			wantErr: false,
		},
		{
			name:    "docker action",
			uses:    "docker://alpine:3.18",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workflow := &Workflow{
				Jobs: map[string]*Job{
					"test": {
						RunsOn: "ubuntu-latest",
						Steps: []*Step{
							{Uses: tt.uses},
						},
					},
				},
			}

			err := ValidateWorkflow(workflow)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateWorkflow() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateWorkflow_Matrix(t *testing.T) {
	tests := []struct {
		name     string
		strategy any
		wantErr  bool
	}{
		{
			name: "simple matrix",
			strategy: map[string]any{
				"matrix": map[string]any{
					"node": []any{"16", "18", "20"},
				},
			},
			wantErr: false,
		},
		{
			name: "matrix with fromJSON",
			strategy: map[string]any{
				"matrix": map[string]any{
					"include": "${{ fromJSON(needs.setup.outputs.matrix) }}",
				},
			},
			wantErr: true,
		},
		{
			name:     "no strategy",
			strategy: nil,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workflow := &Workflow{
				Jobs: map[string]*Job{
					"test": {
						RunsOn:   "ubuntu-latest",
						Strategy: tt.strategy,
					},
				},
			}

			err := ValidateWorkflow(workflow)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateWorkflow() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateWorkflow_MultipleErrors(t *testing.T) {
	workflow := &Workflow{
		Jobs: map[string]*Job{
			"build": {
				RunsOn: "macos-latest",
				Services: map[string]any{
					"db": map[string]any{"image": "postgres"},
				},
			},
			"test": {
				RunsOn: "windows-latest",
				Steps: []*Step{
					{Uses: "./.github/workflows/reusable.yml"},
				},
			},
		},
	}

	err := ValidateWorkflow(workflow)
	if err == nil {
		t.Error("expected multiple errors, got nil")
		return
	}

	validationErrs, ok := err.(ValidationErrors)
	if !ok {
		t.Errorf("expected ValidationErrors type, got %T", err)
		return
	}

	// Should have at least 4 errors:
	// 1. macos-latest not supported
	// 2. services not supported
	// 3. windows-latest not supported
	// 4. reusable workflow not supported
	if len(validationErrs) < 4 {
		t.Errorf("expected at least 4 errors, got %d: %v", len(validationErrs), err)
	}
}

func TestValidateWorkflow_NilWorkflow(t *testing.T) {
	err := ValidateWorkflow(nil)
	if err != nil {
		t.Errorf("expected nil error for nil workflow, got: %v", err)
	}
}

func TestValidateWorkflow_EmptyJobs(t *testing.T) {
	workflow := &Workflow{
		Jobs: nil,
	}
	err := ValidateWorkflow(workflow)
	if err != nil {
		t.Errorf("expected nil error for empty jobs, got: %v", err)
	}
}

func TestValidationError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      *ValidationError
		expected string
	}{
		{
			name: "workflow level error",
			err: &ValidationError{
				Feature:     "runs-on",
				Description: "test description",
			},
			expected: `unsupported feature "runs-on": test description`,
		},
		{
			name: "job level error",
			err: &ValidationError{
				Feature:     "services",
				Description: "test description",
				JobID:       "build",
			},
			expected: `unsupported feature "services" (job: build): test description`,
		},
		{
			name: "step level error",
			err: &ValidationError{
				Feature:     "reusable-workflow",
				Description: "test description",
				JobID:       "build",
				StepName:    "call-reusable",
			},
			expected: `unsupported feature "reusable-workflow" (job: build, step: call-reusable): test description`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.err.Error()
			if result != tt.expected {
				t.Errorf("Error() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestValidationErrors_Error(t *testing.T) {
	tests := []struct {
		name     string
		errors   ValidationErrors
		contains []string
	}{
		{
			name:     "empty errors",
			errors:   ValidationErrors{},
			contains: []string{},
		},
		{
			name: "single error",
			errors: ValidationErrors{
				{Feature: "runs-on", Description: "not supported"},
			},
			contains: []string{"runs-on"},
		},
		{
			name: "multiple errors",
			errors: ValidationErrors{
				{Feature: "runs-on", Description: "not supported"},
				{Feature: "services", Description: "limited support"},
			},
			contains: []string{"2 unsupported features", "runs-on", "services"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.errors.Error()
			for _, want := range tt.contains {
				if !strings.Contains(result, want) {
					t.Errorf("Error() should contain %q, got: %q", want, result)
				}
			}
		})
	}
}

func TestValidateWorkflows(t *testing.T) {
	workflows := []*Workflow{
		{
			Jobs: map[string]*Job{
				"test1": {RunsOn: "ubuntu-latest"},
			},
		},
		{
			Jobs: map[string]*Job{
				"test2": {RunsOn: "macos-latest"},
			},
		},
	}

	err := ValidateWorkflows(workflows)
	if err == nil {
		t.Error("expected error for macos-latest, got nil")
		return
	}

	if !strings.Contains(err.Error(), "macos-latest") {
		t.Errorf("error should mention macos-latest: %v", err)
	}
}

func TestValidateWorkflows_AllValid(t *testing.T) {
	workflows := []*Workflow{
		{
			Jobs: map[string]*Job{
				"test1": {RunsOn: "ubuntu-latest"},
			},
		},
		{
			Jobs: map[string]*Job{
				"test2": {RunsOn: "ubuntu-22.04"},
			},
		},
	}

	err := ValidateWorkflows(workflows)
	if err != nil {
		t.Errorf("expected no error for valid workflows, got: %v", err)
	}
}

func TestValidateWorkflow_LargeRunners(t *testing.T) {
	tests := []struct {
		name       string
		runsOn     string
		wantErr    bool
		wantErrMsg string
	}{
		{
			name:       "large runner",
			runsOn:     "ubuntu-latest-large",
			wantErr:    true,
			wantErrMsg: "large runner",
		},
		{
			name:       "xlarge runner",
			runsOn:     "ubuntu-22.04-xlarge",
			wantErr:    true,
			wantErrMsg: "large runner",
		},
		{
			name:       "4xlarge runner",
			runsOn:     "ubuntu-latest-4xlarge",
			wantErr:    true,
			wantErrMsg: "large runner",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workflow := &Workflow{
				Jobs: map[string]*Job{
					"test": {RunsOn: tt.runsOn},
				},
			}

			err := ValidateWorkflow(workflow)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateWorkflow() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.wantErrMsg != "" {
				if !strings.Contains(err.Error(), tt.wantErrMsg) {
					t.Errorf("error should contain %q, got: %v", tt.wantErrMsg, err)
				}
			}
		})
	}
}

func TestValidateWorkflow_Environment(t *testing.T) {
	tests := []struct {
		name        string
		environment any
		wantErr     bool
		wantErrMsg  string
	}{
		{
			name:        "string environment",
			environment: "production",
			wantErr:     true,
			wantErrMsg:  "environment",
		},
		{
			name: "object environment",
			environment: map[string]any{
				"name": "staging",
				"url":  "https://staging.example.com",
			},
			wantErr:    true,
			wantErrMsg: "environment",
		},
		{
			name:        "no environment",
			environment: nil,
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workflow := &Workflow{
				Jobs: map[string]*Job{
					"test": {
						RunsOn:      "ubuntu-latest",
						Environment: tt.environment,
					},
				},
			}

			err := ValidateWorkflow(workflow)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateWorkflow() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.wantErrMsg != "" {
				if !strings.Contains(err.Error(), tt.wantErrMsg) {
					t.Errorf("error should contain %q, got: %v", tt.wantErrMsg, err)
				}
			}
		})
	}
}

func TestValidateWorkflow_JobLevelReusableWorkflow(t *testing.T) {
	tests := []struct {
		name       string
		uses       string
		wantErr    bool
		wantErrMsg string
	}{
		{
			name:       "local reusable workflow at job level",
			uses:       "./.github/workflows/build.yml",
			wantErr:    true,
			wantErrMsg: "reusable workflow",
		},
		{
			name:       "external reusable workflow at job level",
			uses:       "org/repo/.github/workflows/ci.yml@main",
			wantErr:    true,
			wantErrMsg: "reusable workflow",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workflow := &Workflow{
				Jobs: map[string]*Job{
					"test": {
						Uses: tt.uses,
					},
				},
			}

			err := ValidateWorkflow(workflow)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateWorkflow() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.wantErrMsg != "" {
				if !strings.Contains(err.Error(), tt.wantErrMsg) {
					t.Errorf("error should contain %q, got: %v", tt.wantErrMsg, err)
				}
			}
		})
	}
}

func TestValidateWorkflow_WorkflowCall(t *testing.T) {
	workflow := &Workflow{
		On: map[string]any{
			"workflow_call": map[string]any{
				"inputs": map[string]any{
					"config": map[string]any{
						"required": true,
					},
				},
			},
		},
		Jobs: map[string]*Job{
			"test": {RunsOn: "ubuntu-latest"},
		},
	}

	err := ValidateWorkflow(workflow)
	if err == nil {
		t.Error("expected error for workflow_call trigger, got nil")
		return
	}

	if !strings.Contains(err.Error(), "workflow_call") {
		t.Errorf("error should mention workflow_call: %v", err)
	}
}

func TestValidationErrors_Severity(t *testing.T) {
	errors := ValidationErrors{
		{Feature: "runs-on", Severity: SeverityError},
		{Feature: "services", Severity: SeverityWarning},
		{Feature: "environment", Severity: SeverityWarning},
		{Feature: "reusable-workflow", Severity: SeverityError},
	}

	if !errors.HasErrors() {
		t.Error("HasErrors() should return true when there are errors")
	}

	errs := errors.Errors()
	if len(errs) != 2 {
		t.Errorf("Errors() should return 2 errors, got %d", len(errs))
	}

	warnings := errors.Warnings()
	if len(warnings) != 2 {
		t.Errorf("Warnings() should return 2 warnings, got %d", len(warnings))
	}
}

func TestValidationErrors_WarningsOnly(t *testing.T) {
	errors := ValidationErrors{
		{Feature: "services", Severity: SeverityWarning},
		{Feature: "environment", Severity: SeverityWarning},
	}

	if errors.HasErrors() {
		t.Error("HasErrors() should return false when there are only warnings")
	}
}

func TestValidationError_Suggestion(t *testing.T) {
	err := &ValidationError{
		Feature:     "runs-on",
		Description: "macos-latest is not supported",
		Suggestion:  "Use ubuntu-latest instead",
		JobID:       "build",
	}

	errStr := err.Error()
	if !strings.Contains(errStr, "Use ubuntu-latest instead") {
		t.Errorf("error message should contain suggestion: %s", errStr)
	}
}

func TestValidateWorkflow_OIDCTokens(t *testing.T) {
	tests := []struct {
		name    string
		step    *Step
		wantErr bool
	}{
		{
			name: "OIDC token in run",
			step: &Step{
				Run: "curl -H 'Authorization: bearer ${{ secrets.ACTIONS_ID_TOKEN_REQUEST_TOKEN }}' ${{ secrets.ACTIONS_ID_TOKEN_REQUEST_URL }}",
			},
			wantErr: true,
		},
		{
			name: "OIDC token in env",
			step: &Step{
				Env: map[string]string{
					"TOKEN": "${{ secrets.ACTIONS_ID_TOKEN_REQUEST_TOKEN }}",
				},
			},
			wantErr: true,
		},
		{
			name: "OIDC token in with",
			step: &Step{
				Uses: "aws-actions/configure-aws-credentials@v4",
				With: map[string]any{
					"role-to-assume": "arn:aws:iam::123456789:role/my-role",
					"token":          "${{ secrets.ACTIONS_ID_TOKEN_REQUEST_TOKEN }}",
				},
			},
			wantErr: true,
		},
		{
			name: "regular secret usage",
			step: &Step{
				Env: map[string]string{
					"TOKEN": "${{ secrets.MY_SECRET }}",
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workflow := &Workflow{
				Jobs: map[string]*Job{
					"test": {
						RunsOn: "ubuntu-latest",
						Steps:  []*Step{tt.step},
					},
				},
			}

			err := ValidateWorkflow(workflow)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateWorkflow() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateWorkflow_ToolDetection(t *testing.T) {
	tests := []struct {
		name           string
		run            string
		wantWarning    bool
		wantToolInMsg  string
	}{
		{
			name:          "supported go test",
			run:           "go test ./...",
			wantWarning:   false,
		},
		{
			name:          "supported golangci-lint",
			run:           "golangci-lint run ./...",
			wantWarning:   false,
		},
		{
			name:          "supported tsc",
			run:           "tsc --noEmit",
			wantWarning:   false,
		},
		{
			name:          "supported eslint",
			run:           "eslint src/",
			wantWarning:   false,
		},
		{
			name:          "supported cargo test",
			run:           "cargo test",
			wantWarning:   false,
		},
		{
			name:          "unsupported pytest",
			run:           "pytest tests/",
			wantWarning:   true,
			wantToolInMsg: "pytest",
		},
		{
			name:          "unsupported jest",
			run:           "npx jest --coverage",
			wantWarning:   true,
			wantToolInMsg: "jest",
		},
		{
			name:          "unsupported biome",
			run:           "biome check .",
			wantWarning:   true,
			wantToolInMsg: "biome",
		},
		{
			name:          "no tool detected",
			run:           "echo hello",
			wantWarning:   false,
		},
		{
			name:          "npm install (no tool)",
			run:           "npm install",
			wantWarning:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workflow := &Workflow{
				Jobs: map[string]*Job{
					"test": {
						RunsOn: "ubuntu-latest",
						Steps:  []*Step{{Name: "Run", Run: tt.run}},
					},
				},
			}

			err := ValidateWorkflow(workflow)

			if tt.wantWarning {
				if err == nil {
					t.Error("expected warning for unsupported tool, got nil")
					return
				}

				validationErrs, ok := err.(ValidationErrors)
				if !ok {
					t.Errorf("expected ValidationErrors type, got %T", err)
					return
				}

				// Should have warning(s), not blocking errors
				if validationErrs.HasErrors() {
					t.Errorf("expected only warnings, but HasErrors() returned true: %v", err)
				}

				warnings := validationErrs.Warnings()
				if len(warnings) == 0 {
					t.Error("expected at least one warning")
					return
				}

				// Check the warning mentions the tool
				if tt.wantToolInMsg != "" {
					found := false
					for _, w := range warnings {
						if strings.Contains(w.Error(), tt.wantToolInMsg) {
							found = true
							break
						}
					}
					if !found {
						t.Errorf("warning should mention %q, got: %v", tt.wantToolInMsg, err)
					}
				}
			} else {
				// No warning expected - either no error or no warnings
				if err != nil {
					if validationErrs, ok := err.(ValidationErrors); ok {
						// Check there are no tool-parsing warnings
						for _, e := range validationErrs {
							if e.Feature == "tool-parsing" {
								t.Errorf("unexpected tool-parsing warning: %v", e)
							}
						}
					}
				}
			}
		})
	}
}

func TestValidateWorkflow_ToolDetectionShowsSupportedTools(t *testing.T) {
	// Test with an unsupported tool (pytest) to verify supported tools are listed
	workflow := &Workflow{
		Jobs: map[string]*Job{
			"test": {
				RunsOn: "ubuntu-latest",
				Steps:  []*Step{{Name: "Test", Run: "pytest tests/"}},
			},
		},
	}

	err := ValidateWorkflow(workflow)
	if err == nil {
		t.Error("expected warning for pytest, got nil")
		return
	}

	errStr := err.Error()

	// Should mention supported tools in the suggestion
	if !strings.Contains(errStr, "go") || !strings.Contains(errStr, "typescript") {
		t.Errorf("warning should list supported tools (go, typescript), got: %s", errStr)
	}
}

func TestValidateWorkflow_MultipleToolsInStep(t *testing.T) {
	// Test that multiple unsupported tools in a single step are detected
	workflow := &Workflow{
		Jobs: map[string]*Job{
			"test": {
				RunsOn: "ubuntu-latest",
				Steps: []*Step{{
					Name: "Lint and Test",
					Run:  "pytest tests/ && jest --coverage",
				}},
			},
		},
	}

	err := ValidateWorkflow(workflow)
	if err == nil {
		t.Error("expected warning for unsupported tools, got nil")
		return
	}

	errStr := err.Error()

	// Should mention both tools
	if !strings.Contains(errStr, "pytest") {
		t.Errorf("warning should mention pytest, got: %s", errStr)
	}
	if !strings.Contains(errStr, "jest") {
		t.Errorf("warning should mention jest, got: %s", errStr)
	}
}
