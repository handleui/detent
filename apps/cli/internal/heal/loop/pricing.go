package loop

import "strings"

// Model pricing in USD per million tokens.
// Source: https://www.anthropic.com/pricing (as of 2025)
type modelPricing struct {
	inputPerMillion  float64
	outputPerMillion float64
}

// modelPrefixes maps model name prefixes to their pricing.
// This handles versioned model names like "claude-sonnet-4-5-20250929".
var modelPrefixes = []struct {
	prefix  string
	pricing modelPricing
}{
	{"claude-opus-4-5", modelPricing{inputPerMillion: 15.00, outputPerMillion: 75.00}},
	{"claude-opus-4", modelPricing{inputPerMillion: 15.00, outputPerMillion: 75.00}},
	{"claude-sonnet-4-5", modelPricing{inputPerMillion: 3.00, outputPerMillion: 15.00}},
	{"claude-sonnet-4", modelPricing{inputPerMillion: 3.00, outputPerMillion: 15.00}},
	{"claude-haiku-4-5", modelPricing{inputPerMillion: 0.80, outputPerMillion: 4.00}},
	{"claude-3-5-haiku", modelPricing{inputPerMillion: 0.80, outputPerMillion: 4.00}},
	{"claude-3-7-sonnet", modelPricing{inputPerMillion: 3.00, outputPerMillion: 15.00}},
	{"claude-3-5-sonnet", modelPricing{inputPerMillion: 3.00, outputPerMillion: 15.00}},
	{"claude-3-opus", modelPricing{inputPerMillion: 15.00, outputPerMillion: 75.00}},
}

// defaultPricing used for unknown models (sonnet pricing as fallback).
var defaultPricing = modelPricing{inputPerMillion: 3.00, outputPerMillion: 15.00}

// CalculateCost computes the USD cost for token usage.
func CalculateCost(model string, inputTokens, outputTokens int64) float64 {
	p := getPricing(model)

	inputCost := float64(inputTokens) / 1_000_000 * p.inputPerMillion
	outputCost := float64(outputTokens) / 1_000_000 * p.outputPerMillion

	return inputCost + outputCost
}

// getPricing returns the pricing for a model, using prefix matching
// to handle versioned model names like "claude-sonnet-4-5-20250929".
func getPricing(model string) modelPricing {
	for _, mp := range modelPrefixes {
		if strings.HasPrefix(model, mp.prefix) {
			return mp.pricing
		}
	}
	return defaultPricing
}
