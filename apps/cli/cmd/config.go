package cmd

import (
	"fmt"
	"os"

	"github.com/charmbracelet/lipgloss"
	"github.com/detent/cli/internal/persistence"
	"github.com/goccy/go-yaml"
	"github.com/spf13/cobra"
)

var (
	configSuccessStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	configDimStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	configTextStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	configKeyStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("45"))
	configValueStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("220"))
)

var forceReset bool

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage detent configuration",
	Long: `View and manage the global detent configuration stored in ~/.detent/config.yaml.

The configuration file contains settings for the heal command, including:
  - model: Claude model to use (claude-sonnet-4-5, claude-opus-4-5, claude-haiku-4-5)
  - max_iterations: Maximum tool call rounds (default: 20)
  - max_tokens: Max tokens per response (default: 4096)
  - timeout_mins: Total timeout in minutes (default: 10)`,
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Display current configuration",
	RunE:  runConfigShow,
}

var configResetCmd = &cobra.Command{
	Use:   "reset",
	Short: "Reset configuration to defaults",
	Long: `Reset the configuration file to default values.

This will preserve your API key but reset all other settings to defaults.
Use --force to skip confirmation.`,
	RunE: runConfigReset,
}

var configPathCmd = &cobra.Command{
	Use:   "path",
	Short: "Show configuration file path",
	RunE:  runConfigPath,
}

func init() {
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configResetCmd)
	configCmd.AddCommand(configPathCmd)

	configResetCmd.Flags().BoolVarP(&forceReset, "force", "f", false, "skip confirmation")
}

func runConfigShow(_ *cobra.Command, _ []string) error {
	cfg, err := persistence.LoadGlobalConfig()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Apply defaults for display
	healWithDefaults := cfg.Heal.WithDefaults()

	fmt.Println()
	fmt.Printf("%s\n\n", configTextStyle.Render("Current Configuration"))

	// API Key (masked - show only last 4 chars for security)
	apiKeyDisplay := configDimStyle.Render("(not set)")
	if cfg.AnthropicAPIKey != "" {
		masked := "****" + cfg.AnthropicAPIKey[max(0, len(cfg.AnthropicAPIKey)-4):]
		apiKeyDisplay = configValueStyle.Render(masked)
	}
	fmt.Printf("  %s %s\n", configKeyStyle.Render("anthropic_api_key:"), apiKeyDisplay)

	fmt.Printf("\n  %s\n", configKeyStyle.Render("heal:"))
	fmt.Printf("    %s %s\n", configKeyStyle.Render("model:"), configValueStyle.Render(healWithDefaults.Model))
	fmt.Printf("    %s %s\n", configKeyStyle.Render("max_iterations:"), configValueStyle.Render(fmt.Sprintf("%d", healWithDefaults.MaxIterations)))
	fmt.Printf("    %s %s\n", configKeyStyle.Render("max_tokens:"), configValueStyle.Render(fmt.Sprintf("%d", healWithDefaults.MaxTokens)))
	fmt.Printf("    %s %s\n", configKeyStyle.Render("timeout_mins:"), configValueStyle.Render(fmt.Sprintf("%d", healWithDefaults.TimeoutMins)))

	// Show if any values are using defaults
	if cfg.Heal.Model == "" || cfg.Heal.MaxIterations == 0 || cfg.Heal.MaxTokens == 0 || cfg.Heal.TimeoutMins == 0 {
		fmt.Printf("\n  %s\n", configDimStyle.Render("(some values using defaults)"))
	}

	fmt.Println()
	return nil
}

func runConfigReset(_ *cobra.Command, _ []string) error {
	if !forceReset {
		fmt.Printf("%s Reset configuration to defaults? [y/N] ", configTextStyle.Render("?"))
		var response string
		if _, err := fmt.Scanln(&response); err != nil || (response != "y" && response != "Y") {
			fmt.Println(configDimStyle.Render("Cancelled"))
			return nil
		}
	}

	// Load existing config to preserve API key
	existingCfg, _ := persistence.LoadGlobalConfig()
	apiKey := ""
	if existingCfg != nil {
		apiKey = existingCfg.AnthropicAPIKey
	}

	// Create new config with defaults
	newCfg := &persistence.GlobalConfig{
		AnthropicAPIKey: apiKey,
		Heal:            persistence.DefaultHealConfig(),
	}

	if err := persistence.SaveGlobalConfig(newCfg); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	fmt.Printf("%s %s\n", configSuccessStyle.Render("+"), configTextStyle.Render("Configuration reset to defaults"))

	// Show the new config (mask API key for security)
	displayCfg := *newCfg
	if displayCfg.AnthropicAPIKey != "" {
		displayCfg.AnthropicAPIKey = "****" + displayCfg.AnthropicAPIKey[max(0, len(displayCfg.AnthropicAPIKey)-4):]
	}
	data, err := yaml.Marshal(displayCfg)
	if err != nil {
		return fmt.Errorf("marshaling config for display: %w", err)
	}
	fmt.Printf("\n%s\n", configDimStyle.Render(string(data)))

	return nil
}

func runConfigPath(_ *cobra.Command, _ []string) error {
	path, err := persistence.GetConfigPath()
	if err != nil {
		return err
	}

	fmt.Println(path)

	// Check if file exists
	if _, statErr := os.Stat(path); os.IsNotExist(statErr) {
		fmt.Fprintf(os.Stderr, "%s\n", configDimStyle.Render("(file does not exist yet)"))
	}

	return nil
}
