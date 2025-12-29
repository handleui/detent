package loop

import (
	"math"
	"testing"
)

func TestGetPricing(t *testing.T) {
	tests := []struct {
		name    string
		model   string
		wantIn  float64
		wantOut float64
	}{
		{
			name:    "exact sonnet 4.5 match",
			model:   "claude-sonnet-4-5",
			wantIn:  3.00,
			wantOut: 15.00,
		},
		{
			name:    "versioned sonnet 4.5",
			model:   "claude-sonnet-4-5-20250929",
			wantIn:  3.00,
			wantOut: 15.00,
		},
		{
			name:    "opus 4.5",
			model:   "claude-opus-4-5",
			wantIn:  5.00,
			wantOut: 25.00,
		},
		{
			name:    "opus 4.1",
			model:   "claude-opus-4-1-20250101",
			wantIn:  15.00,
			wantOut: 75.00,
		},
		{
			name:    "opus 4",
			model:   "claude-opus-4",
			wantIn:  15.00,
			wantOut: 75.00,
		},
		{
			name:    "haiku 4.5",
			model:   "claude-haiku-4-5",
			wantIn:  1.00,
			wantOut: 5.00,
		},
		{
			name:    "legacy 3.5 haiku",
			model:   "claude-3-5-haiku-20241022",
			wantIn:  0.80,
			wantOut: 4.00,
		},
		{
			name:    "legacy 3 haiku",
			model:   "claude-3-haiku-20240307",
			wantIn:  0.25,
			wantOut: 1.25,
		},
		{
			name:    "unknown model falls back to sonnet",
			model:   "claude-unknown-model",
			wantIn:  3.00,
			wantOut: 15.00,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := getPricing(tt.model)
			if p.inputPerMillion != tt.wantIn {
				t.Errorf("inputPerMillion = %v, want %v", p.inputPerMillion, tt.wantIn)
			}
			if p.outputPerMillion != tt.wantOut {
				t.Errorf("outputPerMillion = %v, want %v", p.outputPerMillion, tt.wantOut)
			}
		})
	}
}

func TestCalculateCost(t *testing.T) {
	tests := []struct {
		name         string
		model        string
		inputTokens  int64
		outputTokens int64
		wantCost     float64
	}{
		{
			name:         "1M tokens sonnet",
			model:        "claude-sonnet-4-5",
			inputTokens:  1_000_000,
			outputTokens: 1_000_000,
			wantCost:     3.00 + 15.00,
		},
		{
			name:         "10K tokens haiku 4.5",
			model:        "claude-haiku-4-5",
			inputTokens:  10_000,
			outputTokens: 5_000,
			wantCost:     0.01 + 0.025, // $1/MTok input, $5/MTok output
		},
		{
			name:         "zero tokens",
			model:        "claude-sonnet-4-5",
			inputTokens:  0,
			outputTokens: 0,
			wantCost:     0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CalculateCost(tt.model, tt.inputTokens, tt.outputTokens)
			if math.Abs(got-tt.wantCost) > 0.0001 {
				t.Errorf("CalculateCost() = %v, want %v", got, tt.wantCost)
			}
		})
	}
}

func TestCalculateCostWithCache(t *testing.T) {
	tests := []struct {
		name     string
		model    string
		usage    TokenUsage
		wantCost float64
	}{
		{
			name:  "sonnet with cache read (0.1x input price)",
			model: "claude-sonnet-4-5",
			usage: TokenUsage{
				InputTokens:          100_000,
				OutputTokens:         10_000,
				CacheReadInputTokens: 1_000_000, // 1M cache read tokens
			},
			// Input: 100K * $3/MTok = $0.30
			// Cache read: 1M * $3/MTok * 0.1 = $0.30
			// Output: 10K * $15/MTok = $0.15
			wantCost: 0.30 + 0.30 + 0.15,
		},
		{
			name:  "sonnet with cache write (1.25x input price)",
			model: "claude-sonnet-4-5",
			usage: TokenUsage{
				InputTokens:              100_000,
				OutputTokens:             10_000,
				CacheCreationInputTokens: 1_000_000, // 1M cache write tokens
			},
			// Input: 100K * $3/MTok = $0.30
			// Cache write: 1M * $3/MTok * 1.25 = $3.75
			// Output: 10K * $15/MTok = $0.15
			wantCost: 0.30 + 3.75 + 0.15,
		},
		{
			name:  "opus 4.5 with all token types",
			model: "claude-opus-4-5",
			usage: TokenUsage{
				InputTokens:              500_000,
				OutputTokens:             100_000,
				CacheCreationInputTokens: 200_000,
				CacheReadInputTokens:     300_000,
			},
			// Input: 500K * $5/MTok = $2.50
			// Cache write: 200K * $5/MTok * 1.25 = $1.25
			// Cache read: 300K * $5/MTok * 0.1 = $0.15
			// Output: 100K * $25/MTok = $2.50
			wantCost: 2.50 + 1.25 + 0.15 + 2.50,
		},
		{
			name:  "zero tokens",
			model: "claude-sonnet-4-5",
			usage: TokenUsage{},
			wantCost: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CalculateCostWithCache(tt.model, tt.usage)
			if math.Abs(got-tt.wantCost) > 0.0001 {
				t.Errorf("CalculateCostWithCache() = %v, want %v", got, tt.wantCost)
			}
		})
	}
}
