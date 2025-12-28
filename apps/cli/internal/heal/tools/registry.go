package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
)

// Registry holds all registered tools and handles dispatch.
type Registry struct {
	tools   map[string]Tool
	toolCtx *Context
}

// NewRegistry creates a new tool registry with the given context.
func NewRegistry(ctx *Context) *Registry {
	return &Registry{
		tools:   make(map[string]Tool),
		toolCtx: ctx,
	}
}

// Register adds a tool to the registry.
func (r *Registry) Register(tool Tool) {
	r.tools[tool.Name()] = tool
}

// Get returns a tool by name, or nil if not found.
func (r *Registry) Get(name string) Tool {
	return r.tools[name]
}

// Dispatch executes a tool by name with the given input.
// Returns a Result that should be sent back to Claude.
func (r *Registry) Dispatch(ctx context.Context, name string, input json.RawMessage) Result {
	tool := r.tools[name]
	if tool == nil {
		return ErrorResult(fmt.Sprintf("unknown tool: %s", name))
	}

	result, err := tool.Execute(ctx, input)
	if err != nil {
		// Only return Go errors for fatal issues (context cancelled, etc.)
		// Tool execution errors should be in the Result
		return ErrorResult(fmt.Sprintf("tool execution failed: %v", err))
	}

	return result
}

// ToAnthropicTools converts registered tools to Anthropic SDK format.
func (r *Registry) ToAnthropicTools() []anthropic.ToolUnionParam {
	tools := make([]anthropic.ToolUnionParam, 0, len(r.tools))

	for _, tool := range r.tools {
		schema := tool.InputSchema()

		// Extract required fields from schema if present
		var required []string
		if req, ok := schema["required"].([]string); ok {
			required = req
		}

		// Extract properties from schema
		var properties any
		if props, ok := schema["properties"]; ok {
			properties = props
		} else {
			properties = schema
		}

		toolParam := anthropic.ToolParam{
			Name:        tool.Name(),
			Description: anthropic.String(tool.Description()),
			InputSchema: anthropic.ToolInputSchemaParam{
				Properties: properties,
				Required:   required,
			},
		}
		tools = append(tools, anthropic.ToolUnionParam{OfTool: &toolParam})
	}

	return tools
}

// Context returns the tool execution context.
func (r *Registry) Context() *Context {
	return r.toolCtx
}

// Names returns the list of registered tool names.
func (r *Registry) Names() []string {
	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	return names
}
