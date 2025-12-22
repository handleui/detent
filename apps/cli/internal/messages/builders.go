package messages

import (
	"regexp"
	"strings"
)

// PythonMessageBuilder handles Python error message construction.
// Python tracebacks are multi-line and require combining the exception
// type with the message (e.g., "ValueError: invalid literal for int()").
type PythonMessageBuilder struct{}

// NewPythonMessageBuilder creates a new PythonMessageBuilder instance.
func NewPythonMessageBuilder() *PythonMessageBuilder {
	return &PythonMessageBuilder{}
}

// BuildMessage combines Python exception type and message.
// This matches the format: "ExceptionType: error message"
// For example: "ValueError: invalid literal for int() with base 10: 'abc'"
func (b *PythonMessageBuilder) BuildMessage(exceptionType, message string) string {
	return exceptionType + ": " + message
}

// RustMessageBuilder handles Rust compiler error message construction.
// Rust errors are multi-line with the error code appearing before the location.
type RustMessageBuilder struct{}

// NewRustMessageBuilder creates a new RustMessageBuilder instance.
func NewRustMessageBuilder() *RustMessageBuilder {
	return &RustMessageBuilder{}
}

// BuildMessage combines Rust error code and message.
// The error code is already included in the message by the rustc compiler,
// so this just returns the message as-is.
// Example: "mismatched types" (the error code E0308 is stored separately in RuleID)
func (b *RustMessageBuilder) BuildMessage(message string) string {
	return message
}

// ESLintMessageBuilder handles ESLint error message parsing.
// ESLint messages include rule IDs at the end (e.g., "Message text rule-name").
type ESLintMessageBuilder struct {
	rulePattern *regexp.Regexp
}

// NewESLintMessageBuilder creates a new ESLintMessageBuilder instance.
func NewESLintMessageBuilder() *ESLintMessageBuilder {
	// ESLint rule name pattern: splits "Message text rule-name" into message and rule
	// Group 1: message text
	// Group 2: rule name (e.g., "no-var", "react/no-unsafe")
	// This matches the pattern from errors/patterns.go exactly
	rulePattern := regexp.MustCompile(`^(.+?)\s+([a-z0-9]+(?:[/@-][a-z0-9]+)*)$`)
	return &ESLintMessageBuilder{
		rulePattern: rulePattern,
	}
}

// ParseRuleID extracts the ESLint rule ID from a message.
// It returns the cleaned message (without rule ID) and the rule ID separately.
// If no rule ID is found, it returns the original message and an empty rule ID.
//
// Example inputs and outputs:
//   "Unexpected var, use let or const instead no-var" -> ("Unexpected var, use let or const instead", "no-var")
//   "'foo' is assigned a value but never used @typescript-eslint/no-unused-vars" -> ("'foo' is assigned a value but never used", "@typescript-eslint/no-unused-vars")
//   "Parsing error" -> ("Parsing error", "")
func (b *ESLintMessageBuilder) ParseRuleID(message string) (cleanMsg, ruleID string) {
	if match := b.rulePattern.FindStringSubmatch(message); match != nil {
		return strings.TrimSpace(match[1]), match[2]
	}
	return message, ""
}

// GoMessageBuilder handles Go compiler error message construction.
type GoMessageBuilder struct{}

// NewGoMessageBuilder creates a new GoMessageBuilder instance.
func NewGoMessageBuilder() *GoMessageBuilder {
	return &GoMessageBuilder{}
}

// BuildMessage formats Go compiler error messages.
// Go errors are already well-formatted by the compiler, so this just
// returns the message with whitespace trimmed.
func (b *GoMessageBuilder) BuildMessage(message string) string {
	return strings.TrimSpace(message)
}

// TypeScriptMessageBuilder handles TypeScript compiler error message construction.
type TypeScriptMessageBuilder struct{}

// NewTypeScriptMessageBuilder creates a new TypeScriptMessageBuilder instance.
func NewTypeScriptMessageBuilder() *TypeScriptMessageBuilder {
	return &TypeScriptMessageBuilder{}
}

// BuildMessage formats TypeScript compiler error messages.
// TypeScript errors are already well-formatted by tsc, so this just
// returns the message with whitespace trimmed.
func (b *TypeScriptMessageBuilder) BuildMessage(message string) string {
	return strings.TrimSpace(message)
}

// NodeJSMessageBuilder handles Node.js runtime error message construction.
type NodeJSMessageBuilder struct{}

// NewNodeJSMessageBuilder creates a new NodeJSMessageBuilder instance.
func NewNodeJSMessageBuilder() *NodeJSMessageBuilder {
	return &NodeJSMessageBuilder{}
}

// BuildMessage formats Node.js runtime error messages.
// For stack traces, we use a generic message since the actual error
// is usually on a different line.
func (b *NodeJSMessageBuilder) BuildMessage() string {
	return "Node.js error"
}

// GoTestMessageBuilder handles Go test failure message construction.
type GoTestMessageBuilder struct{}

// NewGoTestMessageBuilder creates a new GoTestMessageBuilder instance.
func NewGoTestMessageBuilder() *GoTestMessageBuilder {
	return &GoTestMessageBuilder{}
}

// BuildMessage formats Go test failure messages.
// It prepends "Test failed: " to the test name.
func (b *GoTestMessageBuilder) BuildMessage(testName string) string {
	return "Test failed: " + testName
}

// DockerMessageBuilder handles Docker infrastructure error message construction.
type DockerMessageBuilder struct{}

// NewDockerMessageBuilder creates a new DockerMessageBuilder instance.
func NewDockerMessageBuilder() *DockerMessageBuilder {
	return &DockerMessageBuilder{}
}

// BuildMessage formats Docker error messages.
// Docker errors are returned as-is with whitespace trimmed.
func (b *DockerMessageBuilder) BuildMessage(message string) string {
	return strings.TrimSpace(message)
}

// GenericMessageBuilder handles generic error message construction.
type GenericMessageBuilder struct{}

// NewGenericMessageBuilder creates a new GenericMessageBuilder instance.
func NewGenericMessageBuilder() *GenericMessageBuilder {
	return &GenericMessageBuilder{}
}

// BuildMessage formats generic error messages.
// It simply trims whitespace from the message.
func (b *GenericMessageBuilder) BuildMessage(message string) string {
	return strings.TrimSpace(message)
}

// ExitCodeMessageBuilder handles exit code message construction.
type ExitCodeMessageBuilder struct{}

// NewExitCodeMessageBuilder creates a new ExitCodeMessageBuilder instance.
func NewExitCodeMessageBuilder() *ExitCodeMessageBuilder {
	return &ExitCodeMessageBuilder{}
}

// BuildMessage formats exit code messages.
// It creates a message like "Exit code 1".
func (b *ExitCodeMessageBuilder) BuildMessage(exitCode string) string {
	return "Exit code " + exitCode
}
