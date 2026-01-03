package act

import (
	"errors"
	"testing"
)

func TestIsTransientError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "docker daemon not running",
			err:      errors.New("Cannot connect to the Docker daemon at unix:///var/run/docker.sock"),
			expected: true,
		},
		{
			name:     "connection refused",
			err:      errors.New("dial tcp 127.0.0.1:2375: connection refused"),
			expected: true,
		},
		{
			name:     "docker socket error",
			err:      errors.New("dial unix /var/run/docker.sock: connect: no such file"),
			expected: true,
		},
		{
			name:     "image pull failed",
			err:      errors.New("failed to pull image catthehacker/ubuntu:act-latest"),
			expected: true,
		},
		{
			name:     "network unreachable",
			err:      errors.New("network is unreachable"),
			expected: true,
		},
		{
			name:     "dns failure",
			err:      errors.New("temporary failure in name resolution"),
			expected: true,
		},
		{
			name:     "tls timeout",
			err:      errors.New("TLS handshake timeout"),
			expected: true,
		},
		{
			name:     "connection reset",
			err:      errors.New("connection reset by peer"),
			expected: true,
		},
		{
			name:     "context deadline",
			err:      errors.New("context deadline exceeded"),
			expected: true,
		},
		{
			name:     "error response from daemon",
			err:      errors.New("Error response from daemon: conflict"),
			expected: true,
		},
		{
			name:     "non-transient error",
			err:      errors.New("workflow file not found"),
			expected: false,
		},
		{
			name:     "syntax error",
			err:      errors.New("YAML syntax error at line 10"),
			expected: false,
		},
		{
			name:     "permission denied",
			err:      errors.New("permission denied"),
			expected: false,
		},
		{
			name:     "case insensitive match",
			err:      errors.New("CANNOT CONNECT TO THE DOCKER DAEMON"),
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsTransientError(tt.err)
			if result != tt.expected {
				t.Errorf("IsTransientError(%v) = %v, want %v", tt.err, result, tt.expected)
			}
		})
	}
}

func TestTransientPatternsAreAllLowercase(t *testing.T) {
	for _, pattern := range transientPatterns {
		for _, r := range pattern {
			if r >= 'A' && r <= 'Z' {
				t.Errorf("pattern %q contains uppercase letter, should be lowercase for case-insensitive matching", pattern)
				break
			}
		}
	}
}
