package loop

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/detent/cli/internal/heal/tools"
	"github.com/detent/cli/internal/persistence"
)

const (
	// DefaultMaxIterations is the maximum number of tool call rounds.
	DefaultMaxIterations = 20

	// DefaultMaxTokens is the maximum tokens per response.
	DefaultMaxTokens = 4096

	// DefaultTimeout is the total timeout for the healing loop.
	DefaultTimeout = 10 * time.Minute

	// DefaultModel is the model to use for healing.
	DefaultModel = anthropic.ModelClaudeSonnet4_5
)

// Config configures the healing loop.
type Config struct {
	MaxIterations int
	MaxTokens     int
	Timeout       time.Duration
	Model         anthropic.Model
}

// DefaultConfig returns a config with default values.
func DefaultConfig() Config {
	return Config{
		MaxIterations: DefaultMaxIterations,
		MaxTokens:     DefaultMaxTokens,
		Timeout:       DefaultTimeout,
		Model:         DefaultModel,
	}
}

// ConfigFromHealConfig creates a loop Config from a persistence.HealConfig.
// Applies defaults for any zero values.
func ConfigFromHealConfig(hc persistence.HealConfig) Config {
	hc = hc.WithDefaults()
	return Config{
		MaxIterations: hc.MaxIterations,
		MaxTokens:     hc.MaxTokens,
		Timeout:       time.Duration(hc.TimeoutMins) * time.Minute,
		Model:         anthropic.Model(hc.Model),
	}
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
}

// HealLoop orchestrates the agentic healing process.
type HealLoop struct {
	client   anthropic.Client
	registry *tools.Registry
	config   Config
}

// New creates a new healing loop.
func New(client anthropic.Client, registry *tools.Registry, config Config) *HealLoop {
	return &HealLoop{
		client:   client,
		registry: registry,
		config:   config,
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

	for iteration := 0; iteration < l.config.MaxIterations; iteration++ {
		result.Iterations = iteration + 1

		// Make API call
		response, err := l.client.Messages.New(ctx, anthropic.MessageNewParams{
			Model:     l.config.Model,
			MaxTokens: int64(l.config.MaxTokens),
			System: []anthropic.TextBlockParam{
				{Text: systemPrompt},
			},
			Messages: messages,
			Tools:    l.registry.ToAnthropicTools(),
		})
		if err != nil {
			result.Duration = time.Since(startTime)
			return result, fmt.Errorf("API call failed: %w", err)
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
	return result, fmt.Errorf("max iterations (%d) exceeded", l.config.MaxIterations)
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

