package config

import (
	"fmt"

	"github.com/detent/cli/internal/persistence"
)

// FieldType indicates the type of a configuration field.
type FieldType int

// Field types for configuration values.
const (
	FieldString FieldType = iota
	FieldInt
	FieldFloat
	FieldBool
)

// Field describes a single configuration setting.
type Field struct {
	Key         string    // Display name
	FieldType   FieldType // Value type
	GlobalOnly  bool      // true = only editable in global config
	Description string    // Help text
}

// EditableFields defines all configurable settings in display order.
var EditableFields = []Field{
	{Key: "api_key", FieldType: FieldString, GlobalOnly: true, Description: "Anthropic API key"},
	{Key: "model", FieldType: FieldString, Description: "Claude model for AI healing"},
	{Key: "timeout", FieldType: FieldInt, Description: "Maximum time per run (minutes)"},
	{Key: "budget", FieldType: FieldFloat, Description: "Maximum spend per run (0 = unlimited)"},
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

	// Budget
	values["budget"] = FieldValue{
		DisplayValue: formatBudget(cfg.BudgetUSD.Value),
		Source:       cfg.BudgetUSD.Source,
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
