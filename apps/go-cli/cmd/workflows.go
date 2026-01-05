package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/detent/go-cli/internal/repo"
	"github.com/detent/go-cli/internal/tui"
	tuiworkflows "github.com/detent/go-cli/internal/tui/workflows"
	"github.com/detentsh/core/git"
	"github.com/detentsh/core/workflow"
	"github.com/spf13/cobra"
)

var workflowsCmd = &cobra.Command{
	Use:   "workflows",
	Short: "Manage which workflow jobs run",
	Long: `Interactively configure which jobs run when using detent check.

Each job can be set to:
  auto  - Default behavior (sensitive jobs skip, others run)
  run   - Force job to run (bypass security skip)
  skip  - Force job to skip

Jobs marked with ! are detected as sensitive (they may publish, release, or deploy).
These jobs are skipped by default to prevent accidental production releases.`,
	RunE:          runWorkflows,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	// Command is registered in root.go
}

func runWorkflows(_ *cobra.Command, _ []string) error {
	// Resolve repo context to get first commit SHA
	repoCtx, err := repo.Resolve(repo.WithFirstCommit())
	if err != nil {
		return fmt.Errorf("resolving repo: %w", err)
	}

	repoSHA := repoCtx.FirstCommitSHA

	// Discover workflows
	workflowDir := filepath.Join(repoCtx.Path, workflowsDir)
	wfPaths, err := workflow.DiscoverWorkflows(workflowDir)
	if err != nil {
		return fmt.Errorf("discovering workflows: %w", err)
	}

	if len(wfPaths) == 0 {
		fmt.Fprintf(os.Stderr, "%s No workflow files found in %s\n", tui.MutedStyle.Render("i"), workflowDir)
		fmt.Println()
		return nil
	}

	// Parse all workflows and collect jobs
	var jobs []tuiworkflows.JobItem
	seenJobs := make(map[string]bool)

	for _, wfPath := range wfPaths {
		wf, parseErr := workflow.ParseWorkflowFile(wfPath)
		if parseErr != nil {
			fmt.Fprintf(os.Stderr, "%s Failed to parse %s: %s\n",
				tui.WarningStyle.Render("!"),
				filepath.Base(wfPath),
				tui.MutedStyle.Render(parseErr.Error()))
			continue
		}

		// Extract job info
		jobInfos := workflow.ExtractJobInfo(wf)
		for _, info := range jobInfos {
			// Skip duplicate job IDs (can happen with multiple workflow files)
			if seenJobs[info.ID] {
				continue
			}
			seenJobs[info.ID] = true

			// Check if job is sensitive
			job := wf.Jobs[info.ID]
			sensitive := workflow.IsSensitiveJob(info.ID, job)

			jobs = append(jobs, tuiworkflows.JobItem{
				ID:        info.ID,
				Name:      info.Name,
				Sensitive: sensitive,
				State:     tuiworkflows.StateAuto,
			})
		}
	}

	if len(jobs) == 0 {
		fmt.Fprintf(os.Stderr, "%s No jobs found in workflow files\n", tui.MutedStyle.Render("i"))
		fmt.Println()
		return nil
	}

	// Load existing overrides
	overrides := cfg.GetJobOverrides(repoSHA)

	// Show job count
	fmt.Println()
	sensitiveCount := 0
	for _, job := range jobs {
		if job.Sensitive {
			sensitiveCount++
		}
	}
	if sensitiveCount > 0 {
		fmt.Fprintf(os.Stderr, "%s Found %d jobs (%d sensitive)\n\n",
			tui.Bullet(), len(jobs), sensitiveCount)
	} else {
		fmt.Fprintf(os.Stderr, "%s Found %d jobs\n\n", tui.Bullet(), len(jobs))
	}

	// Create and run TUI
	model := tuiworkflows.NewModel(tuiworkflows.Options{
		Jobs:      jobs,
		Overrides: overrides,
	})

	program := tea.NewProgram(model)
	if _, runErr := program.Run(); runErr != nil {
		return runErr
	}

	// Handle save
	if model.WasSaved() {
		// Get remote URL for context (best effort)
		remoteURL, _ := git.GetRemoteURL(repoCtx.Path)

		newOverrides := model.GetOverrides()
		cfg.SetJobOverrides(repoSHA, remoteURL, newOverrides)
		if saveErr := cfg.SaveGlobal(); saveErr != nil {
			return fmt.Errorf("saving config: %w", saveErr)
		}
		fmt.Println(tui.ExitSuccess("Job overrides saved"))
	} else {
		fmt.Printf("%s No changes\n", tui.Bullet())
	}
	fmt.Println()

	return nil
}
