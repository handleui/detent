package config

import (
	"fmt"

	"github.com/detent/cli/internal/persistence"
)

// Field describes a single configuration setting.
type Field struct {
	Key         string // Display name
	GlobalOnly  bool   // true = only editable in global config
	Description string // Help text
}

// EditableFields defines all configurable settings in display order.
var EditableFields = []Field{
	{Key: "api_key", GlobalOnly: true, Description: "Anthropic API key"},
	{Key: "model", Description: "Claude model for AI healing"},
	{Key: "timeout", Description: "Maximum time per run (minutes)"},
	{Key: "budget_per_run", Description: "Maximum spend per run (0 = unlimited)"},
	{Key: "budget_monthly", GlobalOnly: true, Description: "Maximum spend per month (0 = unlimited)"},
}

// FieldValue holds the current value and source for a field.
type FieldValue struct {
	DisplayValue string
	Source       persistence.ValueSource
}

// GetFieldValues extracts display values from a ConfigWithSources.
func GetFieldValues(cfg *persistence.ConfigWithSources) map[string]FieldValue {
	values := make(map[string]FieldValue)

	// API Key (masked)
	apiKeyDisplay := ""
	if cfg.APIKey.Value != "" {
		apiKeyDisplay = persistence.MaskAPIKey(cfg.APIKey.Value)
	}
	values["api_key"] = FieldValue{
		DisplayValue: apiKeyDisplay,
		Source:       cfg.APIKey.Source,
	}

	// Model
	values["model"] = FieldValue{
		DisplayValue: cfg.Model.Value,
		Source:       cfg.Model.Source,
	}

	// Timeout
	values["timeout"] = FieldValue{
		DisplayValue: formatTimeout(cfg.TimeoutMins.Value),
		Source:       cfg.TimeoutMins.Source,
	}

	// Budget per run
	values["budget_per_run"] = FieldValue{
		DisplayValue: formatBudget(cfg.BudgetPerRunUSD.Value),
		Source:       cfg.BudgetPerRunUSD.Source,
	}

	// Budget monthly
	values["budget_monthly"] = FieldValue{
		DisplayValue: formatBudgetMonthly(cfg.BudgetMonthlyUSD.Value, cfg.MonthlySpend),
		Source:       cfg.BudgetMonthlyUSD.Source,
	}

	return values
}

func formatTimeout(mins int) string {
	return fmt.Sprintf("%d min", mins)
}

func formatBudget(usd float64) string {
	if usd == 0 {
		return "unlimited"
	}
	return fmt.Sprintf("$%.2f", usd)
}

func formatBudgetMonthly(budget, spent float64) string {
	if budget == 0 {
		return "unlimited"
	}
	if spent > 0 {
		return fmt.Sprintf("$%.2f ($%.2f used)", budget, spent)
	}
	return fmt.Sprintf("$%.2f", budget)
}
