package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/detent/cli/internal/git"
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
  verbose         Show tool calls in real-time

Interactive mode:
  Navigate with j/k or arrow keys.
  Toggle between global and local config with 'g'.
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
	repoRoot, _ := filepath.Abs(".")
	repoIdentifier := git.GetRepoIdentifier(repoRoot)
	fmt.Println()
	fmt.Println(tui.Header(Version, repoIdentifier))
	fmt.Println()

	showCfg, err := persistence.LoadWithSources(repoRoot)
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
		fmt.Printf("  API Key      %-20s %s\n", tui.PrimaryStyle.Render(masked), sourceBadge(showCfg.APIKey.Source))
	} else {
		fmt.Printf("  API Key      %s\n", tui.WarningStyle.Render("not configured"))
	}

	// Section: Healing
	fmt.Printf("\n%s\n", tui.SecondaryStyle.Render("Healing"))
	fmt.Printf("  Model        %-20s %s\n", tui.PrimaryStyle.Render(showCfg.Model.Value), sourceBadge(showCfg.Model.Source))
	fmt.Printf("  Timeout      %-20s %s\n", tui.PrimaryStyle.Render(fmt.Sprintf("%d min", showCfg.TimeoutMins.Value)), sourceBadge(showCfg.TimeoutMins.Source))

	// Budget per run
	if showCfg.BudgetPerRunUSD.Value == 0 {
		fmt.Printf("  Per-run      %-20s %s\n", tui.MutedStyle.Render("unlimited"), sourceBadge(showCfg.BudgetPerRunUSD.Source))
	} else {
		fmt.Printf("  Per-run      %-20s %s\n", tui.PrimaryStyle.Render(fmt.Sprintf("$%.2f", showCfg.BudgetPerRunUSD.Value)), sourceBadge(showCfg.BudgetPerRunUSD.Source))
	}

	// Budget monthly
	if showCfg.BudgetMonthlyUSD.Value == 0 {
		fmt.Printf("  Monthly      %-20s %s\n", tui.MutedStyle.Render("unlimited"), sourceBadge(showCfg.BudgetMonthlyUSD.Source))
	} else {
		monthlyDisplay := fmt.Sprintf("$%.2f", showCfg.BudgetMonthlyUSD.Value)
		if showCfg.MonthlySpend > 0 {
			monthlyDisplay = fmt.Sprintf("$%.2f ($%.2f used)", showCfg.BudgetMonthlyUSD.Value, showCfg.MonthlySpend)
		}
		fmt.Printf("  Monthly      %-20s %s\n", tui.PrimaryStyle.Render(monthlyDisplay), sourceBadge(showCfg.BudgetMonthlyUSD.Source))
	}

	// Section: Local Config
	if len(showCfg.ExtraCommands) > 0 {
		fmt.Printf("\n%s\n", tui.SecondaryStyle.Render("Local Config (detent.json)"))
		fmt.Printf("  Commands     %s\n", tui.MutedStyle.Render(strings.Join(showCfg.ExtraCommands, ", ")))
	}

	// Section: File
	fmt.Printf("\n%s\n", tui.SecondaryStyle.Render("File"))
	fmt.Printf("  %s\n", tui.MutedStyle.Render(configPath))

	fmt.Println()
	return nil
}

// sourceBadge returns a styled badge for the given source.
func sourceBadge(source persistence.ValueSource) string {
	if source == persistence.SourceLocal {
		return tui.BadgeLocal()
	}
	return tui.Badge(source.String())
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
	existingCfg, _ := persistence.Load("")
	apiKey := ""
	if existingCfg != nil {
		apiKey = existingCfg.APIKey
	}

	// Create new config with defaults and save
	newCfg, _ := persistence.Load("")
	if apiKey != "" {
		_ = newCfg.SetAPIKey(apiKey)
	}
	if err := newCfg.SaveGlobal(); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	fmt.Printf("%s Configuration reset\n\n", tui.SuccessStyle.Render("✓"))
	fmt.Printf("  Model        %s\n", tui.PrimaryStyle.Render(persistence.DefaultModel))
	fmt.Printf("  Timeout      %s\n", tui.PrimaryStyle.Render(fmt.Sprintf("%d min", persistence.DefaultTimeoutMins)))
	fmt.Printf("  Per-run      %s\n", tui.PrimaryStyle.Render(fmt.Sprintf("$%.2f", persistence.DefaultBudgetPerRunUSD)))
	fmt.Printf("  Monthly      %s\n", tui.MutedStyle.Render("unlimited"))
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

func runConfigSchema(_ *cobra.Command, _ []string) error {
	fmt.Print(schema.JSON)
	return nil
}

func runConfigInteractive(_ *cobra.Command, _ []string) error {
	// Detect if in a git repo
	repoRoot, _ := filepath.Abs(".")
	_, branchErr := os.Stat(filepath.Join(repoRoot, ".git"))
	inRepo := branchErr == nil
	repoIdentifier := git.GetRepoIdentifier(repoRoot)

	if !inRepo {
		repoRoot = "" // Force global-only mode
	}

	// Load config with sources
	cfg, err := persistence.LoadWithSources(repoRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\n%s Failed to load configuration\n", tui.ErrorStyle.Render("✗"))
		fmt.Fprintf(os.Stderr, "%s %s\n\n", tui.Bullet(), tui.MutedStyle.Render(err.Error()))
		return nil
	}

	// Create TUI model
	model := tuiconfig.NewModel(cfg, tuiconfig.Options{
		InRepo:         inRepo,
		GlobalPath:     tuiconfig.GetGlobalPath(),
		LocalPath:      tuiconfig.GetLocalPath(repoRoot),
		Version:        Version,
		RepoIdentifier: repoIdentifier,
	})

	// Run interactive TUI with alt screen to avoid duplication on editor open
	program := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := program.Run(); err != nil {
		return err
	}

	// Outro: show header again after alt screen clears, then status
	fmt.Println()
	fmt.Println(tui.Header(Version, repoIdentifier))
	if model.WasSaved() {
		fmt.Printf("  %s Config updated\n\n", tui.SuccessStyle.Render("✓"))
	} else {
		fmt.Printf("  %s No changes\n\n", tui.MutedStyle.Render("·"))
	}

	return nil
}
