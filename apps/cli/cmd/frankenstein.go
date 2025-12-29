package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/detent/cli/internal/git"
	"github.com/detent/cli/internal/heal/client"
	"github.com/detent/cli/internal/heal/tools"
	"github.com/detent/cli/internal/tui"
	"github.com/handleui/shimmer"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
)

var (
	monsterMode  bool
	verboseMode  bool
)

var frankensteinCmd = &cobra.Command{
	Use:   "frankenstein",
	Short: "Test AI tools integration",
	Long: `Frankenstein tests that Claude can use its tools correctly.

By default, tests read-only tools (glob, grep, read_file).
Use --monster to test all tools including edit_file, run_check, and run_command.

In monster mode, an isolated git worktree is created to safely test write operations.`,
	Example: `  # Test read-only tools (fast)
  detent frankenstein

  # Test all tools including write operations
  detent frankenstein --monster

  # Verbose output showing tool calls
  detent frankenstein --monster --verbose`,
	Args:          cobra.NoArgs,
	RunE:          runFrankenstein,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	frankensteinCmd.Flags().BoolVar(&monsterMode, "monster", false, "test all tools including edit_file, run_check, run_command")
	frankensteinCmd.Flags().BoolVarP(&verboseMode, "verbose", "v", false, "show detailed tool call output")
}

// toolTracker tracks which tools were called and their results.
type toolTracker struct {
	mu       sync.RWMutex
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
	t.mu.Lock()
	defer t.mu.Unlock()
	t.called[name] = true
	if isError {
		t.errors[name] = errorMsg
	}
}

func (t *toolTracker) allCalled() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	for name := range t.expected {
		if !t.called[name] {
			return false
		}
	}
	return true
}

func (t *toolTracker) hasErrors() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.errors) > 0
}

func (t *toolTracker) getCallStatus(name string) (called bool, errMsg string, hasError bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	called = t.called[name]
	errMsg, hasError = t.errors[name]
	return
}

// frankensteinModel is the Bubble Tea model for the frankenstein command.
type frankensteinModel struct {
	shimmer       shimmer.Model
	tracker       *toolTracker
	expectedTools []string
	done          bool
	err           error
	monster       bool
}

// frankensteinDoneMsg signals the test loop completed.
type frankensteinDoneMsg struct {
	err error
}

func newFrankensteinModel(tracker *toolTracker, expectedTools []string, monster bool) frankensteinModel {
	text := "Testing AI tools"
	if monster {
		text = "Testing AI tools (full)"
	}
	return frankensteinModel{
		shimmer:       shimmer.New(text, "#00D787"),
		tracker:       tracker,
		expectedTools: expectedTools,
		monster:       monster,
	}
}

func (m frankensteinModel) Init() tea.Cmd {
	return m.shimmer.Init()
}

func (m frankensteinModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" || msg.String() == "q" {
			return m, tea.Quit
		}

	case frankensteinDoneMsg:
		m.done = true
		m.err = msg.err
		m.shimmer = m.shimmer.SetLoading(false)
		return m, tea.Quit

	case shimmer.TickMsg:
		var cmd tea.Cmd
		m.shimmer, cmd = m.shimmer.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m frankensteinModel) View() string {
	if m.done {
		return m.renderResults()
	}

	return fmt.Sprintf("%s %s\n", tui.Bullet(), m.shimmer.View())
}

func (m frankensteinModel) renderResults() string {
	var out string

	// Header
	out += fmt.Sprintf("%s %s\n", tui.Bullet(), m.shimmer.View())

	// Sort tools for consistent output
	sortedTools := make([]string, len(m.expectedTools))
	copy(sortedTools, m.expectedTools)
	sort.Strings(sortedTools)

	for _, name := range sortedTools {
		var status string
		called, errMsg, hasError := m.tracker.getCallStatus(name)
		if called {
			if hasError {
				status = tui.ErrorStyle.Render(fmt.Sprintf("✗ (%s)", truncateError(errMsg)))
			} else {
				status = tui.SuccessStyle.Render("✓")
			}
		} else {
			status = tui.MutedStyle.Render("-")
		}

		out += fmt.Sprintf("  %s %s %s\n", tui.Arrow(), name, status)
	}

	// Summary
	switch {
	case m.err != nil:
		out += fmt.Sprintf("%s Test failed: %v\n", tui.ErrorStyle.Render("✗"), m.err)
	case m.tracker.hasErrors():
		out += fmt.Sprintf("%s Some tools failed\n", tui.ErrorStyle.Render("✗"))
	case m.tracker.allCalled():
		out += fmt.Sprintf("%s All tools working\n", tui.SuccessStyle.Render("✓"))
	default:
		out += fmt.Sprintf("%s Not all tools tested\n", tui.MutedStyle.Render("·"))
	}

	out += "\n"
	return out
}

func runFrankenstein(cmd *cobra.Command, _ []string) error {
	apiKey, err := ensureAPIKey()
	if err != nil {
		return err
	}

	repoPath, err := filepath.Abs(".")
	if err != nil {
		return fmt.Errorf("resolving current directory: %w", err)
	}

	if _, refsErr := git.GetCurrentRefs(repoPath); refsErr != nil {
		return fmt.Errorf("not a git repository: %w", refsErr)
	}

	c, err := client.New(apiKey)
	if err != nil {
		return err
	}

	// For monster mode, create an isolated worktree to safely test write operations
	worktreePath := repoPath
	var cleanup func()

	if monsterMode {
		if verboseMode {
			fmt.Fprintf(os.Stderr, "%s Creating isolated worktree for testing...\n", tui.Bullet())
		}

		worktreeInfo, worktreeCleanup, worktreeErr := git.PrepareWorktree(cmd.Context(), repoPath, "")
		if worktreeErr != nil {
			return fmt.Errorf("creating test worktree: %w", worktreeErr)
		}
		worktreePath = worktreeInfo.Path
		cleanup = worktreeCleanup

		if verboseMode {
			fmt.Fprintf(os.Stderr, "%s Worktree: %s\n", tui.Arrow(), tui.MutedStyle.Render(worktreePath))
		}

		// Create a test file for edit_file to edit (it cannot create new files)
		testFilePath := filepath.Join(worktreePath, ".detent-test.txt")
		// #nosec G306 - test file with standard permissions
		if writeErr := os.WriteFile(testFilePath, []byte("PLACEHOLDER\n"), 0o644); writeErr != nil {
			cleanup()
			return fmt.Errorf("creating test file: %w", writeErr)
		}
	}

	// Ensure cleanup runs
	if cleanup != nil {
		defer cleanup()
	}

	toolCtx := &tools.Context{
		WorktreePath: worktreePath,
		RepoRoot:     repoPath,
		RunID:        "frankenstein-test",
	}

	registry := tools.NewRegistry(toolCtx)
	var expectedTools []string

	registry.Register(tools.NewGlobTool(toolCtx))
	registry.Register(tools.NewReadFileTool(toolCtx))
	registry.Register(tools.NewGrepTool(toolCtx))
	expectedTools = []string{"glob", "read_file", "grep"}

	if monsterMode {
		registry.Register(tools.NewEditFileTool(toolCtx))
		registry.Register(tools.NewRunCommandTool(toolCtx))
		expectedTools = append(expectedTools, "edit_file", "run_command")
	}

	tracker := newToolTracker(expectedTools)
	systemPrompt := buildFrankensteinSystemPrompt(monsterMode)
	userPrompt := buildFrankensteinUserPrompt(monsterMode)

	timeout := 60 * time.Second
	if monsterMode {
		timeout = 120 * time.Second
	}
	ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
	defer cancel()

	// Check if we have a TTY for the shimmer animation
	isTTY := isatty.IsTerminal(os.Stderr.Fd())

	if isTTY && !verboseMode {
		// Interactive mode with shimmer animation
		return runFrankensteinInteractive(ctx, c.API(), registry, tracker, expectedTools, systemPrompt, userPrompt)
	}

	// Non-interactive mode (no TTY) or verbose mode
	return runFrankensteinSimple(ctx, c.API(), registry, tracker, expectedTools, systemPrompt, userPrompt)
}

func runFrankensteinInteractive(
	ctx context.Context,
	api anthropic.Client,
	registry *tools.Registry,
	tracker *toolTracker,
	expectedTools []string,
	systemPrompt, userPrompt string,
) error {
	model := newFrankensteinModel(tracker, expectedTools, monsterMode)
	p := tea.NewProgram(model, tea.WithOutput(os.Stderr))

	go func() {
		loopErr := runFrankensteinLoop(ctx, api, registry, tracker, systemPrompt, userPrompt, false)
		p.Send(frankensteinDoneMsg{err: loopErr})
	}()

	finalModel, err := p.Run()
	if err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}

	fm, ok := finalModel.(frankensteinModel)
	if !ok {
		return fmt.Errorf("unexpected model type")
	}
	if fm.err != nil {
		return fm.err
	}
	if fm.tracker.hasErrors() {
		return fmt.Errorf("some tools failed")
	}

	return nil
}

func runFrankensteinSimple(
	ctx context.Context,
	api anthropic.Client,
	registry *tools.Registry,
	tracker *toolTracker,
	expectedTools []string,
	systemPrompt, userPrompt string,
) error {
	// Simple non-animated output
	if monsterMode {
		fmt.Fprintf(os.Stderr, "%s %s\n",
			tui.MutedStyle.Render(">"),
			tui.PrimaryStyle.Render("Testing AI tools (full)..."))
	} else {
		fmt.Fprintf(os.Stderr, "%s %s\n",
			tui.MutedStyle.Render(">"),
			tui.PrimaryStyle.Render("Testing AI tools..."))
	}

	err := runFrankensteinLoop(ctx, api, registry, tracker, systemPrompt, userPrompt, verboseMode)

	// Display results
	displayFrankensteinResultsSimple(tracker, expectedTools)
	fmt.Fprintln(os.Stderr)

	if err != nil {
		return err
	}
	if tracker.hasErrors() {
		return fmt.Errorf("some tools failed")
	}
	return nil
}

func displayFrankensteinResultsSimple(tracker *toolTracker, expectedTools []string) {
	sortedTools := make([]string, len(expectedTools))
	copy(sortedTools, expectedTools)
	sort.Strings(sortedTools)

	for _, name := range sortedTools {
		var status string
		called, errMsg, hasError := tracker.getCallStatus(name)
		if called {
			if hasError {
				status = tui.ErrorStyle.Render(fmt.Sprintf("✗ (%s)", truncateError(errMsg)))
			} else {
				status = tui.SuccessStyle.Render("✓")
			}
		} else {
			status = tui.MutedStyle.Render("-")
		}

		fmt.Fprintf(os.Stderr, "  %s %s %s\n",
			tui.MutedStyle.Render("↳"),
			tui.PrimaryStyle.Render(name),
			status)
	}

	switch {
	case tracker.hasErrors():
		fmt.Fprintf(os.Stderr, "%s %s\n",
			tui.ErrorStyle.Render("✗"),
			tui.PrimaryStyle.Render("Some tools failed"))
	case tracker.allCalled():
		fmt.Fprintf(os.Stderr, "%s %s\n",
			tui.SuccessStyle.Render("+"),
			tui.PrimaryStyle.Render("All tools working"))
	default:
		fmt.Fprintf(os.Stderr, "%s %s\n",
			tui.MutedStyle.Render("~"),
			tui.PrimaryStyle.Render("Not all tools were tested"))
	}
}

func runFrankensteinLoop(
	ctx context.Context,
	api anthropic.Client,
	registry *tools.Registry,
	tracker *toolTracker,
	systemPrompt, userPrompt string,
	verbose bool,
) error {
	messages := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock(userPrompt)),
	}

	maxIterations := 10

	for iteration := range maxIterations {
		if verbose {
			fmt.Fprintf(os.Stderr, "%s Iteration %d...\n", tui.Bullet(), iteration+1)
		}

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
			if verbose {
				fmt.Fprintf(os.Stderr, "%s Model finished\n", tui.Arrow())
			}
			return nil
		}

		// Process tool calls
		var toolResults []anthropic.ContentBlockParamUnion
		hasToolUse := false

		for i := range response.Content {
			block := response.Content[i]
			if toolUse, ok := block.AsAny().(anthropic.ToolUseBlock); ok {
				hasToolUse = true

				if verbose {
					fmt.Fprintf(os.Stderr, "%s Tool: %s\n", tui.Arrow(), tui.PrimaryStyle.Render(toolUse.Name))
					fmt.Fprintf(os.Stderr, "    Input: %s\n", tui.MutedStyle.Render(truncateVerbose(toolUse.JSON.Input.Raw(), 100)))
				}

				result := registry.Dispatch(ctx, toolUse.Name, json.RawMessage(toolUse.JSON.Input.Raw()))
				tracker.recordCall(toolUse.Name, result.IsError, result.Content)

				if verbose {
					status := tui.SuccessStyle.Render("OK")
					if result.IsError {
						status = tui.ErrorStyle.Render("ERROR")
					}
					fmt.Fprintf(os.Stderr, "    Result: %s - %s\n", status, tui.MutedStyle.Render(truncateVerbose(result.Content, 80)))
				}

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
			if verbose {
				fmt.Fprintf(os.Stderr, "%s All tools tested\n", tui.Arrow())
			}
			return nil
		}
	}

	return fmt.Errorf("max iterations exceeded")
}

// truncateVerbose truncates a string for verbose output display.
func truncateVerbose(s string, maxLen int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > maxLen {
		return s[:maxLen-3] + "..."
	}
	return s
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
4. Use edit_file to edit .detent-test.txt - replace "PLACEHOLDER" with "test passed"
5. Use run_command with "bun install"
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
		return "Test all 5 tools now. Start with glob."
	}
	return "Test all 3 tools now. Start with glob."
}
