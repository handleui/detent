package loop

import "strings"

// Model pricing in USD per million tokens.
// Source: https://platform.claude.com/docs/en/about-claude/pricing (as of December 2025)
// Cache pricing: read = 0.1x base input, write (5-min) = 1.25x base input
type modelPricing struct {
	inputPerMillion  float64
	outputPerMillion float64
}

// modelPrefixes maps model name prefixes to their pricing.
// This handles versioned model names like "claude-sonnet-4-5-20250929".
// Order matters: more specific prefixes should come before less specific ones.
var modelPrefixes = []struct {
	prefix  string
	pricing modelPricing
}{
	// Claude 4.5 models
	{"claude-opus-4-5", modelPricing{inputPerMillion: 5.00, outputPerMillion: 25.00}},
	{"claude-sonnet-4-5", modelPricing{inputPerMillion: 3.00, outputPerMillion: 15.00}},
	{"claude-haiku-4-5", modelPricing{inputPerMillion: 1.00, outputPerMillion: 5.00}},
	// Claude 4.1 models
	{"claude-opus-4-1", modelPricing{inputPerMillion: 15.00, outputPerMillion: 75.00}},
	// Claude 4 models
	{"claude-opus-4", modelPricing{inputPerMillion: 15.00, outputPerMillion: 75.00}},
	{"claude-sonnet-4", modelPricing{inputPerMillion: 3.00, outputPerMillion: 15.00}},
	// Claude 3.x models
	{"claude-3-7-sonnet", modelPricing{inputPerMillion: 3.00, outputPerMillion: 15.00}},
	{"claude-3-5-sonnet", modelPricing{inputPerMillion: 3.00, outputPerMillion: 15.00}},
	{"claude-3-5-haiku", modelPricing{inputPerMillion: 0.80, outputPerMillion: 4.00}},
	{"claude-3-opus", modelPricing{inputPerMillion: 15.00, outputPerMillion: 75.00}},
	{"claude-3-haiku", modelPricing{inputPerMillion: 0.25, outputPerMillion: 1.25}},
}

// defaultPricing used for unknown models (sonnet pricing as fallback).
var defaultPricing = modelPricing{inputPerMillion: 3.00, outputPerMillion: 15.00}

// TokenUsage holds all token counts for cost calculation.
type TokenUsage struct {
	InputTokens              int64
	OutputTokens             int64
	CacheCreationInputTokens int64
	CacheReadInputTokens     int64
}

// CalculateCost computes the USD cost for token usage (legacy, without cache).
func CalculateCost(model string, inputTokens, outputTokens int64) float64 {
	return CalculateCostWithCache(model, TokenUsage{
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
	})
}

// CalculateCostWithCache computes the USD cost including cache token pricing.
// Cache read tokens cost 0.1x the base input price.
// Cache write tokens (5-minute TTL) cost 1.25x the base input price.
func CalculateCostWithCache(model string, usage TokenUsage) float64 {
	p := getPricing(model)

	// Standard input tokens at base rate
	inputCost := float64(usage.InputTokens) / 1_000_000 * p.inputPerMillion

	// Cache read tokens at 0.1x base rate
	cacheReadCost := float64(usage.CacheReadInputTokens) / 1_000_000 * p.inputPerMillion * 0.1

	// Cache write tokens at 1.25x base rate
	cacheWriteCost := float64(usage.CacheCreationInputTokens) / 1_000_000 * p.inputPerMillion * 1.25

	// Output tokens at output rate
	outputCost := float64(usage.OutputTokens) / 1_000_000 * p.outputPerMillion

	return inputCost + cacheReadCost + cacheWriteCost + outputCost
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
