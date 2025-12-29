package loop

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/detent/cli/internal/heal/tools"
)

const (
	// maxIterations is the internal limit on tool call rounds (not user-configurable).
	maxIterations = 50

	// maxTokensPerResponse is the internal limit on tokens per response.
	maxTokensPerResponse = 8192

	// DefaultTimeout is the total timeout for the healing loop.
	DefaultTimeout = 10 * time.Minute

	// DefaultModel is the model to use for healing.
	DefaultModel = anthropic.ModelClaudeSonnet4_5

	// DefaultBudgetUSD is the default spending limit per run.
	DefaultBudgetUSD = 1.00
)

// Config configures the healing loop.
type Config struct {
	Timeout   time.Duration
	Model     anthropic.Model
	BudgetUSD float64 // 0 = unlimited
	Verbose   bool
}

// DefaultConfig returns a config with default values.
func DefaultConfig() Config {
	return Config{
		Timeout:   DefaultTimeout,
		Model:     DefaultModel,
		BudgetUSD: DefaultBudgetUSD,
		Verbose:   false,
	}
}

// ConfigFromSettings creates a loop Config from model, timeout, budget and verbose settings.
// This is the canonical way to configure the healing loop from application settings.
func ConfigFromSettings(model string, timeoutMins int, budgetUSD float64, verbose bool) Config {
	cfg := DefaultConfig()
	if model != "" {
		cfg.Model = anthropic.Model(model)
	}
	if timeoutMins > 0 {
		cfg.Timeout = time.Duration(timeoutMins) * time.Minute
	}
	if budgetUSD >= 0 {
		cfg.BudgetUSD = budgetUSD
	}
	cfg.Verbose = verbose
	return cfg
}

// Result contains the outcome of a healing attempt.
type Result struct {
	// Success indicates whether the healing was successful.
	Success bool

	// Iterations is the number of message rounds completed.
	Iterations int

	// FinalMessage is Claude's final response.
	FinalMessage string

	// ToolCalls is the total number of tool calls made.
	ToolCalls int

	// Duration is how long the loop took.
	Duration time.Duration

	// InputTokens is the total input tokens used across all API calls.
	InputTokens int64

	// OutputTokens is the total output tokens used across all API calls.
	OutputTokens int64

	// CostUSD is the calculated cost in USD based on token usage.
	CostUSD float64

	// BudgetExceeded indicates if the loop stopped due to budget limit.
	BudgetExceeded bool
}

// HealLoop orchestrates the agentic healing process.
type HealLoop struct {
	client        anthropic.Client
	registry      *tools.Registry
	config        Config
	verboseWriter io.Writer
}

// New creates a new healing loop.
func New(client anthropic.Client, registry *tools.Registry, config Config) *HealLoop {
	var verboseWriter io.Writer
	if config.Verbose {
		verboseWriter = os.Stderr
	}
	return &HealLoop{
		client:        client,
		registry:      registry,
		config:        config,
		verboseWriter: verboseWriter,
	}
}

// Run executes the healing loop with the given system prompt and initial user message.
func (l *HealLoop) Run(ctx context.Context, systemPrompt, userPrompt string) (*Result, error) {
	startTime := time.Now()

	// Apply timeout
	ctx, cancel := context.WithTimeout(ctx, l.config.Timeout)
	defer cancel()

	// Initialize conversation
	messages := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock(userPrompt)),
	}

	result := &Result{}
	modelName := string(l.config.Model)

	for iteration := range maxIterations {
		result.Iterations = iteration + 1

		// Make API call
		response, err := l.client.Messages.New(ctx, anthropic.MessageNewParams{
			Model:     l.config.Model,
			MaxTokens: maxTokensPerResponse,
			System: []anthropic.TextBlockParam{
				{Text: systemPrompt},
			},
			Messages: messages,
			Tools:    l.registry.ToAnthropicTools(),
		})
		if err != nil {
			result.Duration = time.Since(startTime)
			result.CostUSD = CalculateCost(modelName, result.InputTokens, result.OutputTokens)
			return result, fmt.Errorf("API call failed: %w", err)
		}

		// Track token usage
		result.InputTokens += response.Usage.InputTokens
		result.OutputTokens += response.Usage.OutputTokens
		result.CostUSD = CalculateCost(modelName, result.InputTokens, result.OutputTokens)

		// Check budget (don't return error - let caller decide how to handle)
		if l.config.BudgetUSD > 0 && result.CostUSD > l.config.BudgetUSD {
			result.BudgetExceeded = true
			result.Duration = time.Since(startTime)
			return result, nil
		}

		// Check if we're done (model finished without requesting tools)
		if response.StopReason == anthropic.StopReasonEndTurn {
			result.FinalMessage = l.extractTextContent(response)
			result.Success = true
			result.Duration = time.Since(startTime)
			return result, nil
		}

		// Process tool calls
		var toolResults []anthropic.ContentBlockParamUnion
		hasToolUse := false

		for i := range response.Content {
			block := response.Content[i]
			if toolUse, ok := block.AsAny().(anthropic.ToolUseBlock); ok {
				hasToolUse = true
				result.ToolCalls++

				// Verbose logging
				l.logToolCall(toolUse.Name, toolUse.JSON.Input.Raw())

				// Dispatch the tool
				toolResult := l.registry.Dispatch(ctx, toolUse.Name, json.RawMessage(toolUse.JSON.Input.Raw()))

				// Create tool result block
				toolResults = append(toolResults,
					anthropic.NewToolResultBlock(toolUse.ID, toolResult.Content, toolResult.IsError))
			}
		}

		if !hasToolUse {
			// Model responded with text only, we're done
			result.FinalMessage = l.extractTextContent(response)
			result.Success = true
			result.Duration = time.Since(startTime)
			return result, nil
		}

		// Add assistant response and tool results to conversation
		messages = append(messages,
			response.ToParam(),
			anthropic.NewUserMessage(toolResults...),
		)
	}

	result.Duration = time.Since(startTime)
	return result, fmt.Errorf("max iterations (%d) exceeded", maxIterations)
}

// extractTextContent extracts text content from a response.
func (l *HealLoop) extractTextContent(response *anthropic.Message) string {
	for i := range response.Content {
		if text, ok := response.Content[i].AsAny().(anthropic.TextBlock); ok {
			return text.Text
		}
	}
	return ""
}

// logToolCall logs a tool call in verbose mode with the key parameter.
func (l *HealLoop) logToolCall(toolName, inputRaw string) {
	if l.verboseWriter == nil {
		return
	}

	// Extract key parameter based on tool type
	keyParam := extractKeyParam(toolName, inputRaw)
	if keyParam != "" {
		_, _ = fmt.Fprintf(l.verboseWriter, "  → %s: %s\n", toolName, keyParam)
	} else {
		_, _ = fmt.Fprintf(l.verboseWriter, "  → %s\n", toolName)
	}
}

// keyParamNames maps tool names to their key parameter for verbose output.
var keyParamNames = map[string]string{
	"read_file":   "path",
	"edit_file":   "path",
	"glob":        "pattern",
	"grep":        "pattern",
	"run_check":   "category",
	"run_command": "command",
}

// extractKeyParam extracts the most relevant parameter for verbose output.
func extractKeyParam(toolName, inputRaw string) string {
	var params map[string]any
	if err := json.Unmarshal([]byte(inputRaw), &params); err != nil {
		return ""
	}

	if paramName, ok := keyParamNames[toolName]; ok {
		if value, exists := params[paramName]; exists {
			if s, ok := value.(string); ok {
				// Truncate long values
				if len(s) > 50 {
					return s[:47] + "..."
				}
				return s
			}
		}
	}

	return ""
}

