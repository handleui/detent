package loop

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/detentsh/core/heal/tools"
)

// mockResponse creates a standard API response with the given parameters.
func mockResponse(stopReason string, content []map[string]any, inputTokens, outputTokens int64) map[string]any {
	return map[string]any{
		"id":            "msg_test123",
		"type":          "message",
		"role":          "assistant",
		"content":       content,
		"model":         "claude-sonnet-4-5-20250514",
		"stop_reason":   stopReason,
		"stop_sequence": nil,
		"usage": map[string]any{
			"input_tokens":               inputTokens,
			"output_tokens":              outputTokens,
			"cache_creation_input_tokens": int64(0),
			"cache_read_input_tokens":     int64(0),
		},
	}
}

// textContent creates a text content block.
func textContent(text string) map[string]any {
	return map[string]any{
		"type": "text",
		"text": text,
	}
}

// toolUseContent creates a tool_use content block.
func toolUseContent(id, name string, input map[string]any) map[string]any {
	return map[string]any{
		"type":  "tool_use",
		"id":    id,
		"name":  name,
		"input": input,
	}
}

// mockServer creates a test server that returns the specified responses in order.
// Each call to the server returns the next response in the queue.
func mockServer(t *testing.T, responses []map[string]any) *httptest.Server {
	t.Helper()
	var callCount atomic.Int32

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		idx := int(callCount.Add(1)) - 1
		if idx >= len(responses) {
			t.Errorf("unexpected API call #%d (only %d responses configured)", idx+1, len(responses))
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(responses[idx]); err != nil {
			t.Errorf("failed to encode response: %v", err)
		}
	}))
}

// mockErrorServer creates a test server that returns errors.
func mockErrorServer(t *testing.T, statusCode int, errorType, errorMessage string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		resp := map[string]any{
			"type": "error",
			"error": map[string]any{
				"type":    errorType,
				"message": errorMessage,
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
}

// mockDelayServer creates a test server that delays before responding.
func mockDelayServer(t *testing.T, delay time.Duration, response map[string]any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(delay)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
}

// createTestClient creates an anthropic client pointing to the test server.
func createTestClient(serverURL string) anthropic.Client {
	return anthropic.NewClient(
		option.WithBaseURL(serverURL),
		option.WithAPIKey("test-api-key"),
		option.WithMaxRetries(0), // Disable retries for testing
	)
}

// createEmptyRegistry creates a tool registry with no tools.
func createEmptyRegistry() *tools.Registry {
	return tools.NewRegistry(&tools.Context{
		WorktreePath: "/test",
		RepoRoot:     "/test",
	})
}

// mockTool implements the Tool interface for testing.
type mockTool struct {
	name        string
	description string
	schema      map[string]any
	executeFunc func(ctx context.Context, input json.RawMessage) (tools.Result, error)
}

func (m *mockTool) Name() string                   { return m.name }
func (m *mockTool) Description() string            { return m.description }
func (m *mockTool) InputSchema() map[string]any    { return m.schema }
func (m *mockTool) Execute(ctx context.Context, input json.RawMessage) (tools.Result, error) {
	if m.executeFunc != nil {
		return m.executeFunc(ctx, input)
	}
	return tools.SuccessResult("mock result"), nil
}

// TestRun_SimpleTextResponse tests that the loop completes successfully
// when the model responds with text only (no tool calls).
func TestRun_SimpleTextResponse(t *testing.T) {
	response := mockResponse("end_turn", []map[string]any{
		textContent("Hello! I've analyzed your code and everything looks good."),
	}, 100, 50)

	server := mockServer(t, []map[string]any{response})
	defer server.Close()

	client := createTestClient(server.URL)
	registry := createEmptyRegistry()
	config := Config{
		Timeout:             time.Minute,
		Model:               anthropic.ModelClaudeSonnet4_5,
		BudgetPerRunUSD:     1.0,
		RemainingMonthlyUSD: -1,
	}

	loop := New(client, registry, config)
	result, err := loop.Run(context.Background(), "You are a helpful assistant.", "Hello!")

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !result.Success {
		t.Error("expected Success to be true")
	}
	if result.Iterations != 1 {
		t.Errorf("expected 1 iteration, got %d", result.Iterations)
	}
	if result.ToolCalls != 0 {
		t.Errorf("expected 0 tool calls, got %d", result.ToolCalls)
	}
	if result.InputTokens != 100 {
		t.Errorf("expected 100 input tokens, got %d", result.InputTokens)
	}
	if result.OutputTokens != 50 {
		t.Errorf("expected 50 output tokens, got %d", result.OutputTokens)
	}
	if !strings.Contains(result.FinalMessage, "everything looks good") {
		t.Errorf("unexpected final message: %s", result.FinalMessage)
	}
}

// TestRun_ToolCallAndResponse tests the full tool call flow:
// model requests tool -> tool executes -> model responds with result.
func TestRun_ToolCallAndResponse(t *testing.T) {
	// First response: model requests a tool call
	toolCallResponse := mockResponse("tool_use", []map[string]any{
		toolUseContent("toolu_123", "read_file", map[string]any{"path": "test.txt"}),
	}, 100, 50)

	// Second response: model gives final answer after receiving tool result
	finalResponse := mockResponse("end_turn", []map[string]any{
		textContent("The file contains test data."),
	}, 150, 30)

	server := mockServer(t, []map[string]any{toolCallResponse, finalResponse})
	defer server.Close()

	client := createTestClient(server.URL)
	registry := createEmptyRegistry()

	// Register a mock tool
	registry.Register(&mockTool{
		name:        "read_file",
		description: "Reads a file",
		schema:      map[string]any{"properties": map[string]any{"path": map[string]any{"type": "string"}}},
		executeFunc: func(ctx context.Context, input json.RawMessage) (tools.Result, error) {
			return tools.SuccessResult("file contents here"), nil
		},
	})

	config := Config{
		Timeout:             time.Minute,
		Model:               anthropic.ModelClaudeSonnet4_5,
		BudgetPerRunUSD:     1.0,
		RemainingMonthlyUSD: -1,
	}

	loop := New(client, registry, config)
	result, err := loop.Run(context.Background(), "You are a helpful assistant.", "Read test.txt")

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !result.Success {
		t.Error("expected Success to be true")
	}
	if result.Iterations != 2 {
		t.Errorf("expected 2 iterations, got %d", result.Iterations)
	}
	if result.ToolCalls != 1 {
		t.Errorf("expected 1 tool call, got %d", result.ToolCalls)
	}
	if result.InputTokens != 250 {
		t.Errorf("expected 250 input tokens, got %d", result.InputTokens)
	}
	if result.OutputTokens != 80 {
		t.Errorf("expected 80 output tokens, got %d", result.OutputTokens)
	}
}

// TestRun_MultipleToolCalls tests handling of multiple sequential tool calls.
func TestRun_MultipleToolCalls(t *testing.T) {
	// Response with two tool calls in one message
	toolCallResponse := mockResponse("tool_use", []map[string]any{
		toolUseContent("toolu_1", "read_file", map[string]any{"path": "a.txt"}),
		toolUseContent("toolu_2", "read_file", map[string]any{"path": "b.txt"}),
	}, 100, 50)

	finalResponse := mockResponse("end_turn", []map[string]any{
		textContent("Both files read successfully."),
	}, 200, 40)

	server := mockServer(t, []map[string]any{toolCallResponse, finalResponse})
	defer server.Close()

	client := createTestClient(server.URL)
	registry := createEmptyRegistry()

	var callCount int
	registry.Register(&mockTool{
		name:        "read_file",
		description: "Reads a file",
		schema:      map[string]any{"properties": map[string]any{"path": map[string]any{"type": "string"}}},
		executeFunc: func(ctx context.Context, input json.RawMessage) (tools.Result, error) {
			callCount++
			return tools.SuccessResult("content"), nil
		},
	})

	config := Config{
		Timeout:             time.Minute,
		Model:               anthropic.ModelClaudeSonnet4_5,
		BudgetPerRunUSD:     1.0,
		RemainingMonthlyUSD: -1,
	}

	loop := New(client, registry, config)
	result, err := loop.Run(context.Background(), "System prompt", "Read both files")

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !result.Success {
		t.Error("expected Success to be true")
	}
	if result.ToolCalls != 2 {
		t.Errorf("expected 2 tool calls, got %d", result.ToolCalls)
	}
	if callCount != 2 {
		t.Errorf("expected tool to be called 2 times, got %d", callCount)
	}
}

// TestRun_BudgetExceeded_PerRun tests that the loop stops when per-run budget is exceeded.
func TestRun_BudgetExceeded_PerRun(t *testing.T) {
	// Generate a response with high token usage that exceeds budget
	// With sonnet pricing ($3/MTok input, $15/MTok output), 1M output tokens = $15
	response := mockResponse("tool_use", []map[string]any{
		toolUseContent("toolu_1", "read_file", map[string]any{"path": "test.txt"}),
	}, 100_000, 100_000) // ~$0.30 input + $1.50 output = $1.80

	server := mockServer(t, []map[string]any{response})
	defer server.Close()

	client := createTestClient(server.URL)
	registry := createEmptyRegistry()
	registry.Register(&mockTool{
		name:        "read_file",
		description: "Reads a file",
		schema:      map[string]any{},
	})

	config := Config{
		Timeout:             time.Minute,
		Model:               anthropic.ModelClaudeSonnet4_5,
		BudgetPerRunUSD:     0.50, // Set a low budget that will be exceeded
		RemainingMonthlyUSD: -1,
	}

	loop := New(client, registry, config)
	result, err := loop.Run(context.Background(), "System prompt", "Test")

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if result.Success {
		t.Error("expected Success to be false when budget exceeded")
	}
	if !result.BudgetExceeded {
		t.Error("expected BudgetExceeded to be true")
	}
	if result.BudgetExceededReason != "per-run" {
		t.Errorf("expected BudgetExceededReason to be 'per-run', got %q", result.BudgetExceededReason)
	}
	if result.CostUSD <= 0 {
		t.Error("expected CostUSD to be calculated")
	}
}

// TestRun_BudgetExceeded_Monthly tests that the loop stops when monthly budget is exceeded.
func TestRun_BudgetExceeded_Monthly(t *testing.T) {
	// Response with enough tokens to exceed a very small monthly budget
	response := mockResponse("tool_use", []map[string]any{
		toolUseContent("toolu_1", "read_file", map[string]any{"path": "test.txt"}),
	}, 100_000, 100_000) // ~$1.80 cost

	server := mockServer(t, []map[string]any{response})
	defer server.Close()

	client := createTestClient(server.URL)
	registry := createEmptyRegistry()
	registry.Register(&mockTool{
		name:        "read_file",
		description: "Reads a file",
		schema:      map[string]any{},
	})

	config := Config{
		Timeout:             time.Minute,
		Model:               anthropic.ModelClaudeSonnet4_5,
		BudgetPerRunUSD:     10.0, // High per-run budget
		RemainingMonthlyUSD: 0.10, // But very low monthly remaining
	}

	loop := New(client, registry, config)
	result, err := loop.Run(context.Background(), "System prompt", "Test")

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if result.Success {
		t.Error("expected Success to be false when budget exceeded")
	}
	if !result.BudgetExceeded {
		t.Error("expected BudgetExceeded to be true")
	}
	if result.BudgetExceededReason != "monthly" {
		t.Errorf("expected BudgetExceededReason to be 'monthly', got %q", result.BudgetExceededReason)
	}
}

// TestRun_BudgetUnlimited tests that unlimited budgets don't trigger budget exceeded.
func TestRun_BudgetUnlimited(t *testing.T) {
	response := mockResponse("end_turn", []map[string]any{
		textContent("Done!"),
	}, 1_000_000, 500_000) // High token usage

	server := mockServer(t, []map[string]any{response})
	defer server.Close()

	client := createTestClient(server.URL)
	registry := createEmptyRegistry()

	config := Config{
		Timeout:             time.Minute,
		Model:               anthropic.ModelClaudeSonnet4_5,
		BudgetPerRunUSD:     0,  // 0 = unlimited
		RemainingMonthlyUSD: -1, // -1 = unlimited
	}

	loop := New(client, registry, config)
	result, err := loop.Run(context.Background(), "System prompt", "Test")

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !result.Success {
		t.Error("expected Success to be true with unlimited budget")
	}
	if result.BudgetExceeded {
		t.Error("expected BudgetExceeded to be false with unlimited budget")
	}
}

// TestRun_APIError tests handling of API errors.
func TestRun_APIError(t *testing.T) {
	server := mockErrorServer(t, http.StatusInternalServerError, "api_error", "Internal server error")
	defer server.Close()

	client := createTestClient(server.URL)
	registry := createEmptyRegistry()
	config := Config{
		Timeout:             time.Minute,
		Model:               anthropic.ModelClaudeSonnet4_5,
		BudgetPerRunUSD:     1.0,
		RemainingMonthlyUSD: -1,
	}

	loop := New(client, registry, config)
	result, err := loop.Run(context.Background(), "System prompt", "Test")

	if err == nil {
		t.Fatal("expected an error for API failure")
	}
	if !strings.Contains(err.Error(), "API call failed") {
		t.Errorf("expected error to contain 'API call failed', got: %v", err)
	}
	if result.Success {
		t.Error("expected Success to be false on API error")
	}
	if result.Iterations != 1 {
		t.Errorf("expected 1 iteration, got %d", result.Iterations)
	}
}

// TestRun_RateLimitError tests handling of rate limit errors.
func TestRun_RateLimitError(t *testing.T) {
	server := mockErrorServer(t, http.StatusTooManyRequests, "rate_limit_error", "Rate limit exceeded")
	defer server.Close()

	client := createTestClient(server.URL)
	registry := createEmptyRegistry()
	config := Config{
		Timeout:             time.Minute,
		Model:               anthropic.ModelClaudeSonnet4_5,
		BudgetPerRunUSD:     1.0,
		RemainingMonthlyUSD: -1,
	}

	loop := New(client, registry, config)
	_, err := loop.Run(context.Background(), "System prompt", "Test")

	if err == nil {
		t.Fatal("expected an error for rate limit")
	}
}

// TestRun_ContextCancellation tests that the loop respects context cancellation.
func TestRun_ContextCancellation(t *testing.T) {
	// Server that takes a long time to respond
	response := mockResponse("end_turn", []map[string]any{textContent("Done")}, 100, 50)
	server := mockDelayServer(t, 5*time.Second, response)
	defer server.Close()

	client := createTestClient(server.URL)
	registry := createEmptyRegistry()
	config := Config{
		Timeout:             time.Minute,
		Model:               anthropic.ModelClaudeSonnet4_5,
		BudgetPerRunUSD:     1.0,
		RemainingMonthlyUSD: -1,
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel after a short delay
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	loop := New(client, registry, config)
	_, err := loop.Run(ctx, "System prompt", "Test")

	if err == nil {
		t.Fatal("expected an error due to context cancellation")
	}
	if !errors.Is(err, context.Canceled) && !strings.Contains(err.Error(), "context canceled") {
		t.Errorf("expected context canceled error, got: %v", err)
	}
}

// TestRun_Timeout tests that the loop respects timeout configuration.
func TestRun_Timeout(t *testing.T) {
	// Server that takes longer than our timeout
	response := mockResponse("end_turn", []map[string]any{textContent("Done")}, 100, 50)
	server := mockDelayServer(t, 5*time.Second, response)
	defer server.Close()

	client := createTestClient(server.URL)
	registry := createEmptyRegistry()
	config := Config{
		Timeout:             100 * time.Millisecond, // Very short timeout
		Model:               anthropic.ModelClaudeSonnet4_5,
		BudgetPerRunUSD:     1.0,
		RemainingMonthlyUSD: -1,
	}

	loop := New(client, registry, config)
	_, err := loop.Run(context.Background(), "System prompt", "Test")

	if err == nil {
		t.Fatal("expected an error due to timeout")
	}
}

// TestRun_UnknownTool tests handling of requests for unknown tools.
func TestRun_UnknownTool(t *testing.T) {
	// Model requests a tool that doesn't exist
	toolCallResponse := mockResponse("tool_use", []map[string]any{
		toolUseContent("toolu_1", "nonexistent_tool", map[string]any{}),
	}, 100, 50)

	// After getting error, model gives final response
	finalResponse := mockResponse("end_turn", []map[string]any{
		textContent("I apologize, that tool doesn't exist."),
	}, 150, 30)

	server := mockServer(t, []map[string]any{toolCallResponse, finalResponse})
	defer server.Close()

	client := createTestClient(server.URL)
	registry := createEmptyRegistry() // No tools registered

	config := Config{
		Timeout:             time.Minute,
		Model:               anthropic.ModelClaudeSonnet4_5,
		BudgetPerRunUSD:     1.0,
		RemainingMonthlyUSD: -1,
	}

	loop := New(client, registry, config)
	result, err := loop.Run(context.Background(), "System prompt", "Use a tool")

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	// The loop should continue (tool returns error result, model can recover)
	if !result.Success {
		t.Error("expected Success to be true (model should recover from unknown tool)")
	}
	if result.ToolCalls != 1 {
		t.Errorf("expected 1 tool call attempt, got %d", result.ToolCalls)
	}
}

// TestRun_ToolExecutionError tests handling of tool execution errors.
func TestRun_ToolExecutionError(t *testing.T) {
	toolCallResponse := mockResponse("tool_use", []map[string]any{
		toolUseContent("toolu_1", "failing_tool", map[string]any{}),
	}, 100, 50)

	finalResponse := mockResponse("end_turn", []map[string]any{
		textContent("The tool failed, but I can continue."),
	}, 150, 30)

	server := mockServer(t, []map[string]any{toolCallResponse, finalResponse})
	defer server.Close()

	client := createTestClient(server.URL)
	registry := createEmptyRegistry()
	registry.Register(&mockTool{
		name:        "failing_tool",
		description: "A tool that fails",
		schema:      map[string]any{},
		executeFunc: func(ctx context.Context, input json.RawMessage) (tools.Result, error) {
			return tools.Result{}, errors.New("tool execution failed")
		},
	})

	config := Config{
		Timeout:             time.Minute,
		Model:               anthropic.ModelClaudeSonnet4_5,
		BudgetPerRunUSD:     1.0,
		RemainingMonthlyUSD: -1,
	}

	loop := New(client, registry, config)
	result, err := loop.Run(context.Background(), "System prompt", "Use the tool")

	if err != nil {
		t.Fatalf("expected no error (tool errors should be recoverable), got: %v", err)
	}
	if !result.Success {
		t.Error("expected Success to be true (model should recover from tool error)")
	}
}

// TestRun_TokenAccumulation tests that tokens are accumulated across iterations.
func TestRun_TokenAccumulation(t *testing.T) {
	responses := []map[string]any{
		mockResponse("tool_use", []map[string]any{
			toolUseContent("toolu_1", "test_tool", map[string]any{}),
		}, 100, 50),
		mockResponse("tool_use", []map[string]any{
			toolUseContent("toolu_2", "test_tool", map[string]any{}),
		}, 200, 75),
		mockResponse("end_turn", []map[string]any{
			textContent("Done!"),
		}, 150, 40),
	}

	server := mockServer(t, responses)
	defer server.Close()

	client := createTestClient(server.URL)
	registry := createEmptyRegistry()
	registry.Register(&mockTool{
		name:        "test_tool",
		description: "Test tool",
		schema:      map[string]any{},
	})

	config := Config{
		Timeout:             time.Minute,
		Model:               anthropic.ModelClaudeSonnet4_5,
		BudgetPerRunUSD:     10.0,
		RemainingMonthlyUSD: -1,
	}

	loop := New(client, registry, config)
	result, err := loop.Run(context.Background(), "System prompt", "Test")

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	expectedInput := int64(100 + 200 + 150)
	expectedOutput := int64(50 + 75 + 40)

	if result.InputTokens != expectedInput {
		t.Errorf("expected %d input tokens, got %d", expectedInput, result.InputTokens)
	}
	if result.OutputTokens != expectedOutput {
		t.Errorf("expected %d output tokens, got %d", expectedOutput, result.OutputTokens)
	}
	if result.Iterations != 3 {
		t.Errorf("expected 3 iterations, got %d", result.Iterations)
	}
	if result.ToolCalls != 2 {
		t.Errorf("expected 2 tool calls, got %d", result.ToolCalls)
	}
}

// TestRun_CacheTokenTracking tests that cache tokens are tracked correctly.
func TestRun_CacheTokenTracking(t *testing.T) {
	response := map[string]any{
		"id":            "msg_test123",
		"type":          "message",
		"role":          "assistant",
		"content":       []map[string]any{textContent("Done!")},
		"model":         "claude-sonnet-4-5-20250514",
		"stop_reason":   "end_turn",
		"stop_sequence": nil,
		"usage": map[string]any{
			"input_tokens":                int64(100),
			"output_tokens":               int64(50),
			"cache_creation_input_tokens": int64(500),
			"cache_read_input_tokens":     int64(200),
		},
	}

	server := mockServer(t, []map[string]any{response})
	defer server.Close()

	client := createTestClient(server.URL)
	registry := createEmptyRegistry()

	config := Config{
		Timeout:             time.Minute,
		Model:               anthropic.ModelClaudeSonnet4_5,
		BudgetPerRunUSD:     10.0,
		RemainingMonthlyUSD: -1,
	}

	loop := New(client, registry, config)
	result, err := loop.Run(context.Background(), "System prompt", "Test")

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if result.CacheCreationInputTokens != 500 {
		t.Errorf("expected 500 cache creation tokens, got %d", result.CacheCreationInputTokens)
	}
	if result.CacheReadInputTokens != 200 {
		t.Errorf("expected 200 cache read tokens, got %d", result.CacheReadInputTokens)
	}
	// Cost should include cache pricing
	if result.CostUSD <= 0 {
		t.Error("expected CostUSD to be calculated with cache tokens")
	}
}

// TestRun_Duration tests that duration is tracked correctly.
func TestRun_Duration(t *testing.T) {
	response := mockResponse("end_turn", []map[string]any{
		textContent("Done!"),
	}, 100, 50)

	// Add a small delay to the server response
	server := mockDelayServer(t, 50*time.Millisecond, response)
	defer server.Close()

	client := createTestClient(server.URL)
	registry := createEmptyRegistry()
	config := Config{
		Timeout:             time.Minute,
		Model:               anthropic.ModelClaudeSonnet4_5,
		BudgetPerRunUSD:     1.0,
		RemainingMonthlyUSD: -1,
	}

	loop := New(client, registry, config)
	result, err := loop.Run(context.Background(), "System prompt", "Test")

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if result.Duration < 50*time.Millisecond {
		t.Errorf("expected duration >= 50ms, got %v", result.Duration)
	}
}

// TestDefaultConfig tests that DefaultConfig returns sensible defaults.
func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Timeout != DefaultTimeout {
		t.Errorf("expected timeout %v, got %v", DefaultTimeout, cfg.Timeout)
	}
	if cfg.Model != DefaultModel {
		t.Errorf("expected model %v, got %v", DefaultModel, cfg.Model)
	}
	if cfg.BudgetPerRunUSD != DefaultBudgetPerRunUSD {
		t.Errorf("expected budget %v, got %v", DefaultBudgetPerRunUSD, cfg.BudgetPerRunUSD)
	}
	if cfg.RemainingMonthlyUSD != -1 {
		t.Errorf("expected remaining monthly -1 (unlimited), got %v", cfg.RemainingMonthlyUSD)
	}
	if cfg.Verbose {
		t.Error("expected Verbose to be false by default")
	}
}

// TestConfigFromSettings tests configuration creation from settings.
func TestConfigFromSettings(t *testing.T) {
	tests := []struct {
		name                string
		model               string
		timeoutMins         int
		budgetPerRun        float64
		remainingMonthly    float64
		expectedModel       anthropic.Model
		expectedTimeout     time.Duration
		expectedBudgetRun   float64
		expectedBudgetMonth float64
	}{
		{
			name:                "all defaults",
			model:               "",
			timeoutMins:         0,
			budgetPerRun:        -1,
			remainingMonthly:    -1,
			expectedModel:       DefaultModel,
			expectedTimeout:     DefaultTimeout,
			expectedBudgetRun:   DefaultBudgetPerRunUSD,
			expectedBudgetMonth: -1,
		},
		{
			name:                "custom model",
			model:               "claude-3-5-haiku-20241022",
			timeoutMins:         0,
			budgetPerRun:        -1,
			remainingMonthly:    -1,
			expectedModel:       "claude-3-5-haiku-20241022",
			expectedTimeout:     DefaultTimeout,
			expectedBudgetRun:   DefaultBudgetPerRunUSD,
			expectedBudgetMonth: -1,
		},
		{
			name:                "custom timeout",
			model:               "",
			timeoutMins:         5,
			budgetPerRun:        -1,
			remainingMonthly:    -1,
			expectedModel:       DefaultModel,
			expectedTimeout:     5 * time.Minute,
			expectedBudgetRun:   DefaultBudgetPerRunUSD,
			expectedBudgetMonth: -1,
		},
		{
			name:                "custom budgets",
			model:               "",
			timeoutMins:         0,
			budgetPerRun:        2.50,
			remainingMonthly:    50.00,
			expectedModel:       DefaultModel,
			expectedTimeout:     DefaultTimeout,
			expectedBudgetRun:   2.50,
			expectedBudgetMonth: 50.00,
		},
		{
			name:                "zero per-run budget (unlimited)",
			model:               "",
			timeoutMins:         0,
			budgetPerRun:        0,
			remainingMonthly:    -1,
			expectedModel:       DefaultModel,
			expectedTimeout:     DefaultTimeout,
			expectedBudgetRun:   0,
			expectedBudgetMonth: -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := ConfigFromSettings(tt.model, tt.timeoutMins, tt.budgetPerRun, tt.remainingMonthly)

			if cfg.Model != tt.expectedModel {
				t.Errorf("expected model %v, got %v", tt.expectedModel, cfg.Model)
			}
			if cfg.Timeout != tt.expectedTimeout {
				t.Errorf("expected timeout %v, got %v", tt.expectedTimeout, cfg.Timeout)
			}
			if cfg.BudgetPerRunUSD != tt.expectedBudgetRun {
				t.Errorf("expected budget per run %v, got %v", tt.expectedBudgetRun, cfg.BudgetPerRunUSD)
			}
			if cfg.RemainingMonthlyUSD != tt.expectedBudgetMonth {
				t.Errorf("expected remaining monthly %v, got %v", tt.expectedBudgetMonth, cfg.RemainingMonthlyUSD)
			}
		})
	}
}

// TestExtractKeyParam tests the key parameter extraction for verbose logging.
func TestExtractKeyParam(t *testing.T) {
	tests := []struct {
		name     string
		toolName string
		input    string
		expected string
	}{
		{
			name:     "read_file path",
			toolName: "read_file",
			input:    `{"path": "src/main.go"}`,
			expected: "src/main.go",
		},
		{
			name:     "glob pattern",
			toolName: "glob",
			input:    `{"pattern": "**/*.ts"}`,
			expected: "**/*.ts",
		},
		{
			name:     "grep pattern",
			toolName: "grep",
			input:    `{"pattern": "TODO:.*"}`,
			expected: "TODO:.*",
		},
		{
			name:     "run_command",
			toolName: "run_command",
			input:    `{"command": "npm test"}`,
			expected: "npm test",
		},
		{
			name:     "unknown tool",
			toolName: "unknown_tool",
			input:    `{"foo": "bar"}`,
			expected: "",
		},
		{
			name:     "invalid json",
			toolName: "read_file",
			input:    `not valid json`,
			expected: "",
		},
		{
			name:     "long value truncation",
			toolName: "read_file",
			input:    `{"path": "this/is/a/very/long/path/that/exceeds/fifty/characters/and/should/be/truncated.go"}`,
			expected: "this/is/a/very/long/path/that/exceeds/fifty/cha...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractKeyParam(tt.toolName, tt.input)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

// TestRun_StopReasonMaxTokens tests handling of max_tokens stop reason.
func TestRun_StopReasonMaxTokens(t *testing.T) {
	// Model responds but hits max tokens limit (text only, no tool use)
	response := mockResponse("max_tokens", []map[string]any{
		textContent("This response was truncated because..."),
	}, 100, 8192)

	server := mockServer(t, []map[string]any{response})
	defer server.Close()

	client := createTestClient(server.URL)
	registry := createEmptyRegistry()
	config := Config{
		Timeout:             time.Minute,
		Model:               anthropic.ModelClaudeSonnet4_5,
		BudgetPerRunUSD:     10.0,
		RemainingMonthlyUSD: -1,
	}

	loop := New(client, registry, config)
	result, err := loop.Run(context.Background(), "System prompt", "Test")

	// max_tokens with text content should still be considered successful
	// (the model gave a response, just truncated)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !result.Success {
		t.Error("expected Success to be true for max_tokens with text content")
	}
}

// TestRun_EmptyContent tests handling of responses with no content.
func TestRun_EmptyContent(t *testing.T) {
	response := mockResponse("end_turn", []map[string]any{}, 100, 50)

	server := mockServer(t, []map[string]any{response})
	defer server.Close()

	client := createTestClient(server.URL)
	registry := createEmptyRegistry()
	config := Config{
		Timeout:             time.Minute,
		Model:               anthropic.ModelClaudeSonnet4_5,
		BudgetPerRunUSD:     1.0,
		RemainingMonthlyUSD: -1,
	}

	loop := New(client, registry, config)
	result, err := loop.Run(context.Background(), "System prompt", "Test")

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !result.Success {
		t.Error("expected Success to be true")
	}
	if result.FinalMessage != "" {
		t.Errorf("expected empty final message, got %q", result.FinalMessage)
	}
}

// TestRun_CostCalculation tests that costs are calculated correctly.
func TestRun_CostCalculation(t *testing.T) {
	// Use precise token counts for predictable cost calculation
	// Sonnet 4.5: $3/MTok input, $15/MTok output
	response := mockResponse("end_turn", []map[string]any{
		textContent("Done!"),
	}, 1_000_000, 100_000) // $3 input + $1.50 output = $4.50

	server := mockServer(t, []map[string]any{response})
	defer server.Close()

	client := createTestClient(server.URL)
	registry := createEmptyRegistry()
	config := Config{
		Timeout:             time.Minute,
		Model:               "claude-sonnet-4-5", // Match the pricing lookup
		BudgetPerRunUSD:     10.0,
		RemainingMonthlyUSD: -1,
	}

	loop := New(client, registry, config)
	result, err := loop.Run(context.Background(), "System prompt", "Test")

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	expectedCost := 3.0 + 1.5 // $3 input + $1.50 output
	if result.CostUSD < expectedCost-0.01 || result.CostUSD > expectedCost+0.01 {
		t.Errorf("expected cost ~$%.2f, got $%.2f", expectedCost, result.CostUSD)
	}
}

// TestRun_BudgetCheckOrder tests that per-run budget is checked before monthly.
func TestRun_BudgetCheckOrder(t *testing.T) {
	// Both budgets would be exceeded, but per-run should be reported first
	response := mockResponse("tool_use", []map[string]any{
		toolUseContent("toolu_1", "test", map[string]any{}),
	}, 1_000_000, 500_000) // High cost exceeding both budgets

	server := mockServer(t, []map[string]any{response})
	defer server.Close()

	client := createTestClient(server.URL)
	registry := createEmptyRegistry()
	registry.Register(&mockTool{name: "test", description: "test", schema: map[string]any{}})

	config := Config{
		Timeout:             time.Minute,
		Model:               anthropic.ModelClaudeSonnet4_5,
		BudgetPerRunUSD:     0.10, // Very low
		RemainingMonthlyUSD: 0.05, // Even lower
	}

	loop := New(client, registry, config)
	result, err := loop.Run(context.Background(), "System prompt", "Test")

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !result.BudgetExceeded {
		t.Error("expected BudgetExceeded to be true")
	}
	// Per-run is checked first in the code
	if result.BudgetExceededReason != "per-run" {
		t.Errorf("expected 'per-run' budget exceeded first, got %q", result.BudgetExceededReason)
	}
}
