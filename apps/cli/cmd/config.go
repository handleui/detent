package cmd

import (
	"fmt"
	"os"

	"github.com/detent/cli/internal/persistence"
	"github.com/detent/cli/internal/tui"
	"github.com/spf13/cobra"
)

var forceReset bool

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage detent configuration",
	Long: `View and manage the global detent configuration.

Settings:
  model        Claude model for AI healing
  timeout      Maximum time per healing run
  budget       Maximum spend per run (0 = unlimited)
  verbose      Show tool calls in real-time`,
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Display current configuration",
	RunE:  runConfigShow,
}

var configResetCmd = &cobra.Command{
	Use:   "reset",
	Short: "Reset configuration to defaults",
	Long: `Reset all settings to default values.

Your API key will be preserved.`,
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
		fmt.Fprintf(os.Stderr, "\n%s Failed to load configuration\n", tui.ErrorStyle.Render("✗"))
		fmt.Fprintf(os.Stderr, "%s %s\n", tui.Bullet(), tui.MutedStyle.Render(err.Error()))
		fmt.Fprintf(os.Stderr, "\n%s %s\n\n", tui.Bullet(), tui.SecondaryStyle.Render("Run: detent config reset"))
		return nil
	}

	healCfg := cfg.Heal.WithDefaults()
	configPath, _ := persistence.GetConfigPath()

	fmt.Println()

	// Section: Authentication
	fmt.Printf("%s\n", tui.SecondaryStyle.Render("Authentication"))
	if cfg.AnthropicAPIKey != "" {
		masked := "····" + cfg.AnthropicAPIKey[max(0, len(cfg.AnthropicAPIKey)-4):]
		fmt.Printf("  API Key      %s\n", tui.PrimaryStyle.Render(masked))
	} else {
		fmt.Printf("  API Key      %s\n", tui.WarningStyle.Render("not configured"))
	}

	// Section: Healing
	fmt.Printf("\n%s\n", tui.SecondaryStyle.Render("Healing"))
	fmt.Printf("  Model        %s\n", tui.PrimaryStyle.Render(healCfg.Model))
	fmt.Printf("  Timeout      %s\n", tui.PrimaryStyle.Render(fmt.Sprintf("%d min", healCfg.TimeoutMins)))

	if healCfg.BudgetUSD == 0 {
		fmt.Printf("  Budget       %s\n", tui.MutedStyle.Render("unlimited"))
	} else {
		fmt.Printf("  Budget       %s\n", tui.PrimaryStyle.Render(fmt.Sprintf("$%.2f", healCfg.BudgetUSD)))
	}

	if healCfg.Verbose {
		fmt.Printf("  Verbose      %s\n", tui.PrimaryStyle.Render("on"))
	} else {
		fmt.Printf("  Verbose      %s\n", tui.MutedStyle.Render("off"))
	}

	// Section: File
	fmt.Printf("\n%s\n", tui.SecondaryStyle.Render("File"))
	fmt.Printf("  %s\n", tui.MutedStyle.Render(configPath))

	fmt.Println()
	return nil
}

func runConfigReset(_ *cobra.Command, _ []string) error {
	if !forceReset {
		fmt.Println()
		fmt.Printf("%s Reset to defaults?\n", tui.WarningStyle.Render("!"))
		fmt.Printf("%s Your API key will be preserved\n", tui.Bullet())
		fmt.Printf("%s All other settings reset to defaults\n\n", tui.Bullet())
		fmt.Printf("Continue? [y/N] ")

		var response string
		if _, err := fmt.Scanln(&response); err != nil || (response != "y" && response != "Y") {
			fmt.Printf("\n%s Cancelled\n\n", tui.MutedStyle.Render("·"))
			return nil
		}
		fmt.Println()
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

	healCfg := newCfg.Heal

	fmt.Printf("%s Configuration reset\n\n", tui.SuccessStyle.Render("✓"))
	fmt.Printf("  Model        %s\n", tui.PrimaryStyle.Render(healCfg.Model))
	fmt.Printf("  Timeout      %s\n", tui.PrimaryStyle.Render(fmt.Sprintf("%d min", healCfg.TimeoutMins)))
	fmt.Printf("  Budget       %s\n", tui.PrimaryStyle.Render(fmt.Sprintf("$%.2f", healCfg.BudgetUSD)))
	fmt.Printf("  Verbose      %s\n\n", tui.MutedStyle.Render("off"))

	return nil
}

func runConfigPath(_ *cobra.Command, _ []string) error {
	path, err := persistence.GetConfigPath()
	if err != nil {
		return err
	}

	fmt.Println(path)

	if _, statErr := os.Stat(path); os.IsNotExist(statErr) {
		fmt.Fprintf(os.Stderr, "%s file does not exist yet\n", tui.Bullet())
	}

	return nil
}
