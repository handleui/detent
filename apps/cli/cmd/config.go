package cmd

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/detent/cli/internal/persistence"
	"github.com/detent/cli/internal/tui"
	tuiconfig "github.com/detent/cli/internal/tui/config"
	"github.com/detent/cli/schema"
	"github.com/spf13/cobra"
)

var forceReset bool

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage detent configuration",
	Long: `View and manage the global detent configuration.

Settings:
  model           Claude model for AI healing
  timeout         Maximum time per healing run
  budget_per_run  Maximum spend per run (0 = unlimited)
  budget_monthly  Maximum spend per month (0 = unlimited)

Interactive mode:
  Navigate with j/k or arrow keys.
  Open config in editor with 'e'.`,
	RunE: runConfigInteractive,
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

var configSchemaCmd = &cobra.Command{
	Use:   "schema",
	Short: "Output JSON schema for config files",
	Long: `Output the JSON schema for detent configuration files.

Use this for IDE support or offline validation:
  detent config schema > ~/.detent/schema.json

Then reference in your config:
  {"$schema": "./schema.json", ...}`,
	Args: cobra.NoArgs,
	RunE: runConfigSchema,
}

func init() {
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configResetCmd)
	configCmd.AddCommand(configPathCmd)
	configCmd.AddCommand(configSchemaCmd)

	configResetCmd.Flags().BoolVarP(&forceReset, "force", "f", false, "skip confirmation")
}

func runConfigShow(_ *cobra.Command, _ []string) error {
	// Show header
	fmt.Println()
	fmt.Println(tui.Header(Version, "config"))
	fmt.Println()

	showCfg, err := persistence.LoadWithSources()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s Failed to load configuration\n", tui.ErrorStyle.Render("✗"))
		fmt.Fprintf(os.Stderr, "%s %s\n", tui.Bullet(), tui.MutedStyle.Render(err.Error()))
		fmt.Fprintf(os.Stderr, "%s %s\n\n", tui.Bullet(), tui.SecondaryStyle.Render("Run: detent config reset"))
		return nil
	}

	configPath, _ := persistence.GetConfigPath()

	// Section: Authentication
	fmt.Printf("%s\n", tui.SecondaryStyle.Render("Authentication"))
	if showCfg.APIKey.Value != "" {
		masked := persistence.MaskAPIKey(showCfg.APIKey.Value)
		fmt.Printf("  API Key      %-20s %s\n", tui.PrimaryStyle.Render(masked), tui.SourceBadge(showCfg.APIKey.Source.String()))
	} else {
		fmt.Printf("  API Key      %s\n", tui.WarningStyle.Render("not configured"))
	}

	// Section: Healing
	fmt.Printf("\n%s\n", tui.SecondaryStyle.Render("Healing"))
	fmt.Printf("  Model        %-20s %s\n", tui.PrimaryStyle.Render(showCfg.Model.Value), tui.SourceBadge(showCfg.Model.Source.String()))
	fmt.Printf("  Timeout      %-20s %s\n", tui.PrimaryStyle.Render(fmt.Sprintf("%d min", showCfg.TimeoutMins.Value)), tui.SourceBadge(showCfg.TimeoutMins.Source.String()))

	// Budget per run
	perRunBudget := persistence.FormatBudget(showCfg.BudgetPerRunUSD.Value)
	perRunStyle := tui.PrimaryStyle
	if showCfg.BudgetPerRunUSD.Value == 0 {
		perRunStyle = tui.MutedStyle
	}
	fmt.Printf("  Per-run      %-20s %s\n", perRunStyle.Render(perRunBudget), tui.SourceBadge(showCfg.BudgetPerRunUSD.Source.String()))

	// Budget monthly
	monthlyBudget := persistence.FormatBudget(showCfg.BudgetMonthlyUSD.Value)
	monthlyStyle := tui.PrimaryStyle
	if showCfg.BudgetMonthlyUSD.Value == 0 {
		monthlyStyle = tui.MutedStyle
	}
	fmt.Printf("  Monthly      %-20s %s\n", monthlyStyle.Render(monthlyBudget), tui.SourceBadge(showCfg.BudgetMonthlyUSD.Source.String()))

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

	// Load existing config to preserve API key and trusted repos
	existingCfg, _ := persistence.Load()
	var apiKey string
	var trustedRepos map[string]persistence.TrustedRepo
	var allowedCommands map[string]persistence.RepoCommands
	if existingCfg != nil {
		apiKey = existingCfg.APIKey
		// Get trusted repos and allowed commands from the underlying global config
		if global := existingCfg.GetGlobal(); global != nil {
			trustedRepos = global.TrustedRepos
			allowedCommands = global.AllowedCommands
		}
	}

	// Create fresh config with only preserved fields
	newCfg := persistence.NewConfigWithDefaults()
	if apiKey != "" {
		newCfg.SetAPIKeyValue(apiKey)
	}
	if trustedRepos != nil {
		newCfg.SetTrustedRepos(trustedRepos)
	}
	if allowedCommands != nil {
		newCfg.SetAllowedCommands(allowedCommands)
	}
	if err := newCfg.SaveGlobal(); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	fmt.Println(tui.ExitSuccess("Configuration reset"))
	fmt.Println()
	fmt.Printf("  Model        %s\n", tui.PrimaryStyle.Render(persistence.DefaultModel))
	fmt.Printf("  Timeout      %s\n", tui.PrimaryStyle.Render(fmt.Sprintf("%d min", persistence.DefaultTimeoutMins)))
	fmt.Printf("  Per-run      %s\n", tui.PrimaryStyle.Render(persistence.FormatBudget(persistence.DefaultBudgetPerRunUSD)))
	fmt.Printf("  Monthly      %s\n\n", tui.MutedStyle.Render(persistence.FormatBudget(0)))

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

func runConfigSchema(_ *cobra.Command, _ []string) error {
	fmt.Print(schema.JSON)
	return nil
}

func runConfigInteractive(_ *cobra.Command, _ []string) error {
	// Load config with sources
	cfg, err := persistence.LoadWithSources()
	if err != nil {
		fmt.Fprintf(os.Stderr, "\n%s Failed to load configuration\n", tui.ErrorStyle.Render("✗"))
		fmt.Fprintf(os.Stderr, "%s %s\n\n", tui.Bullet(), tui.MutedStyle.Render(err.Error()))
		return nil
	}

	// Create TUI model
	model := tuiconfig.NewModel(cfg, tuiconfig.Options{
		GlobalPath: tuiconfig.GetGlobalPath(),
		Version:    Version,
	})

	// Run interactive TUI with alt screen to avoid duplication on editor open
	program := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := program.Run(); err != nil {
		return err
	}

	// Outro: show header again after alt screen clears, then status
	fmt.Println()
	fmt.Println(tui.Header(Version, "config"))
	if model.WasSaved() {
		fmt.Println(tui.ExitSuccess("Configuration saved"))
	} else {
		fmt.Printf("%s No changes\n", tui.Bullet())
	}
	fmt.Println()

	return nil
}
