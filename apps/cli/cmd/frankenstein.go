package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/charmbracelet/lipgloss"
	"github.com/detent/cli/internal/git"
	"github.com/detent/cli/internal/heal/client"
	"github.com/detent/cli/internal/heal/tools"
	"github.com/detent/cli/internal/persistence"
	"github.com/spf13/cobra"
)

var monsterMode bool

var (
	frankensteinSuccessStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	frankensteinDimStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	frankensteinTextStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	frankensteinErrorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
)

var frankensteinCmd = &cobra.Command{
	Use:   "frankenstein",
	Short: "Test AI tools integration",
	Long: `Frankenstein tests that Claude can use its tools correctly.

By default, tests read-only tools (glob, grep, read_file).
Use --monster to test all tools including edit_file, run_check, and run_command.`,
	Example: `  # Test read-only tools (fast)
  detent frankenstein

  # Test all tools including write operations
  detent frankenstein --monster`,
	Args:          cobra.NoArgs,
	RunE:          runFrankenstein,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	frankensteinCmd.Flags().BoolVar(&monsterMode, "monster", false, "test all tools including edit_file, run_check, run_command")
}

// toolTracker tracks which tools were called and their results.
type toolTracker struct {
	expected map[string]bool
	called   map[string]bool
	errors   map[string]string
}

func newToolTracker(toolNames []string) *toolTracker {
	expected := make(map[string]bool)
	for _, name := range toolNames {
		expected[name] = true
	}
	return &toolTracker{
		expected: expected,
		called:   make(map[string]bool),
		errors:   make(map[string]string),
	}
}

func (t *toolTracker) recordCall(name string, isError bool, errorMsg string) {
	t.called[name] = true
	if isError {
		t.errors[name] = errorMsg
	}
}

func (t *toolTracker) allCalled() bool {
	for name := range t.expected {
		if !t.called[name] {
			return false
		}
	}
	return true
}

func (t *toolTracker) hasErrors() bool {
	return len(t.errors) > 0
}

func runFrankenstein(cmd *cobra.Command, _ []string) error {
	config, err := persistence.LoadGlobalConfig()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	apiKey, err := ensureAPIKey(config)
	if err != nil {
		return err
	}

	repoPath, err := filepath.Abs(".")
	if err != nil {
		return fmt.Errorf("resolving current directory: %w", err)
	}

	// Verify we're in a git repo
	if _, refsErr := git.GetCurrentRefs(repoPath); refsErr != nil {
		return fmt.Errorf("not a git repository: %w", refsErr)
	}

	c, err := client.New(apiKey)
	if err != nil {
		return err
	}

	// Set up tool context (using current dir, no isolation needed for read-only)
	toolCtx := &tools.Context{
		WorktreePath: repoPath,
		RepoRoot:     repoPath,
		RunID:        "frankenstein-test",
	}

	registry := tools.NewRegistry(toolCtx)
	var expectedTools []string

	// Always register read-only tools
	registry.Register(tools.NewGlobTool(toolCtx))
	registry.Register(tools.NewReadFileTool(toolCtx))
	registry.Register(tools.NewGrepTool(toolCtx))
	expectedTools = []string{"glob", "read_file", "grep"}

	if monsterMode {
		registry.Register(tools.NewEditFileTool(toolCtx))
		registry.Register(tools.NewVerifyTool(toolCtx))
		registry.Register(tools.NewRunCommandTool(toolCtx))
		expectedTools = append(expectedTools, "edit_file", "run_check", "run_command")
	}

	tracker := newToolTracker(expectedTools)

	// Build prompt
	systemPrompt := buildFrankensteinSystemPrompt(monsterMode)
	userPrompt := buildFrankensteinUserPrompt(monsterMode)

	if monsterMode {
		fmt.Fprintf(os.Stderr, "%s %s\n",
			frankensteinDimStyle.Render(">"),
			frankensteinTextStyle.Render("Testing AI tools (full)..."))
	} else {
		fmt.Fprintf(os.Stderr, "%s %s\n",
			frankensteinDimStyle.Render(">"),
			frankensteinTextStyle.Render("Testing AI tools..."))
	}

	// Run the test loop
	ctx, cancel := context.WithTimeout(cmd.Context(), 60*time.Second)
	defer cancel()

	if monsterMode {
		ctx, cancel = context.WithTimeout(cmd.Context(), 120*time.Second)
		defer cancel()
	}

	err = runFrankensteinLoop(ctx, c.API(), registry, tracker, systemPrompt, userPrompt)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %s\n",
			frankensteinErrorStyle.Render("✗"),
			frankensteinTextStyle.Render(fmt.Sprintf("Test failed: %v", err)))
		return err
	}

	// Display results
	displayFrankensteinResults(tracker, expectedTools)

	if tracker.hasErrors() {
		return fmt.Errorf("some tools failed")
	}

	return nil
}

func runFrankensteinLoop(
	ctx context.Context,
	api anthropic.Client,
	registry *tools.Registry,
	tracker *toolTracker,
	systemPrompt, userPrompt string,
) error {
	messages := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock(userPrompt)),
	}

	maxIterations := 10

	for iteration := range maxIterations {
		response, err := api.Messages.New(ctx, anthropic.MessageNewParams{
			Model:     anthropic.ModelClaude3_5HaikuLatest,
			MaxTokens: 1024,
			System: []anthropic.TextBlockParam{
				{Text: systemPrompt},
			},
			Messages: messages,
			Tools:    registry.ToAnthropicTools(),
		})
		if err != nil {
			return fmt.Errorf("API call failed: %w", err)
		}

		// Check if we're done
		if response.StopReason == anthropic.StopReasonEndTurn {
			return nil
		}

		// Process tool calls
		var toolResults []anthropic.ContentBlockParamUnion
		hasToolUse := false

		for i := range response.Content {
			block := response.Content[i]
			if toolUse, ok := block.AsAny().(anthropic.ToolUseBlock); ok {
				hasToolUse = true

				result := registry.Dispatch(ctx, toolUse.Name, json.RawMessage(toolUse.JSON.Input.Raw()))
				tracker.recordCall(toolUse.Name, result.IsError, result.Content)

				toolResults = append(toolResults,
					anthropic.NewToolResultBlock(toolUse.ID, result.Content, result.IsError))
			}
		}

		if !hasToolUse {
			return nil
		}

		messages = append(messages,
			response.ToParam(),
			anthropic.NewUserMessage(toolResults...),
		)

		// Check if all expected tools have been called
		if tracker.allCalled() && iteration > 0 {
			return nil
		}
	}

	return fmt.Errorf("max iterations exceeded")
}

func displayFrankensteinResults(tracker *toolTracker, expectedTools []string) {
	// Sort tools for consistent output
	sort.Strings(expectedTools)

	for _, name := range expectedTools {
		var status string
		if tracker.called[name] {
			if errMsg, hasError := tracker.errors[name]; hasError {
				status = frankensteinErrorStyle.Render(fmt.Sprintf("✗ (%s)", truncateError(errMsg)))
			} else {
				status = frankensteinSuccessStyle.Render("✓")
			}
		} else {
			status = frankensteinDimStyle.Render("-")
		}

		fmt.Fprintf(os.Stderr, "  %s %s %s\n",
			frankensteinDimStyle.Render("↳"),
			frankensteinTextStyle.Render(name),
			status)
	}

	switch {
	case tracker.hasErrors():
		fmt.Fprintf(os.Stderr, "%s %s\n",
			frankensteinErrorStyle.Render("✗"),
			frankensteinTextStyle.Render("Some tools failed"))
	case tracker.allCalled():
		fmt.Fprintf(os.Stderr, "%s %s\n",
			frankensteinSuccessStyle.Render("+"),
			frankensteinTextStyle.Render("All tools working"))
	default:
		fmt.Fprintf(os.Stderr, "%s %s\n",
			frankensteinDimStyle.Render("~"),
			frankensteinTextStyle.Render("Not all tools were tested"))
	}
}

func truncateError(msg string) string {
	if len(msg) > 30 {
		return msg[:27] + "..."
	}
	return msg
}

func buildFrankensteinSystemPrompt(monster bool) string {
	if monster {
		return `Test your tools work. Be minimal and efficient.
1. Use glob to find a .go file
2. Use read_file to read 5 lines from it
3. Use grep to search for "func"
4. Use edit_file to create .detent/test.txt with "test"
5. Use run_check with category "go-build"
6. Use run_command with "go version"
Reply "OK" when done or describe errors.`
	}
	return `Test your tools work. Be minimal and efficient.
1. Use glob to find a .go file
2. Use read_file to read 5 lines from it
3. Use grep to search for "func"
Reply "OK" when done or describe errors.`
}

func buildFrankensteinUserPrompt(monster bool) string {
	if monster {
		return "Test all 6 tools now. Start with glob."
	}
	return "Test all 3 tools now. Start with glob."
}
