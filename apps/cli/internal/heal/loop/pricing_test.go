package loop

import (
	"math"
	"testing"
)

func TestGetPricing(t *testing.T) {
	tests := []struct {
		name     string
		model    string
		wantIn   float64
		wantOut  float64
	}{
		{
			name:    "exact sonnet match",
			model:   "claude-sonnet-4-5",
			wantIn:  3.00,
			wantOut: 15.00,
		},
		{
			name:    "versioned sonnet",
			model:   "claude-sonnet-4-5-20250929",
			wantIn:  3.00,
			wantOut: 15.00,
		},
		{
			name:    "opus",
			model:   "claude-opus-4-5",
			wantIn:  15.00,
			wantOut: 75.00,
		},
		{
			name:    "haiku",
			model:   "claude-haiku-4-5",
			wantIn:  0.80,
			wantOut: 4.00,
		},
		{
			name:    "legacy 3.5 haiku",
			model:   "claude-3-5-haiku-20241022",
			wantIn:  0.80,
			wantOut: 4.00,
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
			name:         "10K tokens haiku",
			model:        "claude-haiku-4-5",
			inputTokens:  10_000,
			outputTokens: 5_000,
			wantCost:     0.008 + 0.020,
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
