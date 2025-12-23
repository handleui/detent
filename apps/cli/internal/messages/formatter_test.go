package messages

import "testing"

func TestDefaultFormatter_Format(t *testing.T) {
	formatter := NewDefaultFormatter()

	tests := []struct {
		name     string
		matches  []string
		expected string
	}{
		{
			name:     "single match",
			matches:  []string{"error message"},
			expected: "error message",
		},
		{
			name:     "multiple matches - returns last",
			matches:  []string{"group1", "group2", "error message"},
			expected: "error message",
		},
		{
			name:     "match with whitespace",
			matches:  []string{"  error message  "},
			expected: "error message",
		},
		{
			name:     "empty matches",
			matches:  []string{},
			expected: "",
		},
		{
			name:     "nil matches",
			matches:  nil,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatter.Format(tt.matches, nil)
			if result != tt.expected {
				t.Errorf("Format() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestPythonMessageBuilder_BuildMessage(t *testing.T) {
	builder := NewPythonMessageBuilder()

	tests := []struct {
		name          string
		exceptionType string
		message       string
		expected      string
	}{
		{
			name:          "ValueError",
			exceptionType: "ValueError",
			message:       "invalid literal for int() with base 10: 'abc'",
			expected:      "ValueError: invalid literal for int() with base 10: 'abc'",
		},
		{
			name:          "TypeError",
			exceptionType: "TypeError",
			message:       "unsupported operand type(s) for +: 'int' and 'str'",
			expected:      "TypeError: unsupported operand type(s) for +: 'int' and 'str'",
		},
		{
			name:          "RuntimeError",
			exceptionType: "RuntimeError",
			message:       "maximum recursion depth exceeded",
			expected:      "RuntimeError: maximum recursion depth exceeded",
		},
		{
			name:          "empty message",
			exceptionType: "ValueError",
			message:       "",
			expected:      "ValueError: ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := builder.BuildMessage(tt.exceptionType, tt.message)
			if result != tt.expected {
				t.Errorf("BuildMessage() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestRustMessageBuilder_BuildMessage(t *testing.T) {
	builder := NewRustMessageBuilder()

	tests := []struct {
		name     string
		message  string
		expected string
	}{
		{
			name:     "type mismatch",
			message:  "mismatched types",
			expected: "mismatched types",
		},
		{
			name:     "borrow checker error",
			message:  "cannot borrow `x` as mutable more than once at a time",
			expected: "cannot borrow `x` as mutable more than once at a time",
		},
		{
			name:     "empty message",
			message:  "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := builder.BuildMessage(tt.message)
			if result != tt.expected {
				t.Errorf("BuildMessage() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestESLintMessageBuilder_ParseRuleID(t *testing.T) {
	builder := NewESLintMessageBuilder()

	tests := []struct {
		name            string
		input           string
		expectedMessage string
		expectedRuleID  string
	}{
		{
			name:            "simple rule",
			input:           "Unexpected var, use let or const instead no-var",
			expectedMessage: "Unexpected var, use let or const instead",
			expectedRuleID:  "no-var",
		},
		{
			name:            "rule with slash",
			input:           "Unsafe assignment of an any value react/no-unsafe",
			expectedMessage: "Unsafe assignment of an any value",
			expectedRuleID:  "react/no-unsafe",
		},
		{
			name:            "rule with dashes",
			input:           "Expected blank line before this statement padding-line-between-statements",
			expectedMessage: "Expected blank line before this statement",
			expectedRuleID:  "padding-line-between-statements",
		},
		{
			name:            "scoped rule with @",
			input:           "'foo' is assigned a value but never used @typescript-eslint/no-unused-vars",
			expectedMessage: "'foo' is assigned a value but never used",
			expectedRuleID:  "@typescript-eslint/no-unused-vars",
		},
		{
			name:            "message ending with punctuation (no rule match)",
			input:           "Parsing error.",
			expectedMessage: "Parsing error.",
			expectedRuleID:  "",
		},
		{
			name:            "message ending with uppercase (no rule match)",
			input:           "Cannot find module FOO",
			expectedMessage: "Cannot find module FOO",
			expectedRuleID:  "",
		},
		{
			name:            "message ending with single word (no rule match)",
			input:           "Parsing error: Unexpected token",
			expectedMessage: "Parsing error: Unexpected token",
			expectedRuleID:  "",
		},
		{
			name:            "empty message",
			input:           "",
			expectedMessage: "",
			expectedRuleID:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleanMsg, ruleID := builder.ParseRuleID(tt.input)
			if cleanMsg != tt.expectedMessage {
				t.Errorf("ParseRuleID() cleanMsg = %q, want %q", cleanMsg, tt.expectedMessage)
			}
			if ruleID != tt.expectedRuleID {
				t.Errorf("ParseRuleID() ruleID = %q, want %q", ruleID, tt.expectedRuleID)
			}
		})
	}
}

func TestGoMessageBuilder_BuildMessage(t *testing.T) {
	builder := NewGoMessageBuilder()

	tests := []struct {
		name     string
		message  string
		expected string
	}{
		{
			name:     "undeclared variable",
			message:  "undefined: foo",
			expected: "undefined: foo",
		},
		{
			name:     "message with whitespace",
			message:  "  syntax error: unexpected }  ",
			expected: "syntax error: unexpected }",
		},
		{
			name:     "empty message",
			message:  "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := builder.BuildMessage(tt.message)
			if result != tt.expected {
				t.Errorf("BuildMessage() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestTypeScriptMessageBuilder_BuildMessage(t *testing.T) {
	builder := NewTypeScriptMessageBuilder()

	tests := []struct {
		name     string
		message  string
		expected string
	}{
		{
			name:     "type error",
			message:  "Type 'string' is not assignable to type 'number'.",
			expected: "Type 'string' is not assignable to type 'number'.",
		},
		{
			name:     "message with whitespace",
			message:  "  Cannot find name 'foo'.  ",
			expected: "Cannot find name 'foo'.",
		},
		{
			name:     "empty message",
			message:  "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := builder.BuildMessage(tt.message)
			if result != tt.expected {
				t.Errorf("BuildMessage() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestNodeJSMessageBuilder_BuildMessage(t *testing.T) {
	builder := NewNodeJSMessageBuilder()

	result := builder.BuildMessage()
	expected := "Node.js error"
	if result != expected {
		t.Errorf("BuildMessage() = %q, want %q", result, expected)
	}
}

func TestGoTestMessageBuilder_BuildMessage(t *testing.T) {
	builder := NewGoTestMessageBuilder()

	tests := []struct {
		name     string
		testName string
		expected string
	}{
		{
			name:     "simple test name",
			testName: "TestFoo",
			expected: "Test failed: TestFoo",
		},
		{
			name:     "test with subtest",
			testName: "TestFoo/subtest",
			expected: "Test failed: TestFoo/subtest",
		},
		{
			name:     "empty test name",
			testName: "",
			expected: "Test failed: ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := builder.BuildMessage(tt.testName)
			if result != tt.expected {
				t.Errorf("BuildMessage() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestDockerMessageBuilder_BuildMessage(t *testing.T) {
	builder := NewDockerMessageBuilder()

	tests := []struct {
		name     string
		message  string
		expected string
	}{
		{
			name:     "no such container",
			message:  "No such container: abc123",
			expected: "No such container: abc123",
		},
		{
			name:     "message with whitespace",
			message:  "  Cannot connect to the Docker daemon  ",
			expected: "Cannot connect to the Docker daemon",
		},
		{
			name:     "empty message",
			message:  "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := builder.BuildMessage(tt.message)
			if result != tt.expected {
				t.Errorf("BuildMessage() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestGenericMessageBuilder_BuildMessage(t *testing.T) {
	builder := NewGenericMessageBuilder()

	tests := []struct {
		name     string
		message  string
		expected string
	}{
		{
			name:     "generic error",
			message:  "something went wrong",
			expected: "something went wrong",
		},
		{
			name:     "message with whitespace",
			message:  "  error occurred  ",
			expected: "error occurred",
		},
		{
			name:     "empty message",
			message:  "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := builder.BuildMessage(tt.message)
			if result != tt.expected {
				t.Errorf("BuildMessage() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestExitCodeMessageBuilder_BuildMessage(t *testing.T) {
	builder := NewExitCodeMessageBuilder()

	tests := []struct {
		name     string
		exitCode string
		expected string
	}{
		{
			name:     "exit code 1",
			exitCode: "1",
			expected: "Exit code 1",
		},
		{
			name:     "exit code 127",
			exitCode: "127",
			expected: "Exit code 127",
		},
		{
			name:     "exit code 0",
			exitCode: "0",
			expected: "Exit code 0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := builder.BuildMessage(tt.exitCode)
			if result != tt.expected {
				t.Errorf("BuildMessage() = %q, want %q", result, tt.expected)
			}
		})
	}
}
