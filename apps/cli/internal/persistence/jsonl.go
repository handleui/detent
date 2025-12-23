package persistence

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	detentDir     = ".detent"
	runsSubdir    = "runs"
	bufferSizeKB  = 256
)

// JSONLWriter handles writing JSONL (JSON Lines) format to disk
type JSONLWriter struct {
	file   *os.File
	writer *bufio.Writer
	path   string
}

// NewJSONLWriter creates a new JSONL writer in the .detent/runs directory
// The file is named with a timestamp for easy chronological sorting
func NewJSONLWriter(repoRoot string) (*JSONLWriter, error) {
	// Create .detent/runs directory structure
	detentPath := filepath.Join(repoRoot, detentDir)
	runsPath := filepath.Join(detentPath, runsSubdir)

	// #nosec G301 - standard permissions for app data directory
	if err := os.MkdirAll(runsPath, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create .detent/runs directory: %w", err)
	}

	// Create timestamped filename: 2025-01-15T14-30-45.jsonl
	timestamp := time.Now().Format("2006-01-02T15-04-05")
	filename := fmt.Sprintf("%s.jsonl", timestamp)
	filePath := filepath.Join(runsPath, filename)

	file, err := os.Create(filePath) // #nosec G304 - creating JSONL file in .detent directory, expected behavior
	if err != nil {
		return nil, fmt.Errorf("failed to create JSONL file: %w", err)
	}

	// Use buffered writer for better performance
	writer := bufio.NewWriterSize(file, bufferSizeKB*1024)

	return &JSONLWriter{
		file:   file,
		writer: writer,
		path:   filePath,
	}, nil
}

// WriteFinding writes a single finding record as a JSONL line
func (w *JSONLWriter) WriteFinding(record *FindingRecord) error {
	data, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("failed to marshal finding: %w", err)
	}

	if _, err := w.writer.Write(data); err != nil {
		return fmt.Errorf("failed to write finding: %w", err)
	}

	if _, err := w.writer.WriteString("\n"); err != nil {
		return fmt.Errorf("failed to write newline: %w", err)
	}

	return nil
}

// WriteRunSummary writes the final run summary at the end of the file
func (w *JSONLWriter) WriteRunSummary(record *RunRecord) error {
	data, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("failed to marshal run summary: %w", err)
	}

	if _, err := w.writer.Write(data); err != nil {
		return fmt.Errorf("failed to write run summary: %w", err)
	}

	if _, err := w.writer.WriteString("\n"); err != nil {
		return fmt.Errorf("failed to write newline: %w", err)
	}

	return nil
}

// Flush ensures all buffered data is written to disk
func (w *JSONLWriter) Flush() error {
	if err := w.writer.Flush(); err != nil {
		return fmt.Errorf("failed to flush buffer: %w", err)
	}
	return nil
}

// Close flushes and closes the JSONL file
func (w *JSONLWriter) Close() error {
	if err := w.Flush(); err != nil {
		return err
	}

	if err := w.file.Close(); err != nil {
		return fmt.Errorf("failed to close file: %w", err)
	}

	return nil
}

// Path returns the absolute path to the JSONL file
func (w *JSONLWriter) Path() string {
	return w.path
}
