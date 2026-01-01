package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

const (
	maxGlobResults = 200 // Maximum number of files to return
)

// GlobTool finds files matching glob patterns.
type GlobTool struct {
	ctx *Context
}

// NewGlobTool creates a new glob tool.
func NewGlobTool(ctx *Context) *GlobTool {
	return &GlobTool{ctx: ctx}
}

// Name implements Tool.
func (t *GlobTool) Name() string {
	return "glob"
}

// Description implements Tool.
func (t *GlobTool) Description() string {
	return "Find files matching a glob pattern. Supports ** for recursive matching. Returns file paths sorted by modification time (newest first)."
}

// InputSchema implements Tool.
func (t *GlobTool) InputSchema() map[string]any {
	return NewSchema().
		AddString("pattern", "Glob pattern to match (e.g., '**/*.go', 'src/**/*.ts')").
		AddOptionalString("path", "Directory to search in (relative to repo root, default: root)").
		Build()
}

type globInput struct {
	Pattern string `json:"pattern"`
	Path    string `json:"path"`
}

type fileWithTime struct {
	path    string
	modTime int64
}

// Execute implements Tool.
func (t *GlobTool) Execute(ctx context.Context, input json.RawMessage) (Result, error) {
	// Check for early cancellation
	if err := ctx.Err(); err != nil {
		return ErrorResult("operation cancelled"), nil
	}

	var in globInput
	if err := json.Unmarshal(input, &in); err != nil {
		return ErrorResult("invalid input: " + err.Error()), nil
	}

	if in.Pattern == "" {
		return ErrorResult("pattern is required"), nil
	}

	// Determine search path
	searchPath := t.ctx.WorktreePath
	displayPath := "."
	if in.Path != "" {
		absPath, errResult := t.ctx.ValidatePath(in.Path)
		if errResult != nil {
			return *errResult, nil
		}
		searchPath = absPath
		displayPath = in.Path
	}

	// Check if search path exists and is a directory
	info, err := os.Stat(searchPath)
	if err != nil {
		if os.IsNotExist(err) {
			return ErrorResult("path not found: " + displayPath), nil
		}
		return ErrorResult("failed to access path: " + err.Error()), nil
	}
	if !info.IsDir() {
		return ErrorResult("path is not a directory: " + displayPath), nil
	}

	// Use doublestar to match files
	fsys := os.DirFS(searchPath)
	matches, err := doublestar.Glob(fsys, in.Pattern)
	if err != nil {
		return ErrorResult("invalid glob pattern: " + err.Error()), nil
	}

	if len(matches) == 0 {
		return SuccessResult("no files match pattern: " + in.Pattern), nil
	}

	// Get modification times for sorting
	files := make([]fileWithTime, 0, len(matches))
	for _, match := range matches {
		// Check for cancellation periodically during file processing
		if ctx.Err() != nil {
			return ErrorResult("operation cancelled"), nil
		}

		fullPath := filepath.Join(searchPath, match)
		fileInfo, statErr := os.Stat(fullPath)
		if statErr != nil {
			continue // Skip files we can't stat
		}
		if fileInfo.IsDir() {
			continue // Skip directories
		}
		files = append(files, fileWithTime{
			path:    match,
			modTime: fileInfo.ModTime().UnixNano(),
		})
	}

	// Handle case where all matches were directories
	if len(files) == 0 {
		return SuccessResult("no files match pattern: " + in.Pattern + " (only directories matched)"), nil
	}

	// Sort by modification time (newest first)
	sort.Slice(files, func(i, j int) bool {
		return files[i].modTime > files[j].modTime
	})

	// Limit results
	truncated := false
	if len(files) > maxGlobResults {
		files = files[:maxGlobResults]
		truncated = true
	}

	// Build result
	var result strings.Builder
	for _, f := range files {
		result.WriteString(f.path)
		result.WriteString("\n")
	}

	if truncated {
		result.WriteString("\n... (showing first 200 results, refine your pattern for more specific matches)")
	}

	return SuccessResult(result.String()), nil
}
