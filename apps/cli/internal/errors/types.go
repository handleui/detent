package errors

import (
	"path/filepath"
	"strings"
)

// ExtractedError represents a single error extracted from act output
type ExtractedError struct {
	Message  string `json:"message"`
	File     string `json:"file,omitempty"`
	Line     int    `json:"line,omitempty"`
	Column   int    `json:"column,omitempty"`
	Severity string `json:"severity,omitempty"` // "error" or "warning"
	Raw      string `json:"raw,omitempty"`
}

// GroupedErrors groups errors by file path for organized output
type GroupedErrors struct {
	ByFile map[string][]*ExtractedError `json:"by_file"`
	NoFile []*ExtractedError            `json:"no_file"`
	Total  int                          `json:"total"`
}

// GroupByFile organizes extracted errors by their file paths
func GroupByFile(errs []*ExtractedError) *GroupedErrors {
	return GroupByFileWithBase(errs, "")
}

// GroupByFileWithBase organizes extracted errors by their file paths,
// making paths relative to basePath if provided
func GroupByFileWithBase(errs []*ExtractedError, basePath string) *GroupedErrors {
	grouped := &GroupedErrors{
		ByFile: make(map[string][]*ExtractedError),
		Total:  len(errs),
	}

	for _, err := range errs {
		if err.File != "" {
			file := err.File
			if basePath != "" {
				file = makeRelative(file, basePath)
			}
			grouped.ByFile[file] = append(grouped.ByFile[file], err)
		} else {
			grouped.NoFile = append(grouped.NoFile, err)
		}
	}

	return grouped
}

// makeRelative converts an absolute path to relative if it starts with basePath
func makeRelative(path, basePath string) string {
	if basePath == "" || !filepath.IsAbs(path) {
		return path
	}

	// Clean both paths
	path = filepath.Clean(path)
	basePath = filepath.Clean(basePath)

	// If path starts with basePath, make it relative
	if strings.HasPrefix(path, basePath) {
		rel := strings.TrimPrefix(path, basePath)
		rel = strings.TrimPrefix(rel, string(filepath.Separator))
		if rel != "" {
			return rel
		}
	}

	return path
}
