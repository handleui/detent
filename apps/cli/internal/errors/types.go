package errors

// ExtractedError represents a single error extracted from act output
type ExtractedError struct {
	Message string `json:"message"`
	File    string `json:"file,omitempty"`
	Line    int    `json:"line,omitempty"`
	Column  int    `json:"column,omitempty"`
	Raw     string `json:"raw,omitempty"`
}

// GroupedErrors groups errors by file path for organized output
type GroupedErrors struct {
	ByFile map[string][]*ExtractedError `json:"by_file"`
	NoFile []*ExtractedError            `json:"no_file"`
	Total  int                          `json:"total"`
}

// GroupByFile organizes extracted errors by their file paths
func GroupByFile(errs []*ExtractedError) *GroupedErrors {
	grouped := &GroupedErrors{
		ByFile: make(map[string][]*ExtractedError),
		Total:  len(errs),
	}

	for _, err := range errs {
		if err.File != "" {
			grouped.ByFile[err.File] = append(grouped.ByFile[err.File], err)
		} else {
			grouped.NoFile = append(grouped.NoFile, err)
		}
	}

	return grouped
}
