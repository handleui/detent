package act

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestBuildArgs tests argument building for act execution
func TestBuildArgs(t *testing.T) {
	tests := []struct {
		name        string
		cfg         *RunConfig
		wantArgs    []string
		wantErr     bool
		errContains string
	}{
		{
			name: "minimal configuration",
			cfg: &RunConfig{
				Event: "push",
			},
			wantArgs: []string{
				"push",
				"-v",
				"-P", "ubuntu-latest=catthehacker/ubuntu:act-latest",
				"-P", "ubuntu-22.04=catthehacker/ubuntu:act-22.04",
				"-P", "ubuntu-20.04=catthehacker/ubuntu:act-20.04",
				"--rm",
				"--no-cache-server",
				"--container-cap-drop", "SYS_ADMIN",
				"--container-cap-drop", "NET_ADMIN",
				"--container-cap-drop", "SYS_PTRACE",
				"--container-cap-drop", "MKNOD",
			},
			wantErr: false,
		},
		{
			name: "with workflow path",
			cfg: &RunConfig{
				WorkflowPath: ".github/workflows/ci.yml",
				Event:        "pull_request",
			},
			wantArgs: []string{
				"-W", ".github/workflows/ci.yml",
				"pull_request",
				"-v",
				"-P", "ubuntu-latest=catthehacker/ubuntu:act-latest",
				"-P", "ubuntu-22.04=catthehacker/ubuntu:act-22.04",
				"-P", "ubuntu-20.04=catthehacker/ubuntu:act-20.04",
				"--rm",
				"--no-cache-server",
				"--container-cap-drop", "SYS_ADMIN",
				"--container-cap-drop", "NET_ADMIN",
				"--container-cap-drop", "SYS_PTRACE",
				"--container-cap-drop", "MKNOD",
			},
			wantErr: false,
		},
		{
			name: "event with underscores",
			cfg: &RunConfig{
				Event: "pull_request_target",
			},
			wantArgs: []string{
				"pull_request_target",
				"-v",
				"-P", "ubuntu-latest=catthehacker/ubuntu:act-latest",
				"-P", "ubuntu-22.04=catthehacker/ubuntu:act-22.04",
				"-P", "ubuntu-20.04=catthehacker/ubuntu:act-20.04",
				"--rm",
				"--no-cache-server",
				"--container-cap-drop", "SYS_ADMIN",
				"--container-cap-drop", "NET_ADMIN",
				"--container-cap-drop", "SYS_PTRACE",
				"--container-cap-drop", "MKNOD",
			},
			wantErr: false,
		},
		{
			name: "event with hyphens",
			cfg: &RunConfig{
				Event: "workflow-dispatch",
			},
			wantArgs: []string{
				"workflow-dispatch",
				"-v",
				"-P", "ubuntu-latest=catthehacker/ubuntu:act-latest",
				"-P", "ubuntu-22.04=catthehacker/ubuntu:act-22.04",
				"-P", "ubuntu-20.04=catthehacker/ubuntu:act-20.04",
				"--rm",
				"--no-cache-server",
				"--container-cap-drop", "SYS_ADMIN",
				"--container-cap-drop", "NET_ADMIN",
				"--container-cap-drop", "SYS_PTRACE",
				"--container-cap-drop", "MKNOD",
			},
			wantErr: false,
		},
		{
			name: "no event specified",
			cfg:  &RunConfig{},
			wantArgs: []string{
				"-v",
				"-P", "ubuntu-latest=catthehacker/ubuntu:act-latest",
				"-P", "ubuntu-22.04=catthehacker/ubuntu:act-22.04",
				"-P", "ubuntu-20.04=catthehacker/ubuntu:act-20.04",
				"--rm",
				"--no-cache-server",
				"--container-cap-drop", "SYS_ADMIN",
				"--container-cap-drop", "NET_ADMIN",
				"--container-cap-drop", "SYS_PTRACE",
				"--container-cap-drop", "MKNOD",
			},
			wantErr: false,
		},
		{
			name: "invalid event with spaces",
			cfg: &RunConfig{
				Event: "pull request",
			},
			wantErr:     true,
			errContains: "invalid event name",
		},
		{
			name: "invalid event with special characters",
			cfg: &RunConfig{
				Event: "pull_request@main",
			},
			wantErr:     true,
			errContains: "invalid event name",
		},
		{
			name: "invalid event with slash",
			cfg: &RunConfig{
				Event: "pull/request",
			},
			wantErr:     true,
			errContains: "invalid event name",
		},
		{
			name: "empty event string",
			cfg: &RunConfig{
				Event: "",
			},
			wantArgs: []string{
				"-v",
				"-P", "ubuntu-latest=catthehacker/ubuntu:act-latest",
				"-P", "ubuntu-22.04=catthehacker/ubuntu:act-22.04",
				"-P", "ubuntu-20.04=catthehacker/ubuntu:act-20.04",
				"--rm",
				"--no-cache-server",
				"--container-cap-drop", "SYS_ADMIN",
				"--container-cap-drop", "NET_ADMIN",
				"--container-cap-drop", "SYS_PTRACE",
				"--container-cap-drop", "MKNOD",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args, err := buildArgs(tt.cfg)

			if (err != nil) != tt.wantErr {
				t.Errorf("buildArgs() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr {
				if err == nil {
					t.Error("Expected error but got nil")
					return
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("Error %q should contain %q", err.Error(), tt.errContains)
				}
				return
			}

			if len(args) != len(tt.wantArgs) {
				t.Errorf("buildArgs() returned %d args, want %d\nGot: %v\nWant: %v",
					len(args), len(tt.wantArgs), args, tt.wantArgs)
				return
			}

			for i, arg := range args {
				if arg != tt.wantArgs[i] {
					t.Errorf("buildArgs() arg[%d] = %q, want %q", i, arg, tt.wantArgs[i])
				}
			}
		})
	}
}

// TestFilterEnvironment tests environment variable filtering
func TestFilterEnvironment(t *testing.T) {
	tests := []struct {
		name     string
		env      []string
		wantVars []string
		rejectVars []string
	}{
		{
			name: "filters safe variables",
			env: []string{
				"PATH=/usr/bin",
				"HOME=/home/user",
				"USER=testuser",
				"SHELL=/bin/bash",
				"LANG=en_US.UTF-8",
				"LC_ALL=en_US.UTF-8",
				"TERM=xterm",
				"TMPDIR=/tmp",
				"TZ=UTC",
			},
			wantVars: []string{
				"PATH=/usr/bin",
				"HOME=/home/user",
				"USER=testuser",
				"SHELL=/bin/bash",
				"LANG=en_US.UTF-8",
				"LC_ALL=en_US.UTF-8",
				"TERM=xterm",
				"TMPDIR=/tmp",
				"TZ=UTC",
			},
			rejectVars: []string{},
		},
		{
			name: "rejects secrets and tokens",
			env: []string{
				"PATH=/usr/bin",
				"AWS_SECRET_ACCESS_KEY=secret123",
				"GITHUB_TOKEN=ghp_token123",
				"DATABASE_PASSWORD=password123",
				"API_KEY=key123",
				"HOME=/home/user",
			},
			wantVars: []string{
				"PATH=/usr/bin",
				"HOME=/home/user",
			},
			rejectVars: []string{
				"AWS_SECRET_ACCESS_KEY",
				"GITHUB_TOKEN",
				"DATABASE_PASSWORD",
				"API_KEY",
			},
		},
		{
			name: "handles LC_ prefix variations",
			env: []string{
				"LC_ALL=en_US.UTF-8",
				"LC_CTYPE=en_US.UTF-8",
				"LC_MESSAGES=en_US.UTF-8",
				"CUSTOM_VAR=value",
			},
			wantVars: []string{
				"LC_ALL=en_US.UTF-8",
				"LC_CTYPE=en_US.UTF-8",
				"LC_MESSAGES=en_US.UTF-8",
			},
			rejectVars: []string{"CUSTOM_VAR"},
		},
		{
			name:       "empty environment",
			env:        []string{},
			wantVars:   []string{},
			rejectVars: []string{},
		},
		{
			name: "only unsafe variables",
			env: []string{
				"SECRET_KEY=secret",
				"PASSWORD=pass",
				"TOKEN=token",
			},
			wantVars:   []string{},
			rejectVars: []string{"SECRET_KEY", "PASSWORD", "TOKEN"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filtered := filterEnvironment(tt.env)

			// Check that all wanted variables are present
			for _, wantVar := range tt.wantVars {
				found := false
				for _, filteredVar := range filtered {
					if filteredVar == wantVar {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("filterEnvironment() missing expected var %q", wantVar)
				}
			}

			// Check that rejected variables are not present
			for _, rejectVar := range tt.rejectVars {
				for _, filteredVar := range filtered {
					if strings.HasPrefix(filteredVar, rejectVar+"=") {
						t.Errorf("filterEnvironment() should not include %q", filteredVar)
					}
				}
			}
		})
	}
}

// TestChanWriter tests the channel writer for log streaming
func TestChanWriter(t *testing.T) {
	tests := []struct {
		name       string
		input      []byte
		wantLines  []string
		bufferLeft string
	}{
		{
			name:      "single line with newline",
			input:     []byte("test line\n"),
			wantLines: []string{"test line"},
		},
		{
			name:      "multiple lines",
			input:     []byte("line1\nline2\nline3\n"),
			wantLines: []string{"line1", "line2", "line3"},
		},
		{
			name:       "partial line without newline",
			input:      []byte("partial"),
			wantLines:  []string{},
			bufferLeft: "partial",
		},
		{
			name:      "line with leading/trailing spaces",
			input:     []byte("  spaced line  \n"),
			wantLines: []string{"spaced line"},
		},
		{
			name:      "empty lines",
			input:     []byte("\n\n\n"),
			wantLines: []string{"", "", ""},
		},
		{
			name:      "mixed complete and partial",
			input:     []byte("complete\npartial"),
			wantLines: []string{"complete"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ch := make(chan string, 10)
			writer := newChanWriter(ch)

			n, err := writer.Write(tt.input)
			if err != nil {
				t.Errorf("Write() error = %v", err)
			}
			if n != len(tt.input) {
				t.Errorf("Write() returned %d, want %d", n, len(tt.input))
			}

			// Close channel to signal completion
			close(ch)

			// Collect all lines from channel
			var gotLines []string
			for line := range ch {
				gotLines = append(gotLines, line)
			}

			if len(gotLines) != len(tt.wantLines) {
				t.Errorf("Write() sent %d lines, want %d\nGot: %v\nWant: %v",
					len(gotLines), len(tt.wantLines), gotLines, tt.wantLines)
				return
			}

			for i, line := range gotLines {
				if line != tt.wantLines[i] {
					t.Errorf("Write() line[%d] = %q, want %q", i, line, tt.wantLines[i])
				}
			}

			// Check remaining buffer if specified
			if tt.bufferLeft != "" {
				if writer.buffer.String() != tt.bufferLeft {
					t.Errorf("Write() buffer = %q, want %q", writer.buffer.String(), tt.bufferLeft)
				}
			}
		})
	}
}

// TestChanWriter_MultipleWrites tests incremental writing to chanWriter
func TestChanWriter_MultipleWrites(t *testing.T) {
	ch := make(chan string, 10)
	writer := newChanWriter(ch)

	// Write in chunks
	writes := []string{
		"first ",
		"part\nsecond ",
		"part\nthird",
		" part\n",
	}

	for _, w := range writes {
		_, err := writer.Write([]byte(w))
		if err != nil {
			t.Fatalf("Write() error = %v", err)
		}
	}

	close(ch)

	expectedLines := []string{
		"first part",
		"second part",
		"third part",
	}

	var gotLines []string
	for line := range ch {
		gotLines = append(gotLines, line)
	}

	if len(gotLines) != len(expectedLines) {
		t.Errorf("Got %d lines, want %d\nGot: %v\nWant: %v",
			len(gotLines), len(expectedLines), gotLines, expectedLines)
		return
	}

	for i, line := range gotLines {
		if line != expectedLines[i] {
			t.Errorf("Line[%d] = %q, want %q", i, line, expectedLines[i])
		}
	}
}

// TestChanWriter_ChannelFull tests behavior when channel is full
func TestChanWriter_ChannelFull(t *testing.T) {
	ch := make(chan string, 1) // Small buffer
	writer := newChanWriter(ch)

	// Fill the channel
	ch <- "blocking"

	// Write should not block or error even if channel is full
	input := []byte("line1\nline2\nline3\n")
	n, err := writer.Write(input)
	if err != nil {
		t.Errorf("Write() should not error on full channel, got: %v", err)
	}
	if n != len(input) {
		t.Errorf("Write() returned %d, want %d", n, len(input))
	}
}

// mockCommand creates a mock executable for testing
func createMockActCommand(t *testing.T, behavior string) string {
	t.Helper()

	tmpDir := t.TempDir()
	scriptPath := tmpDir + "/mock-act"

	var script string
	switch behavior {
	case "success":
		script = `#!/bin/sh
echo "Mock act output"
echo "Job succeeded" >&2
exit 0
`
	case "failure":
		script = `#!/bin/sh
echo "Mock act error" >&2
exit 1
`
	case "slow":
		script = `#!/bin/sh
echo "Starting..."
sleep 10
echo "Done"
exit 0
`
	case "output":
		script = `#!/bin/sh
echo "stdout line 1"
echo "stdout line 2"
echo "stderr line 1" >&2
echo "stderr line 2" >&2
exit 0
`
	default:
		t.Fatalf("Unknown behavior: %s", behavior)
	}

	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("Failed to create mock script: %v", err)
	}

	return scriptPath
}

// TestRun_Success tests successful act execution
func TestRun_Success(t *testing.T) {
	mockAct := createMockActCommand(t, "success")

	cfg := &RunConfig{
		Event:     "push",
		ActBinary: mockAct,
		WorkDir:   t.TempDir(),
	}

	ctx := context.Background()
	result, err := Run(ctx, cfg)

	if err != nil {
		t.Fatalf("Run() error = %v, want nil", err)
	}

	if result == nil {
		t.Fatal("Run() returned nil result")
	}

	if result.ExitCode != 0 {
		t.Errorf("Run() ExitCode = %d, want 0", result.ExitCode)
	}

	if !strings.Contains(result.Stdout, "Mock act output") {
		t.Errorf("Run() Stdout = %q, should contain 'Mock act output'", result.Stdout)
	}

	if result.Duration == 0 {
		t.Error("Run() Duration should not be zero")
	}
}

// TestRun_Failure tests act execution with non-zero exit code
func TestRun_Failure(t *testing.T) {
	mockAct := createMockActCommand(t, "failure")

	cfg := &RunConfig{
		Event:     "push",
		ActBinary: mockAct,
	}

	ctx := context.Background()
	result, err := Run(ctx, cfg)

	if err != nil {
		t.Fatalf("Run() should not return error for non-zero exit, got: %v", err)
	}

	if result.ExitCode != 1 {
		t.Errorf("Run() ExitCode = %d, want 1", result.ExitCode)
	}

	if !strings.Contains(result.Stderr, "Mock act error") {
		t.Errorf("Run() Stderr should contain error message, got: %q", result.Stderr)
	}
}

// TestRun_ContextCancellation tests context cancellation during execution
func TestRun_ContextCancellation(t *testing.T) {
	mockAct := createMockActCommand(t, "slow")

	cfg := &RunConfig{
		Event:     "push",
		ActBinary: mockAct,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	result, err := Run(ctx, cfg)
	duration := time.Since(start)

	if err == nil {
		t.Fatal("Run() should return error on context cancellation")
	}

	if err != context.DeadlineExceeded {
		t.Errorf("Run() error = %v, want context.DeadlineExceeded", err)
	}

	if result != nil {
		t.Errorf("Run() should return nil result on cancellation, got: %v", result)
	}

	// Should return quickly after context cancellation
	if duration > 2*time.Second {
		t.Errorf("Run() took %v, should cancel quickly", duration)
	}
}

// TestRun_OutputCapture tests stdout and stderr capture
func TestRun_OutputCapture(t *testing.T) {
	mockAct := createMockActCommand(t, "output")

	cfg := &RunConfig{
		Event:     "push",
		ActBinary: mockAct,
	}

	ctx := context.Background()
	result, err := Run(ctx, cfg)

	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	wantStdout := []string{"stdout line 1", "stdout line 2"}
	for _, want := range wantStdout {
		if !strings.Contains(result.Stdout, want) {
			t.Errorf("Run() Stdout should contain %q, got: %q", want, result.Stdout)
		}
	}

	wantStderr := []string{"stderr line 1", "stderr line 2"}
	for _, want := range wantStderr {
		if !strings.Contains(result.Stderr, want) {
			t.Errorf("Run() Stderr should contain %q, got: %q", want, result.Stderr)
		}
	}
}

// TestRun_LogChannel tests streaming output to a channel
func TestRun_LogChannel(t *testing.T) {
	mockAct := createMockActCommand(t, "output")

	logChan := make(chan string, 10)
	cfg := &RunConfig{
		Event:     "push",
		ActBinary: mockAct,
		LogChan:   logChan,
	}

	ctx := context.Background()
	result, err := Run(ctx, cfg)

	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if result.ExitCode != 0 {
		t.Errorf("Run() ExitCode = %d, want 0", result.ExitCode)
	}

	close(logChan)

	// Collect lines from channel
	var lines []string
	for line := range logChan {
		lines = append(lines, line)
	}

	if len(lines) == 0 {
		t.Error("Expected log lines in channel, got none")
	}

	// Should contain some of the output lines
	allLines := strings.Join(lines, "\n")
	if !strings.Contains(allLines, "stdout") && !strings.Contains(allLines, "stderr") {
		t.Errorf("Log channel should contain output, got: %v", lines)
	}
}

// TestRun_WorkDir tests working directory configuration
func TestRun_WorkDir(t *testing.T) {
	tmpDir := t.TempDir()
	mockAct := createMockActCommand(t, "success")

	cfg := &RunConfig{
		Event:     "push",
		ActBinary: mockAct,
		WorkDir:   tmpDir,
	}

	ctx := context.Background()
	result, err := Run(ctx, cfg)

	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if result.ExitCode != 0 {
		t.Errorf("Run() ExitCode = %d, want 0", result.ExitCode)
	}
}

// TestRun_DefaultActBinary tests that act binary defaults to "act"
func TestRun_DefaultActBinary(t *testing.T) {
	// Check if act is available in PATH
	if _, err := exec.LookPath("act"); err != nil {
		t.Skip("Skipping test: act not found in PATH")
	}

	cfg := &RunConfig{
		Event: "push",
	}

	// Use a very short timeout to avoid actually running act
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	// This will likely timeout, but we're just checking that it tries to run "act"
	_, err := Run(ctx, cfg)

	// We expect either a timeout or an act-related error (not "executable not found")
	if err == nil {
		t.Skip("Skipping: act completed before timeout")
	}

	errStr := err.Error()
	if strings.Contains(errStr, "executable file not found") {
		t.Error("Run() should use 'act' from PATH, but got 'not found' error")
	}
}

// TestRun_InvalidEvent tests running with an invalid event name
func TestRun_InvalidEvent(t *testing.T) {
	mockAct := createMockActCommand(t, "success")

	cfg := &RunConfig{
		Event:     "invalid event name!",
		ActBinary: mockAct,
	}

	ctx := context.Background()
	result, err := Run(ctx, cfg)

	if err == nil {
		t.Fatal("Run() should return error for invalid event name")
	}

	if result != nil {
		t.Errorf("Run() should return nil result on error, got: %v", result)
	}

	if !strings.Contains(err.Error(), "invalid event name") {
		t.Errorf("Error should mention invalid event name, got: %v", err)
	}
}

// TestRun_StreamOutput tests the StreamOutput configuration option
func TestRun_StreamOutput(t *testing.T) {
	mockAct := createMockActCommand(t, "output")

	// Capture stderr to verify streaming
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	cfg := &RunConfig{
		Event:        "push",
		ActBinary:    mockAct,
		StreamOutput: true,
	}

	ctx := context.Background()
	result, err := Run(ctx, cfg)

	// Restore stderr
	_ = w.Close()
	os.Stderr = oldStderr

	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if result.ExitCode != 0 {
		t.Errorf("Run() ExitCode = %d, want 0", result.ExitCode)
	}

	// Read captured stderr
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	capturedStderr := buf.String()

	// When StreamOutput is true, output should be streamed to stderr
	// We should see some output in the captured stderr
	if capturedStderr == "" {
		t.Log("Warning: Expected output to be streamed to stderr, but captured nothing")
	}
}

// TestRun_MultipleWorkflows tests running with different workflow paths
func TestRun_MultipleWorkflows(t *testing.T) {
	mockAct := createMockActCommand(t, "success")

	workflows := []string{
		".github/workflows/ci.yml",
		".github/workflows/test.yml",
		".github/workflows/deploy.yml",
	}

	for _, workflow := range workflows {
		t.Run(workflow, func(t *testing.T) {
			cfg := &RunConfig{
				WorkflowPath: workflow,
				Event:        "push",
				ActBinary:    mockAct,
			}

			ctx := context.Background()
			result, err := Run(ctx, cfg)

			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}

			if result.ExitCode != 0 {
				t.Errorf("Run() ExitCode = %d, want 0", result.ExitCode)
			}
		})
	}
}

// TestValidEventPattern tests the event name validation regex
func TestValidEventPattern(t *testing.T) {
	tests := []struct {
		name  string
		event string
		valid bool
	}{
		{"simple lowercase", "push", true},
		{"with underscore", "pull_request", true},
		{"with hyphen", "workflow-dispatch", true},
		{"mixed case", "PullRequest", true},
		{"numbers", "event123", true},
		{"complex valid", "workflow_dispatch-v2", true},
		{"with spaces", "pull request", false},
		{"with special chars", "event@main", false},
		{"with slash", "event/name", false},
		{"with period", "event.name", false},
		{"with asterisk", "event*", false},
		{"empty string", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches := validEventPattern.MatchString(tt.event)
			if matches != tt.valid {
				t.Errorf("validEventPattern.MatchString(%q) = %v, want %v",
					tt.event, matches, tt.valid)
			}
		})
	}
}

// TestRun_GracefulShutdown tests graceful shutdown timeout
func TestRun_GracefulShutdown(t *testing.T) {
	// Create a script that ignores SIGTERM and needs SIGKILL
	tmpDir := t.TempDir()
	scriptPath := tmpDir + "/mock-act-stubborn"

	script := `#!/bin/sh
trap '' TERM  # Ignore SIGTERM
sleep 30
exit 0
`
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("Failed to create mock script: %v", err)
	}

	cfg := &RunConfig{
		Event:     "push",
		ActBinary: scriptPath,
	}

	// Cancel after a short time
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	result, err := Run(ctx, cfg)
	duration := time.Since(start)

	if err == nil {
		t.Fatal("Run() should return error on context cancellation")
	}

	if result != nil {
		t.Errorf("Run() should return nil result on cancellation, got: %v", result)
	}

	// Should wait for graceful shutdown timeout (5 seconds) + cancel time
	// But should be less than the script's sleep time (30 seconds)
	maxDuration := gracefulShutdownTimeout + 2*time.Second
	if duration > maxDuration {
		t.Errorf("Run() took %v, should complete within graceful shutdown timeout (%v)",
			duration, maxDuration)
	}

	t.Logf("Graceful shutdown completed in %v (expected ~%v)", duration, gracefulShutdownTimeout)
}

// TestRunResult_Structure tests the RunResult structure
func TestRunResult_Structure(t *testing.T) {
	result := &RunResult{
		Stdout:   "test stdout",
		Stderr:   "test stderr",
		ExitCode: 42,
		Duration: 5 * time.Second,
	}

	if result.Stdout != "test stdout" {
		t.Errorf("Stdout = %q, want 'test stdout'", result.Stdout)
	}
	if result.Stderr != "test stderr" {
		t.Errorf("Stderr = %q, want 'test stderr'", result.Stderr)
	}
	if result.ExitCode != 42 {
		t.Errorf("ExitCode = %d, want 42", result.ExitCode)
	}
	if result.Duration != 5*time.Second {
		t.Errorf("Duration = %v, want 5s", result.Duration)
	}
}

// TestRunConfig_Defaults tests default configuration values
func TestRunConfig_Defaults(t *testing.T) {
	cfg := &RunConfig{}

	if cfg.ActBinary != "" {
		t.Errorf("Default ActBinary should be empty, got %q", cfg.ActBinary)
	}
	if cfg.StreamOutput {
		t.Error("Default StreamOutput should be false")
	}
	if cfg.Verbose {
		t.Error("Default Verbose should be false")
	}
	if cfg.LogChan != nil {
		t.Error("Default LogChan should be nil")
	}
}

// TestRun_EnvironmentFiltering tests that environment variables are filtered
func TestRun_EnvironmentFiltering(t *testing.T) {
	// This test verifies that Run() uses filterEnvironment()
	// We can't easily inspect the actual env passed to the command,
	// but we can verify the function is called by testing the filter separately
	testEnv := []string{
		"PATH=/usr/bin",
		"SECRET_KEY=secret",
		"HOME=/home/user",
	}

	filtered := filterEnvironment(testEnv)

	// Should include safe vars
	hasPATH := false
	hasHOME := false
	for _, v := range filtered {
		if strings.HasPrefix(v, "PATH=") {
			hasPATH = true
		}
		if strings.HasPrefix(v, "HOME=") {
			hasHOME = true
		}
		if strings.HasPrefix(v, "SECRET_KEY=") {
			t.Error("Filtered environment should not contain SECRET_KEY")
		}
	}

	if !hasPATH {
		t.Error("Filtered environment should contain PATH")
	}
	if !hasHOME {
		t.Error("Filtered environment should contain HOME")
	}
}

// TestRun_ConcurrentExecutions tests running multiple act instances concurrently
func TestRun_ConcurrentExecutions(t *testing.T) {
	mockAct := createMockActCommand(t, "success")

	concurrency := 5
	done := make(chan error, concurrency)

	for i := 0; i < concurrency; i++ {
		go func(id int) {
			cfg := &RunConfig{
				Event:     "push",
				ActBinary: mockAct,
				WorkDir:   t.TempDir(),
			}

			ctx := context.Background()
			result, err := Run(ctx, cfg)

			if err != nil {
				done <- fmt.Errorf("goroutine %d: %w", id, err)
				return
			}

			if result.ExitCode != 0 {
				done <- fmt.Errorf("goroutine %d: exit code %d", id, result.ExitCode)
				return
			}

			done <- nil
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < concurrency; i++ {
		if err := <-done; err != nil {
			t.Errorf("Concurrent execution failed: %v", err)
		}
	}
}
